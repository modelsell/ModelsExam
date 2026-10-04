package claudecheck

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

func TestNewRunsRetirePreferenceButKeepBedrockOptional(t *testing.T) {
	for _, suite := range []string{"focused", ""} {
		for _, flags := range []string{"", `,"fingerprint":false,"bedrock":false`, `,"fingerprint":true,"bedrock":false`, `,"fingerprint":false,"bedrock":true`, `,"fingerprint":true,"bedrock":true`} {
			t.Run(suite+flags, func(t *testing.T) {
				var options Options
				require.NoError(t, common.UnmarshalJsonStr(fmt.Sprintf(`{"suite":%q,"model":"claude-opus-5","benchmark":true%s}`, suite, flags), &options))
				var probes []string
				report := RunWithObserver(context.Background(), options, focusedTransport(t), func(event Event) {
					require.NotEqual(t, "fingerprint", event.Type)
					require.Nil(t, event.Fingerprint)
					if event.Probe != "" {
						probes = append(probes, event.Probe)
					}
					if event.Check != nil {
						require.False(t, retiredCheckID(event.Check.ID), event.Check.ID)
					}
				})
				expected := 15 // Legacy retains its eight protocol requests.
				if suite == "focused" {
					expected = 15
					require.Equal(t, 20, report.Version)
					require.NotEmpty(t, resultCheck(t, report, "performance_sampling"))
				}
				if options.Bedrock {
					expected += BedrockRequests
				}
				require.Equal(t, expected, MaxRequests(options))
				require.Equal(t, expected, report.Limits.MaxRequests)
				require.Len(t, report.Samples, expected)
				require.Nil(t, report.Fingerprint)
				require.False(t, report.Options.Fingerprint)
				require.Equal(t, options.Bedrock, report.Options.Bedrock)
				require.Equal(t, Plan(options), report.Plan)
				var bedrockPlanIDs, bedrockProbeIDs []string
				for _, item := range report.Plan {
					require.False(t, retiredCheckID(item.ID), item.ID)
					if isBedrockProbe(item.ID) {
						bedrockPlanIDs = append(bedrockPlanIDs, item.ID)
					}
				}
				for _, sample := range report.Samples {
					probes = append(probes, sample.Probe)
					if isBedrockProbe(sample.Probe) {
						bedrockProbeIDs = append(bedrockProbeIDs, sample.Probe)
					}
				}
				if options.Bedrock {
					expectedIDs := []string{"bedrock_role", "bedrock_beta", "bedrock_web_search", "bedrock_web_fetch", "bedrock_code_execution", "bedrock_advisor", "bedrock_sampling", "bedrock_signature"}
					require.Equal(t, expectedIDs, bedrockPlanIDs)
					require.Equal(t, expectedIDs, bedrockProbeIDs)
				} else {
					require.Empty(t, bedrockPlanIDs)
					require.Empty(t, bedrockProbeIDs)
					for _, probe := range probes {
						require.False(t, isBedrockProbe(probe), probe)
					}
				}
				for _, probe := range probes {
					require.False(t, retiredCheckID(probe), probe)
				}
				require.Equal(t, "pass", resultCheck(t, report, "basic").Status)
				require.Equal(t, "pass", resultCheck(t, report, "model_consistency").Status)
				require.Len(t, report.Benchmark.Items, 6)
				for _, item := range report.Benchmark.Items {
					require.NotNil(t, item.Score)
					require.Equal(t, 100, *item.Score, item.ID)
				}
				require.NotNil(t, ReportScore(report))
				withoutBedrock := report
				withoutBedrock.Checks = nil
				for _, check := range report.Checks {
					if !isBedrockProbe(check.ID) {
						withoutBedrock.Checks = append(withoutBedrock.Checks, check)
					}
				}
				require.Equal(t, ReportScore(withoutBedrock), ReportScore(report))
			})
		}
	}
}

func retiredCheckID(id string) bool {
	return id == "behavior_fingerprint" || strings.HasPrefix(id, "fingerprint_")
}

