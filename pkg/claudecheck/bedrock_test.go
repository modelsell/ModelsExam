package claudecheck

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

func TestBedrockErrorTaxonomy(t *testing.T) {
	for exception, rule := range bedrockErrors {
		t.Run(exception, func(t *testing.T) {
			body, _ := common.Marshal(map[string]any{"__type": "com.amazonaws.bedrock#" + exception, "message": "diagnostic fixture"})
			d := DiagnoseBedrock(Response{Bedrock: true, Status: rule.status, Body: body}, nil)
			require.NotNil(t, d)
			require.Equal(t, rule.code, d.Code)
			require.Equal(t, exception, d.Exception)
			require.Equal(t, rule.status, d.ExpectedStatus)
			require.Equal(t, rule.retry, d.Retryable)
		})
	}
}

func TestBedrockWrappedErrorsAndStreaming(t *testing.T) {
	cases := []struct {
		name     string
		response Response
		err      error
		code     string
	}{
		{"wrapped validation", Response{Status: 400, Body: []byte(`{"error":{"type":"invalid_request_error","message":"InvokeModel: Bedrock Runtime ValidationException: invalid beta flag"}}`)}, nil, "beta"},
		{"profile", Response{Status: 400}, errors.New("Bedrock ValidationException: Invocation with on-demand throughput isn't supported. Retry with an inference profile"), "inference_profile"},
		{"SSE after 200", Response{Status: 200, Events: [][]byte{[]byte(`{"type":"message_start"}`), []byte(`{"type":"error","error":{"type":"ThrottlingException","message":"limit"}}`)}}, nil, "throttling"},
		{"HTTP200 error envelope", Response{Bedrock: true, Status: 200, Body: []byte(`{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`)}, nil, "service"},
		{"header", Response{Status: 400, Header: http.Header{"X-Amzn-Errortype": []string{"ServiceQuotaExceededException:http"}}}, nil, "quota"},
		{"model error", Response{Status: 424, Body: []byte(`{"__type":"ModelErrorException","originalStatusCode":503,"message":"internal"}`)}, nil, "model_error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := DiagnoseBedrock(c.response, c.err)
			require.NotNil(t, d)
			require.Equal(t, c.code, d.Code)
			if c.name == "model error" {
				require.Equal(t, 503, d.OriginalStatus)
			}
		})
	}
}

func TestBedrockNoInventedAttributionOrSecretPersistence(t *testing.T) {
	for _, response := range []Response{
		{Status: 403, Body: []byte(`<html>Forbidden</html>`)},
		{Status: 400, Body: []byte(`{"error":{"type":"invalid_request_error","message":"invalid role"}}`)},
		{Status: 529, Body: []byte(`{"error":{"type":"overloaded_error","message":"busy"}}`)},
		{Bedrock: true, Status: 200, Body: []byte(`{"type":"message","content":[{"type":"text","text":"ValidationException: Bedrock invalid beta flag"}]}`)},
	} {
		require.Nil(t, DiagnoseBedrock(response, nil))
	}
	require.Nil(t, DiagnoseBedrock(Response{Bedrock: true}, context.DeadlineExceeded))
	d := DiagnoseBedrock(Response{Status: 400, Body: []byte(`{"__type":"ValidationException","message":"invalid beta flag secret-sk-fixture"}`)}, nil)
	encoded, err := common.Marshal(d)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "secret-sk-fixture")
}

func TestBedrockDiagnosticsBudgetAndExpectedRejections(t *testing.T) {
	base := focusedTransport(t)
	calls := 0
	r := Run(context.Background(), Options{Suite: "focused", Model: "claude-opus-5", Bedrock: true}, func(ctx context.Context, request Request) (Response, error) {
		calls++
		if request.Body["system"] == capabilitySystem {
			return base(ctx, request)
		}
		if request.Body["thinking"] != nil {
			return base(ctx, request)
		}
		reason := ""
		if request.Beta == bedrockInvalidBeta {
			reason = "invalid beta flag"
		}
		if request.Body["temperature"] != nil {
			reason = "temperature is not supported"
		}
		if definitions, ok := request.Body["tools"].([]any); ok {
			require.Len(t, definitions, 1)
			tool := definitions[0].(map[string]any)
			reason = fmt.Sprintf("tools: %s is unsupported", tool["type"])
			if tool["name"] == "advisor" {
				require.Equal(t, bedrockAdvisorBeta, request.Beta)
			}
		}
		body, _ := common.Marshal(request.Body)
		if strings.Contains(string(body), "model_check_invalid_role") {
			reason = "messages: invalid role"
		}
		if reason != "" {
			require.Equal(t, 32, request.Body["max_tokens"])
			payload, _ := common.Marshal(map[string]any{"__type": "ValidationException", "message": reason})
			return Response{Status: 400, Body: payload}, nil
		}
		return base(ctx, request)
	})
	require.Equal(t, 23, calls)
	require.Equal(t, 23, r.Limits.MaxRequests)
	require.True(t, r.Options.Bedrock)
	require.Empty(t, r.StopReason)
	for _, c := range r.Checks {
		if isBedrockProbe(c.ID) {
			require.Equal(t, "pass", c.Status)
			require.Equal(t, "bedrock_expected_rejection", c.Code)
		}
	}
}

