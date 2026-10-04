package claudecheck

import (
	"math"
	"sort"
	"strings"
)

// v5 evaluates each condition independently. Reference absence and a single
// outlier no longer discard otherwise usable measurements or imply a defect.
type promptEvidenceGroup struct {
	summary PromptSampleGroup
	items   []TokenComparison
	median  int64
}

func promptModelMatches(a, b string) bool {
	return declaredModelRelation(a, b) == 1 || (a != "" && strings.EqualFold(a, b))
}

func promptEvidenceModel(item TokenComparison, fallback string) string {
	if item.ResponseModel != "" {
		return item.ResponseModel
	}
	if item.Model != "" {
		return item.Model
	}
	return fallback
}

func promptEvidenceItems(audit *TokenAuditReport, fixture promptAuditFixture) []TokenComparison {
	if audit == nil {
		return nil
	}
	var items []TokenComparison
	seen := map[string]bool{}
	for _, item := range audit.Prompt {
		if seen[item.ID] || !usablePromptInput(&item) || item.PromptChanged {
			continue
		}
		seen[item.ID] = true
		// Content, roles and tools must match. Model IDs and transport wrappers
		// are not prompt content. Legacy rows without content hashes may only
		// match an exact known request hash; never match by question name alone.
		matches := item.PromptProfile == promptContentProfile(fixture.body)
		if item.PromptProfile == "" {
			matches = item.Profile != "" && item.Profile == bodyProfile(fixture.body)
		}
		if !matches || declaredModelRelation(item.Model, item.ResponseModel) < 0 {
			continue
		}
		items = append(items, item)
	}
	return items
}

