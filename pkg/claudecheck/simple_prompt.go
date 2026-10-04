package claudecheck

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const SimplePromptRequests = 3

func simplePromptFixtures(model string) []promptAuditFixture {
	// Keep a fixed minimal question and parameters so the input allowance
	// applies to the same request, independent of any saved reference.
	var fixtures []promptAuditFixture
	for round := 1; round <= SimplePromptRequests; round++ {
		fixtures = append(fixtures, promptAuditFixture{fmt.Sprintf("minimal_r%d", round), promptChannelFixtures(model)[0].body})
	}
	return fixtures
}

func (r *runner) simplePromptAudit() {
	for i, fixture := range simplePromptFixtures(r.report.Model) {
		if r.ctx.Err() != nil {
			break
		}
		item := &r.report.TokenAudit.Prompt[i]
		m, response, ok := r.probe("prompt_audit_"+fixture.id, Request{Body: fixture.body})
		item.Profile, item.ClientProfile = response.RequestProfile, bodyProfile(fixture.body)
		item.PromptProfile = response.PromptProfile
		item.PromptChanged = response.PromptProfile != "" && response.PromptProfile != promptContentProfile(fixture.body)
		item.RequestChanged = item.Profile != "" && item.Profile != item.ClientProfile
		item.Model, item.ResponseModel = response.Model, m.Model
		if item.Model == "" {
			item.Model = m.Model
		}
		item.Platform = "anthropic"
		if response.Bedrock || r.report.Transport == "bedrock_runtime" || strings.HasPrefix(m.ID, "msg_bdrk_") ||
			strings.Contains(m.Model, "anthropic.claude") || response.Header.Get("x-amzn-requestid") != "" {
			item.Platform = "aws"
		}
		item.Code = "probe_unavailable"
		if ok {
			item.Code = "missing_usage"
			if actual, valid := totalInput(m.Usage); valid && actual > 0 {
				item.Actual, item.Code = &actual, "input_observed"
			}
		}
		// Response text and same-endpoint CountTokens do not establish whether
		// the intermediary appended context. Only record total input usage.
		r.emitTokenAudit()
		if response.ErrorCode == "access_denied" || response.Status == 429 {
			break
		}
	}
	code := "prompt_input_compared"
	if r.report.TokenAudit.Version >= 7 {
		code = "prompt_input_budget_checked"
	}
	r.check("prompt_integrity", "inconclusive", code, map[string]any{"injection_verified": false})
	r.emitTokenAudit()
}

// This is a product comparison margin, not a provider guarantee. A small
// request needs an absolute floor as well as a relative allowance.
func simplePromptTolerance(expected int64) int64 {
	return int64(math.Max(3, math.Ceil(float64(expected)*0.20)))
}

// Baseline types are the operator's declarations, not verified provenance.
// Wrapped products remain useful elsewhere but cannot certify a clean prompt.
func nativePromptPlatform(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "anthropic":
		return "anthropic"
	case "aws", "bedrock", "aws_bedrock", "aws-bedrock", "aws bedrock":
		return "aws"
	}
	return ""
}

func simplePromptItems(audit *TokenAuditReport, model string) []TokenComparison {
	if audit == nil {
		return nil
	}
	fixture := simplePromptFixtures(model)[0]
	content, client := promptContentProfile(fixture.body), bodyProfile(fixture.body)
	seen := map[string]bool{}
	var items []TokenComparison
	for _, item := range audit.Prompt {
		if seen[item.ID] || !usablePromptInput(&item) || item.PromptChanged || item.Profile == "" {
			continue
		}
		seen[item.ID] = true
		if item.PromptProfile != content || (item.ClientProfile != client && item.Profile != client) ||
			!promptModelMatches(model, promptEvidenceModel(item, model)) || declaredModelRelation(item.Model, item.ResponseModel) < 0 {
			continue
		}
		items = append(items, item)
	}
	return items
}

func simplePromptModelConflict(audit *TokenAuditReport, model string) bool {
	if audit == nil {
		return false
	}
	content := promptContentProfile(simplePromptFixtures(model)[0].body)
	for _, item := range audit.Prompt {
		if usablePromptInput(&item) && item.PromptProfile == content &&
			(!promptModelMatches(model, promptEvidenceModel(item, model)) || declaredModelRelation(item.Model, item.ResponseModel) < 0) {
			return true
		}
	}
	return false
}

