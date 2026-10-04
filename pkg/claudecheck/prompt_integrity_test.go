package claudecheck

import (
	"context"
	"testing"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

func TestPromptIntegrityEvidenceLayersAndCountFallback(t *testing.T) {
	for _, mode := range []string{"equal", "unavailable", "count_forbidden", "small_delta", "extra_tokens", "wrong_answer", "truncated", "all_unavailable", "profile_changed"} {
		t.Run(mode, func(t *testing.T) {
			items := []TokenComparison{{ID: "short"}, {ID: "system"}, {ID: "long"}}
			r := runner{ctx: context.Background(), report: Report{Version: 9, Model: "claude-test", TokenAudit: &TokenAuditReport{Version: 2, Prompt: items}}}
			fixtures := promptIntegrityFixtures(r.report.Model)
			inferences, counts, events := 0, 0, 0
			r.observe = func(e Event) {
				if e.Type == "token_audit" {
					events++
				}
			}
			r.call = func(ctx context.Context, req Request) (Response, error) {
				profile := bodyProfile(req.Body)
				if req.Count {
					counts++
					require.Equal(t, bodyProfile(fixtures[inferences-1].body), profile)
					if mode == "count_forbidden" {
						return Response{Status: 403}, nil
					}
					if mode == "unavailable" {
						return Response{Status: 404}, nil
					}
					out := testResponse(t, map[string]any{"input_tokens": 100})
					out.RequestProfile = profile
					if mode == "profile_changed" {
						out.RequestProfile = "different"
					}
					return out, nil
				}
				inferences++
				if mode == "all_unavailable" {
					return Response{Status: 503}, nil
				}
				answer := []string{"PONG", "MAPLE-7391", "701544"}[inferences-1]
				if mode == "wrong_answer" {
					answer = "CEDAR-0000"
				}
				input, output, zero := int64(100), int64(8), int64(0)
				if mode == "small_delta" {
					input += 2
				}
				if mode == "extra_tokens" {
					input = 200
				}
				stop := "end_turn"
				if mode == "truncated" {
					stop = "max_tokens"
				}
				out := testResponse(t, message{ID: "msg_test", Type: "message", Role: "assistant", Model: r.report.Model, StopReason: stop,
					Content: []map[string]any{{"type": "text", "text": answer}}, Usage: Usage{Input: &input, Output: &output, CacheRead: &zero, CacheWrite: &zero}})
				out.RequestProfile = profile
				return out, nil
			}
			r.promptTokenAudit()
			a := r.report.TokenAudit.PromptAssessment
			if mode == "count_forbidden" {
				require.False(t, r.focusedStop())
			}
			require.Equal(t, 3, inferences)
			require.Equal(t, 3, events)
			switch mode {
			case "all_unavailable":
				require.Nil(t, a.Score)
				require.Zero(t, a.Coverage)
				require.Zero(t, counts)
			case "unavailable", "profile_changed", "count_forbidden":
				require.Equal(t, 30, *a.Score)
				require.Equal(t, 30, a.Coverage)
				if mode == "unavailable" || mode == "count_forbidden" {
					require.Equal(t, 1, counts)
				}
			case "extra_tokens":
				require.Equal(t, 65, *a.Score)
			case "wrong_answer":
				require.Equal(t, 70, *a.Score)
			case "truncated":
				require.Equal(t, 70, *a.Score)
				require.Equal(t, 70, a.Coverage)
			default:
				require.Equal(t, 100, *a.Score)
				require.Equal(t, 100, a.Coverage)
			}
			if mode == "small_delta" {
				for _, item := range r.report.TokenAudit.Prompt {
					require.Equal(t, "tokens_within_tolerance", item.Code)
					require.Equal(t, int64(2), *item.Difference)
				}
			}
			data, err := common.Marshal(SnapshotBaseline(r.report, "baseline", "aws", 0))
			require.NoError(t, err)
			require.Contains(t, string(data), "prompt_assessment")
			require.NotContains(t, string(data), "CEDAR-0000")
		})
	}
}

func TestPromptIntegrityDoesNotTreatSmallZeroCountersAsEqual(t *testing.T) {
	for _, values := range [][2]int64{{0, 0}, {2, 0}, {100, 103}} {
		item := TokenComparison{Expected: &values[0], Actual: &values[1]}
		compareTokenCounts(&item)
		comparePromptTokens(&item)
		if values[0] == 0 {
			require.Nil(t, item.Score)
		} else {
			require.Less(t, *item.Score, 100)
		}
	}
}
