package claudecheck

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"unicode"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

func usageTokenHappyResponse(t *testing.T, request Request) Response {
	t.Helper()
	for i, fixture := range usageTokenFixtures("claude-opus-5") {
		if bodyProfile(fixture.body) == bodyProfile(request.Body) {
			input, output := []int64{54, 324, 1524, 2109}[i], int64(2)
			response := testResponse(t, message{ID: "msg_usage_fixture", Model: "claude-opus-5", Type: "message", Role: "assistant",
				StopReason: "end_turn", Content: []map[string]any{{"type": "text", "text": "PONG"}}, Usage: Usage{Input: &input, Output: &output}})
			response.RequestProfile = bodyProfile(request.Body)
			return response
		}
	}
	t.Fatal("unrecognized usage fixture")
	return Response{}
}

func usageTokenTestReport(values ...int64) Report {
	valid := true
	report := Report{Version: 19, Model: "claude-opus-5", Options: &Options{Suite: "focused"}, Plan: []PlanItem{{ID: "usage_token_integrity", Selected: true}}}
	for i, value := range values {
		input := value
		report.Samples = append(report.Samples, Sample{Probe: usageTokenTasks[i].probe, Status: 200, Valid: &valid,
			RequestProfile: fmt.Sprintf("profile-%d", i), Usage: Usage{Input: &input}})
	}
	return report
}

func TestUsageTokenFixturesAreFixedPrefixesAndBounded(t *testing.T) {
	fixtures := usageTokenFixtures("claude-opus-5")
	require.Len(t, fixtures, 4)
	var previous string
	for i, fixture := range fixtures {
		require.Equal(t, usageTokenTasks[i].probe, fixture.probe)
		require.Equal(t, 32, fixture.body["max_tokens"])
		require.Equal(t, usageTokensSystem, fixture.body["system"])
		require.Equal(t, map[string]any{"type": "disabled"}, fixture.body["thinking"])
		content := fixture.body["messages"].([]any)[0].(map[string]any)["content"].(string)
		if i < 3 {
			require.Len(t, strings.Fields(content), usageTokenTasks[i].words)
			require.True(t, strings.HasPrefix(content, previous))
			previous = content
		} else {
			require.Len(t, []rune(content), 1000)
			for _, r := range content {
				require.True(t, unicode.Is(unicode.Han, r))
			}
		}
		body, err := common.Marshal(fixture.body)
		require.NoError(t, err)
		require.NotContains(t, string(body), "cache_control")
		require.NotContains(t, string(body), "temperature")
		require.Equal(t, bodyProfile(fixture.body), bodyProfile(usageTokenFixtures("global.anthropic.claude-opus-5")[i].body))
	}
}

func TestUsageTokenGrowthScoring(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values []int64
		score  int
	}{
		{"linear", []int64{54, 324, 1524, 2109}, 100},
		{"constant", []int64{54, 54, 54, 2109}, 0},
		{"decreasing", []int64{1524, 324, 54, 2109}, 0},
		{"uneven slopes", []int64{54, 324, 624, 2109}, 31},
		{"constant multiplier needs reference", []int64{27, 162, 762, 1054}, 100},
		{"Chinese uses no fixed conversion", []int64{54, 324, 1524, 1}, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := AssessUsageTokens(usageTokenTestReport(tc.values...))
			require.True(t, got.Active)
			require.Equal(t, tc.score, *got.Score)
			require.Equal(t, tc.score, *got.GrowthScore)
			require.Nil(t, got.BaselineScore)
			require.Equal(t, "not_selected", got.BaselineCode)
			require.Equal(t, 4, got.Measured)
		})
	}
	got := AssessUsageTokens(usageTokenTestReport(54, 324, 1524, 2109))
	require.Equal(t, float64(1), *got.SlopeRatio)
	require.InDelta(t, 28.2222222222, *got.LongShortRatio, 1e-9)
	got = AssessUsageTokens(usageTokenTestReport(0, 270, 1470, 2109))
	require.Nil(t, got.Score)
	require.Equal(t, "nonpositive_input", got.Rows[0].Code)
	require.Nil(t, got.LongShortRatio)
}

