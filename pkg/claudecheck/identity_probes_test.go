package claudecheck

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
	"model-check/common"
)

// Real signatures from a 2026-10-10 Bedrock request log (claude-sonnet-5 and
// claude-opus-5 behind a new-api relay).
const sonnet5Signature = "EocCCnkIEhABGAIqQG1991uFvpS7dJaUDE/mQ+SZKNl4EVh5p561bwV0pcEmyTaojS3RqSu649UNrJMCc6Wy+5DKMoq3LwfMfbeTRdsyDmNsYXVkZS1zYWZmcm9uOABCCHRoaW5raW5nWgw5NTY1MTExODMxMDmoAfOHqdYGEgyB6HnGKbxm+5P2niMaDK0kNPDDzO6zZ+8ctyIwwKJw6LmxhAN0h9ynA8sewfshBBRCR9VKgxBwyNpL/1CXfppmmaIe50UUZ0lAJVjJKjz14OswQ7zGrr8A5IPTOSZkoCLbBj6Fm4bt47jTkyknQo4rkwLI2kltdAFzp2h/iWvytOFpQHbUbtXS3EUYAQ=="
const opus5Signature = "CAISuAcKngUIEhABGAIqQM/WEbR5NL7PaN1IjRkaT5S/kKP3b12g5XbnSa5fHgakeJGZBnwNp9bxWVbmBW6+IRLmfsvSmqCJuqZ+EhRMnzsyDGNsYXVkZS1ob25leTgBQgh0aGlua2luZ1oMOTU2NTExMTgzMTA5qAHxh6nWBsIBwAMIogIQogEagAMF7ycDTzcFdF8EJq0F7ycDTzcFqEoEJq0F7ycDTzcFd4UEJq0F7ycDTzcF7hkEJq0F7ycDTzcFgMgEJq0F7ycDTzcFThkEJq0F7ycDTzcFqlwEJq0F7ycDTzcFf+4EJq0F7ycDTzcFQNgEJq0F7ycDTzcFl/4EJq0F7ycDTzcFL4kEJq0F7ycDTzcFbaMEJq0F7ycDTzcFEbwEJq0F7ycDTzcFyMMEJq0F7ycDTzcFpEgEJq0F7ycDTzcFtI8EJq0F7ycDTzcFoBIEJq0F7ycDTzcFc0kEJq0F7ycDTzcFb/8EJq0F7ycDTzcF51IEJq0F7ycDTzcFJ64EJq0F7ycDTzcF5VkEJq0F7ycDTzcF0cgEJq0F7ycDTzcFpHQEJq0F7ycDTzcFtZAEJq0F7ycDTzcFdo4EJq0F7ycDTzcFgkUEJq0F7ycDTzcFXXkEJq0F7ycDTzcF5loEJq0F7ycDTzcFP3QEJq0F7ycDTzcF8wIEJq0F7ycDTzcFSY4EJq0gAyoMA083BW84BCatBe8nMgRI0rlJOgRT6RGxQgQTW6UJSgQKiAEsUAZaBNkF4VZgkAFoAXAC2gFgChZwcmVmaXhfYmVmb3JlX2Jsb2NrX3YxEiBrXFspJ716edE3kEQ/R4dcVwDWmyGjT6HAnduMjiVLhCIAMiDCwBPIapyS4i+Wbz2GF3aPi3pEyQKtzAl7+h4JivtOH0gDEgzJ89UdBm8V3J+KT4kaDI9B9uvwmLK+Yet8hyIw+emRIWiNz/FyOzwV1NdKIFAqY9bBf/TKGL/9yw3Sa45RFk6pVBMB30ISxK0dFpnRKsYBnOPYTNEPQNQMv/sbipp/X2GyKzCsdqDFLd6J+etc08bMIE/+rKP68hEbGdt00x811RDsrEK4NH4Chkuh92pEOfFtpK8sDXQWCE5RJGUXHZ4KOQS9R6QMClJEGjLvjHFYiDGRj0XEj+dLJz1KRn42mXRmhyx080aTLg7Z4PSCqR+uVBz98+YHehUyHdTFSSsdpwbzPAHBPUZEjb2MLBBMfD+1zmjYcmiOe343x1ett1LnzJQ0zQgGHsUHOuP6ArdWLypVUL4MGAE="

func syntheticSignature(name string) string {
	data := append([]byte{0x12, 0x04, 'x', 'x', 'x', 'x', 0x32, byte(len(name))}, name...)
	return base64.StdEncoding.EncodeToString(append(data, 0x38, 0x01))
}

