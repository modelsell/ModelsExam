package claudecheck

import (
	"context"
	"net/http"
	"testing"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

func TestTokenLimitOfficialResponseSemantics(t *testing.T) {
	cases := []struct {
		name               string
		limit              int
		mutate             func(map[string]any)
		httpStatus         int
		effective          *int64
		transportErr       error
		state, code, issue string
	}{
		{name: "read_timeout_after_headers", limit: 1, transportErr: context.DeadlineExceeded, state: "inconclusive", code: "token_limit_unavailable"},
		{name: "one_token_empty_content", limit: 1, state: "pass", code: "token_limit_observed"},
		{name: "one_token_text", limit: 1, mutate: func(m map[string]any) { m["content"] = []any{map[string]any{"type": "text", "text": "1"}} }, state: "pass", code: "token_limit_observed"},
		{name: "one_token_exceeded", limit: 1, mutate: func(m map[string]any) { m["usage"].(map[string]any)["output_tokens"] = 2 }, state: "fail", code: "token_limit_exceeded"},
		{name: "missing_content", limit: 1, mutate: func(m map[string]any) { delete(m, "content") }, state: "fail", code: "token_limit_invalid_response", issue: "missing_content_array"},
		{name: "null_content", limit: 1, mutate: func(m map[string]any) { m["content"] = nil }, state: "fail", code: "token_limit_invalid_response", issue: "missing_content_array"},
		{name: "missing_usage", limit: 1, mutate: func(m map[string]any) { delete(m["usage"].(map[string]any), "output_tokens") }, state: "fail", code: "token_limit_invalid_response", issue: "missing_output_tokens"},
		{name: "wrong_role", limit: 1, mutate: func(m map[string]any) { m["role"] = "user" }, state: "fail", code: "token_limit_invalid_response", issue: "role_not_assistant"},
		{name: "negative_usage", limit: 1, mutate: func(m map[string]any) { m["usage"].(map[string]any)["output_tokens"] = -1 }, state: "fail", code: "token_limit_invalid_response", issue: "negative_output_tokens"},
		{name: "early_natural_end", limit: 1, mutate: func(m map[string]any) {
			m["stop_reason"] = "end_turn"
			m["usage"].(map[string]any)["output_tokens"] = 0
		}, state: "inconclusive", code: "token_limit_not_reached"},
		{name: "refusal", limit: 1, mutate: func(m map[string]any) { m["stop_reason"] = "refusal"; m["usage"].(map[string]any)["output_tokens"] = 0 }, state: "inconclusive", code: "request_refused"},
		{name: "zero_prewarm", limit: 0, state: "pass", code: "zero_output_observed"},
		{name: "zero_generated_output", limit: 0, mutate: func(m map[string]any) { m["usage"].(map[string]any)["output_tokens"] = 24 }, state: "fail", code: "token_limit_exceeded"},
		{name: "zero_unexpected_content", limit: 0, mutate: func(m map[string]any) {
			m["content"] = []any{map[string]any{"type": "text", "text": "PRIVATE_GENERATED_TEXT"}}
		}, state: "fail", code: "zero_output_has_content"},
		{name: "zero_rejected", limit: 0, httpStatus: 400, state: "inconclusive", code: "token_limit_unavailable"},
		{name: "zero_rate_limited", limit: 0, httpStatus: 429, state: "inconclusive", code: "token_limit_unavailable"},
		{name: "zero_override", limit: 0, effective: common.GetPointer(int64(128)), state: "inconclusive", code: "token_limit_overridden"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			call := happyTransport(t, true)
			report := Run(context.Background(), Options{Model: "claude-test"}, func(ctx context.Context, req Request) (Response, error) {
				if !req.Count && req.Body["max_tokens"] == tc.limit {
					require.Nil(t, req.Body["stream"])
					require.Nil(t, req.Body["thinking"])
					require.Nil(t, req.Body["tools"])
					m := map[string]any{"type": "message", "id": "msg_test", "role": "assistant", "model": "claude-test", "content": []any{}, "stop_reason": "max_tokens", "usage": map[string]any{"input_tokens": 10, "output_tokens": tc.limit}}
					if tc.mutate != nil {
						tc.mutate(m)
					}
					res := testResponse(t, m)
					if tc.httpStatus != 0 {
						res.Status = tc.httpStatus
					}
					res.EffectiveMaxTokens = tc.effective
					return res, tc.transportErr
				}
				return call(ctx, req)
			})
			id := "max_tokens"
			if tc.limit == 0 {
				id = "zero_output"
			}
			check := resultCheck(t, report, id)
			require.Equal(t, tc.state, check.Status)
			require.Equal(t, tc.code, check.Code)
			if tc.issue != "" {
				require.Contains(t, check.Evidence["validation_errors"], tc.issue)
			}
			for _, s := range report.Samples {
				if s.Probe == id {
					require.NotNil(t, s.RequestedMaxTokens)
					require.EqualValues(t, tc.limit, *s.RequestedMaxTokens)
				}
			}
			encoded, err := common.Marshal(report)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "PRIVATE_GENERATED_TEXT")
			require.Equal(t, 8, report.Version)
		})
	}
}

func TestTokenLimitEmptyExceptionDoesNotWeakenBaseline(t *testing.T) {
	report := Run(context.Background(), Options{Model: "claude-test"}, func(context.Context, Request) (Response, error) {
		return Response{Status: http.StatusOK, Body: []byte(`{"type":"message","id":"msg_test","role":"assistant","model":"claude-test","content":[],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":0}}`)}, nil
	})
	require.Len(t, report.Samples, 2)
	require.Equal(t, "fail", resultCheck(t, report, "basic").Status)
	require.Contains(t, report.Samples[0].ValidationErrors, "empty_content")
}
