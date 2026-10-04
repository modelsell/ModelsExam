package claudecheck

import (
	"errors"
	"regexp"
	"strings"

	"model-check/common"
)

// Only allowlisted codes are persisted. Provider messages are untrusted and
// are never copied into this structured diagnosis, including SDK error fields.
type BedrockDiagnostic struct {
	Code           string `json:"code"`
	Exception      string `json:"exception,omitempty"`
	Source         string `json:"source"`
	Retryable      bool   `json:"retryable"`
	ExpectedStatus int    `json:"expected_status,omitempty"`
	OriginalStatus int    `json:"original_status,omitempty"`
}

type bedrockErrorRule struct {
	code   string
	status int
	retry  bool
}

var bedrockErrors = map[string]bedrockErrorRule{
	"AccessDeniedException":   {"access", 403, false},
	"NotAuthorized":           {"access", 400, false},
	"FTUFormNotFilled":        {"use_case", 404, false},
	"MPAgreementBeingCreated": {"marketplace", 403, true},
	"AWS Marketplace Agreement Pending after 15 minutes": {"marketplace", 403, true},
	"AWS Marketplace Agreement Failed within 15 minutes": {"marketplace", 403, false},
	"IncompleteSignature":                                {"credentials", 400, false},
	"InvalidSignatureException":                          {"credentials", 403, false},
	"InvalidClientTokenId":                               {"credentials", 403, false},
	"UnrecognizedClientException":                        {"credentials", 403, false},
	"ExpiredTokenException":                              {"credentials", 403, false},
	"RequestExpired":                                     {"clock", 400, false},
	"ValidationException":                                {"validation", 400, false},
	"ValidationError":                                    {"validation", 400, false},
	"ResourceNotFoundException":                          {"model_region", 404, false},
	"ResourceNotFound":                                   {"model_region", 404, false},
	"ServiceQuotaExceededException":                      {"quota", 400, false},
	"ThrottlingException":                                {"throttling", 429, true},
	"ModelNotReadyException":                             {"model_not_ready", 429, true},
	"ModelTimeoutException":                              {"timeout", 408, true},
	"ModelErrorException":                                {"model_error", 424, true},
	"ModelStreamErrorException":                          {"model_error", 424, true},
	"InternalServerException":                            {"service", 500, true},
	"InternalFailure":                                    {"service", 500, true},
	"ServiceUnavailableException":                        {"service", 503, true},
	"ServiceUnavailable":                                 {"service", 503, true},
	"overloaded_error":                                   {"service", 529, true},
}
var bedrockExceptionPattern = regexp.MustCompile(`\b(?:AccessDeniedException|NotAuthorized|FTUFormNotFilled|MPAgreementBeingCreated|IncompleteSignature|InvalidSignatureException|InvalidClientTokenId|UnrecognizedClientException|ExpiredTokenException|RequestExpired|ValidationException|ValidationError|ResourceNotFoundException|ResourceNotFound|ServiceQuotaExceededException|ThrottlingException|ModelNotReadyException|ModelTimeoutException|ModelErrorException|ModelStreamErrorException|InternalServerException|InternalFailure|ServiceUnavailableException|ServiceUnavailable|overloaded_error)\b`)

func normalizedException(value string) string {
	if i := strings.LastIndex(value, "#"); i >= 0 {
		value = value[i+1:]
	}
	return strings.SplitN(value, ":", 2)[0]
}

