package claudecheck

import (
	"context"
	"encoding/base64"
	"image/png"
	"strings"
	"testing"

	"model-check/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func resultCheck(t *testing.T, report Report, id string) Check {
	t.Helper()
	for _, c := range report.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("check %s missing", id)
	return Check{}
}

func TestVersionedPlanAndRepeatedProbes(t *testing.T) {
	call := happyTransport(t, true)
	options := Options{Model: "claude-alias", Cache: true, Thinking: true, Repeat: true, Vision: true}
	report := Run(context.Background(), options, func(ctx context.Context, req Request) (Response, error) {
		res, err := call(ctx, req)
		if len(res.Events) > 0 {
			var start map[string]any
			require.NoError(t, common.Unmarshal(res.Events[0], &start))
			start["message"].(map[string]any)["id"] = uuid.NewString()
			res.Events[0], err = common.Marshal(start)
			require.NoError(t, err)
		}
		messages := req.Body["messages"].([]any)
		if content, ok := messages[0].(map[string]any)["content"].([]any); ok {
			source := content[0].(map[string]any)["source"].(map[string]any)
			data, err := base64.StdEncoding.DecodeString(source["data"].(string))
			require.NoError(t, err)
			img, err := png.Decode(strings.NewReader(string(data)))
			require.NoError(t, err)
			r, g, b, _ := img.At(0, 0).RGBA()
			name := "RED"
			if g > r {
				name = "GREEN"
			}
			if b > r && b > g {
				name = "BLUE"
			}
			var m message
			require.NoError(t, common.Unmarshal(res.Body, &m))
			m.Content = []map[string]any{{"type": "text", "text": name}}
			res = testResponse(t, m)
		}
		return res, err
	})
	require.Equal(t, 8, report.Version)
	require.Len(t, report.Plan, 28)
	require.Len(t, report.Checks, len(report.Plan))
	require.Len(t, report.Samples, 18)
	ids := map[string]bool{}
	for _, item := range report.Plan {
		require.False(t, ids[item.ID])
		ids[item.ID] = true
		resultCheck(t, report, item.ID)
	}
	require.Equal(t, "pass", resultCheck(t, report, "repeatability").Status)
	require.Equal(t, "pass", resultCheck(t, report, "vision").Status)
	require.Equal(t, 2, resultCheck(t, report, "cache").Evidence["warm_hits"])
}

func TestRepeatedIDsAreAnObservationNotForgeryProof(t *testing.T) {
	report := Run(context.Background(), Options{Model: "claude-test", Repeat: true}, happyTransport(t, false))
	check := resultCheck(t, report, "repeatability")
	require.Equal(t, "inconclusive", check.Status)
	require.Equal(t, 1, check.Evidence["unique_ids"])
}

func TestBaselineFailureResolvesSelectedAndUnselectedPlan(t *testing.T) {
	report := Run(context.Background(), Options{Model: "claude-test"}, func(context.Context, Request) (Response, error) { return Response{Status: 401}, nil })
	require.Len(t, report.Samples, 1)
	require.Len(t, report.Checks, len(report.Plan))
	require.Equal(t, "not_requested", resultCheck(t, report, "vision").Code)
	require.Equal(t, "run_stopped", resultCheck(t, report, "stream").Code)
}

func TestCountDifferenceDoesNotAssertBillingFraud(t *testing.T) {
	call := happyTransport(t, false)
	report := Run(context.Background(), Options{Model: "claude-test"}, func(ctx context.Context, req Request) (Response, error) {
		if req.Count {
			return testResponse(t, map[string]any{"input_tokens": 11}), nil
		}
		return call(ctx, req)
	})
	check := resultCheck(t, report, "token_count")
	require.Equal(t, "inconclusive", check.Status)
	require.Equal(t, int64(-1), check.Evidence["difference"])
}

func TestAWSCountersSkipAmbiguousCachedInputsAndPreserveZero(t *testing.T) {
	zero, ten := int64(0), int64(10)
	r := runner{ctx: context.Background(), report: Report{Samples: []Sample{
		{Status: 200, Usage: Usage{Input: &ten, Output: &zero, CacheWrite: &zero, CacheRead: &zero}, Headers: map[string]string{"x-amzn-bedrock-input-token-count": "10", "x-amzn-bedrock-output-token-count": "0"}},
		{Status: 200, Usage: Usage{Input: &ten, Output: &zero, CacheWrite: &zero, CacheRead: &ten}, Headers: map[string]string{"x-amzn-bedrock-input-token-count": "20"}},
	}}}
	r.usageAudit()
	check := resultCheck(t, r.report, "aws_usage")
	require.Equal(t, "pass", check.Status)
	require.Equal(t, 2, check.Evidence["compared_fields"])
}

func TestStreamMetricsExcludePingsThinkingAndEmptyText(t *testing.T) {
	var metrics StreamMetrics
	metrics.Observe([]byte(`{"type":"ping"}`), 1)
	metrics.Observe([]byte(`{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"private"}}`), 10)
	metrics.Observe([]byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":""}}`), 12)
	metrics.Observe([]byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"  "}}`), 13)
	require.Nil(t, metrics.FirstTextMS)
	metrics.Observe([]byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"P"}}`), 20)
	metrics.Observe([]byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"ONG"}}`), 70)
	require.Equal(t, int64(20), *metrics.FirstTextMS)
	require.Equal(t, int64(50), *metrics.MaxGapMS)
	require.Equal(t, 2, metrics.TextEvents)
	data, err := common.Marshal(metrics)
	require.NoError(t, err)
	require.NotContains(t, string(data), "private")
}

func TestStreamReassemblesOpaqueThinkingAndAllowsNewMetadata(t *testing.T) {
	events := [][]byte{
		[]byte(`{"type":"message_start","message":{"type":"message","id":"msg_test","role":"assistant","model":"claude","usage":{"input_tokens":10,"output_tokens":0}}}`),
		[]byte(`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`),
		[]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"opaque=="}}`),
		[]byte(`{"type":"content_block_stop","index":0}`),
		[]byte(`{"type":"new_metadata_event"}`),
		[]byte(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":0}}`),
		[]byte(`{"type":"message_stop"}`),
	}
	m, valid := parseStream(events)
	require.True(t, valid)
	require.Equal(t, "opaque==", m.Content[0]["signature"])
	_, valid = parseStream(events[:len(events)-1])
	require.False(t, valid)
}
