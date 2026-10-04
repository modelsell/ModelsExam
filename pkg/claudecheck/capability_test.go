package claudecheck

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

func capabilityHappyResponse(t *testing.T, request Request) Response {
	t.Helper()
	body := request.Body
	var fixture *capabilityFixture
	for _, f := range capabilityFixtures("claude-opus-5") {
		if bodyProfile(f.body) == bodyProfile(body) {
			copy := f
			fixture = &copy
			break
		}
	}
	input, output := int64(30), int64(8)
	m := message{ID: "msg_capability", Model: "claude-opus-5", Type: "message", Role: "assistant", StopReason: "end_turn", Usage: Usage{Input: &input, Output: &output}}
	if fixture != nil {
		m.Content = []map[string]any{{"type": "text", "text": fixture.expected}}
		if fixture.category == "tools" {
			name := "lookup_inventory"
			args := map[string]any{"sku": "SKU-204", "warehouse": "north", "include_reserved": false}
			if fixture.id == "tool_roundtrip" {
				name = "lookup_order"
				args = map[string]any{"order_id": "ORD-731"}
			}
			m.StopReason = "tool_use"
			m.Content = []map[string]any{{"type": "tool_use", "id": "toolu_round_987", "name": name, "input": args}}
		}
	} else {
		messages, ok := body["messages"].([]any)
		require.True(t, ok)
		require.Len(t, messages, 3)
		assistant := messages[1].(map[string]any)["content"].([]map[string]any)
		result := messages[2].(map[string]any)["content"].([]any)[0].(map[string]any)
		var toolID any
		for _, block := range assistant {
			if block["type"] == "tool_use" {
				toolID = block["id"]
			}
		}
		require.NotNil(t, toolID)
		require.Equal(t, toolID, result["tool_use_id"])
		require.Equal(t, capabilityToolResult, result["content"])
		m.Content = []map[string]any{{"type": "text", "text": capabilityToolResult}}
	}
	response := testResponse(t, m)
	response.RequestProfile = bodyProfile(body)
	return response
}

func collectCapability(t *testing.T, transport Transport, observer func(Event)) Report {
	t.Helper()
	r := runner{ctx: context.Background(), call: transport, observe: observer, report: Report{Model: "claude-opus-5", Options: &Options{Suite: "focused"}}}
	r.benchmark()
	return r.report
}

func TestCapabilityFullSuiteExcludesRetiredQuestions(t *testing.T) {
	starts, progress := 0, 0
	r := collectCapability(t, func(_ context.Context, request Request) (Response, error) {
		return capabilityHappyResponse(t, request), nil
	}, func(event Event) {
		if event.Type == "benchmark" {
			progress++
			require.Len(t, event.Benchmark.Items, 6)
			if progress == 1 {
				for _, item := range event.Benchmark.Items {
					require.Equal(t, "pending", item.Code)
					require.Nil(t, item.Score)
				}
				starts++
			}
		}
	})
	require.Equal(t, 1, starts)
	require.Greater(t, progress, 6)
	require.Len(t, r.Samples, 7)
	require.Equal(t, 3, r.Benchmark.Version)
	categories := map[string]int{}
	for _, item := range r.Benchmark.Items {
		require.Equal(t, 100, *item.Score, item.ID)
		require.True(t, *item.Correct, item.ID)
		require.Equal(t, "scored", item.Code)
		categories[item.Category]++
		for _, old := range []string{"digit_count", "boolean_count", "python_alias", "javascript_queue", "zh_constraint", "context_retrieval"} {
			require.NotEqual(t, old, item.ID)
			for _, sample := range r.Samples {
				require.NotEqual(t, "benchmark_"+old, sample.Probe)
			}
		}
	}
	require.Equal(t, map[string]int{"instruction": 1, "structured": 2, "context": 1, "tools": 2}, categories)
}

func TestCapabilityJSONGradingAndPartialCredit(t *testing.T) {
	expected := map[string]any{"id": "0007", "n": float64(0), "missing": nil, "active": false}
	for _, text := range []string{`{"id":"0007","n":0,"missing":null,"active":false}`, "{\n\"active\": false, \"missing\":null,\"n\":0,\"id\":\"0007\"}"} {
		item := BenchmarkItem{}
		scoreCapability(&item, gradeCapabilityJSON(text, expected))
		require.Equal(t, 100, *item.Score)
	}
	for _, text := range []string{`{"id":7,"n":"0","missing":"null","active":"false"}`, `{"id":"0007","n":0,"missing":null,"active":false,"invented":"x"}`} {
		item := BenchmarkItem{}
		scoreCapability(&item, gradeCapabilityJSON(text, expected))
		require.Greater(t, *item.Score, 0)
		require.Less(t, *item.Score, 100)
	}
	for _, text := range []string{"```json\n{}\n```", "answer {\"id\":\"0007\"}", "{} {}", "null", "[]"} {
		item := BenchmarkItem{}
		scoreCapability(&item, gradeCapabilityJSON(text, expected))
		require.Zero(t, *item.Score)
	}
	item := BenchmarkItem{}
	scoreCapability(&item, gradeInstructionFormat("REPORT\nred,green,blue\nEND"))
	require.Equal(t, 75, *item.Score)
}