// DiagnoseBedrock recognizes native AWS exceptions, wrapped gateway errors and
// SSE error frames. HTTP status alone never establishes an AWS origin.
func DiagnoseBedrock(response Response, err error) *BedrockDiagnostic {
	exception, source, message := "", "", ""
	original := 0
	var apiErr interface {
		ErrorCode() string
		ErrorMessage() string
	}
	if errors.As(err, &apiErr) {
		exception, message, source = normalizedException(apiErr.ErrorCode()), apiErr.ErrorMessage(), "sdk"
	}
	if exception == "" {
		exception = normalizedException(response.Header.Get("x-amzn-errortype"))
		if exception != "" {
			source = "header"
		}
	}
	// Inspect error envelopes only; normal generated text must not classify a
	// successful response as a provider error or become authenticity evidence.
	bodies := [][]byte{response.Body}
	errorEnvelope := false
	bodies = append(bodies, response.Events...)
	for _, body := range bodies {
		var envelope struct {
			Type     string `json:"type"`
			AWS      string `json:"__type"`
			Code     string `json:"code"`
			Message  string `json:"message"`
			Original int    `json:"originalStatusCode"`
			Error    struct {
				Type    string `json:"type"`
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if common.Unmarshal(body, &envelope) != nil {
			continue
		}
		if response.Status == 200 && envelope.Type != "error" {
			continue
		}
		errorEnvelope = true
		if envelope.Error.Message != "" {
			message = envelope.Error.Message
		} else if envelope.Message != "" {
			message = envelope.Message
		}
		if envelope.Original >= 400 && envelope.Original <= 599 {
			original = envelope.Original
		}
		if exception == "" {
			for _, code := range []string{envelope.AWS, envelope.Error.Type, envelope.Error.Code, envelope.Code, envelope.Type} {
				if _, ok := bedrockErrors[normalizedException(code)]; ok {
					exception, source = normalizedException(code), "envelope"
					break
				}
			}
		}
	}
	if err != nil && message == "" {
		message = err.Error()
	}
	contextual := response.Bedrock || strings.Contains(strings.ToLower(message), "bedrock") || strings.Contains(message, "InvokeModel")
	if exception == "" && contextual {
		exception = bedrockExceptionPattern.FindString(message)
		source = "message"
	}
	rule, known := bedrockErrors[exception]
	// Anthropic uses overloaded_error too; do not attribute it to AWS without
	// context. Most AWS exception names can also be reproduced by a gateway.
	if exception == "overloaded_error" && !contextual {
		return nil
	}
	if !known && !contextual {
		return nil
	}
	if !known && response.Status < 300 && !errorEnvelope {
		return nil
	}
	code := rule.code
	if code == "" {
		code = "unknown"
	}
	if code == "validation" || code == "unknown" {
		if specific := bedrockValidationCode(message); specific != "" {
			code = specific
		}
	}
	if !known {
		exception, source = "", "message"
	}
	return &BedrockDiagnostic{Code: code, Exception: exception, Source: source, Retryable: rule.retry, ExpectedStatus: rule.status, OriginalStatus: original}
}

func bedrockValidationCode(message string) string {
	text := strings.ToLower(message)
	switch {
	case strings.Contains(text, "on-demand throughput") && strings.Contains(text, "inference profile"):
		return "inference_profile"
	case strings.Contains(text, "invalid beta") || (strings.Contains(text, "beta") && strings.Contains(text, "not supported")):
		return "beta"
	case strings.Contains(text, "temperature") || strings.Contains(text, "top_p") || strings.Contains(text, "top_k"):
		return "sampling"
	case strings.Contains(text, "thinking") && strings.Contains(text, "signature"):
		return "thinking_signature"
	case strings.Contains(text, "thinking") || strings.Contains(text, "budget_tokens"):
		return "thinking"
	case strings.Contains(text, "web_search") || strings.Contains(text, "web_fetch") || strings.Contains(text, "code_execution") || strings.Contains(text, "advisor") || strings.Contains(text, "server-side tool"):
		return "server_tools"
	case strings.Contains(text, "source") && (strings.Contains(text, "url") || strings.Contains(text, "file_id")):
		return "input_source"
	case strings.Contains(text, "role") || strings.Contains(text, "system message"):
		return "message_role"
	case strings.Contains(text, "counttokens") || strings.Contains(text, "count_tokens") || strings.Contains(text, "count tokens"):
		return "count_tokens"
	case strings.Contains(text, "pdf") || strings.Contains(text, "document"):
		return "document"
	case strings.Contains(text, "image"):
		return "image"
	case strings.Contains(text, "maximum context") || strings.Contains(text, "too many tokens") || strings.Contains(text, "input is too long") || strings.Contains(text, "max_tokens"):
		return "token_limit"
	case strings.Contains(text, "model identifier") || strings.Contains(text, "model id") || strings.Contains(text, "region"):
		return "model_region"
	}
	return ""
}

func (d *BedrockDiagnostic) failureCode() string {
	switch d.Code {
	case "access", "credentials", "clock", "use_case", "marketplace":
		return "access_denied"
	case "quota", "throttling":
		return "rate_limited"
	case "model_not_ready", "service":
		return "upstream_unavailable"
	case "timeout":
		return "probe_timeout"
	case "model_region", "inference_profile":
		return "route_unavailable"
	case "model_error":
		if d.OriginalStatus >= 400 && d.OriginalStatus < 500 && d.OriginalStatus != 408 && d.OriginalStatus != 429 {
			return "request_rejected"
		}
		return "upstream_unavailable"
	case "unknown":
		return ""
	default:
		return "request_rejected"
	}
}
