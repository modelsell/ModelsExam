package claudecheck

import (
	"math"
	"strconv"
	"testing"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

func collectBudgetPrompt(t *testing.T, rounds []simplePromptRound, refs ...ComparisonBaseline) Report {
	t.Helper()
	return collectSimplePromptVersion(t, "budget-test", "anthropic", rounds, refs, 7)
}

func TestBudgetPromptNoReferenceScoresObservedAmounts(t *testing.T) {
	for _, tc := range []struct {
		input int64
		score int
	}{{15, 100}, {43, 100}, {64, 100}, {65, 98}, {128, 50}, {256, 25}, {7078, 1}, {math.MaxInt64, 0}} {
		t.Run(strconv.FormatInt(tc.input, 10), func(t *testing.T) {
			r := collectBudgetPrompt(t, simplePromptValues(tc.input, tc.input, tc.input))
			a, result := r.TokenAudit.PromptAssessment, r.TokenAudit.Injection
			require.Equal(t, "input_budget", a.Scoring)
			require.Equal(t, "minimal_request_budget", a.TokenSource)
			require.Equal(t, tc.score, *a.Score)
			require.Equal(t, 100, a.Coverage)
			require.Equal(t, PromptInputAllowance, result.AllowanceTokens)
			require.Equal(t, max(int64(0), tc.input-64), *result.EstimatedExtraTokens)
			require.Equal(t, *result.EstimatedExtraTokens, *result.MaxExtraTokens)
			require.Empty(t, result.SelectedReferenceID)
			require.Empty(t, result.References)
			if tc.input <= 64 {
				require.Equal(t, "budget_no_obvious_injection", result.Code)
				require.Zero(t, result.ExtraRounds)
			} else {
				require.Equal(t, "budget_extra_input", result.Code)
				require.Equal(t, 3, result.ExtraRounds)
			}
		})
	}
}

func TestBudgetPromptCountsCachedHiddenInput(t *testing.T) {
	r := collectBudgetPrompt(t, []simplePromptRound{{input: 43, write: 7035}, {input: 43, read: 7035}, {input: 0, read: 7078}})
	require.Equal(t, 1, *r.TokenAudit.PromptAssessment.Score)
	require.EqualValues(t, 7014, *r.TokenAudit.Injection.EstimatedExtraTokens)
	for _, item := range r.TokenAudit.Prompt {
		require.EqualValues(t, 7078, *item.Actual)
	}
}

func TestBudgetPromptKeepsSingleAnomalousRound(t *testing.T) {
	r := collectBudgetPrompt(t, simplePromptValues(43, 43, 7078))
	require.Equal(t, 67, *r.TokenAudit.PromptAssessment.Score)
	require.EqualValues(t, 43, *r.TokenAudit.Injection.Sampling[0].Median)
	require.EqualValues(t, 2338, *r.TokenAudit.Injection.EstimatedExtraTokens)
	require.EqualValues(t, 7014, *r.TokenAudit.Injection.MaxExtraTokens)
	require.Equal(t, 1, r.TokenAudit.Injection.ExtraRounds)
	r = collectBudgetPrompt(t, simplePromptValues(64, 64, 65))
	require.Less(t, *r.TokenAudit.PromptAssessment.Score, 100)
}

func TestBudgetPromptDoesNotDependOnBaselinesOrModelClaims(t *testing.T) {
	rounds := simplePromptValues(7078, 7078, 7078)
	rounds[1].model = "claude-sonnet-5"
	without := collectBudgetPrompt(t, rounds)
	with := collectBudgetPrompt(t, rounds, simplePromptReference(t, "same-high-count", "anthropic", "anthropic", 100, 7078, 7078, 7078))
	require.Equal(t, without.TokenAudit.PromptAssessment, with.TokenAudit.PromptAssessment)
	require.Equal(t, without.TokenAudit.Injection, with.TokenAudit.Injection)
	require.Equal(t, 1, *with.TokenAudit.PromptAssessment.Score)
}

func TestBudgetPromptPartialUsageHasSeparateCoverage(t *testing.T) {
	r := collectBudgetPrompt(t, []simplePromptRound{{input: 43}, {status: 500}, {status: 502}})
	require.Equal(t, 100, *r.TokenAudit.PromptAssessment.Score)
	require.Equal(t, 33, r.TokenAudit.PromptAssessment.Coverage)
	require.Equal(t, 1, r.TokenAudit.PromptAssessment.TokenMeasured)
	require.Len(t, r.TokenAudit.Prompt, 3)
	require.Nil(t, r.TokenAudit.Prompt[1].Score)
	require.Nil(t, r.TokenAudit.Prompt[2].Actual)
	r = collectBudgetPrompt(t, simplePromptValues(0, -1, 0))
	require.Nil(t, r.TokenAudit.PromptAssessment.Score)
	require.Nil(t, r.TokenAudit.Injection.EstimatedExtraTokens)
	require.Equal(t, "budget_insufficient_samples", r.TokenAudit.Injection.Code)
}

func TestBudgetPromptReportsKnownLocalRewrite(t *testing.T) {
	rounds := simplePromptValues(43, 43, 43)
	rounds[1].edit = func(body map[string]any) { body["system"] = "An added instruction." }
	r := collectBudgetPrompt(t, rounds)
	require.Nil(t, r.TokenAudit.PromptAssessment.Score)
	require.Equal(t, "local_prompt_changed", r.TokenAudit.Injection.Code)
	require.Equal(t, 1, r.TokenAudit.Injection.LocalChanges)
	require.Nil(t, r.TokenAudit.Injection.EstimatedExtraTokens)
	require.Nil(t, r.TokenAudit.Injection.MaxExtraTokens)
	require.Nil(t, r.TokenAudit.Prompt[1].Score)
}

func TestBudgetPromptPersistenceAndRescoring(t *testing.T) {
	r := collectBudgetPrompt(t, simplePromptValues(43, 43, 7078))
	encoded, err := common.Marshal(r)
	require.NoError(t, err)
	var restored Report
	require.NoError(t, common.Unmarshal(encoded, &restored))
	require.Equal(t, r.TokenAudit, restored.TokenAudit)
	// Metadata availability and platform labels do not gate the amount score.
	for i := range restored.TokenAudit.Prompt {
		restored.TokenAudit.Prompt[i].Profile = ""
		restored.TokenAudit.Prompt[i].PromptProfile = ""
		restored.TokenAudit.Prompt[i].Platform = "custom"
	}
	runner := runner{report: restored}
	runner.assessPromptInjection()
	require.Equal(t, 67, *runner.report.TokenAudit.PromptAssessment.Score)
	// A stale score cannot survive removal of all input evidence.
	for i := range runner.report.TokenAudit.Prompt {
		runner.report.TokenAudit.Prompt[i].Actual = nil
	}
	runner.assessPromptInjection()
	require.Nil(t, runner.report.TokenAudit.PromptAssessment.Score)
	for _, item := range runner.report.TokenAudit.Prompt {
		require.Nil(t, item.Score)
		require.Equal(t, "missing_usage", item.Code)
	}
}
