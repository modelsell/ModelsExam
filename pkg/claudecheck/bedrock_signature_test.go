package claudecheck

import (
	"context"
	"fmt"
	"testing"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

func TestBedrockSignatureRejectionRequiresSpecificEvidence(t *testing.T) {
	for _, test := range []struct {
		message string
		matches bool
	}{
		{"Invalid `signature` in `thinking` block", true},
		{"messages.1.content.0: invalid signature in thinking block", true},
		{"thinking block signature verification failed", true},
		{"thinking.signature: signature is not valid", true},
		{"Unable to verify the thinking block signature", true},
		{"Invalid request: thinking signature=fixture", false},
		{"thinking.signature: budget_tokens must be at least 1024", false},
		{"thinking with signature is not supported", false},
		{"thinking signature accepted; invalid tool_result id", false},
		{"SignatureDoesNotMatch: AWS request signature is invalid", false},
	} {
		t.Run(test.message, func(t *testing.T) {
			require.Equal(t, test.matches, bedrockSignatureRejection(test.message))
		})
	}
}

func TestBedrockSignatureProbeClassification(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		code   string
	}{
		{"native rejection", 400, `{"__type":"ValidationException","message":"Invalid signature in thinking block"}`, "bedrock_expected_rejection"},
		{"proxy rejection", 400, `{"error":{"type":"invalid_request_error","message":"Invalid signature in thinking block"}}`, "bedrock_expected_rejection"},
		{"generic validation", 400, `{"message":"Invalid request: thinking signature=fixture"}`, "bedrock_rejection_unresolved"},
		{"budget validation", 400, `{"message":"thinking signature: budget_tokens must be at least 1024"}`, "bedrock_rejection_unresolved"},
		{"unsupported thinking", 400, `{"message":"thinking with signature is not supported"}`, "bedrock_rejection_unresolved"},
		{"AWS credentials", 400, `{"__type":"InvalidSignatureException","message":"AWS request signature is invalid"}`, "bedrock_rejection_unresolved"},
		{"permission", 403, `{"message":"Invalid signature in thinking block"}`, "bedrock_rejection_unresolved"},
		{"rate limit", 429, `{"message":"Invalid signature in thinking block"}`, "bedrock_rejection_unresolved"},
		{"upstream error", 500, `{"message":"Invalid signature in thinking block"}`, "bedrock_rejection_unresolved"},
		{"HTTP 200 error envelope", 200, `{"error":{"message":"Invalid signature in thinking block"}}`, "bedrock_rejection_unresolved"},
		{"accepted", 200, `{"type":"message","id":"msg_fixture","model":"claude-opus-5","role":"assistant","content":[{"type":"thinking","thinking":"private-response-thinking","signature":"private-response-signature"},{"type":"text","text":"PONG"}],"stop_reason":"end_turn","usage":{"input_tokens":20,"output_tokens":2}}`, "bedrock_boundary_difference"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var events []Event
			r := &runner{ctx: context.Background(), report: Report{Model: "claude-opus-5"}, observe: func(event Event) { events = append(events, event) },
				call: func(_ context.Context, request Request) (Response, error) {
					if request.Body["thinking"] != nil {
						return Response{Status: test.status, Body: []byte(test.body)}, nil
					}
					return Response{Status: 400, Body: []byte(`{"message":"unrelated validation"}`)}, nil
				}}
			r.bedrock()
			check := resultCheck(t, r.report, "bedrock_signature")
			require.Equal(t, test.code, check.Code)
			state := "inconclusive"
			if test.code == "bedrock_expected_rejection" {
				state = "pass"
			}
			require.Equal(t, state, check.Status)
			require.Equal(t, false, check.Evidence["identity_verified"])
			require.Equal(t, "thinking_signature", check.Evidence["expected_rejection"])
			require.Len(t, r.report.Samples, BedrockRequests)
			for _, value := range []any{r.report, events} {
				encoded, err := common.Marshal(value)
				require.NoError(t, err)
				require.NotContains(t, string(encoded), "private-response-")
				require.NotContains(t, string(encoded), "bW9kZWwtY2hlY2s")
			}
		})
	}
}

func TestBedrockSignatureProbeUsesCurrentToolTurnAndMappedThinkingMode(t *testing.T) {
	for _, test := range []struct{ model, mode string }{
		{"global.anthropic.claude-opus-5", "adaptive"},
		{"us.anthropic.claude-sonnet-4-5-20250929-v1:0", "enabled"},
	} {
		t.Run(test.model, func(t *testing.T) {
			r := &runner{report: Report{Model: "alias"}, upstreamModel: test.model}
			body := r.bedrockSignatureBody()
			require.Equal(t, test.mode, body["thinking"].(map[string]any)["type"])
			require.Equal(t, 2048, body["max_tokens"])
			require.Nil(t, body["tool_choice"])
			require.Nil(t, body["temperature"])
			messages := body["messages"].([]any)
			require.Len(t, messages, 3)
			assistant := messages[1].(map[string]any)
			require.Equal(t, "assistant", assistant["role"])
			blocks := assistant["content"].([]map[string]any)
			require.Equal(t, "thinking", blocks[0]["type"])
			require.NotEmpty(t, blocks[0]["signature"])
			require.Equal(t, "tool_use", blocks[1]["type"])
			result := messages[2].(map[string]any)
			require.Equal(t, "user", result["role"])
			require.Equal(t, blocks[1]["id"], result["content"].([]map[string]any)[0]["tool_use_id"])
			require.Contains(t, fmt.Sprint(body["tools"]), "record_probe")
		})
	}
}
