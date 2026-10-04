package claudecheck

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// Documented in https://platform.claude.com/docs/en/api/errors.
func TestOfficialErrorTypeFollowsStatusTable(t *testing.T) {
	for status, kind := range map[int]string{
		400: "invalid_request_error", 401: "authentication_error", 402: "billing_error",
		403: "permission_error", 404: "not_found_error", 409: "conflict_error",
		413: "request_too_large", 429: "rate_limit_error", 500: "api_error",
		504: "timeout_error", 529: "overloaded_error",
	} {
		require.True(t, officialErrorType(status, kind), "%d %s", status, kind)
		require.False(t, officialErrorType(status, "wrong_error"), "%d", status)
	}
	// Other 4xx statuses use invalid_request_error; other 5xx use api_error.
	require.True(t, officialErrorType(422, "invalid_request_error"))
	require.False(t, officialErrorType(422, "api_error"))
	require.True(t, officialErrorType(503, "api_error"))
	require.False(t, officialErrorType(200, "invalid_request_error"))
}

func errorShapeResult(t *testing.T, status int, body string, header http.Header) Check {
	r := runner{ctx: context.Background(), report: Report{}}
	r.errorShape(Response{Status: status, Body: []byte(body), Header: header})
	return resultCheck(t, r.report, "error_shape")
}

func TestErrorShapePassesOnlyForTheDocumentedEnvelope(t *testing.T) {
	official := `{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens: must be positive"},"request_id":"req_011CSHoEeqs5C35K2UUqR7Fy"}`
	check := errorShapeResult(t, 400, official, http.Header{"Request-Id": {"req_1"}})
	require.Equal(t, "pass", check.Status)
	require.Equal(t, "anthropic", check.Evidence["shape"])
	require.Equal(t, true, check.Evidence["type_matches_status"])
	require.Equal(t, true, check.Evidence["request_id_in_body"])
	require.Equal(t, true, check.Evidence["request_id_header"])

	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"no request_id":     {400, `{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}`},
		"type vs status":    {400, `{"type":"error","error":{"type":"api_error","message":"bad"},"request_id":"req_1"}`},
		"openai envelope":   {400, `{"error":{"message":"bad","type":"invalid_request_error","param":null,"code":null}}`},
		"missing top type":  {400, `{"error":{"type":"invalid_request_error","message":"bad"},"request_id":"req_1"}`},
		"plain message":     {400, `{"message":"bad"}`},
		"not json":          {400, `oops`},
		"wrong status type": {401, `{"type":"error","error":{"type":"invalid_request_error","message":"bad"},"request_id":"req_1"}`},
	} {
		got := errorShapeResult(t, tc.status, tc.body, nil)
		require.Equal(t, "inconclusive", got.Status, name)
		require.Equal(t, "error_envelope", got.Code, name)
	}
}

// Claude Opus 5.5, Sonnet 5.5, Fable 5.1 and Mythos 5.1 reject forced tool use.
func TestForcedToolChoiceIsOnlySentWhereSupported(t *testing.T) {
	for model, forced := range map[string]bool{
		"claude-opus-5": true, "claude-sonnet-5": true, "claude-opus-4-8": true, "claude-sonnet-4-5-20250929": true,
		"claude-haiku-4-5-20251001": true, "claude-mythos-5": true, "claude-fable-5": true,
		"claude-opus-5-5": false, "claude-sonnet-5-5": false, "claude-fable-5-1": false, "claude-mythos-5-1": false,
		"anthropic.claude-opus-5-5-v1:0": false, "claude-sonnet-5.5": false, "CLAUDE-FABLE-5-1": false,
	} {
		require.Equal(t, forced, supportsForcedToolChoice(model), model)
	}
}

func TestToolProbeUsesAutoChoiceOnModelsRejectingForcedUse(t *testing.T) {
	for model, want := range map[string]string{"claude-opus-5": "tool", "claude-sonnet-5-5": "auto"} {
		good := semanticTransport(t)
		var seen any
		r := Run(context.Background(), Options{Model: model}, func(ctx context.Context, req Request) (Response, error) {
			if req.Body["tools"] != nil && !req.Count {
				if choice, ok := req.Body["tool_choice"].(map[string]any); ok {
					seen = choice["type"]
				}
			}
			return good(ctx, req)
		})
		require.Equal(t, want, seen, model)
		require.Equal(t, want, resultCheck(t, r, "tool").Evidence["tool_choice"], model)
	}
}