func TestBedrockProbeDoesNotScoreUnrelatedRejections(t *testing.T) {
	for _, status := range []int{400, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			r := &runner{ctx: context.Background(), report: Report{Model: "claude-opus-5"}, call: func(context.Context, Request) (Response, error) {
				return Response{Status: status, Body: []byte(`{"error":{"message":"unrelated rejection"}}`)}, nil
			}}
			r.bedrock()
			for _, c := range r.report.Checks {
				require.Equal(t, "inconclusive", c.Status)
			}
			if status == 403 || status == 429 {
				require.Len(t, r.report.Samples, 1)
			}
		})
	}
}

func TestBedrockSamplingUnknownModelsSkip(t *testing.T) {
	for _, model := range []string{"alias-opus", "claude-opus-99", "claude-opus-4-6", "claude-sonnet-4"} {
		require.Empty(t, bedrockSamplingRule(model))
	}
	require.Equal(t, "exclusive", bedrockSamplingRule("us.anthropic.claude-sonnet-4-5-20250929-v1:0"))
	require.Equal(t, "fixed", bedrockSamplingRule("global.anthropic.claude-opus-5"))
	base := focusedTransport(t)
	r := Run(context.Background(), Options{Suite: "focused", Model: "claude-opus-4-6", Bedrock: true}, func(ctx context.Context, request Request) (Response, error) {
		response, err := base(ctx, request)
		response.Model = "claude-opus-4-6"
		return response, err
	})
	require.Len(t, r.Samples, 22)
	for _, c := range r.Checks {
		if c.ID == "bedrock_sampling" {
			require.Equal(t, "bedrock_rule_unknown", c.Code)
		}
	}
}

func TestBedrockStreamErrorIsNotAValidSample(t *testing.T) {
	r := Run(context.Background(), Options{Suite: "focused", Model: "claude-opus-5"}, func(context.Context, Request) (Response, error) {
		return Response{Status: 200, Body: []byte(`{"type":"error","error":{"type":"ThrottlingException","message":"limit"}}`)}, nil
	})
	require.Len(t, r.Samples, 1)
	require.False(t, *r.Samples[0].Valid)
	require.Equal(t, "rate_limited", r.StopReason)
}

func TestBedrockEchoedErrorParametersAreNotEvidence(t *testing.T) {
	r := &runner{ctx: context.Background(), report: Report{Model: "claude-opus-5"}, call: func(context.Context, Request) (Response, error) {
		return Response{Status: 400, Body: []byte(`{"message":"Request rejected. Submitted role=user temperature=0.5"}`)}, nil
	}}
	r.bedrock()
	for _, check := range r.report.Checks {
		require.Equal(t, "inconclusive", check.Status)
	}
}

func TestBedrockUnknown503KeepsAvailabilityRecovery(t *testing.T) {
	calls := 0
	base := focusedTransport(t)
	r := Run(context.Background(), Options{Suite: "focused", Model: "claude-opus-5"}, func(ctx context.Context, request Request) (Response, error) {
		calls++
		if calls == 1 {
			return Response{Bedrock: true, Status: 503, Body: []byte(`{"message":"busy"}`)}, nil
		}
		return base(ctx, request)
	})
	require.Equal(t, 15, calls)
	require.Equal(t, "upstream_unavailable", r.Samples[0].ErrorCode)
	require.Empty(t, r.StopReason)
}

func TestBedrockUnknownExceptionDoesNotPersistUntrustedHeader(t *testing.T) {
	d := DiagnoseBedrock(Response{Bedrock: true, Status: 400, Header: http.Header{"X-Amzn-Errortype": []string{"untrusted-private-value"}}, Body: []byte(`{"message":"request rejected"}`)}, nil)
	require.NotNil(t, d)
	require.Empty(t, d.Exception)
}

func TestBedrockGenericInvalidRequestDoesNotConfirmBoundary(t *testing.T) {
	for _, message := range []string{"Invalid request: role=model_check_invalid_role", "invalid_request_error: temperature=0.5", "Submitted model_check_invalid_role"} {
		require.False(t, explicitBedrockRejection(message))
	}
	require.True(t, explicitBedrockRejection("invalid beta flag"))
	require.True(t, explicitBedrockRejection("role: Input tag does not match any of the expected tags"))
}