func TestUsageTokenInvalidAndIncompleteDataIsUnscored(t *testing.T) {
	for _, kind := range []string{"missing_sample", "duplicate", "invalid", "unknown_validity", "non_200", "missing_input", "negative_input", "negative_cache", "unsafe_integer", "int64_overflow", "sum_overflow", "zero_english", "zero_chinese"} {
		t.Run(kind, func(t *testing.T) {
			report := usageTokenTestReport(54, 324, 1524, 2109)
			sample := &report.Samples[0]
			want := "invalid_usage"
			switch kind {
			case "missing_sample":
				report.Samples = report.Samples[1:]
				want = "not_collected"
			case "duplicate":
				report.Samples = append(report.Samples, *sample)
				want = "ambiguous_sample"
			case "invalid":
				value := false
				sample.Valid = &value
				want = "invalid_response"
			case "unknown_validity":
				sample.Valid = nil
				want = "invalid_response"
			case "non_200":
				sample.Status = 201
				want = "invalid_response"
			case "missing_input":
				sample.Usage.Input = nil
				want = "missing_input"
			case "negative_input":
				*sample.Usage.Input = -1
			case "negative_cache":
				value := int64(-1)
				sample.Usage.CacheRead = &value
			case "unsafe_integer":
				*sample.Usage.Input = usageTokensMaxSafeInteger + 1
			case "int64_overflow":
				*sample.Usage.Input = math.MaxInt64
			case "sum_overflow":
				*sample.Usage.Input = usageTokensMaxSafeInteger
				value := int64(1)
				sample.Usage.CacheWrite = &value
				want = "usage_overflow"
			case "zero_chinese":
				*report.Samples[3].Usage.Input = 0
				want = "nonpositive_input"
			case "zero_english":
				*sample.Usage.Input = 0
				want = "nonpositive_input"
			}
			got := AssessUsageTokens(report)
			require.Equal(t, "usage_incomplete", got.Code)
			require.Nil(t, got.Score)
			require.Nil(t, got.GrowthScore)
			require.Nil(t, got.BaselineScore)
			index := 0
			if kind == "zero_chinese" {
				index = 3
			}
			require.Equal(t, want, got.Rows[index].Code)
		})
	}
}

func TestUsageTokenCacheAccountingAddsAllThreeCounters(t *testing.T) {
	report := usageTokenTestReport(54, 324, 1524, 2109)
	for i := range report.Samples {
		input, write, read := int64(10), int64(20), *report.Samples[i].Usage.Input-30
		report.Samples[i].Usage = Usage{Input: &input, CacheWrite: &write, CacheRead: &read}
	}
	got := AssessUsageTokens(report)
	require.Equal(t, 100, *got.Score)
	for i, value := range []int64{54, 324, 1524, 2109} {
		require.Equal(t, value, *got.Rows[i].TotalInput)
	}
}

