package claudecheck

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

func TestTokenCounterScore(t *testing.T) {
	for _, tc := range []struct {
		expected, actual int64
		score            int
	}{
		{100, 100, 100}, {100, 125, 80}, {125, 100, 80}, {6000, 0, 0}, {6000, 5999, 99}, {0, 0, 100},
	} {
		item := TokenComparison{Expected: &tc.expected, Actual: &tc.actual}
		compareTokenCounts(&item)
		require.Equal(t, tc.score, *item.Score)
		require.Equal(t, tc.actual-tc.expected, *item.Difference)
	}
	item := TokenComparison{}
	compareTokenCounts(&item)
	require.Nil(t, item.Score)
}

func TestPromptTokenAuditUsesThreeExactRequestPairs(t *testing.T) {
	base := happyTransport(t, true)
	options := Options{Model: "claude-opus-5", PromptAudit: true}
	for _, mode := range []string{"equal", "extra", "missing_count", "changed_count", "invalid_count", "missing_usage"} {
		t.Run(mode, func(t *testing.T) {
			lastProfile := ""
			var inferenceBodies, countBodies []string
			events := 0
			report := RunWithObserver(context.Background(), options, func(ctx context.Context, req Request) (Response, error) {
				if req.Body["thinking"] == nil {
					return base(ctx, req)
				}
				profile := bodyProfile(req.Body)
				if req.Count {
					countBodies = append(countBodies, profile)
					require.Equal(t, lastProfile, profile)
					if mode == "missing_count" {
						return Response{Status: 404}, nil
					}
					count := int64(110)
					if mode == "invalid_count" {
						count = -1
					}
					response := testResponse(t, map[string]any{"input_tokens": count})
					response.RequestProfile = profile
					if mode == "changed_count" {
						response.RequestProfile = "different"
					}
					return response, nil
				}
				inferenceBodies = append(inferenceBodies, profile)
				lastProfile = profile
				input, output, read, write := int64(10), int64(1), int64(40), int64(60)
				if mode == "extra" {
					input += 110
				}
				usage := Usage{Input: &input, Output: &output, CacheRead: &read, CacheWrite: &write}
				if mode == "missing_usage" {
					usage.Input = nil
				}
				response := testResponse(t, message{ID: "msg_test", Type: "message", Role: "assistant", Model: options.Model, StopReason: "end_turn", Content: []map[string]any{{"type": "text", "text": "PONG"}}, Usage: usage})
				response.RequestProfile = profile
				return response, nil
			}, func(event Event) {
				if event.Type == "token_audit" {
					events++
				}
			})
			require.Len(t, inferenceBodies, 3)
			if mode == "missing_usage" {
				require.Empty(t, countBodies)
			} else {
				require.Len(t, countBodies, 3)
				require.Equal(t, inferenceBodies, countBodies)
			}
			require.NotEqual(t, inferenceBodies[0], inferenceBodies[1])
			require.NotEqual(t, inferenceBodies[1], inferenceBodies[2])
			if mode == "missing_usage" {
				require.Len(t, report.Samples, MaxRequests(options)-3)
			} else {
				require.Len(t, report.Samples, MaxRequests(options))
			}
			require.Equal(t, 14, report.Limits.MaxRequests)
			require.Equal(t, 3, events)
			for _, item := range report.TokenAudit.Prompt {
				if mode == "equal" {
					require.Equal(t, 100, *item.Score)
				} else if mode == "extra" {
					require.Equal(t, int64(110), *item.Difference)
					require.Equal(t, 50, *item.Score)
				} else {
					require.Nil(t, item.Score)
				}
			}
			// Count-only replies do not contaminate usage totals or model evidence.
			if mode == "missing_usage" {
				require.Equal(t, 3, resultCheck(t, report, "usage_fields").Evidence["missing"])
			} else {
				require.Zero(t, resultCheck(t, report, "usage_fields").Evidence["missing"])
			}
			require.Equal(t, "inconclusive", resultCheck(t, report, "prompt_integrity").Status)
		})
	}
}

func TestCacheTokenAuditMissingAndRepeatedWrites(t *testing.T) {
	valid := true
	zero, size, half := int64(0), int64(6000), int64(3000)
	for _, mode := range []string{"equal", "partial", "miss", "no_write", "mixed_first", "different_request", "missing_read", "http_error"} {
		t.Run(mode, func(t *testing.T) {
			first := Sample{Valid: &valid, RequestProfile: "same", Usage: Usage{CacheWrite: &size, CacheRead: &zero}}
			current := Sample{Valid: &valid, RequestProfile: "same", Usage: Usage{CacheWrite: &zero, CacheRead: &size}}
			switch mode {
			case "partial":
				current.Usage.CacheRead = &half
			case "miss":
				current.Usage.CacheRead, current.Usage.CacheWrite = &zero, &size
			case "no_write":
				first.Usage.CacheWrite = &zero
			case "mixed_first":
				first.Usage.CacheRead = &half
			case "different_request":
				current.RequestProfile = "other"
			case "missing_read":
				current.Usage.CacheRead = nil
			case "http_error":
				no := false
				current.Valid = &no
			}
			r := runner{report: Report{TokenAudit: &TokenAuditReport{Cache: []TokenComparison{{ID: "cache_read_1"}}}}}
			r.compareCacheTokens(0, first, current)
			item := r.report.TokenAudit.Cache[0]
			switch mode {
			case "equal":
				require.Equal(t, 100, *item.Score)
			case "partial":
				require.Equal(t, 50, *item.Score)
			case "miss":
				require.Zero(t, *item.Score)
				require.Equal(t, size, *item.RepeatedWrite)
			default:
				require.Nil(t, item.Score)
			}
		})
	}
}

func TestTokenAuditCancellationAndBaselineSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	base := happyTransport(t, true)
	report := Run(ctx, Options{Model: "claude-opus-5", PromptAudit: true, Cache: true}, func(ctx context.Context, request Request) (Response, error) {
		if request.Body["thinking"] != nil {
			cancel()
			return Response{Status: http.StatusRequestTimeout}, ctx.Err()
		}
		response, err := base(ctx, request)
		response.RequestProfile = bodyProfile(request.Body)
		return response, err
	})
	require.True(t, report.Cancelled)
	require.NotNil(t, report.TokenAudit.Cache[0].Score)
	require.Nil(t, report.TokenAudit.Prompt[0].Score)
	require.Equal(t, "not_collected", report.TokenAudit.Prompt[1].Code)
	snapshot := SnapshotBaseline(report, "reference", "aws", 0)
	data, err := common.Marshal(snapshot)
	require.NoError(t, err)
	require.Contains(t, string(data), "token_audit")
	require.NotContains(t, string(data), "MAPLE-7391")
	require.Less(t, len(data), 32<<10)
	for _, sample := range report.Samples {
		if strings.HasSuffix(sample.Probe, "_count") {
			require.True(t, IsCountProbe(sample.Probe))
		}
	}
}
