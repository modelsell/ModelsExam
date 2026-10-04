package claudecheck

import (
	"context"
	"strings"
	"testing"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

func collectionTransport(t *testing.T, base Transport) Transport {
	return func(ctx context.Context, request Request) (Response, error) {
		if request.Body["system"] == capabilitySystem {
			return capabilityHappyResponse(t, request), nil
		}
		if request.Body["system"] != fingerprintSystem {
			return base(ctx, request)
		}
		require.Equal(t, "disabled", request.Body["thinking"].(map[string]any)["type"])
		for _, key := range []string{"temperature", "top_p", "top_k"} {
			require.NotContains(t, request.Body, key)
		}
		prompt := request.Body["messages"].([]any)[0].(map[string]any)["content"].(string)
		answer := ""
		for _, fixture := range choiceFixtures {
			if prompt == fixture.prompt {
				answer = fixture.choices[0]
			}
		}
		require.NotEmpty(t, answer)
		input, output := int64(30), int64(5)
		response := testResponse(t, message{ID: "msg_test", Model: "claude-opus-5", Type: "message", Role: "assistant", StopReason: "end_turn", Content: []map[string]any{{"type": "text", "text": answer}}, Usage: Usage{Input: &input, Output: &output}})
		response.RequestProfile = bodyProfile(request.Body)
		return response, nil
	}
}

func TestCollectionSuitesRespectBudgetAndPersistProgress(t *testing.T) {
	options := Options{Model: "claude-opus-5", Fingerprint: true, Benchmark: true}
	var fingerprints, benchmarks int
	r := RunWithObserver(context.Background(), options, collectionTransport(t, happyTransport(t, true)), func(event Event) {
		if event.Type == "fingerprint" {
			fingerprints++
		}
		if event.Type == "benchmark" {
			benchmarks++
		}
	})
	require.Equal(t, 15, MaxRequests(options))
	require.Len(t, r.Samples, 15)
	require.Zero(t, fingerprints)
	require.Equal(t, 8, benchmarks)
	require.Nil(t, r.Fingerprint)
	require.False(t, r.Options.Fingerprint)
	require.Len(t, r.Benchmark.Items, 6)
	for _, item := range r.Benchmark.Items {
		require.NotNil(t, item.Correct)
		require.True(t, *item.Correct)
	}
	for _, item := range r.Plan {
		if strings.HasSuffix(item.ID, "fingerprint") || item.ID == "capability_benchmark" {
			require.Equal(t, "boundary", item.Kind)
		}
	}
	snapshot := SnapshotBaseline(r, "reference-id", "aws", 123)
	data, err := common.Marshal(snapshot)
	require.NoError(t, err)
	require.Less(t, len(data), 32<<10)
	require.NotContains(t, string(data), "msg_test")
}

func TestCollectionRejectsOverridesAndIncompleteResponses(t *testing.T) {
	good := collectionTransport(t, happyTransport(t, true))
	for _, reason := range []string{"profile", "truncated", "refusal", "invalid_choice"} {
		t.Run(reason, func(t *testing.T) {
			r := Run(context.Background(), Options{Model: "claude-opus-5", Fingerprint: true, Benchmark: true}, func(ctx context.Context, request Request) (Response, error) {
				response, err := good(ctx, request)
				if request.Body["system"] != fingerprintSystem && request.Body["system"] != capabilitySystem {
					return response, err
				}
				if reason == "profile" {
					response.RequestProfile = "different"
					return response, err
				}
				var m message
				require.NoError(t, common.Unmarshal(response.Body, &m))
				switch reason {
				case "truncated":
					m.StopReason = "max_tokens"
				case "refusal":
					m.StopReason = "refusal"
				case "invalid_choice":
					m.Content = []map[string]any{{"type": "text", "text": "outside the allowed choices"}}
					m.StopReason = "end_turn"
				}
				response.Body, _ = common.Marshal(m)
				return response, err
			})
			require.Nil(t, r.Fingerprint)
			if reason != "invalid_choice" {
				for _, item := range r.Benchmark.Items {
					require.Nil(t, item.Correct)
					require.Nil(t, item.Score)
				}
			} else {
				for _, item := range r.Benchmark.Items {
					require.NotNil(t, item.Score)
					require.Zero(t, *item.Score)
				}
			}
		})
	}
}

func TestCollectionLegacyRequestsOnlyExecuteCurrentCapabilities(t *testing.T) {
	options := Options{Model: "claude-opus-5", Benchmark: true}
	r := Run(context.Background(), options, collectionTransport(t, happyTransport(t, true)))
	require.Equal(t, CapabilityVersion, r.Benchmark.Version)
	require.Equal(t, 15, MaxRequests(options))
	require.Len(t, r.Samples, 15)
	var ids, probes []string
	for _, item := range r.Benchmark.Items {
		ids = append(ids, item.ID)
	}
	for _, sample := range r.Samples {
		if strings.HasPrefix(sample.Probe, "benchmark_") {
			probes = append(probes, sample.Probe)
		}
	}
	require.Equal(t, []string{"instruction_format", "json_extraction", "json_types", "context_multihop", "tool_selection", "tool_roundtrip"}, ids)
	require.Equal(t, []string{"benchmark_instruction_format", "benchmark_json_extraction", "benchmark_json_types", "benchmark_context_multihop", "benchmark_tool_selection", "benchmark_tool_roundtrip", "benchmark_tool_roundtrip_result"}, probes)
}

func TestReferenceProfileIncludesAllBehaviorSettings(t *testing.T) {
	body := referenceBody("claude-opus-5", "example", 64)
	expected := bodyProfile(body)
	body["model"] = "global.anthropic.claude-opus-5"
	body["anthropic_version"] = "bedrock-2023-05-31"
	require.Equal(t, expected, bodyProfile(body))
	body["temperature"] = 0
	require.NotEqual(t, expected, bodyProfile(body))
}

func TestPerformanceCollectionBudgetAndObservationScope(t *testing.T) {
	var probes int
	options := Options{Model: "claude-opus-5", Performance: true}
	base := happyTransport(t, true)
	r := Run(context.Background(), options, func(ctx context.Context, request Request) (Response, error) {
		if request.Body["system"] == fingerprintSystem {
			probes++
			require.Equal(t, true, request.Body["stream"])
			require.Equal(t, 512, request.Body["max_tokens"])
			require.Equal(t, "disabled", request.Body["thinking"].(map[string]any)["type"])
		}
		return base(ctx, request)
	})
	require.Equal(t, 3, probes)
	require.Equal(t, 11, MaxRequests(options))
	require.Len(t, r.Samples, 11)
	require.Equal(t, "inconclusive", resultCheck(t, r, "performance_sampling").Status)
}