func TestSignatureModelDecodesPublicIDsAndCodenames(t *testing.T) {
	require.Equal(t, "claude-saffron", signatureModel(sonnet5Signature))
	require.Equal(t, "claude-honey", signatureModel(opus5Signature))
	require.Equal(t, "claude-opus-4-6", signatureModel(syntheticSignature("claude-opus-4-6")))
	require.Empty(t, signatureModel("bW9kZWwtY2hlY2staW52YWxpZC1zaWduYXR1cmU="))
	require.Empty(t, signatureModel("not base64 at all"))
	require.Empty(t, signatureModel(syntheticSignature("claude-Bad Name")))

	require.Equal(t, "sonnet-5", canonicalModel("claude-saffron"))
	require.Equal(t, "opus-5", canonicalModel("claude-honey"))
	require.Equal(t, "opus-4-8", canonicalModel("claude-quince"))
	require.Equal(t, "opus-4-6", canonicalModel("us.anthropic.claude-opus-4-6-v1:0"))
	require.Equal(t, "sonnet-5", canonicalModel("claude-sonnet-5-20261001"))
	require.Empty(t, canonicalModel("claude-unknown-codename"))
}

func signedSample(probe, signature string) Sample {
	return Sample{Probe: probe, SignatureModel: signatureModel(signature)}
}

