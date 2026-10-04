package claudecheck

import "math"

type PromptReferencePair struct {
	ID         string `json:"id"`
	Expected   int64  `json:"expected"`
	Actual     int64  `json:"actual"`
	Difference int64  `json:"difference"`
	Tolerance  int64  `json:"tolerance"`
}

type PromptReferenceMatch struct {
	ID       string                `json:"id"`
	ReportID string                `json:"report_id"`
	Type     string                `json:"type"`
	Code     string                `json:"code"`
	Pairs    []PromptReferencePair `json:"pairs"`
}

// This is attribution evidence, separate from the behavior/count quality score.
// A proxy controls both response text and counters; black-box matches cannot
// authenticate its hidden context. All references are operator-selected claims.
type PromptInjectionReport struct {
	AllowanceTokens      int64                  `json:"allowance_tokens,omitempty"`
	EstimatedExtraTokens *int64                 `json:"estimated_extra_tokens,omitempty"`
	MaxExtraTokens       *int64                 `json:"max_extra_tokens,omitempty"`
	ExtraRounds          int                    `json:"extra_rounds,omitempty"`
	BaselineSamples      int                    `json:"baseline_samples,omitempty"`
	Platform             string                 `json:"platform,omitempty"`
	SelectedReferenceID  string                 `json:"selected_reference_id,omitempty"`
	Sampling             []PromptSampleGroup    `json:"sampling,omitempty"`
	Code                 string                 `json:"code"`
	LocalChanges         int                    `json:"local_changes"`
	RepeatDelta          *int64                 `json:"repeat_delta"`
	SystemDelta          *int64                 `json:"system_delta"`
	Incompatible         int                    `json:"incompatible_references"`
	References           []PromptReferenceMatch `json:"references"`
}

func promptTolerance(expected int64) int64 {
	return int64(math.Max(2, math.Ceil(float64(expected)*0.01)))
}

