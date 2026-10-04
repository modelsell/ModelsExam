package claudecheck

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func focusedTransport(t *testing.T) Transport {
	base := happyTransport(t, true)
	collection := collectionTransport(t, base)
	return func(ctx context.Context, request Request) (Response, error) {
		if request.Body["thinking"] != nil && len(request.Body["messages"].([]any)) == 3 && request.Body["system"] == nil {
			return Response{Status: 400, Body: []byte(`{"__type":"ValidationException","message":"Invalid signature in thinking block"}`)}, nil
		}
		if request.Body["system"] == usageTokensSystem {
			return usageTokenHappyResponse(t, request), nil
		}
		if stream, _ := request.Body["stream"].(bool); stream {
			return base(ctx, request)
		}
		return collection(ctx, request)
	}
}

func TestFocusedScopeBudgetAndBaselineCompatibility(t *testing.T) {
	options := Options{Suite: "focused", Model: "claude-opus-5", Cache: true, Fingerprint: true, Bedrock: true, Thinking: true, Repeat: true, StreamComparison: true}
	r := Run(context.Background(), options, focusedTransport(t))
	require.Equal(t, 20, r.Version)
	require.Len(t, r.Samples, 31)
	require.Equal(t, 31, r.Limits.MaxRequests)
	require.Equal(t, float64(25), *r.Options.PerformanceTolerance)
	require.False(t, r.Options.Thinking)
	require.False(t, r.Options.Repeat)
	require.False(t, r.Options.StreamComparison)
	require.False(t, r.Options.Fingerprint)
	require.True(t, r.Options.Bedrock)
	require.Len(t, r.Benchmark.Items, 6)
	require.Nil(t, r.Fingerprint)
	require.Len(t, r.TokenAudit.Cache, 6)
	for _, id := range []string{"token_count", "zero_output", "aws_usage", "source", "billing", "system", "stop_sequence", "repeatability", "thinking", "prompt_integrity"} {
		for _, check := range r.Checks {
			require.NotEqual(t, id, check.ID)
		}
	}
	snapshot := SnapshotBaseline(r, "reference", "aws", 1)
	require.Nil(t, snapshot.Fingerprint)
	options.Fingerprint, options.Bedrock = false, false
	require.Len(t, Run(context.Background(), options, focusedTransport(t)).Samples, 23)
	options.Fingerprint, options.PromptAudit, options.PDF, options.Vision = true, true, true, true
	require.Equal(t, 28, MaxRequests(options))
	options.Bedrock = true
	require.Equal(t, 36, MaxRequests(options))
}

func TestFocusedRecoveryReusesPlannedPerformanceRequest(t *testing.T) {
	base := focusedTransport(t)
	calls := 0
	r := Run(context.Background(), Options{Suite: "focused", Model: "claude-opus-5"}, func(ctx context.Context, request Request) (Response, error) {
		calls++
		if calls == 1 {
			return Response{Status: 504}, fmt.Errorf("timeout")
		}
		return base(ctx, request)
	})
	require.Equal(t, 15, calls)
	require.Equal(t, "performance_1", r.BaselineProbe)
	require.Empty(t, r.StopReason)
}

func TestFocusedStopsOnMidRunAccessAndRateLimits(t *testing.T) {
	for _, status := range []int{401, 429} {
		calls := 0
		base := focusedTransport(t)
		r := Run(context.Background(), Options{Suite: "focused", Model: "claude-opus-5", Cache: true, Fingerprint: true}, func(ctx context.Context, request Request) (Response, error) {
			calls++
			if calls == 3 {
				return Response{Status: status}, fmt.Errorf("rejected")
			}
			return base(ctx, request)
		})
		require.Equal(t, 3, calls)
		require.NotEmpty(t, r.StopReason)
		require.Equal(t, "run_stopped", resultCheck(t, r, "capability_benchmark").Code)
	}
}

func TestFocusedPromptUsesThreeRequestsWithoutCountEndpoint(t *testing.T) {
	base := focusedTransport(t)
	counts := 0
	r := Run(context.Background(), Options{Suite: "focused", Model: "claude-opus-5", PromptAudit: true}, func(ctx context.Context, request Request) (Response, error) {
		if request.Count {
			counts++
			return Response{Status: 404}, fmt.Errorf("not found")
		}
		if request.Body["thinking"] != nil && request.Body["system"] != fingerprintSystem && request.Body["system"] != capabilitySystem && request.Body["system"] != usageTokensSystem {
			input, output := int64(10), int64(1)
			response := testResponse(t, message{ID: "msg_test", Type: "message", Role: "assistant", Model: "claude-opus-5", StopReason: "end_turn", Content: []map[string]any{{"type": "text", "text": "PONG"}}, Usage: Usage{Input: &input, Output: &output}})
			response.RequestProfile = bodyProfile(request.Body)
			response.PromptProfile = promptContentProfile(request.Body)
			return response, nil
		}
		return base(ctx, request)
	})
	require.Zero(t, counts)
	require.Zero(t, r.TokenAudit.PromptAssessment.BehaviorMeasured)
	require.Equal(t, 100, r.TokenAudit.PromptAssessment.Coverage)
	require.Equal(t, 100, *r.TokenAudit.PromptAssessment.Score)
	require.Equal(t, "budget_no_obvious_injection", r.TokenAudit.Injection.Code)
	require.Len(t, r.Samples, 18)
	require.Empty(t, r.StopReason)
	for _, item := range r.TokenAudit.Prompt {
		require.Equal(t, "budget_within_allowance", item.Code)
		require.Equal(t, 100, *item.Score)
		require.Nil(t, item.Behavior)
	}
}

func TestFocusedToleranceValidation(t *testing.T) {
	zero := float64(0)
	r := Run(context.Background(), Options{Suite: "focused", Model: "claude-opus-5", PerformanceTolerance: &zero}, focusedTransport(t))
	require.Zero(t, *r.Options.PerformanceTolerance)
	for _, v := range []float64{-1, 101} {
		require.False(t, ValidOptions(Options{Suite: "focused", PerformanceTolerance: &v}))
	}
	require.False(t, ValidOptions(Options{Suite: "unknown"}))
}

func TestFocusedPromptRateLimitStopsRemainingSamples(t *testing.T) {
	base := focusedTransport(t)
	calls := 0
	r := Run(context.Background(), Options{Suite: "focused", Model: "claude-opus-5", PromptAudit: true}, func(ctx context.Context, request Request) (Response, error) {
		calls++
		if calls == 16 {
			return Response{Status: 429}, fmt.Errorf("rate limited")
		}
		if request.Body["thinking"] != nil && request.Body["system"] != fingerprintSystem && request.Body["system"] != capabilitySystem && request.Body["system"] != usageTokensSystem {
			input, output := int64(10), int64(1)
			response := testResponse(t, message{ID: "msg_test", Type: "message", Role: "assistant", Model: "claude-opus-5", StopReason: "end_turn", Content: []map[string]any{{"type": "text", "text": "PONG"}}, Usage: Usage{Input: &input, Output: &output}})
			response.RequestProfile = bodyProfile(request.Body)
			return response, nil
		}
		return base(ctx, request)
	})
	require.Equal(t, 16, calls)
	require.Equal(t, "rate_limited", r.StopReason)
	require.Equal(t, "not_collected", r.TokenAudit.Prompt[1].Code)
}