func TestUsageTokenSelectedBaselineAndMedian(t *testing.T) {
	report := usageTokenTestReport(54, 324, 1524, 2109)
	report.Options.BaselineID, report.Options.BaselineType = "selected", "aws"
	baselineReport := usageTokenTestReport(94, 724, 3524, 4034)
	report.Baselines = []ComparisonBaseline{SnapshotBaseline(baselineReport, "selected", "aws", 1)}
	got := AssessUsageTokens(report)
	require.Equal(t, 100, *got.GrowthScore)
	require.Equal(t, 49, *got.BaselineScore)
	require.Equal(t, 49, *got.Score)
	require.Equal(t, "baseline_complete", got.BaselineCode)
	for i, expected := range []int{57, 45, 43, 52} {
		require.Equal(t, expected, *got.Rows[i].BaselineScore)
	}
	// Appending invalid reference rows cannot influence the median.
	bad := report.Baselines[0].Samples[0]
	bad.Status = 500
	report.Baselines[0].Samples = append(report.Baselines[0].Samples, bad)
	require.Equal(t, 49, *AssessUsageTokens(report).Score)
	// Even-sized, same-profile reference samples use the arithmetic median.
	reference := usageTokenTestReport(54, 324, 1524, 2109)
	for i, sample := range append([]Sample(nil), reference.Samples...) {
		low, high := *sample.Usage.Input-10, *sample.Usage.Input+10
		reference.Samples[i].Usage.Input = &low
		sample.Usage.Input = &high
		reference.Samples = append(reference.Samples, sample)
	}
	report.Baselines[0] = SnapshotBaseline(reference, "selected", "aws", 1)
	got = AssessUsageTokens(report)
	require.Equal(t, 100, *got.Score)
	for i, sample := range report.Samples {
		require.Equal(t, float64(*sample.Usage.Input), *got.Rows[i].ReferenceInput)
	}
}

func TestUsageTokenBaselineSelectionNeverFallsBackOrMixes(t *testing.T) {
	for _, mode := range []string{"no_selection", "wrong_id", "empty_type", "wrong_type", "wrong_model", "duplicate_id", "duplicate_id_other_type", "old_without_questions", "missing_question", "missing_profile", "blank_profile", "changed_profile", "invalid_reference", "zero_reference"} {
		t.Run(mode, func(t *testing.T) {
			report := usageTokenTestReport(27, 162, 762, 1054)
			report.Options.BaselineID, report.Options.BaselineType = "selected", "aws"
			report.Baselines = []ComparisonBaseline{SnapshotBaseline(usageTokenTestReport(54, 324, 1524, 2109), "selected", "aws", 1)}
			switch mode {
			case "no_selection":
				report.Options.BaselineID = ""
			case "wrong_id":
				report.Options.BaselineID = "missing"
			case "wrong_type":
				report.Options.BaselineType = "anthropic"
			case "empty_type":
				report.Options.BaselineType = " "
			case "wrong_model":
				report.Baselines[0].Model = "claude-sonnet-5"
			case "duplicate_id":
				report.Baselines = append(report.Baselines, report.Baselines[0])
			case "duplicate_id_other_type":
				other := report.Baselines[0]
				other.Type = "anthropic"
				report.Baselines = append(report.Baselines, other)
			case "old_without_questions":
				report.Baselines[0].Samples = nil
				report.Baselines[0].Version = 18
			case "missing_question":
				report.Baselines[0].Samples = report.Baselines[0].Samples[:3]
			case "missing_profile":
				report.Samples[0].RequestProfile, report.Baselines[0].Samples[0].RequestProfile = "", ""
			case "blank_profile":
				report.Samples[0].RequestProfile, report.Baselines[0].Samples[0].RequestProfile = " \t\n", " \t\n"
			case "changed_profile":
				report.Baselines[0].Samples[0].RequestProfile = "another-request"
			case "invalid_reference":
				report.Baselines[0].Samples[0].Valid = nil
			case "zero_reference":
				*report.Baselines[0].Samples[0].Usage.Input = 0
			}
			// An unrelated complete reference is never substituted.
			report.Baselines = append(report.Baselines, SnapshotBaseline(usageTokenTestReport(54, 324, 1524, 2109), "unselected", "aws", 2))
			got := AssessUsageTokens(report)
			require.Equal(t, 100, *got.Score)
			require.Nil(t, got.BaselineScore)
			require.NotEqual(t, "baseline_complete", got.BaselineCode)
		})
	}
	report := usageTokenTestReport(27, 162, 762, 1054)
	report.Options.BaselineID, report.Options.BaselineType = "selected", "aws"
	report.Baselines = []ComparisonBaseline{SnapshotBaseline(usageTokenTestReport(54, 324, 1524, 2109), "selected", "aws", 1)}
	report.Baselines[0].Model = "global.anthropic.claude-opus-5"
	report.Options.BaselineType, report.Baselines[0].Type = " AWS ", " aWs "
	require.Equal(t, 50, *AssessUsageTokens(report).Score)
}