func promptItem(items []TokenComparison, id string) *TokenComparison {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

func usablePromptInput(item *TokenComparison) bool {
	return item != nil && item.Actual != nil && *item.Actual > 0
}

func promptDelta(a, b *TokenComparison) *int64 {
	if !usablePromptInput(a) || !usablePromptInput(b) {
		return nil
	}
	delta := *b.Actual - *a.Actual
	return &delta
}

func validPromptReference(reference ComparisonBaseline, model string, fixtures []promptAuditFixture) bool {
	if reference.TokenAudit == nil || reference.TokenAudit.Version != 3 || declaredModelRelation(model, reference.Model) != 1 {
		return false
	}
	for _, f := range fixtures {
		item := promptItem(reference.TokenAudit.Prompt, f.id)
		if !usablePromptInput(item) || item.PromptChanged || item.Profile == "" ||
			item.ClientProfile != bodyProfile(f.body) || item.PromptProfile != promptContentProfile(f.body) ||
			declaredModelRelation(reference.Model, item.Model) != 1 {
			return false
		}
	}
	items := reference.TokenAudit.Prompt
	floor := promptItem(items, "floor_a")
	repeat := promptItem(items, "floor_b")
	canary := promptItem(items, "system_canary")
	return floor.Profile == repeat.Profile && math.Abs(float64(*repeat.Actual-*floor.Actual)) <= float64(promptTolerance(*floor.Actual)) && *canary.Actual > *floor.Actual
}

func comparePromptReference(report *Report, ref ComparisonBaseline, fixtures []promptAuditFixture) PromptReferenceMatch {
	result := PromptReferenceMatch{ID: ref.ID, ReportID: ref.ReportID, Type: ref.Type, Code: "reference_incomplete", Pairs: []PromptReferencePair{}}
	for _, fixture := range fixtures {
		item, reference := promptItem(report.TokenAudit.Prompt, fixture.id), promptItem(ref.TokenAudit.Prompt, fixture.id)
		if !usablePromptInput(item) || item.Profile != reference.Profile || item.ClientProfile != reference.ClientProfile ||
			item.PromptProfile != reference.PromptProfile || declaredModelRelation(item.Model, reference.Model) != 1 {
			continue
		}
		delta := *item.Actual - *reference.Actual
		result.Pairs = append(result.Pairs, PromptReferencePair{fixture.id, *reference.Actual, *item.Actual, delta, promptTolerance(*reference.Actual)})
	}
	if len(result.Pairs) != 3 {
		return result
	}
	excess, matches := 0, 0
	min, max, tolerance := result.Pairs[0].Difference, result.Pairs[0].Difference, int64(2)
	for _, pair := range result.Pairs {
		if pair.Difference > pair.Tolerance {
			excess++
		}
		if math.Abs(float64(pair.Difference)) <= float64(pair.Tolerance) {
			matches++
		}
		if pair.Difference < min {
			min = pair.Difference
		}
		if pair.Difference > max {
			max = pair.Difference
		}
		if pair.Tolerance > tolerance {
			tolerance = pair.Tolerance
		}
	}
	switch {
	case matches == 3:
		result.Code = "reference_aligned"
	case excess == 3 && max-min <= 2*tolerance:
		result.Code = "reference_fixed_excess"
	case excess >= 2:
		result.Code = "reference_extra_input"
	default:
		result.Code = "reference_input_deviation"
	}
	return result
}

func (r *runner) assessPromptInjection() {
	audit := r.report.TokenAudit
	if audit != nil && audit.Version >= 7 {
		r.assessBudgetPrompt()
		return
	}
	if audit != nil && audit.Version >= 6 {
		r.assessSimplePrompt()
		return
	}
	if audit != nil && audit.Version >= 5 {
		r.assessPracticalPrompt()
		return
	}
	if audit != nil && audit.Version >= 4 {
		r.assessRepeatedInjection()
		return
	}
	if audit == nil || audit.Version != 3 || len(audit.Prompt) == 0 {
		return
	}
	result := &PromptInjectionReport{Code: "injection_unverified", References: []PromptReferenceMatch{}}
	fixtures := promptChannelFixtures(r.report.Model)
	for _, item := range audit.Prompt {
		if item.PromptChanged {
			result.LocalChanges++
		}
	}
	a, b, system := promptItem(audit.Prompt, "floor_a"), promptItem(audit.Prompt, "floor_b"), promptItem(audit.Prompt, "system_canary")
	if a != nil && b != nil && a.Profile != "" && a.Profile == b.Profile {
		result.RepeatDelta = promptDelta(a, b)
	}
	result.SystemDelta = promptDelta(a, system)
	for _, ref := range r.report.Baselines {
		if ref.ReportID == r.report.ID || !validPromptReference(ref, r.report.Model, fixtures) {
			result.Incompatible++
			continue
		}
		result.References = append(result.References, comparePromptReference(&r.report, ref, fixtures))
	}
	// Do not select the closest baseline and hide conflicting references.
	var complete []PromptReferenceMatch
	for _, ref := range result.References {
		if ref.Code != "reference_incomplete" {
			complete = append(complete, ref)
		}
	}
	if len(complete) > 0 {
		result.Code = complete[0].Code
		for _, ref := range complete[1:] {
			if ref.Code != result.Code {
				result.Code = "references_disagree"
				break
			}
		}
	} else if result.RepeatDelta != nil && math.Abs(float64(*result.RepeatDelta)) > float64(promptTolerance(*a.Actual)) {
		result.Code = "repeat_input_changed"
	} else {
		for _, item := range audit.Prompt {
			if item.Difference != nil && item.Tolerance != nil && *item.Difference > *item.Tolerance {
				result.Code = "same_endpoint_extra_input"
				break
			}
		}
	}
	if result.LocalChanges > 0 {
		result.Code = "local_prompt_changed"
	}
	audit.Injection = result
	assessReferencePoints(audit, complete)
}

func assessReferencePoints(audit *TokenAuditReport, references []PromptReferenceMatch) {
	a := audit.PromptAssessment
	if a == nil {
		return
	}
	// Same-endpoint counts can be rewritten together. In v3 only compatible
	// saved reference usage contributes the 70 reference points.
	a.TokenSource, a.TokenPoints, a.TokenMeasured = "reference_unavailable", 0, 0
	if len(references) > 0 && audit.Injection.Code != "references_disagree" && audit.Injection.LocalChanges == 0 {
		a.TokenSource, a.TokenMeasured = "saved_reference", 3
		sum := 0
		for i := 0; i < 3; i++ {
			minimum := 100
			for _, reference := range references {
				pair := reference.Pairs[i]
				item := TokenComparison{Expected: &pair.Expected, Actual: &pair.Actual}
				compareTokenCounts(&item)
				comparePromptTokens(&item)
				if item.Score != nil && *item.Score < minimum {
					minimum = *item.Score
				}
			}
			sum += minimum
		}
		a.TokenPoints = int(math.Round(float64(sum) * 70 / 300))
	}
	if audit.Injection.Code == "references_disagree" {
		a.TokenSource = "references_disagree"
	}
	planned := 3
	if a.BehaviorPlanned > 0 {
		planned = a.BehaviorPlanned
	}
	a.Coverage = int(math.Round(float64(a.BehaviorMeasured)*30/float64(planned) + float64(a.TokenMeasured)*70/3))
	a.Score = nil
	if a.BehaviorMeasured+a.TokenMeasured > 0 {
		score := a.BehaviorPoints + a.TokenPoints
		a.Score = &score
	}
}