func TestSignatureModelCheck(t *testing.T) {
	for _, tc := range []struct {
		name, model, code, status string
		samples                   []Sample
	}{
		{"match", "claude-sonnet-5", "signature_model_match", "pass", []Sample{signedSample("a", sonnet5Signature), signedSample("b", sonnet5Signature)}},
		{"substituted", "claude-opus-5", "signature_model_mismatch", "fail", []Sample{signedSample("a", opus5Signature), signedSample("b", sonnet5Signature)}},
		{"downgraded to public id", "claude-opus-4-8", "signature_model_mismatch", "fail", []Sample{signedSample("a", syntheticSignature("claude-opus-4-6"))}},
		{"unknown codename", "claude-opus-5", "signature_model_unknown", "inconclusive", []Sample{signedSample("a", syntheticSignature("claude-newname"))}},
		{"alias", "my-alias", "requested_model_unknown", "inconclusive", []Sample{signedSample("a", sonnet5Signature)}},
		{"none", "claude-opus-5", "no_signature", "inconclusive", []Sample{{Probe: "a"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &runner{ctx: context.Background(), report: Report{Model: tc.model, Samples: tc.samples}}
			r.signatureModelCheck()
			check := resultCheck(t, r.report, "signature_model")
			require.Equal(t, tc.status, check.Status)
			require.Equal(t, tc.code, check.Code)
		})
	}
}

func TestMessageSignatureModelReadsThinkingBlocksOnly(t *testing.T) {
	m := message{Content: []map[string]any{{"type": "text", "text": sonnet5Signature}, {"type": "thinking", "thinking": "", "signature": sonnet5Signature}}}
	require.Equal(t, "claude-saffron", messageSignatureModel(m))
	require.Empty(t, messageSignatureModel(message{Content: []map[string]any{{"type": "text", "text": sonnet5Signature}}}))
}

// Error texts copied from the same request log.
func TestNewBedrockValidationWording(t *testing.T) {
	beta := "InvokeModel: operation error Bedrock Runtime: InvokeModel, https response error StatusCode: 400, RequestID: x, ValidationException: Unexpected value(s) `model-check-unsupported-2099-01-01` for the `anthropic-beta` header. Please consult our documentation at ***.***.com/docs or try again without the header."
	require.Equal(t, "beta", bedrockValidationCode(beta))
	require.True(t, explicitBedrockRejection(beta))
	require.Equal(t, "beta", bedrockValidationCode("ValidationException: invalid beta flag"))
	require.Equal(t, "document", bedrockValidationCode("messages.0.content.0: document source is not supported"))
	require.Equal(t, "server_tools", bedrockValidationCode("ValidationException: tool type 'web_search_20250305' is not supported for this model"))
	require.True(t, bedrockToolRejection("ValidationException: tool type 'web_search_20250305' is not supported for this model", "web_search", "web_search_20250305"))
	require.True(t, bedrockSignatureRejection("ValidationException: ***.***.content.0: Invalid `signature` in `thinking` block"))

	r := &runner{ctx: context.Background(), report: Report{Model: "claude-opus-5"}, call: func(_ context.Context, request Request) (Response, error) {
		if request.Beta != bedrockInvalidBeta {
			return Response{Status: 200}, nil
		}
		body, _ := common.Marshal(map[string]any{"error": map[string]any{"type": "<nil>", "message": beta}})
		return Response{Status: 400, Body: body}, responseErrorForTest(body)
	}}
	r.bedrock()
	check := resultCheck(t, r.report, "bedrock_beta")
	require.Equal(t, "pass", check.Status)
}

func responseErrorForTest(body []byte) error {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = common.Unmarshal(body, &e)
	return errorString(e.Error.Message)
}

type errorString string

func (e errorString) Error() string { return string(e) }

func TestModelNotFoundStopsWithoutRetry(t *testing.T) {
	body := []byte(`{"error":{"message":"The model is currently unavailable, please try again later (request id: x)","type":"model_not_found"},"type":"error"}`)
	err := errorString("model_not_found: The model is currently unavailable, please try again later (request id: x)")
	require.Equal(t, "model_unavailable", FailureCode(503, err))
	require.Equal(t, "model_unavailable", FailureCode(404, err))
	require.False(t, baselineRecoverable("model_unavailable"))
	calls := 0
	r := Run(context.Background(), Options{Suite: "focused", Model: "claude-sonnet-5-5", Cache: true, Thinking: true}, func(context.Context, Request) (Response, error) {
		calls++
		return Response{Status: 503, Body: body}, err
	})
	require.Equal(t, 1, calls)
	require.Equal(t, "model_unavailable", r.StopReason)
}

func TestPrefillRule(t *testing.T) {
	rejection := errorString("ValidationException: This model does not support assistant message prefill. The conversation must end with a user message.")
	for _, tc := range []struct {
		name, model, status, code string
		response                  func(t *testing.T) (Response, error)
	}{
		{"rejected as expected", "claude-opus-5", "pass", "prefill_as_expected", func(*testing.T) (Response, error) { return Response{Status: 400}, rejection }},
		{"accepted by a newer model", "claude-sonnet-5", "fail", "prefill_accepted", nil},
		{"accepted by an older model", "claude-sonnet-4-5-20250929", "pass", "prefill_as_expected", nil},
		{"unknown generation", "claude-fable-5", "inconclusive", "prefill_rule_unknown", nil},
		{"unrelated error", "claude-opus-5", "inconclusive", "request_rejected", func(*testing.T) (Response, error) { return Response{Status: 400}, errorString("max_tokens: too large") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := happyTransport(t, false)
			r := &runner{ctx: context.Background(), report: Report{Model: tc.model}, call: func(ctx context.Context, request Request) (Response, error) {
				messages := request.Body["messages"].([]any)
				require.Equal(t, "assistant", messages[len(messages)-1].(map[string]any)["role"])
				if tc.response != nil {
					return tc.response(t)
				}
				return base(ctx, request)
			}}
			r.prefill()
			check := resultCheck(t, r.report, "prefill")
			require.Equal(t, tc.status, check.Status)
			if tc.code != "request_rejected" {
				require.Equal(t, tc.code, check.Code)
			}
		})
	}
}

const opus48StopEvent = `{"type":"message_stop","amazon-bedrock-invocationMetrics":{"inputTokenCount":101,"outputTokenCount":500,"invocationLatency":5457,"firstByteLatency":713,"cacheReadInputTokenCount":0,"cacheWriteInputTokenCount":2607}}`

func TestInvocationMetrics(t *testing.T) {
	metrics := streamBedrockMetrics([][]byte{[]byte(`{"type":"message_delta"}`), []byte(opus48StopEvent)})
	require.NotNil(t, metrics)
	require.Equal(t, int64(101), *metrics.Input)
	require.Equal(t, int64(2607), *metrics.CacheWrite)
	require.Nil(t, streamBedrockMetrics([][]byte{[]byte(`{"type":"message_stop"}`)}))

	n := func(v int64) *int64 { return &v }
	tier := "standard"
	sample := Sample{Probe: "performance_1", MessageID: "msg_bdrk_rz3", BedrockMetrics: metrics, Usage: Usage{Input: n(101), Output: n(500), CacheRead: n(0), CacheWrite: n(2607), ServiceTier: &tier}}
	r := &runner{ctx: context.Background(), report: Report{Samples: []Sample{sample}}}
	r.bedrockMetricsCheck()
	check := resultCheck(t, r.report, "invocation_metrics")
	require.Equal(t, "pass", check.Status)
	require.Equal(t, []string{"standard"}, check.Evidence["service_tiers"])
	require.Equal(t, []string{"msg_bdrk_"}, check.Evidence["message_id_prefixes"])

	sample.Usage.Output = n(900)
	r = &runner{ctx: context.Background(), report: Report{Samples: []Sample{sample}}}
	r.bedrockMetricsCheck()
	require.Equal(t, "metrics_mismatch", resultCheck(t, r.report, "invocation_metrics").Code)

	r = &runner{ctx: context.Background(), report: Report{Samples: []Sample{{Probe: "basic"}}}}
	r.bedrockMetricsCheck()
	require.Equal(t, "metrics_absent", resultCheck(t, r.report, "invocation_metrics").Code)
}

func TestUsageKeepsGenerationFields(t *testing.T) {
	events := [][]byte{
		[]byte(`{"type":"message_start","message":{"id":"msg_bdrk_x","type":"message","role":"assistant","model":"claude-opus-4-8","content":[],"usage":{"input_tokens":101,"output_tokens":2,"service_tier":"standard"}}}`),
		[]byte(`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`),
		[]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"` + sonnet5Signature + `"}}`),
		[]byte(`{"type":"content_block_stop","index":0}`),
		[]byte(`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`),
		[]byte(`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"21"}}`),
		[]byte(`{"type":"content_block_stop","index":1}`),
		[]byte(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":500,"output_tokens_details":{"thinking_tokens":306}}}`),
		[]byte(opus48StopEvent),
	}
	m, ok := parseStream(events)
	require.True(t, ok)
	require.Equal(t, "standard", *m.Usage.ServiceTier)
	require.Equal(t, int64(306), *m.Usage.OutputDetails.ThinkingTokens)
	require.Equal(t, "claude-saffron", messageSignatureModel(m))
}