func TestUsageTokenBaselineToleranceAndGrowthCannotBeRaised(t *testing.T) {
	report := usageTokenTestReport(54, 324, 1524, 2109)
	report.Options.BaselineID, report.Options.BaselineType = "selected", "aws"
	report.Baselines = []ComparisonBaseline{SnapshotBaseline(usageTokenTestReport(58, 340, 1600, 2214), "selected", "aws", 1)}
	require.Equal(t, 100, *AssessUsageTokens(report).Score)
	report = usageTokenTestReport(54, 324, 624, 2109)
	report.Options.BaselineID, report.Options.BaselineType = "selected", "aws"
	report.Baselines = []ComparisonBaseline{SnapshotBaseline(report, "selected", "aws", 1)}
	got := AssessUsageTokens(report)
	require.Equal(t, 100, *got.BaselineScore)
	require.Equal(t, 31, *got.Score)
}

func TestUsageTokenVersionAndPlanGate(t *testing.T) {
	report := usageTokenTestReport(54, 324, 1524, 2109)
	for _, version := range []int{1, 14, 18} {
		report.Version = version
		require.False(t, AssessUsageTokens(report).Active)
		require.Nil(t, ReportScore(report))
	}
	report.Version = 19
	report.Options.Suite = ""
	require.False(t, AssessUsageTokens(report).Active)
	report.Options.Suite = "focused"
	report.Plan = nil
	require.True(t, AssessUsageTokens(report).Active)
	report.Samples = nil
	report.Checks = []Check{{ID: "usage_token_integrity", Status: "pass", Code: "usage_consistent", Evidence: map[string]any{"score": 100}}}
	require.False(t, AssessUsageTokens(report).Active)
	require.Nil(t, ReportScore(report))
	report.Plan = []PlanItem{{ID: "usage_token_integrity", Selected: true}}
	require.True(t, AssessUsageTokens(report).Active)
	require.Nil(t, ReportScore(report))
}

func TestUsageTokenCollectionOrderBudgetAndAccessStops(t *testing.T) {
	base := focusedTransport(t)
	report := Run(context.Background(), Options{Suite: "focused", Model: "claude-opus-5"}, base)
	require.Equal(t, 20, report.Version)
	require.Len(t, report.Samples, 15)
	require.Equal(t, 15, report.Limits.MaxRequests)
	for i, task := range usageTokenTasks {
		require.Equal(t, task.probe, report.Samples[4+i].Probe)
	}
	require.Equal(t, "benchmark_instruction_format", report.Samples[8].Probe)
	require.Equal(t, "usage_consistent", resultCheck(t, report, "usage_token_integrity").Code)
	for _, status := range []int{401, 403, 429} {
		calls, usageCalls := 0, 0
		report = Run(context.Background(), Options{Suite: "focused", Model: "claude-opus-5", Cache: true}, func(ctx context.Context, request Request) (Response, error) {
			calls++
			require.False(t, request.Count)
			if request.Body["system"] == usageTokensSystem {
				usageCalls++
				if usageCalls == 2 {
					return Response{Status: status}, fmt.Errorf("synthetic rejection")
				}
			}
			return base(ctx, request)
		})
		require.Equal(t, 6, calls)
		require.Equal(t, 2, usageCalls)
		require.NotEmpty(t, report.StopReason)
		require.Nil(t, AssessUsageTokens(report).Score)
		require.Equal(t, "usage_incomplete", resultCheck(t, report, "usage_token_integrity").Code)
		require.Equal(t, "run_stopped", resultCheck(t, report, "capability_benchmark").Code)
	}
}
