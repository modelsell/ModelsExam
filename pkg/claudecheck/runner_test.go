package claudecheck

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

func TestProgressReflectsActualSequentialProbes(t *testing.T) {
	var events []Event
	var active string
	var requests int
	transport := happyTransport(t, true)
	report := RunWithObserver(context.Background(), Options{Model: "claude-test", Thinking: true, Cache: true}, func(ctx context.Context, req Request) (Response, error) {
		require.NotEmpty(t, active, "probe_start must be delivered before requesting the upstream")
		requests++
		return transport(ctx, req)
	}, func(event Event) {
		// Copy events through the wire format so later report mutations cannot
		// turn a start snapshot into the final report in this test.
		b, err := common.Marshal(event)
		require.NoError(t, err)
		var snapshot Event
		require.NoError(t, common.Unmarshal(b, &snapshot))
		events = append(events, snapshot)
		if event.Type == "probe_start" {
			require.Empty(t, active)
			active = event.Probe
		}
		if event.Type == "sample" {
			require.Equal(t, active, event.Sample.Probe)
			active = ""
		}
	})
	require.Equal(t, "start", events[0].Type)
	require.Empty(t, events[0].Report.Samples)
	require.Equal(t, "done", events[len(events)-1].Type)
	require.Equal(t, 14, requests)
	require.Equal(t, len(report.Checks), len(events[len(events)-1].Report.Checks))
	require.Equal(t, report.Summary, events[len(events)-1].Report.Summary)
}

func testResponse(t *testing.T, value any) Response {
	t.Helper()
	body, err := common.Marshal(value)
	require.NoError(t, err)
	return Response{Status: 200, Body: body, Model: "global.anthropic.claude-opus-5", Header: http.Header{}}
}

func happyTransport(t *testing.T, cacheHit bool) Transport {
	cacheCalls := 0
	return func(ctx context.Context, request Request) (Response, error) {
		require.NoError(t, ctx.Err())
		if request.Count {
			return testResponse(t, map[string]any{"input_tokens": 10}), nil
		}
		input, output, write, read := int64(10), int64(5), int64(0), int64(0)
		m := message{ID: "msg_bdrk_test", Type: "message", Role: "assistant", Model: "global.anthropic.claude-opus-5", Content: []map[string]any{{"type": "text", "text": "PONG"}}, StopReason: "end_turn", Usage: Usage{Input: &input, Output: &output, CacheWrite: &write, CacheRead: &read}}
		if stream, _ := request.Body["stream"].(bool); stream {
			start := m
			start.Content = nil
			zero := int64(0)
			start.Usage.Output = &zero
			events := []any{map[string]any{"type": "message_start", "message": start}, map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}}, map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": "PONG"}}, map[string]any{"type": "content_block_stop", "index": 0}, map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}, "usage": map[string]any{"output_tokens": 0}}, map[string]any{"type": "message_stop"}}
			res := Response{Status: 200}
			for _, e := range events {
				b, err := common.Marshal(e)
				require.NoError(t, err)
				res.Events = append(res.Events, b)
			}
			return res, nil
		}
		if _, yes := request.Body["tools"]; yes {
			m.StopReason = "tool_use"
			m.Content = []map[string]any{{"type": "tool_use", "id": "toolu_bdrk_test", "name": "record_probe", "input": map[string]any{"value": "PONG"}}}
		}
		if tools, yes := request.Body["tools"].([]any); yes && tools[0].(map[string]any)["name"] == "get_weather" {
			m.Content = []map[string]any{{"type": "tool_use", "id": "toolu_test", "name": "get_weather", "input": map[string]any{"city": "Tokyo", "unit": "celsius"}}}
		}
		if _, yes := request.Body["thinking"]; yes {
			require.Equal(t, "adaptive", request.Body["thinking"].(map[string]any)["type"])
			messages := request.Body["messages"].([]any)
			if len(messages) > 1 {
				assistant := messages[1].(map[string]any)
				blocks := assistant["content"].([]map[string]any)
				require.Equal(t, "opaque-signature-with-padding==", blocks[0]["signature"])
				require.Equal(t, "private reasoning", blocks[0]["thinking"])
				m.StopReason = "end_turn"
				m.Content = []map[string]any{{"type": "text", "text": "Done"}}
			} else {
				m.Content = append([]map[string]any{{"type": "thinking", "thinking": "private reasoning", "signature": "opaque-signature-with-padding=="}}, m.Content...)
			}
		}
		if n := request.Body["max_tokens"]; n == 1 {
			output = 1
			m.StopReason = "max_tokens"
		} else if n == 0 {
			output = 0
			m.StopReason = "max_tokens"
			m.Content = []map[string]any{}
		}
		if _, yes := request.Body["stop_sequences"]; yes {
			m.StopReason = "stop_sequence"
			m.Content = []map[string]any{{"type": "text", "text": "ALPHA "}}
		}
		messages := request.Body["messages"].([]any)
		if len(messages) == 3 && request.Body["thinking"] == nil {
			first := messages[0].(map[string]any)["content"].(string)
			m.Content = []map[string]any{{"type": "text", "text": strings.TrimPrefix(first, "Remember this code: ")}}
		}
		if _, yes := request.Body["system"].([]any); yes {
			cacheCalls++
			if cacheCalls == 1 || !cacheHit {
				write = 6000
			} else {
				read = 6000
			}
		}
		return testResponse(t, m), nil
	}
}

