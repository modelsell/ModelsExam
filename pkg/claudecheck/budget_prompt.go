package claudecheck

import "math"

// This allowance belongs only to simplePromptFixtures. It is a conservative
// product heuristic, not an official token count or proof of a clean request.
// In particular it allows the observed 43-token short responses without using
// any saved channel as a trusted reference. Small additions can be missed.
const PromptInputAllowance int64 = 64

func budgetPromptScore(actual int64) int {
	if actual <= PromptInputAllowance {
		return 100
	}
	return min(99, int(math.Round(100*float64(PromptInputAllowance)/float64(actual))))
}

func (r *runner) assessBudgetPrompt() {
	audit := r.report.TokenAudit
	if len(audit.Prompt) == 0 {
		return
	}
	a := &PromptAssessment{Scoring: "input_budget", TokenSource: "minimal_request_budget"}
	result := &PromptInjectionReport{Code: "budget_collecting", AllowanceTokens: PromptInputAllowance,
		References: []PromptReferenceMatch{}}
	audit.PromptAssessment, audit.Injection = a, result
	fixtures := simplePromptFixtures(r.report.Model)
	content := promptContentProfile(fixtures[0].body)
	allowed := map[string]bool{}
	for _, fixture := range fixtures {
		allowed[fixture.id] = true
	}
	seen := map[string]bool{}
	var items []TokenComparison
	sum := 0
	var extras []int64
	for i := range audit.Prompt {
		item := &audit.Prompt[i]
		if !allowed[item.ID] || seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		// Reassessment must never retain an earlier score for an unusable row.
		item.Expected, item.Difference, item.Score = nil, nil, nil
		if item.Code == "budget_within_allowance" || item.Code == "budget_extra_input" {
			item.Code = "missing_usage"
		}
		if item.PromptChanged {
			result.LocalChanges++
			item.Code = "local_prompt_changed"
			continue
		}
		// The amount check does not depend on platform/model fingerprints or
		// other channels. Only a completed, unchanged minimal prompt qualifies.
		if !usablePromptInput(item) || (item.PromptProfile != "" && item.PromptProfile != content) {
			continue
		}
		allowance := PromptInputAllowance
		extra, score := max(int64(0), *item.Actual-allowance), budgetPromptScore(*item.Actual)
		item.Expected, item.Difference, item.Score = &allowance, &extra, &score
		item.Code = "budget_within_allowance"
		if extra > 0 {
			item.Code = "budget_extra_input"
			result.ExtraRounds++
		}
		sum += score
		extras = append(extras, extra)
		items = append(items, *item)
	}
	result.Sampling = []PromptSampleGroup{simplePromptSummary(items)}
	a.TokenMeasured, a.Coverage = len(items), len(items)*100/SimplePromptRequests
	if len(items) > 0 && result.LocalChanges == 0 {
		// Divide before summing to avoid overflow for untrusted usage counters.
		var average, remainder, peak int64
		n := int64(len(items))
		for _, extra := range extras {
			average += extra / n
			remainder += extra % n
			peak = max(peak, extra)
		}
		average += (remainder + n/2) / n
		result.EstimatedExtraTokens, result.MaxExtraTokens = &average, &peak
	}
	if !r.hasCheck("prompt_integrity") {
		return
	}
	if result.LocalChanges > 0 {
		result.Code = "local_prompt_changed"
		return // Rewriting is visible, but its token amount is not known.
	}
	result.Code = "budget_insufficient_samples"
	if len(items) == 0 {
		return
	}
	score := int(math.Round(float64(sum) / float64(len(items))))
	result.Code = "budget_no_obvious_injection"
	if result.ExtraRounds > 0 {
		score = min(99, score)
		result.Code = "budget_extra_input"
	}
	a.Score, a.TokenScore = &score, &score
}
