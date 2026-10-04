package claudecheck

import (
	"context"
	"fmt"
	"os"
	"testing"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

type simplePromptRound struct {
	input, write, read int64
	status             int
	model              string
	edit               func(map[string]any)
}

// The meter is controlled test evidence, not a tokenizer implementation. It
// deliberately returns an unrelated answer so behavior cannot supply points.
func collectSimplePrompt(t *testing.T, id, platform string, rounds []simplePromptRound, refs []ComparisonBaseline) Report {
	return collectSimplePromptVersion(t, id, platform, rounds, refs, 6)
}

func collectSimplePromptVersion(t *testing.T, id, platform string, rounds []simplePromptRound, refs []ComparisonBaseline, version int) Report {
	t.Helper()
	require.Len(t, rounds, 3)
	r := runner{ctx: context.Background(), report: Report{Version: version + 6, ID: id, Model: "claude-opus-5", Baselines: refs,
		TokenAudit: &TokenAuditReport{Version: version, Prompt: []TokenComparison{}, Cache: []TokenComparison{}}}}
	if platform == "aws" {
		r.report.Transport = "bedrock_runtime"
	} else {
		r.report.Transport = "anthropic_proxy"
	}
	for _, f := range simplePromptFixtures(r.report.Model) {
		r.report.TokenAudit.Prompt = append(r.report.TokenAudit.Prompt, TokenComparison{ID: f.id, Code: "pending"})
	}
	calls := 0
	r.observe = func(event Event) {
		if event.Type == "token_audit" && !r.hasCheck("prompt_integrity") {
			require.Nil(t, event.TokenAudit.PromptAssessment.Score, "collection must not publish a final score")
		}
	}
	r.call = func(_ context.Context, req Request) (Response, error) {
		require.False(t, req.Count, "minimal sampling must not call the target's CountTokens endpoint")
		require.Empty(t, req.Beta)
		require.NotContains(t, req.Body, "system")
		require.NotContains(t, req.Body, "tools")
		require.NotContains(t, req.Body, "temperature")
		messages := req.Body["messages"].([]any)
		require.Len(t, messages, 1)
		require.Equal(t, "user", messages[0].(map[string]any)["role"])
		require.Equal(t, "Return only the word PONG.", messages[0].(map[string]any)["content"])
		require.Less(t, calls, len(rounds))
		round := rounds[calls]
		calls++
		if round.status != 0 && round.status != 200 {
			return Response{Status: round.status}, fmt.Errorf("controlled upstream response")
		}
		encoded, err := common.Marshal(req.Body)
		require.NoError(t, err)
		var effective map[string]any
		require.NoError(t, common.Unmarshal(encoded, &effective))
		if round.edit != nil {
			round.edit(effective)
		}
		model := round.model
		if model == "" {
			model = r.report.Model
		}
		output := int64(7)
		out := testResponse(t, message{ID: fmt.Sprintf("msg_simple_%d", calls), Type: "message", Role: "assistant", Model: model, StopReason: "end_turn",
			Content: []map[string]any{{"type": "text", "text": "An unrelated but valid answer."}},
			Usage:   Usage{Input: &round.input, Output: &output, CacheWrite: &round.write, CacheRead: &round.read}})
		out.Model, out.Bedrock = model, platform == "aws"
		out.RequestProfile, out.PromptProfile = bodyProfile(effective), promptContentProfile(effective)
		return out, nil
	}
	r.simplePromptAudit()
	require.Equal(t, 3, calls)
	require.Len(t, r.report.TokenAudit.Prompt, 3)
	require.Len(t, r.report.Samples, 3)
	for _, row := range r.report.TokenAudit.Prompt {
		require.Nil(t, row.Behavior)
		if version == 6 {
			require.Nil(t, row.Expected, "a same-endpoint count must not be invented")
		}
	}
	return r.report
}

func simplePromptValues(values ...int64) []simplePromptRound {
	rounds := make([]simplePromptRound, len(values))
	for i, value := range values {
		rounds[i].input = value
	}
	return rounds
}

func simplePromptReference(t *testing.T, id, platform, kind string, created int64, values ...int64) ComparisonBaseline {
	t.Helper()
	return SnapshotBaseline(collectSimplePrompt(t, "source-"+id, platform, simplePromptValues(values...), nil), id, kind, created)
}

func TestSimplePromptSamplesOnlyInputAndDoesNotSelfCertify(t *testing.T) {
	r := collectSimplePrompt(t, "target", "anthropic", simplePromptValues(15, 15, 15), nil)
	require.Equal(t, "simple_no_reference", r.TokenAudit.Injection.Code)
	require.Equal(t, "input_consistency", r.TokenAudit.PromptAssessment.Scoring)
	require.Equal(t, "unavailable", r.TokenAudit.PromptAssessment.TokenSource)
	require.Equal(t, 100, r.TokenAudit.PromptAssessment.Coverage)
	require.Nil(t, r.TokenAudit.PromptAssessment.Score)
	require.Nil(t, r.TokenAudit.PromptAssessment.BehaviorScore)
	require.Zero(t, r.TokenAudit.PromptAssessment.BehaviorMeasured)
	require.Equal(t, int64(15), *r.TokenAudit.Injection.Sampling[0].Median)
	require.Equal(t, false, resultCheck(t, r, "prompt_integrity").Evidence["injection_verified"])
}

func TestSimplePromptSelectsLatestNativeReferenceInsteadOfClosest(t *testing.T) {
	for _, platform := range []string{"anthropic", "aws"} {
		t.Run(platform, func(t *testing.T) {
			old := simplePromptReference(t, "old-closest", platform, platform, 1, 26, 26, 26)
			latest := simplePromptReference(t, "latest", platform, platform, 2, 15, 15, 15)
			refs := []ComparisonBaseline{old, latest}
			for _, kind := range []string{"kiro", "ccmax", "aws-ccmax", "my anthropic", "custom"} {
				refs = append(refs, simplePromptReference(t, kind, platform, kind, 3, 26, 26, 26))
			}
			r := collectSimplePrompt(t, "target", platform, simplePromptValues(26, 26, 26), refs)
			require.Equal(t, "latest", r.TokenAudit.Injection.SelectedReferenceID)
			require.Equal(t, "simple_extra_input", r.TokenAudit.Injection.Code)
			require.Equal(t, 69, *r.TokenAudit.PromptAssessment.Score)
			require.Len(t, r.TokenAudit.Injection.References, 2)
			require.Equal(t, 5, r.TokenAudit.Injection.Incompatible)
		})
	}
	for _, kind := range []string{" AWS ", "bedrock", "aws_bedrock", "aws-bedrock", "aws bedrock"} {
		t.Run(kind, func(t *testing.T) {
			ref := simplePromptReference(t, "manual", "aws", kind, 1, 15, 15, 15)
			r := collectSimplePrompt(t, "target", "aws", simplePromptValues(15, 15, 15), []ComparisonBaseline{ref})
			require.Equal(t, "manual", r.TokenAudit.Injection.SelectedReferenceID)
			require.Equal(t, 100, *r.TokenAudit.PromptAssessment.Score)
		})
	}
}

func TestSimplePromptExcludesWrappedProductsAndDifferentPlatforms(t *testing.T) {
	refs := []ComparisonBaseline{
		simplePromptReference(t, "kiro", "aws", "kiro", 1, 15, 15, 15),
		simplePromptReference(t, "ccmax", "aws", "ccmax", 2, 15, 15, 15),
		simplePromptReference(t, "anthropic", "anthropic", "anthropic", 3, 15, 15, 15),
		simplePromptReference(t, "mislabeled", "aws", "anthropic", 4, 15, 15, 15),
	}
	r := collectSimplePrompt(t, "target", "aws", simplePromptValues(15, 15, 15), refs)
	require.Equal(t, "simple_no_reference", r.TokenAudit.Injection.Code)
	require.Nil(t, r.TokenAudit.PromptAssessment.Score)
	require.Len(t, r.TokenAudit.Injection.References, 0)
	require.Equal(t, len(refs), r.TokenAudit.Injection.Incompatible)
}

func TestSimplePromptRequiresMatchingParametersAndCompatibleModel(t *testing.T) {
	ref := simplePromptReference(t, "trusted", "aws", "aws", 1, 15, 15, 15)
	wrapped := simplePromptValues(15, 15, 15)
	for i := range wrapped {
		wrapped[i].model = "global.anthropic.claude-opus-5"
	}
	r := collectSimplePrompt(t, "target", "aws", wrapped, []ComparisonBaseline{ref})
	require.Equal(t, "simple_input_aligned", r.TokenAudit.Injection.Code)
	require.Equal(t, 100, *r.TokenAudit.PromptAssessment.Score)
	for _, tc := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"different_max_tokens", func(body map[string]any) { body["max_tokens"] = 256 }},
		{"different_thinking", func(body map[string]any) { delete(body, "thinking") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rounds := simplePromptValues(15, 15, 15)
			for i := range rounds {
				rounds[i].edit = tc.edit
			}
			r := collectSimplePrompt(t, "target", "aws", rounds, []ComparisonBaseline{ref})
			require.Equal(t, "simple_no_reference", r.TokenAudit.Injection.Code)
			require.Nil(t, r.TokenAudit.PromptAssessment.Score)
			require.Equal(t, 1, r.TokenAudit.Injection.Incompatible)
		})
	}
	for _, model := range []string{"claude-sonnet-5", "custom-model-alias"} {
		t.Run(model, func(t *testing.T) {
			rounds := simplePromptValues(15, 15, 15)
			for i := range rounds {
				rounds[i].model = model
			}
			r := collectSimplePrompt(t, "target", "aws", rounds, []ComparisonBaseline{ref})
			require.Equal(t, "simple_request_mismatch", r.TokenAudit.Injection.Code)
			require.Nil(t, r.TokenAudit.PromptAssessment.Score)
			require.Equal(t, model, r.TokenAudit.Prompt[0].ResponseModel)
		})
	}
}

