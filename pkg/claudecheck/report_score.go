package claudecheck

import "math"

// ReportScore returns the same five-dimension mean shown in the report sheet.
// Unmeasured dimensions are omitted, including legacy identity evidence and
// Bedrock boundary probes. A measured zero remains distinct from no score.
// Keep this aligned with the shared report_scores.json frontend/backend cases.
func ReportScore(report Report) *int {
	var identity *int
	if report.Version >= 6 {
		consistency, echo := reportScoreCheck(report.Checks, "model_consistency"), reportScoreCheck(report.Checks, "model_echo")
		switch {
		case consistency != nil && consistency.Status == "fail", echo != nil && echo.Status == "fail":
			identity = reportScoreValue(0)
		case consistency != nil && consistency.Status == "pass":
			identity = reportScoreValue(100)
		}
	}

	var capabilityScores []*int
	if report.Benchmark != nil && report.Benchmark.Version == 1 {
		for _, item := range report.Benchmark.Items {
			switch item.ID {
			case "digit_count", "boolean_count", "python_alias", "javascript_queue", "zh_constraint", "context_retrieval":
				if item.Correct != nil {
					score := 0
					if *item.Correct {
						score = 100
					}
					capabilityScores = append(capabilityScores, reportScoreValue(score))
				}
			}
		}
	}
	if report.Benchmark != nil && (report.Benchmark.Version == 2 || report.Benchmark.Version == 3) {
		seen := make(map[string]bool)
		for _, item := range report.Benchmark.Items {
			switch item.ID {
			case "digit_count", "boolean_count", "python_alias", "javascript_queue", "zh_constraint", "context_retrieval",
				"instruction_format", "json_extraction", "json_types", "context_multihop", "tool_selection", "tool_roundtrip":
				if report.Benchmark.Version == 3 && !currentCapabilityID(item.ID) {
					continue
				}
				if seen[item.ID] {
					continue
				}
				seen[item.ID] = true
				if item.Score != nil && *item.Score >= 0 && *item.Score <= 100 {
					capabilityScores = append(capabilityScores, item.Score)
				}
			}
		}
	}

	var prompt, cache *int
	if audit := report.TokenAudit; audit != nil {
		prompt, cache = reportScoreAudit(audit.Prompt), reportScoreAudit(audit.Cache)
		if audit.Version >= 2 && audit.PromptAssessment != nil {
			prompt = audit.PromptAssessment.Score
		}
		if audit.Version >= 4 {
			cache = nil
			if audit.CacheAssessment != nil {
				cache = audit.CacheAssessment.Score
			}
		}
	}

	var protocolScores []*int
	for _, check := range report.Checks {
		switch check.ID {
		case "basic", "stream", "tool", "structured_tool", "vision", "pdf", "max_tokens", "stop_sequence", "veridrop_pdf", "veridrop_tool":
			// The historical aliases match presentReport in the report page.
			switch check.Status {
			case "pass":
				protocolScores = append(protocolScores, reportScoreValue(100))
			case "fail":
				protocolScores = append(protocolScores, reportScoreValue(0))
			}
		}
	}
	protocolScores = append(protocolScores, AssessUsageTokens(report).Score)
	return reportScoreMean([]*int{identity, reportScoreMean(capabilityScores), prompt, cache, reportScoreMean(protocolScores)})
}

func currentCapabilityID(id string) bool {
	switch id {
	case "instruction_format", "json_extraction", "json_types", "context_multihop", "tool_selection", "tool_roundtrip":
		return true
	default:
		return false
	}
}

func reportScoreCheck(checks []Check, id string) *Check {
	for i := range checks {
		if checks[i].ID == id {
			return &checks[i]
		}
	}
	return nil
}

func reportScoreAudit(items []TokenComparison) *int {
	values := make([]*int, 0, len(items))
	for _, item := range items {
		values = append(values, item.Score)
	}
	return reportScoreMean(values)
}

func reportScoreMean(values []*int) *int {
	var sum float64
	var count int
	for _, value := range values {
		if value != nil && *value >= 0 {
			sum += float64(*value)
			count++
		}
	}
	if count == 0 {
		return nil
	}
	return reportScoreValue(int(math.Round(sum / float64(count))))
}

func reportScoreValue(value int) *int { return &value }