func TestFocusedCoreAndOptionalScoringRequestsStayBounded(t *testing.T) {
	options := Options{Suite: "focused", Model: "claude-opus-5", Cache: true, PromptAudit: true, Vision: true, PDF: true,
		Fingerprint: true, Bedrock: true, Thinking: true, Repeat: true, StreamComparison: true, LegacySuite: true}
	base := focusedTransport(t)
	report := Run(context.Background(), options, func(ctx context.Context, request Request) (Response, error) {
		if request.Body["thinking"] != nil && request.Body["system"] != fingerprintSystem && request.Body["system"] != capabilitySystem && request.Body["system"] != usageTokensSystem {
			input, output := int64(10), int64(1)
			response := testResponse(t, message{ID: "msg_fixture", Type: "message", Role: "assistant", Model: options.Model,
				StopReason: "end_turn", Content: []map[string]any{{"type": "text", "text": "PONG"}}, Usage: Usage{Input: &input, Output: &output}})
			response.RequestProfile, response.PromptProfile = bodyProfile(request.Body), promptContentProfile(request.Body)
			return response, nil
		}
		return base(ctx, request)
	})
	require.Equal(t, 36, MaxRequests(options))
	require.Equal(t, 36, report.Limits.MaxRequests)
	require.Len(t, report.Samples, 36)
	require.Len(t, report.TokenAudit.Cache, 6)
	require.Len(t, report.TokenAudit.Prompt, 3)
	require.NotNil(t, report.TokenAudit.PromptAssessment.Score)
	require.Equal(t, 100, *report.TokenAudit.PromptAssessment.Score)
	counts := map[string]int{}
	for _, sample := range report.Samples {
		for _, prefix := range []string{"performance_", "benchmark_", "cache_", "prompt_audit_", "vision", "pdf", "bedrock_", "usage_tokens_"} {
			if strings.HasPrefix(sample.Probe, prefix) {
				counts[prefix]++
			}
		}
	}
	require.Equal(t, map[string]int{"performance_": 3, "benchmark_": 7, "cache_": 8, "prompt_audit_": 3, "vision": 1, "pdf": 1, "bedrock_": 8, "usage_tokens_": 4}, counts)
	var cachePlanIDs []string
	for _, item := range report.Plan {
		if strings.HasPrefix(item.ID, "cache") {
			cachePlanIDs = append(cachePlanIDs, item.ID)
		}
	}
	require.Equal(t, []string{"cache_token_audit"}, cachePlanIDs)
	require.NotEmpty(t, resultCheck(t, report, "cache"), "raw cache evidence remains available")
	require.Equal(t, 23, MaxRequests(Options{Suite: "focused", Cache: true}))
	require.Equal(t, 31, MaxRequests(Options{Suite: "focused", Cache: true, Bedrock: true}))
	options.Suite, options.Benchmark, options.Performance = "", true, true
	require.Equal(t, 45, MaxRequests(options))
}

func TestVersion17HistoryDoesNotGainRestoredBedrockPlan(t *testing.T) {
	const saved = `{"version":17,"id":"version-17-fixture","model":"claude-opus-5","options":{"suite":"focused","cache":true,"prompt_audit":true,"vision":true,"pdf":true},"limits":{"max_requests":24},"plan":[{"id":"basic","selected":true},{"id":"cache_token_audit","selected":true}],"checks":[{"id":"basic","status":"pass"},{"id":"model_consistency","status":"pass"}],"samples":[{"probe":"basic","http_status":200}]}`
	var report Report
	require.NoError(t, common.UnmarshalJsonStr(saved, &report))
	before, err := common.Marshal(report)
	require.NoError(t, err)
	snapshot := SnapshotBaseline(report, "version-17-baseline", "aws", 1)
	require.Equal(t, 17, snapshot.Version)
	require.False(t, snapshot.Options.Bedrock)
	require.Equal(t, report.Plan, snapshot.Plan)
	require.Equal(t, report.Checks, snapshot.Checks)
	require.Len(t, snapshot.Samples, 1)
	require.Equal(t, 24, report.Limits.MaxRequests)
	require.Equal(t, 100, *ReportScore(report))
	after, err := common.Marshal(report)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestRetiredHistoricalEvidenceAndScoresRemainUnchanged(t *testing.T) {
	// Read an old saved report without applying new-request normalization.
	const saved = `{"version":16,"id":"history-fixture","model":"claude-opus-5","options":{"suite":"focused","fingerprint":true,"bedrock":true},"limits":{"max_requests":51},"plan":[{"id":"behavior_fingerprint","selected":true},{"id":"bedrock_web_search","selected":true}],"fingerprint":{"version":2,"repetitions":10,"cells":[{"id":"letter","profile":"original-profile","attempts":10,"valid":10,"counts":{"a":10}}]},"benchmark":{"version":3,"items":[{"id":"json_types","score":0,"correct":false},{"id":"json_extraction","score":100,"correct":true}]},"checks":[{"id":"basic","status":"pass"},{"id":"model_consistency","status":"pass"},{"id":"behavior_fingerprint","status":"inconclusive","code":"fingerprint_collected"},{"id":"bedrock_web_search","status":"pass","code":"bedrock_expected_rejection"}],"samples":[{"probe":"fingerprint_letter_01","http_status":200,"request_profile":"original-profile","usage":{"input_tokens":30}},{"probe":"bedrock_web_search","http_status":400,"diagnostic":{"code":"server_tools"}}]}`
	var report Report
	require.NoError(t, common.UnmarshalJsonStr(saved, &report))
	before, err := common.Marshal(report)
	require.NoError(t, err)
	score := ReportScore(report)
	require.NotNil(t, score)
	snapshot := SnapshotBaseline(report, "historical-baseline", "aws", 1)
	require.True(t, snapshot.Options.Fingerprint)
	require.True(t, snapshot.Options.Bedrock)
	require.Equal(t, report.Fingerprint, snapshot.Fingerprint)
	require.Equal(t, report.Plan, snapshot.Plan)
	require.Equal(t, report.Checks, snapshot.Checks)
	require.Len(t, snapshot.Samples, 2)
	require.Equal(t, "original-profile", snapshot.Samples[0].RequestProfile)
	require.Equal(t, "bedrock_web_search", snapshot.Samples[1].Probe)
	require.Equal(t, 51, report.Limits.MaxRequests)
	var decoded Report
	require.NoError(t, common.Unmarshal(before, &decoded))
	require.Equal(t, report, decoded)
	withoutObservations := decoded
	withoutObservations.Fingerprint = nil
	withoutObservations.Checks = decoded.Checks[:2]
	require.Equal(t, score, ReportScore(withoutObservations))
	after, err := common.Marshal(report)
	require.NoError(t, err)
	require.Equal(t, before, after, "new budget and baseline helpers must not rewrite history")
}
