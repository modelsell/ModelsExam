package claudecheck

import (
	"fmt"
	"math"
	"sort"
)

const PromptRepetitions = 3
const RepeatedPromptRequests = 3 * PromptRepetitions * 2

// The three conditions vary visible input length and system presence. Repeating
// whole rounds exposes temporal routing changes as well as within-case jitter.
func repeatedPromptConditions(model string) []promptAuditFixture {
	minimal := promptChannelFixtures(model)
	long := promptAuditFixtures(model)[2]
	return []promptAuditFixture{minimal[0], long, minimal[2]}
}

func repeatedPromptFixtures(model string) []promptAuditFixture {
	var fixtures []promptAuditFixture
	for round := 1; round <= PromptRepetitions; round++ {
		for _, f := range repeatedPromptConditions(model) {
			fixtures = append(fixtures, promptAuditFixture{fmt.Sprintf("%s_r%d", f.id, round), f.body})
		}
	}
	return fixtures
}

type PromptSampleGroup struct {
	Outliers int    `json:"outliers,omitempty"`
	ID       string `json:"id"`
	Valid    int    `json:"valid"`
	Planned  int    `json:"planned"`
	Median   *int64 `json:"median"`
	Min      *int64 `json:"min"`
	Max      *int64 `json:"max"`
	Stable   bool   `json:"stable"`
}

// Never reconstruct reference evidence from stored summaries: validate all raw
// samples and their effective request/model profiles before taking a median.
func summarizePromptSamples(audit *TokenAuditReport, model string) ([]PromptSampleGroup, []TokenComparison, bool) {
	var groups []PromptSampleGroup
	var medians []TokenComparison
	ready := true
	for _, f := range repeatedPromptConditions(model) {
		g := PromptSampleGroup{ID: f.id, Planned: PromptRepetitions}
		var values []int64
		var first *TokenComparison
		profilesMatch := true
		for round := 1; round <= PromptRepetitions; round++ {
			item := promptItem(audit.Prompt, fmt.Sprintf("%s_r%d", f.id, round))
			if !usablePromptInput(item) {
				continue
			}
			values = append(values, *item.Actual)
			if first == nil {
				first = item
			}
			profilesMatch = profilesMatch && !item.PromptChanged && item.Profile != "" && item.Profile == first.Profile &&
				item.ClientProfile == bodyProfile(f.body) && item.PromptProfile == promptContentProfile(f.body) &&
				declaredModelRelation(model, item.ResponseModel) == 1 && declaredModelRelation(model, item.Model) == 1 && declaredModelRelation(first.Model, item.Model) == 1
		}
		g.Valid = len(values)
		if len(values) > 0 {
			sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
			median, min, max := values[len(values)/2], values[0], values[len(values)-1]
			g.Median, g.Min, g.Max = &median, &min, &max
			g.Stable = len(values) == PromptRepetitions && profilesMatch && max-min <= 2*promptTolerance(median)
			item := *first
			item.ID, item.Actual = f.id, &median
			medians = append(medians, item)
		}
		ready = ready && g.Stable
		groups = append(groups, g)
	}
	return groups, medians, ready
}

func (audit *TokenAuditReport) assessRepeatedPrompt() {
	a := &PromptAssessment{BehaviorPlanned: 3 * PromptRepetitions}
	sum := 0
	seen := map[string]bool{}
	for _, f := range repeatedPromptFixtures("") {
		item := promptItem(audit.Prompt, f.id)
		if item == nil || seen[item.ID] || item.Behavior == nil || item.Behavior.Score == nil {
			continue
		}
		seen[item.ID] = true
		if score := *item.Behavior.Score; score >= 0 && score <= 100 {
			a.BehaviorMeasured++
			sum += score
		}
	}
	a.BehaviorPoints = int(math.Round(float64(sum) * 30 / float64(a.BehaviorPlanned*100)))
	audit.PromptAssessment = a
}

func (r *runner) assessRepeatedInjection() {
	audit := r.report.TokenAudit
	if len(audit.Prompt) == 0 {
		return
	}
	groups, medians, stable := summarizePromptSamples(audit, r.report.Model)
	result := &PromptInjectionReport{Code: "injection_unverified", Sampling: groups, References: []PromptReferenceMatch{}}
	for _, item := range audit.Prompt {
		if item.PromptChanged {
			result.LocalChanges++
		}
	}
	result.SystemDelta = promptDelta(promptItem(medians, "floor_a"), promptItem(medians, "system_canary"))
	complete := []PromptReferenceMatch{}
	fixtures := repeatedPromptConditions(r.report.Model)
	for _, ref := range r.report.Baselines {
		if ref.ReportID == r.report.ID || ref.TokenAudit == nil || ref.TokenAudit.Version != 4 || declaredModelRelation(r.report.Model, ref.Model) != 1 {
			result.Incompatible++
			continue
		}
		_, refMedians, refStable := summarizePromptSamples(ref.TokenAudit, r.report.Model)
		floor, long, system := promptItem(refMedians, "floor_a"), promptItem(refMedians, "long"), promptItem(refMedians, "system_canary")
		controlsRespond := usablePromptInput(floor) && usablePromptInput(long) && usablePromptInput(system) && *long.Actual > *floor.Actual && *system.Actual > *floor.Actual
		if !refStable || !controlsRespond {
			result.Incompatible++
			continue
		}
		target := Report{TokenAudit: &TokenAuditReport{Prompt: medians}}
		ref.TokenAudit = &TokenAuditReport{Prompt: refMedians}
		match := comparePromptReference(&target, ref, fixtures)
		if !stable {
			match.Code = "reference_incomplete"
		}
		result.References = append(result.References, match)
		if match.Code != "reference_incomplete" {
			complete = append(complete, match)
		}
	}
	if len(complete) > 0 {
		result.Code = complete[0].Code
		for _, match := range complete[1:] {
			if match.Code != result.Code {
				result.Code = "references_disagree"
			}
		}
	} else if stable {
		// Same-endpoint discrepancies must persist in all three repeats of at
		// least two conditions. A single count estimate cannot flag injection.
		excessConditions := 0
		for _, f := range fixtures {
			excess := 0
			for round := 1; round <= PromptRepetitions; round++ {
				item := promptItem(audit.Prompt, fmt.Sprintf("%s_r%d", f.id, round))
				if item != nil && item.Difference != nil && item.Tolerance != nil && *item.Difference > *item.Tolerance {
					excess++
				}
			}
			if excess == PromptRepetitions {
				excessConditions++
			}
		}
		if excessConditions >= 2 {
			result.Code = "same_endpoint_extra_input"
		}
	} else {
		result.Code = "prompt_samples_unstable"
		for _, g := range groups {
			if g.Valid < g.Planned {
				result.Code = "prompt_samples_incomplete"
			}
		}
	}
	if result.LocalChanges > 0 {
		result.Code = "local_prompt_changed"
	}
	audit.Injection = result
	assessReferencePoints(audit, complete)
	// Live events retain all measurements, but do not advertise a final score
	// before the scheduled repetitions finish (including count requests).
	if !r.hasCheck("prompt_integrity") {
		audit.PromptAssessment.Score = nil
	}
}