func TestRunEvidenceAndCacheLedger(t *testing.T) {
	for _, hit := range []bool{true, false} {
		t.Run(map[bool]string{true: "hit", false: "repeated_write"}[hit], func(t *testing.T) {
			r := Run(context.Background(), Options{Model: "claude-alias", Cache: true, Thinking: true}, happyTransport(t, hit))
			require.Len(t, r.Samples, 14)
			states := map[string]Check{}
			for _, c := range r.Checks {
				states[c.ID] = c
			}
			for _, id := range []string{"basic", "system", "stream", "tool", "max_tokens", "stop_sequence", "multi_turn", "zero_output", "thinking", "signature_replay", "token_count"} {
				require.Equal(t, "pass", states[id].Status, id)
			}
			require.Equal(t, "inconclusive", states["source"].Status)
			require.Equal(t, "inconclusive", states["billing"].Status)
			require.Equal(t, "inconclusive", states["prompt_integrity"].Status)
			if hit {
				require.Equal(t, "pass", states["cache"].Status)
			} else {
				require.Equal(t, "inconclusive", states["cache"].Status)
				require.Equal(t, float64(0), states["cache"].Evidence["read_token_ratio"])
			}
			body, err := common.Marshal(r)
			require.NoError(t, err)
			require.NotContains(t, string(body), "opaque-signature")
			require.NotContains(t, string(body), "private reasoning")
		})
	}
}

func TestMissingToolIDFails(t *testing.T) {
	call := happyTransport(t, true)
	r := Run(context.Background(), Options{Model: "claude-test"}, func(ctx context.Context, req Request) (Response, error) {
		res, err := call(ctx, req)
		if req.Body["tools"] != nil {
			var m message
			require.NoError(t, common.Unmarshal(res.Body, &m))
			delete(m.Content[0], "id")
			res = testResponse(t, m)
		}
		return res, err
	})
	for _, check := range r.Checks {
		if check.ID == "tool" {
			require.Equal(t, "fail", check.Status)
		}
	}
}

func TestCountUnavailableAndMissingUsageAreInconclusive(t *testing.T) {
	call := happyTransport(t, true)
	r := Run(context.Background(), Options{Model: "claude-test"}, func(ctx context.Context, req Request) (Response, error) {
		if req.Count {
			return Response{Status: 404}, nil
		}
		return call(ctx, req)
	})
	for _, c := range r.Checks {
		if c.ID == "token_count" {
			require.Equal(t, "inconclusive", c.Status)
		}
	}
	r = Run(context.Background(), Options{Model: "claude-test"}, func(context.Context, Request) (Response, error) {
		return Response{Status: 200, Body: []byte(`{"type":"message","id":"msg_x","role":"assistant","model":"claude","stop_reason":"end_turn","content":[{"type":"text","text":"PONG"}]}`)}, nil
	})
	require.Len(t, r.Samples, 2)
	require.Equal(t, "fail", r.Checks[0].Status)
	require.Nil(t, r.Samples[0].Usage.Input)
}

func TestCancellationStopsDependentCalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	r := Run(ctx, Options{Model: "claude-test", Cache: true}, func(context.Context, Request) (Response, error) {
		calls++
		cancel()
		return Response{}, context.Canceled
	})
	require.Equal(t, 1, calls)
	require.True(t, r.Cancelled)
	require.Equal(t, 0, r.Summary["fail"])
}

func TestStreamLifecycleRejectsErrorsAndTruncation(t *testing.T) {
	transport := happyTransport(t, true)
	res, err := transport(context.Background(), Request{Body: map[string]any{"stream": true}})
	require.NoError(t, err)
	m, ok := parseStream(res.Events)
	require.True(t, ok)
	require.NotNil(t, m.Usage.Output)
	require.Zero(t, *m.Usage.Output)
	_, ok = parseStream(res.Events[:len(res.Events)-1])
	require.False(t, ok)
	_, ok = parseStream(append(res.Events, []byte(`{"type":"error"}`)))
	require.False(t, ok)
	_, ok = parseStream(append([][]byte{[]byte(`{"type":"content_block_delta","index":0}`)}, res.Events...))
	require.False(t, ok)
}

func TestReadSSEBoundsAndFrames(t *testing.T) {
	events, first, err := ReadSSE(strings.NewReader(": heartbeat\n\ndata: {\ndata: \"type\":\"ping\"}\n\n"), time.Now())
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.NotNil(t, first)
	_, _, err = ReadSSE(strings.NewReader("data: {}\n"), time.Now())
	require.Error(t, err)
	_, _, err = ReadSSE(strings.NewReader(strings.Repeat("data: {}\n\n", MaxResponseBytes/8)), time.Now())
	require.Error(t, err)
}