func TestCapabilityUnavailableAndWrongAnswer(t *testing.T) {
	for _, mode := range []string{"http400", "http403", "http429", "timeout", "truncated", "profile", "wrong", "tool_missing", "tool_wrong_type"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			r := collectCapability(t, func(_ context.Context, request Request) (Response, error) {
				calls++
				if strings.HasPrefix(mode, "http") {
					var status int
					_, _ = fmt.Sscanf(mode, "http%d", &status)
					return Response{Status: status}, errors.New("synthetic rejection")
				}
				if mode == "timeout" {
					return Response{Status: 504}, context.DeadlineExceeded
				}
				response := capabilityHappyResponse(t, request)
				if mode == "profile" {
					response.RequestProfile = "modified"
					return response, nil
				}
				var m message
				require.NoError(t, common.Unmarshal(response.Body, &m))
				switch mode {
				case "truncated":
					m.StopReason = "max_tokens"
				case "wrong":
					m.Content = []map[string]any{{"type": "text", "text": "incorrect"}}
					m.StopReason = "end_turn"
				case "tool_missing":
					if request.Body["tools"] != nil {
						m.Content = []map[string]any{{"type": "text", "text": "cannot answer"}}
						m.StopReason = "end_turn"
					}
				case "tool_wrong_type":
					if m.StopReason == "tool_use" {
						m.Content[0]["input"] = map[string]any{"sku": "SKU-204", "warehouse": "north", "include_reserved": "false"}
					}
				}
				response.Body, _ = common.Marshal(m)
				return response, nil
			}, nil)
			if mode == "http403" || mode == "http429" {
				require.Equal(t, 1, calls)
				require.Equal(t, "not_collected", r.Benchmark.Items[1].Code)
			}
			for _, item := range r.Benchmark.Items {
				if mode == "wrong" || ((mode == "tool_missing" || mode == "tool_wrong_type") && item.Category == "tools") {
					require.NotNil(t, item.Score)
					require.Zero(t, *item.Score)
					require.False(t, *item.Correct)
				} else if mode != "tool_missing" && mode != "tool_wrong_type" {
					require.Nil(t, item.Score)
					require.Nil(t, item.Correct)
					require.NotEqual(t, "pending", item.Code)
				}
			}
		})
	}
}

func TestCapabilityToolRoundtripUsesRealCallIDAndChecksSecondProfile(t *testing.T) {
	for _, badProfile := range []bool{false, true} {
		calls := 0
		r := collectCapability(t, func(_ context.Context, request Request) (Response, error) {
			response := capabilityHappyResponse(t, request)
			if len(request.Body["messages"].([]any)) == 3 {
				calls++
				messages := request.Body["messages"].([]any)
				result := messages[2].(map[string]any)["content"].([]any)[0].(map[string]any)
				require.Equal(t, "toolu_round_987", result["tool_use_id"])
				if badProfile {
					response.RequestProfile = "changed-second-round"
				}
			}
			return response, nil
		}, nil)
		require.Equal(t, 1, calls)
		item := r.Benchmark.Items[5]
		if badProfile {
			require.Nil(t, item.Score)
			require.Equal(t, "request_profile_changed", item.Code)
		} else {
			require.Equal(t, 100, *item.Score)
		}
		require.NotContains(t, item.Actual, "toolu_round_987")
	}
}

func TestCapabilityCancellationKeepsUncollectedAndBoundsActual(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := runner{ctx: ctx, report: Report{Model: "claude-opus-5", Options: &Options{Suite: "focused"}}}
	r.call = func(_ context.Context, request Request) (Response, error) {
		cancel()
		return capabilityHappyResponse(t, request), nil
	}
	r.benchmark()
	require.Len(t, r.report.Samples, 1)
	require.Len(t, r.report.Benchmark.Items, 6)
	for _, item := range r.report.Benchmark.Items[1:] {
		require.Equal(t, "not_collected", item.Code)
		require.Nil(t, item.Score)
	}
	require.Len(t, []rune(capabilityActual(strings.Repeat("测试", 600))), 512)
	item := BenchmarkItem{}
	scoreCapability(&item, []BenchmarkAssertion{{"answer", false}})
	data, err := common.Marshal(item)
	require.NoError(t, err)
	require.Contains(t, string(data), `"score":0`)
}