func streamResponse(t *testing.T, events [][]byte) Response {
	t.Helper()
	return Response{Status: 200, Events: events, Model: "claude-sonnet-5"}
}

func TestFocusedThinkingAndSignatureForgery(t *testing.T) {
	thinking := [][]byte{
		[]byte(`{"type":"message_start","message":{"id":"msg_bdrk_x","type":"message","role":"assistant","model":"claude-sonnet-5","content":[],"usage":{"input_tokens":101,"output_tokens":2}}}`),
		[]byte(`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`),
		[]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"` + sonnet5Signature + `"}}`),
		[]byte(`{"type":"content_block_stop","index":0}`),
		[]byte(`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`),
		[]byte(`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Steps...\n21"}}`),
		[]byte(`{"type":"content_block_stop","index":1}`),
		[]byte(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":50,"output_tokens_details":{"thinking_tokens":30}}}`),
		[]byte(`{"type":"message_stop"}`),
	}
	for _, tc := range []struct {
		name, forgery string
		accepted      bool
	}{
		{"forgery rejected", "pass", false},
		{"forgery accepted", "fail", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &runner{ctx: context.Background(), report: Report{Model: "claude-sonnet-5"}, call: func(ctx context.Context, request Request) (Response, error) {
				if stream, _ := request.Body["stream"].(bool); stream {
					require.Equal(t, map[string]any{"type": "adaptive"}, request.Body["thinking"])
					require.Equal(t, map[string]any{"effort": "high"}, request.Body["output_config"])
					return streamResponse(t, thinking), nil
				}
				if !tc.accepted {
					return Response{Status: 400}, errorString("invalid_request_error: messages.1.content.0: Invalid `signature` in `thinking` block")
				}
				input, output := int64(10), int64(1)
				return testResponse(t, message{ID: "msg_x", Type: "message", Role: "assistant", Model: "claude-sonnet-5", StopReason: "end_turn",
					Content: []map[string]any{{"type": "text", "text": "PONG"}}, Usage: Usage{Input: &input, Output: &output}}), nil
			}}
			r.focusedThinking()
			check := resultCheck(t, r.report, "thinking")
			require.Equal(t, "pass", check.Status)
			require.Equal(t, int64(30), check.Evidence["thinking_tokens"])
			require.Equal(t, true, check.Evidence["answer_correct"])
			require.Equal(t, tc.forgery, resultCheck(t, r.report, "signature_forgery").Status)
			require.Equal(t, "claude-saffron", r.report.Samples[0].SignatureModel)
		})
	}
}