func simplePromptSummary(items []TokenComparison) PromptSampleGroup {
	g := PromptSampleGroup{ID: "minimal", Planned: SimplePromptRequests, Valid: len(items)}
	if len(items) == 0 {
		return g
	}
	values := make([]int64, len(items))
	for i, item := range items {
		values[i] = *item.Actual
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	median := values[len(values)/2]
	if len(values)%2 == 0 {
		median = values[len(values)/2-1] + (median-values[len(values)/2-1])/2
	}
	g.Median, g.Min, g.Max = &median, &values[0], &values[len(values)-1]
	g.Stable = len(items) >= 2 && *g.Max-*g.Min <= 2*simplePromptTolerance(median)
	for _, value := range values {
		if math.Abs(float64(value-median)) > float64(simplePromptTolerance(median)) {
			g.Outliers++
		}
	}
	return g
}

func simplePromptScore(actual, expected, tolerance int64) int {
	difference := math.Abs(float64(actual - expected))
	if difference <= float64(tolerance) {
		return 100
	}
	// Outside the allowed range, reduce the score proportionally. Every valid
	// repeat contributes: a single anomalous response must not disappear behind
	// a matching median. The denominator prevents negative scores.
	return min(99, int(math.Round(100*(1-(difference-float64(tolerance))/math.Max(float64(actual), float64(expected))))))
}

func (r *runner) assessSimplePrompt() {
	audit := r.report.TokenAudit
	if len(audit.Prompt) == 0 {
		return
	}
	a := &PromptAssessment{Scoring: "input_consistency", TokenSource: "unavailable"}
	result := &PromptInjectionReport{Code: "simple_collecting", References: []PromptReferenceMatch{}}
	audit.PromptAssessment, audit.Injection = a, result
	items := simplePromptItems(audit, r.report.Model)
	g := simplePromptSummary(items)
	result.Sampling = []PromptSampleGroup{g}
	a.TokenMeasured, a.Coverage = len(items), len(items)*100/SimplePromptRequests
	for _, item := range audit.Prompt {
		if item.PromptChanged {
			result.LocalChanges++
		}
	}
	if !r.hasCheck("prompt_integrity") {
		return
	}
	if result.LocalChanges > 0 {
		result.Code = "local_prompt_changed"
		return // Directly observed rewriting is reported; no invented count score.
	}
	if simplePromptModelConflict(audit, r.report.Model) {
		result.Code = "simple_request_mismatch"
		return
	}
	result.Code = "simple_insufficient_samples"
	if len(items) < 2 {
		return
	}
	result.Platform = items[0].Platform
	for _, item := range items {
		if item.Profile != items[0].Profile || item.Platform != result.Platform {
			result.Code = "simple_request_mismatch"
			return
		}
	}
	result.Code = "simple_no_reference"
	var selected *ComparisonBaseline
	var selectedGroup PromptSampleGroup
	for i := range r.report.Baselines {
		ref := &r.report.Baselines[i]
		platform := nativePromptPlatform(ref.Type)
		if ref.ReportID == r.report.ID || platform == "" || platform != result.Platform ||
			(ref.Transport == "bedrock_runtime" && platform != "aws") || !promptModelMatches(r.report.Model, ref.Model) || simplePromptModelConflict(ref.TokenAudit, ref.Model) {
			result.Incompatible++
			continue
		}
		referenceItems := simplePromptItems(ref.TokenAudit, ref.Model)
		compatible := true
		for _, item := range referenceItems {
			compatible = compatible && item.Profile == items[0].Profile && (item.Platform == "" || item.Platform == platform)
		}
		reference := simplePromptSummary(referenceItems)
		if !compatible || !reference.Stable {
			result.Incompatible++
			continue
		}
		tolerance := max(simplePromptTolerance(*reference.Median), *reference.Median-*reference.Min, *reference.Max-*reference.Median)
		pair := PromptReferencePair{ID: "minimal", Expected: *reference.Median, Actual: *g.Median, Difference: *g.Median - *reference.Median, Tolerance: tolerance}
		result.References = append(result.References, PromptReferenceMatch{ID: ref.ID, ReportID: ref.ReportID, Type: ref.Type,
			Code: promptPairConclusion([]PromptReferencePair{pair}), Pairs: []PromptReferencePair{pair}})
		// Choosing the nearest token count would hide differences. Select by
		// operator save time, with a deterministic tie break, before scoring.
		if selected == nil || ref.CreatedAt > selected.CreatedAt || (ref.CreatedAt == selected.CreatedAt && ref.ID < selected.ID) {
			selected, selectedGroup = ref, reference
		}
	}
	if selected == nil {
		return
	}
	result.SelectedReferenceID, result.BaselineSamples = selected.ID, selectedGroup.Valid
	a.TokenSource = "saved_reference"
	expected := *selectedGroup.Median
	tolerance := max(simplePromptTolerance(expected), expected-*selectedGroup.Min, *selectedGroup.Max-expected)
	sum, aligned, extra, fewer := 0, 0, 0, 0
	for _, item := range items {
		sum += simplePromptScore(*item.Actual, expected, tolerance)
		switch {
		case *item.Actual > expected+tolerance:
			extra++
		case *item.Actual < expected-tolerance:
			fewer++
		default:
			aligned++
		}
	}
	score := int(math.Round(float64(sum) / float64(len(items))))
	if aligned != len(items) {
		score = min(score, 99)
	}
	a.Score, a.TokenScore = &score, &score
	switch {
	case aligned == len(items):
		result.Code = "simple_input_aligned"
	case extra >= 2:
		result.Code = "simple_extra_input"
	case fewer >= 2:
		result.Code = "simple_input_deviation"
	default:
		result.Code = "simple_variable_input"
	}
}