func TestSimplePromptDoesNotHideOneObservedModelOrPlatformChange(t *testing.T) {
	ref := simplePromptReference(t, "trusted", "anthropic", "anthropic", 1, 15, 15, 15)
	for _, tc := range []struct {
		name  string
		model string
		edit  func(map[string]any)
	}{
		{"changed_model", "claude-sonnet-5", nil},
		{"changed_platform", "global.anthropic.claude-opus-5", nil},
		{"changed_parameter", "", func(body map[string]any) { body["max_tokens"] = 256 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rounds := simplePromptValues(15, 15, 15)
			rounds[1].model, rounds[1].edit = tc.model, tc.edit
			r := collectSimplePrompt(t, "target", "anthropic", rounds, []ComparisonBaseline{ref})
			require.Equal(t, "simple_request_mismatch", r.TokenAudit.Injection.Code)
			require.Nil(t, r.TokenAudit.PromptAssessment.Score, "an observed incompatible round is not a missing response")
			require.Equal(t, int64(15), *r.TokenAudit.Prompt[1].Actual, "retain the mismatching round for inspection")
			require.True(t, *r.Samples[1].Valid, "the response was valid, even though the controlled request was inconsistent")
		})
	}
}

func TestSimplePromptRejectsReferenceWithObservedModelSwitch(t *testing.T) {
	ref := simplePromptReference(t, "mixed-model-reference", "anthropic", "anthropic", 1, 15, 15, 15)
	ref.TokenAudit.Prompt[1].Model, ref.TokenAudit.Prompt[1].ResponseModel = "claude-sonnet-5", "claude-sonnet-5"
	r := collectSimplePrompt(t, "target", "anthropic", simplePromptValues(15, 15, 15), []ComparisonBaseline{ref})
	require.Equal(t, "simple_no_reference", r.TokenAudit.Injection.Code)
	require.Nil(t, r.TokenAudit.PromptAssessment.Score, "a reference must not conceal a model switch by dropping that round")
	require.Equal(t, 1, r.TokenAudit.Injection.Incompatible)
}