func TestBedrockToolDefinitionsAndAcceptance(t *testing.T) {
	seen := map[string]bool{}
	r := &runner{ctx: context.Background(), report: Report{Model: "claude-opus-5"}, call: func(_ context.Context, request Request) (Response, error) {
		if definitions, ok := request.Body["tools"].([]any); ok && request.Body["thinking"] == nil {
			require.Len(t, definitions, 1)
			tool := definitions[0].(map[string]any)
			name := tool["name"].(string)
			require.False(t, seen[name], "each tool gets exactly one request")
			seen[name] = true
			require.Nil(t, tool["input_schema"], "must test a server tool, not a same-named custom tool")
			require.Contains(t, fmt.Sprint(request.Body["messages"]), "Do not call any tools")
			switch name {
			case "web_search":
				require.Equal(t, "web_search_20250305", tool["type"])
				require.Equal(t, 1, tool["max_uses"])
			case "web_fetch":
				require.Equal(t, "web_fetch_20250910", tool["type"])
				require.Equal(t, 1, tool["max_uses"])
			case "code_execution":
				require.Equal(t, "code_execution_20250825", tool["type"])
				require.Nil(t, tool["max_uses"])
			case "advisor":
				require.Equal(t, "advisor_20260301", tool["type"])
				require.Equal(t, bedrockAdvisorBeta, request.Beta)
				require.Equal(t, "claude-opus-5", tool["model"])
				require.Equal(t, 1024, tool["max_tokens"])
				require.Equal(t, 1, tool["max_uses"])
			default:
				t.Fatalf("unexpected tool %s", name)
			}
			if name != "advisor" {
				require.Empty(t, request.Beta)
			}
		}
		return Response{Status: 200, Body: []byte(`{"type":"message","id":"msg_fixture","model":"claude-opus-5","role":"assistant","content":[{"type":"text","text":"PONG"}],"stop_reason":"end_turn","usage":{"input_tokens":20,"output_tokens":2}}`)}, nil
	}}
	r.bedrock()
	require.Len(t, seen, 4)
	require.Len(t, r.report.Samples, BedrockRequests)
	require.Len(t, bedrockPlan(), BedrockRequests)
	for _, check := range r.report.Checks {
		require.Equal(t, "inconclusive", check.Status)
		require.Equal(t, "bedrock_boundary_difference", check.Code)
		if check.Evidence["tool_name"] != nil {
			require.Equal(t, "tool_declaration", check.Evidence["scope"])
			require.Equal(t, false, check.Evidence["identity_verified"])
		}
	}
}

func TestBedrockToolRejectionMatchesOnlyTheTestedBoundary(t *testing.T) {
	for _, tool := range bedrockToolProbes {
		t.Run(tool.name, func(t *testing.T) {
			for _, message := range []string{
				tool.version + " is unsupported",
				"Tool type '" + tool.version + "' is not supported on Amazon Bedrock",
				"Invalid tool type: " + tool.version,
				"Bedrock does not support " + tool.name,
				"tools.0: Input tag '" + tool.version + "' found using 'type' does not match any of the expected tags: 'custom', 'bash_20250124'",
			} {
				require.True(t, bedrockToolRejection(message, tool.name, tool.version), message)
			}
			for _, message := range []string{
				tool.name + ": max_uses must be a positive integer",
				tool.name + ": input_schema is required",
				"Invalid request: tools=" + tool.version,
				tool.name + " is not enabled for your organization",
				tool.name + " is not available: missing permission",
				"Invalid beta flag: advisor-tool-2026-03-01",
				"advisor model is unsupported",
			} {
				require.False(t, bedrockToolRejection(message, tool.name, tool.version), message)
			}
			for _, other := range bedrockToolProbes {
				if other.name != tool.name {
					require.False(t, bedrockToolRejection(other.version+" is unsupported", tool.name, tool.version))
				}
			}
		})
	}
}

func TestBedrockToolUnrelatedValidationRemainsUnscored(t *testing.T) {
	for _, message := range []string{"tools: web_search_20250305 is unsupported", "Invalid beta flag: advisor-tool-2026-03-01", "advisor model is unsupported", "web_fetch: max_uses must be positive"} {
		r := &runner{ctx: context.Background(), report: Report{Model: "claude-opus-5"}, call: func(context.Context, Request) (Response, error) {
			body, _ := common.Marshal(map[string]any{"__type": "ValidationException", "message": message})
			return Response{Status: 400, Body: body}, nil
		}}
		r.bedrock()
		for _, check := range r.report.Checks {
			if check.ID == "bedrock_web_fetch" || check.ID == "bedrock_code_execution" || check.ID == "bedrock_advisor" {
				require.Equal(t, "inconclusive", check.Status, "%s: %s", check.ID, message)
			}
		}
	}
}