func TestCapabilityRejectsUnexpectedToolCallsAndTransportMutation(t *testing.T) {
	for _, mode := range []string{"tool_stop", "extra_tool_block", "mutated_body"} {
		t.Run(mode, func(t *testing.T) {
			r := collectCapability(t, func(_ context.Context, request Request) (Response, error) {
				response := capabilityHappyResponse(t, request)
				if request.Body["tools"] != nil {
					return response, nil
				}
				if mode == "mutated_body" {
					request.Body["max_tokens"] = 2048
					response.RequestProfile = bodyProfile(request.Body)
					return response, nil
				}
				var m message
				require.NoError(t, common.Unmarshal(response.Body, &m))
				if mode == "tool_stop" {
					m.StopReason = "tool_use"
				} else {
					m.Content = append(m.Content, map[string]any{"type": "tool_use", "id": "unexpected", "name": "lookup_inventory", "input": map[string]any{}})
				}
				response.Body, _ = common.Marshal(m)
				return response, nil
			}, nil)
			for _, item := range r.Benchmark.Items {
				if item.Category == "tools" {
					continue
				}
				require.Nil(t, item.Score, item.ID)
				if mode == "mutated_body" {
					require.Equal(t, "request_profile_changed", item.Code)
				} else {
					require.Equal(t, "unexpected_stop", item.Code)
				}
			}
		})
	}
}

func TestCapabilityInitialConnectionFailureStillListsAllTasks(t *testing.T) {
	r := Run(context.Background(), Options{Suite: "focused", Model: "claude-opus-5"}, func(context.Context, Request) (Response, error) {
		return Response{Status: 401}, errors.New("synthetic unauthorized")
	})
	require.Equal(t, 3, r.Benchmark.Version)
	require.Len(t, r.Benchmark.Items, 6)
	for _, item := range r.Benchmark.Items {
		require.Equal(t, "not_collected", item.Code)
		require.Nil(t, item.Score)
	}
}

func TestCapabilityRoundtripProfileIgnoresCallIDOnlyAtTaskLevel(t *testing.T) {
	var reports []Report
	for _, id := range []string{"toolu_alpha", "toolu_beta"} {
		reports = append(reports, collectCapability(t, func(_ context.Context, request Request) (Response, error) {
			response := capabilityHappyResponse(t, request)
			var m message
			require.NoError(t, common.Unmarshal(response.Body, &m))
			if m.StopReason == "tool_use" {
				m.Content[0]["id"] = id
				response.Body, _ = common.Marshal(m)
			}
			return response, nil
		}, nil))
	}
	a, b := reports[0], reports[1]
	require.Equal(t, a.Benchmark.Items[5].Profile, b.Benchmark.Items[5].Profile)
	require.NotEqual(t, a.Samples[6].RequestProfile, b.Samples[6].RequestProfile)
	require.Equal(t, 100, *a.Benchmark.Items[5].Score)
	require.Equal(t, 100, *b.Benchmark.Items[5].Score)
}

func TestCapabilityRoundtripUnavailableResultDoesNotInventScore(t *testing.T) {
	for _, status := range []int{400, 403, 429, 503} {
		r := collectCapability(t, func(_ context.Context, request Request) (Response, error) {
			if len(request.Body["messages"].([]any)) == 3 {
				return Response{Status: status}, errors.New("synthetic result unavailable")
			}
			return capabilityHappyResponse(t, request), nil
		}, nil)
		require.Len(t, r.Samples, 7)
		require.Nil(t, r.Benchmark.Items[5].Score)
		require.Nil(t, r.Benchmark.Items[5].Correct)
		require.NotEqual(t, "scored", r.Benchmark.Items[5].Code)
	}
}

func TestCapabilityToolsUseNativeDefaultsAndPreserveReplayBlocks(t *testing.T) {
	for _, f := range capabilityFixtures("claude-opus-5") {
		if f.category != "tools" {
			continue
		}
		require.NotContains(t, f.body, "thinking")
		require.Equal(t, 1024, f.body["max_tokens"])
		require.Equal(t, map[string]any{"type": "auto", "disable_parallel_tool_use": true}, f.body["tool_choice"])
		for _, key := range []string{"temperature", "top_p", "top_k"} {
			require.NotContains(t, f.body, key)
		}
	}
	replayed := false
	r := collectCapability(t, func(_ context.Context, request Request) (Response, error) {
		if len(request.Body["messages"].([]any)) == 3 {
			blocks := request.Body["messages"].([]any)[1].(map[string]any)["content"].([]map[string]any)
			require.Equal(t, "thinking", blocks[0]["type"])
			require.Equal(t, "synthetic-opaque-signature", blocks[0]["signature"])
			replayed = true
		}
		response := capabilityHappyResponse(t, request)
		var m message
		require.NoError(t, common.Unmarshal(response.Body, &m))
		if m.StopReason == "tool_use" {
			m.Content = append([]map[string]any{{"type": "thinking", "thinking": "synthetic-internal-text", "signature": "synthetic-opaque-signature"}}, m.Content...)
			response.Body, _ = common.Marshal(m)
		}
		return response, nil
	}, nil)
	require.True(t, replayed)
	require.Equal(t, 100, *r.Benchmark.Items[5].Score)
	for _, item := range r.Benchmark.Items {
		require.NotContains(t, item.Actual, "synthetic-internal")
		require.NotContains(t, item.Actual, "opaque-signature")
	}
}