func TestSimplePromptKeepsUnavailableRoundsWithoutInventingCounts(t *testing.T) {
	ref := simplePromptReference(t, "trusted", "anthropic", "anthropic", 1, 15, 15, 15)
	rounds := simplePromptValues(14, 0, 16)
	rounds[1].status = 500
	r := collectSimplePrompt(t, "target", "anthropic", rounds, []ComparisonBaseline{ref})
	require.Equal(t, "simple_input_aligned", r.TokenAudit.Injection.Code)
	require.Equal(t, 100, *r.TokenAudit.PromptAssessment.Score)
	require.Equal(t, 66, r.TokenAudit.PromptAssessment.Coverage)
	require.Equal(t, 2, r.TokenAudit.Injection.Sampling[0].Valid)
	require.Equal(t, int64(15), *r.TokenAudit.Injection.Sampling[0].Median)
	require.Equal(t, "probe_unavailable", r.TokenAudit.Prompt[1].Code)
	require.Nil(t, r.TokenAudit.Prompt[1].Actual)
	require.Equal(t, 500, r.Samples[1].Status)
	require.False(t, *r.Samples[1].Valid)
	rounds[0].status = 500
	r = collectSimplePrompt(t, "one-usable", "anthropic", rounds, []ComparisonBaseline{ref})
	require.Equal(t, "simple_insufficient_samples", r.TokenAudit.Injection.Code)
	require.Nil(t, r.TokenAudit.PromptAssessment.Score)
}