func groupPromptEvidence(items []TokenComparison, id string, minimum int) promptEvidenceGroup {
	g := promptEvidenceGroup{summary: PromptSampleGroup{ID: id, Planned: PromptRepetitions, Valid: len(items)}}
	if len(items) == 0 {
		return g
	}
	values := make([]int64, len(items))
	for i, item := range items {
		values[i] = *item.Actual
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	g.median = values[len(values)/2]
	low, high := values[0], values[len(values)-1]
	g.summary.Median, g.summary.Min, g.summary.Max = &g.median, &low, &high
	for _, item := range items {
		if math.Abs(float64(*item.Actual-g.median)) <= float64(promptTolerance(g.median)) {
			g.items = append(g.items, item)
		}
	}
	g.summary.Outliers = len(items) - len(g.items)
	g.summary.Stable = len(g.items) >= minimum && len(g.items)*2 > len(items)
	return g
}

func sameEndpointPromptPair(g promptEvidenceGroup) *PromptReferencePair {
	if !g.summary.Stable {
		return nil
	}
	var expected, actual []int64
	for _, item := range g.items {
		// A matching effective CountTokens request is still mandatory for this
		// source; malformed counts and request-profile mismatches stay excluded.
		if item.Expected == nil || *item.Expected <= 0 || item.Score == nil {
			continue
		}
		expected, actual = append(expected, *item.Expected), append(actual, *item.Actual)
	}
	if len(expected) < 2 {
		return nil
	}
	sort.Slice(expected, func(i, j int) bool { return expected[i] < expected[j] })
	sort.Slice(actual, func(i, j int) bool { return actual[i] < actual[j] })
	e, a := expected[len(expected)/2], actual[len(actual)/2]
	stableCounts := 0
	for _, n := range expected {
		if math.Abs(float64(n-e)) <= float64(promptTolerance(e)) {
			stableCounts++
		}
	}
	if stableCounts < 2 {
		return nil
	}
	return &PromptReferencePair{ID: g.summary.ID, Expected: e, Actual: a, Difference: a - e, Tolerance: promptTolerance(e)}
}

func promptPairScore(pair PromptReferencePair) int {
	item := TokenComparison{Expected: &pair.Expected, Actual: &pair.Actual}
	compareTokenCounts(&item)
	comparePromptTokens(&item)
	return *item.Score
}

func promptPairConclusion(pairs []PromptReferencePair) string {
	if len(pairs) == 0 {
		return "reference_incomplete"
	}
	aligned, excess := 0, 0
	for _, pair := range pairs {
		if math.Abs(float64(pair.Difference)) <= float64(pair.Tolerance) {
			aligned++
		}
		if pair.Difference > pair.Tolerance {
			excess++
		}
	}
	if aligned == len(pairs) {
		return "reference_aligned"
	}
	if excess > 0 {
		return "reference_extra_input"
	}
	return "reference_input_deviation"
}

func (r *runner) assessPracticalPrompt() {
	audit := r.report.TokenAudit
	if len(audit.Prompt) == 0 {
		return
	}
	result := &PromptInjectionReport{Code: "prompt_behavior_only", References: []PromptReferenceMatch{}}
	groups := map[string]promptEvidenceGroup{}
	fixtures := repeatedPromptConditions(r.report.Model)
	for _, fixture := range fixtures {
		items := promptEvidenceItems(audit, fixture)
		// Reject known model conflicts per condition, rather than blocking all
		// other conditions because an unrelated sample is missing a field.
		var compatible []TokenComparison
		for _, item := range items {
			if declaredModelRelation(r.report.Model, promptEvidenceModel(item, r.report.Model)) >= 0 {
				compatible = append(compatible, item)
			}
		}
		g := groupPromptEvidence(compatible, fixture.id, 2)
		groups[fixture.id] = g
		result.Sampling = append(result.Sampling, g.summary)
	}
	for _, item := range audit.Prompt {
		if item.PromptChanged {
			result.LocalChanges++
		}
	}
	if a, b := groups["floor_a"], groups["system_canary"]; a.summary.Median != nil && b.summary.Median != nil {
		d := b.median - a.median
		result.SystemDelta = &d
	}
	byCondition := map[string][]PromptReferencePair{}
	for _, ref := range r.report.Baselines {
		match := PromptReferenceMatch{ID: ref.ID, ReportID: ref.ReportID, Type: ref.Type, Pairs: []PromptReferencePair{}}
		if ref.ReportID != r.report.ID {
			for _, f := range fixtures {
				g := groups[f.id]
				if !g.summary.Stable {
					continue
				}
				var compatible []TokenComparison
				for _, item := range promptEvidenceItems(ref.TokenAudit, f) {
					if promptModelMatches(promptEvidenceModel(g.items[0], r.report.Model), promptEvidenceModel(item, ref.Model)) {
						compatible = append(compatible, item)
					}
				}
				// Saved single observations remain single observations. They are
				// useful references, not fabricated three-repeat certifications.
				reference := groupPromptEvidence(compatible, f.id, 1)
				if !reference.summary.Stable {
					continue
				}
				pair := PromptReferencePair{ID: f.id, Expected: reference.median, Actual: g.median, Difference: g.median - reference.median, Tolerance: promptTolerance(reference.median)}
				match.Pairs = append(match.Pairs, pair)
				byCondition[f.id] = append(byCondition[f.id], pair)
			}
		}
		match.Code = promptPairConclusion(match.Pairs)
		if len(match.Pairs) == 0 {
			result.Incompatible++
		}
		result.References = append(result.References, match)
	}
	// Select one matching reference for scoring; never construct a perfect
	// match by taking each condition from a different baseline type. Other
	// references remain visible as differences, not mandatory simultaneous
	// requirements that would penalize a legitimate platform template.
	var selected *PromptReferenceMatch
	for i := range result.References {
		ref := &result.References[i]
		if len(ref.Pairs) == 0 {
			continue
		}
		if selected == nil || referenceMean(ref.Pairs) > referenceMean(selected.Pairs) ||
			(referenceMean(ref.Pairs) == referenceMean(selected.Pairs) && len(ref.Pairs) > len(selected.Pairs)) {
			selected = ref
		}
	}
	var chosen []PromptReferencePair
	sources := map[string]bool{}
	conflict := false
	for _, f := range fixtures {
		pairs := byCondition[f.id]
		if len(pairs) > 1 {
			code := promptPairConclusion(pairs[:1])
			for _, pair := range pairs[1:] {
				conflict = conflict || promptPairConclusion([]PromptReferencePair{pair}) != code
			}
		}
		matched := false
		if selected != nil {
			result.SelectedReferenceID = selected.ID
			for _, pair := range selected.Pairs {
				if pair.ID == f.id {
					chosen = append(chosen, pair)
					sources["saved_reference"] = true
					matched = true
					break
				}
			}
		}
		if !matched {
			if pair := sameEndpointPromptPair(groups[f.id]); pair != nil {
				chosen = append(chosen, *pair)
				sources["same_endpoint"] = true
			}
		}
	}
	if len(chosen) > 0 {
		result.Code = promptPairConclusion(chosen)
		if !sources["saved_reference"] {
			switch result.Code {
			case "reference_aligned":
				result.Code = "same_endpoint_aligned"
			case "reference_extra_input":
				result.Code = "same_endpoint_extra_input"
			default:
				result.Code = "same_endpoint_input_deviation"
			}
		}
	}
	if conflict {
		result.Code = "references_disagree"
	}
	if result.LocalChanges > 0 {
		result.Code = "local_prompt_changed"
	}
	audit.Injection = result
	r.scorePracticalPrompt(chosen, sources)
}

func (r *runner) scorePracticalPrompt(pairs []PromptReferencePair, sources map[string]bool) {
	audit := r.report.TokenAudit
	a := audit.PromptAssessment
	a.Scoring, a.TokenSource = "measured_checks", "unavailable"
	if len(sources) == 1 {
		for source := range sources {
			a.TokenSource = source
		}
	} else if len(sources) > 1 {
		a.TokenSource = "mixed"
	}
	behavior := 0
	for _, f := range repeatedPromptFixtures(r.report.Model) {
		item := promptItem(audit.Prompt, f.id)
		if item != nil && item.Behavior != nil && item.Behavior.Score != nil {
			behavior += *item.Behavior.Score
		}
	}
	if a.BehaviorMeasured > 0 {
		score := int(math.Round(float64(behavior) / float64(a.BehaviorMeasured)))
		a.BehaviorScore = &score
	}
	tokens := 0
	for _, pair := range pairs {
		tokens += promptPairScore(pair)
	}
	a.TokenMeasured = len(pairs)
	if len(pairs) > 0 {
		score := int(math.Round(float64(tokens) / float64(len(pairs))))
		a.TokenScore = &score
	}
	weighted := float64(behavior)*30/900 + float64(tokens)*70/300
	weight := float64(a.BehaviorMeasured)*30/9 + float64(len(pairs))*70/3
	a.TokenPoints = int(math.Round(float64(tokens) * 70 / 300))
	a.Coverage = int(math.Round(weight))
	a.Score = nil
	if weight > 0 && r.hasCheck("prompt_integrity") {
		score := int(math.Round(weighted * 100 / weight))
		a.Score = &score
	}
	// Directly observed prompt rewriting is adverse evidence, not missing data.
	if audit.Injection.LocalChanges > 0 && r.hasCheck("prompt_integrity") {
		zero := 0
		a.Score = &zero
	}
}

func referenceMean(pairs []PromptReferencePair) float64 {
	sum := 0
	for _, pair := range pairs {
		sum += promptPairScore(pair)
	}
	return float64(sum) / float64(len(pairs))
}
