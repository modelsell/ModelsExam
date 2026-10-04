package claudecheck

import (
	"context"
	"os"
	"testing"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

func practicalAssessment(report Report, refs []ComparisonBaseline) Report {
	audit := *report.TokenAudit
	audit.Prompt = append([]TokenComparison(nil), audit.Prompt...)
	report.TokenAudit = &audit
	r := runner{ctx: context.Background(), report: report}
	r.report.TokenAudit.Version = 5
	r.report.Baselines = refs
	r.report.TokenAudit.assessPrompt()
	r.assessPromptInjection()
	return r.report
}

func TestPracticalPromptReplaysLocalObservationsWithoutCountTokens(t *testing.T) {
	data, err := os.ReadFile("testdata/prompt_observations.json")
	require.NoError(t, err)
	var reports []Report
	require.NoError(t, common.Unmarshal(data, &reports))
	require.Len(t, reports, 3)
	for i := range reports {
		reports[i].Checks = []Check{{ID: "prompt_integrity"}}
	}
	old := ComparisonBaseline{ID: "old", ReportID: "v7", Version: 7, Model: "claude-opus-5"}
	for _, report := range reports {
		r := practicalAssessment(report, []ComparisonBaseline{old})
		require.Equal(t, 100, *r.TokenAudit.PromptAssessment.Score)
		require.Equal(t, 30, r.TokenAudit.PromptAssessment.Coverage)
		require.Equal(t, "prompt_behavior_only", r.TokenAudit.Injection.Code)
		require.Nil(t, r.TokenAudit.PromptAssessment.TokenScore)
	}
	// A v4 reference is reusable without inventing new samples or rewriting
	// saved history. Local observations are not packaged as a trusted default.
	ref := SnapshotBaseline(reports[0], "selected", "test-reference", 1)
	r := practicalAssessment(reports[1], []ComparisonBaseline{ref})
	require.Equal(t, "reference_aligned", r.TokenAudit.Injection.Code)
	require.Equal(t, 100, *r.TokenAudit.PromptAssessment.Score)
	r = practicalAssessment(reports[2], []ComparisonBaseline{ref})
	require.Equal(t, "reference_extra_input", r.TokenAudit.Injection.Code)
	require.Equal(t, 85, *r.TokenAudit.PromptAssessment.Score)
	for _, pair := range r.TokenAudit.Injection.References[0].Pairs {
		require.Equal(t, int64(11), pair.Difference)
	}
}

func TestPracticalPromptUsesPartialConditionsAndSameEndpointFallback(t *testing.T) {
	ref := SnapshotBaseline(collectRepeatedPrompt(t, "clean", nil), "ref", "aws", 1)
	for _, mode := range []string{"matching", "small_jitter", "one_outlier", "incomplete", "no_count"} {
		t.Run(mode, func(t *testing.T) {
			r := practicalAssessment(collectRepeatedPrompt(t, mode, nil), []ComparisonBaseline{ref})
			require.Equal(t, 100, *r.TokenAudit.PromptAssessment.Score)
			require.Equal(t, "reference_aligned", r.TokenAudit.Injection.Code)
			if mode == "one_outlier" {
				require.Equal(t, 1, r.TokenAudit.Injection.Sampling[0].Outliers)
			}
		})
	}
	r := practicalAssessment(collectRepeatedPrompt(t, "matching", nil), nil)
	require.Equal(t, "same_endpoint_aligned", r.TokenAudit.Injection.Code)
	require.Equal(t, "same_endpoint", r.TokenAudit.PromptAssessment.TokenSource)
	require.Equal(t, 100, *r.TokenAudit.PromptAssessment.Score)
	r = practicalAssessment(collectRepeatedPrompt(t, "persistent_count_gap", nil), nil)
	require.Equal(t, "same_endpoint_extra_input", r.TokenAudit.Injection.Code)
	require.Less(t, *r.TokenAudit.PromptAssessment.Score, 100)
	r = practicalAssessment(collectRepeatedPrompt(t, "single_count_gap", nil), nil)
	require.Equal(t, "same_endpoint_aligned", r.TokenAudit.Injection.Code)
	r = practicalAssessment(collectRepeatedPrompt(t, "local_rewrite", nil), nil)
	require.Equal(t, "local_prompt_changed", r.TokenAudit.Injection.Code)
	require.Equal(t, 0, *r.TokenAudit.PromptAssessment.Score)
}

func TestPracticalPromptReferencesMatchContentInsteadOfSuiteVersionOrWrapper(t *testing.T) {
	ref := SnapshotBaseline(collectPromptChannel(t, "old", nil), "v3", "aws", 1)
	for i := range ref.TokenAudit.Prompt {
		ref.TokenAudit.Prompt[i].Profile = "bedrock-wrapper"
	}
	r := practicalAssessment(collectRepeatedPrompt(t, "no_count", nil), []ComparisonBaseline{ref})
	// The v3 fixture's toy meter differs, but the two identical prompts remain
	// comparable. The old long question is not invented to complete a trio.
	require.Len(t, r.TokenAudit.Injection.References[0].Pairs, 2)
	require.Equal(t, 2, r.TokenAudit.PromptAssessment.TokenMeasured)
	require.Equal(t, 77, r.TokenAudit.PromptAssessment.Coverage)
	for i := range ref.TokenAudit.Prompt {
		ref.TokenAudit.Prompt[i].PromptProfile = "changed-content"
	}
	r = practicalAssessment(collectRepeatedPrompt(t, "no_count", nil), []ComparisonBaseline{ref})
	require.Zero(t, r.TokenAudit.PromptAssessment.TokenMeasured)
	require.Equal(t, 1, r.TokenAudit.Injection.Incompatible)
	ref = SnapshotBaseline(collectRepeatedPrompt(t, "clean", nil), "ref", "aws", 1)
	for i := range ref.TokenAudit.Prompt {
		ref.TokenAudit.Prompt[i].ResponseModel = "claude-sonnet-5"
		ref.TokenAudit.Prompt[i].Model = "claude-sonnet-5"
	}
	r = practicalAssessment(collectRepeatedPrompt(t, "no_count", nil), []ComparisonBaseline{ref})
	require.Zero(t, r.TokenAudit.PromptAssessment.TokenMeasured)
}

func TestPracticalPromptReferenceTypesAreComparedSeparately(t *testing.T) {
	clean := SnapshotBaseline(collectRepeatedPrompt(t, "clean", nil), "clean", "aws", 1)
	hidden := SnapshotBaseline(collectRepeatedPrompt(t, "hidden", nil), "hidden", "kiro", 2)
	r := practicalAssessment(collectRepeatedPrompt(t, "matching", nil), []ComparisonBaseline{clean, hidden})
	require.Equal(t, "references_disagree", r.TokenAudit.Injection.Code)
	require.Equal(t, "clean", r.TokenAudit.Injection.SelectedReferenceID)
	require.Equal(t, 100, *r.TokenAudit.PromptAssessment.Score)
	require.Equal(t, 3, r.TokenAudit.PromptAssessment.TokenMeasured)
	// A channel alternating between reference patterns must not receive a
	// synthetic perfect match assembled from different references per question.
	hybrid := collectRepeatedPrompt(t, "hybrid", nil)
	for i := 0; i < len(hybrid.TokenAudit.Prompt); i += 3 {
		n := *hybrid.TokenAudit.Prompt[i].Actual + 64
		hybrid.TokenAudit.Prompt[i].Actual = &n
	}
	r = practicalAssessment(hybrid, []ComparisonBaseline{clean, hidden})
	require.Less(t, *r.TokenAudit.PromptAssessment.Score, 100)
	r = collectRepeatedPrompt(t, "clean", nil)
	r.Checks = nil
	r = practicalAssessment(r, nil)
	require.Nil(t, r.TokenAudit.PromptAssessment.Score, "no final score during collection")
}