func TestSimplePromptIncludesCacheWritesAndReadsInTotalInput(t *testing.T) {
	ref := simplePromptReference(t, "trusted", "aws", "aws", 1, 15, 15, 15)
	rounds := []simplePromptRound{{input: 15}, {input: 5, write: 10}, {input: 2, write: 3, read: 10}}
	r := collectSimplePrompt(t, "target", "aws", rounds, []ComparisonBaseline{ref})
	for _, row := range r.TokenAudit.Prompt {
		require.Equal(t, int64(15), *row.Actual)
	}
	require.Equal(t, "simple_input_aligned", r.TokenAudit.Injection.Code)
	require.Equal(t, 100, *r.TokenAudit.PromptAssessment.Score)
	require.Equal(t, int64(3), *r.Samples[2].Usage.CacheWrite)
	require.Equal(t, int64(10), *r.Samples[2].Usage.CacheRead)
}

func TestSimplePromptPreservesAnomaliesInsteadOfScoringOnlyMedian(t *testing.T) {
	ref := simplePromptReference(t, "trusted", "anthropic", "anthropic", 1, 15, 15, 15)
	for _, tc := range []struct {
		name, code string
		values     []int64
		score      int
	}{
		{"single_outlier", "simple_variable_input", []int64{15, 15, 60}, 77},
		{"persistent_extra", "simple_extra_input", []int64{15, 26, 26}, 79},
		{"persistent_fewer", "simple_input_deviation", []int64{3, 3, 15}, 60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := collectSimplePrompt(t, "target", "anthropic", simplePromptValues(tc.values...), []ComparisonBaseline{ref})
			require.Equal(t, tc.code, r.TokenAudit.Injection.Code)
			require.Equal(t, tc.score, *r.TokenAudit.PromptAssessment.Score)
			require.Equal(t, 3, r.TokenAudit.Injection.Sampling[0].Valid)
			for i, row := range r.TokenAudit.Prompt {
				require.Equal(t, tc.values[i], *row.Actual)
			}
		})
	}
}

func TestSimplePromptRoundingCannotHideOutOfRangeSamples(t *testing.T) {
	for _, tc := range []struct {
		name      string
		expected  int64
		anomalous int64
	}{
		{"averaging_rounds_to_full", 100, 121},
		{"individual_rounds_to_full", 1000, 1201},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref := simplePromptReference(t, "trusted", "anthropic", "anthropic", 1, tc.expected, tc.expected, tc.expected)
			r := collectSimplePrompt(t, "target", "anthropic", simplePromptValues(tc.expected, tc.expected, tc.anomalous), []ComparisonBaseline{ref})
			require.Equal(t, "simple_variable_input", r.TokenAudit.Injection.Code)
			require.Less(t, *r.TokenAudit.PromptAssessment.Score, 100, "rounding must not turn a measured deviation into full consistency")
		})
	}
}

func TestSimplePromptToleranceIncludesObservedNativeReferenceJitter(t *testing.T) {
	ref := simplePromptReference(t, "trusted", "anthropic", "anthropic", 1, 10, 10, 16)
	r := collectSimplePrompt(t, "target", "anthropic", simplePromptValues(16, 16, 16), []ComparisonBaseline{ref})
	require.Equal(t, "simple_input_aligned", r.TokenAudit.Injection.Code)
	require.Equal(t, 100, *r.TokenAudit.PromptAssessment.Score)
	pair := r.TokenAudit.Injection.References[0].Pairs[0]
	require.Equal(t, int64(10), pair.Expected)
	require.Equal(t, int64(6), pair.Tolerance, "the native reference already observed this input fluctuation")
	unstable := simplePromptReference(t, "unstable", "anthropic", "anthropic", 2, 10, 10, 17)
	r = collectSimplePrompt(t, "target", "anthropic", simplePromptValues(10, 10, 10), []ComparisonBaseline{unstable})
	require.Equal(t, "simple_no_reference", r.TokenAudit.Injection.Code)
	require.Nil(t, r.TokenAudit.PromptAssessment.Score)
}

func TestSimplePromptReportsLocallyObservedRewriteWithoutInventedScore(t *testing.T) {
	ref := simplePromptReference(t, "trusted", "anthropic", "anthropic", 1, 15, 15, 15)
	rounds := simplePromptValues(15, 15, 15)
	rounds[1].edit = func(body map[string]any) { body["system"] = "A locally appended instruction." }
	r := collectSimplePrompt(t, "target", "anthropic", rounds, []ComparisonBaseline{ref})
	require.Equal(t, "local_prompt_changed", r.TokenAudit.Injection.Code)
	require.Equal(t, 1, r.TokenAudit.Injection.LocalChanges)
	require.True(t, r.TokenAudit.Prompt[1].PromptChanged)
	require.Equal(t, int64(15), *r.TokenAudit.Prompt[1].Actual)
	require.Nil(t, r.TokenAudit.PromptAssessment.Score)
	require.Nil(t, r.TokenAudit.PromptAssessment.TokenScore)
}

func TestSimplePromptReusesMatchingHistoricalV4MinimalSamples(t *testing.T) {
	data, err := os.ReadFile("testdata/prompt_observations.json")
	require.NoError(t, err)
	var reports []Report
	require.NoError(t, common.Unmarshal(data, &reports))
	require.NotEmpty(t, reports)
	require.Equal(t, 4, reports[0].TokenAudit.Version)
	ref := SnapshotBaseline(reports[0], "historical-native", "anthropic", 1)
	r := collectSimplePrompt(t, "target", "anthropic", simplePromptValues(15, 15, 15), []ComparisonBaseline{ref})
	require.Equal(t, "simple_input_aligned", r.TokenAudit.Injection.Code)
	require.Equal(t, 100, *r.TokenAudit.PromptAssessment.Score)
	require.Equal(t, 3, r.TokenAudit.Injection.BaselineSamples)
	require.Len(t, r.TokenAudit.Injection.References[0].Pairs, 1)
	require.Equal(t, int64(15), r.TokenAudit.Injection.References[0].Pairs[0].Expected)
	// Reusing exact floor rows does not rewrite the old nine-row report.
	require.Equal(t, 4, ref.TokenAudit.Version)
	require.Len(t, ref.TokenAudit.Prompt, 9)
	require.NotNil(t, ref.TokenAudit.Prompt[0].Behavior)
}
