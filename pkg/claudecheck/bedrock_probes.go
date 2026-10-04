package claudecheck

import (
	"regexp"
	"strconv"
	"strings"

	"model-check/common"
)

const BedrockRequests = 8
const bedrockInvalidBeta = "model-check-unsupported-2099-01-01"
const bedrockAdvisorBeta = "advisor-tool-2026-03-01"

// Use the simplest documented tool versions to isolate platform availability
// from newer versions' dynamic filtering and other model-specific features.
var bedrockToolProbes = []struct{ id, name, version string }{
	{"bedrock_web_search", "web_search", "web_search_20250305"},
	{"bedrock_web_fetch", "web_fetch", "web_fetch_20250910"},
	{"bedrock_code_execution", "code_execution", "code_execution_20250825"},
	{"bedrock_advisor", "advisor", "advisor_20260301"},
}

func bedrockPlan() []PlanItem {
	plan := []PlanItem{{"bedrock_role", "bedrock", "observation", true}, {"bedrock_beta", "bedrock", "observation", true}}
	for _, tool := range bedrockToolProbes {
		plan = append(plan, PlanItem{tool.id, "bedrock", "observation", true})
	}
	return append(plan, PlanItem{"bedrock_sampling", "bedrock", "observation", true}, PlanItem{"bedrock_signature", "bedrock", "observation", true})
}

// Only listed, documented versions are tested; future/opaque aliases are not
// assigned an inferred capability based on a lexical model-name comparison.
func bedrockSamplingRule(model string) string {
	m := parseDeclaredModel(model)
	if !m.known {
		return ""
	}
	if (m.name == "sonnet" || m.name == "haiku") && m.major == "4" && m.minor == "5" {
		return "exclusive"
	}
	minor, _ := strconv.Atoi(m.minor)
	if (m.name == "opus" && ((m.major == "4" && (minor == 7 || minor == 8)) || (m.major == "5" && m.minor == ""))) ||
		(m.name == "sonnet" && m.major == "5" && m.minor == "") || (m.name == "fable" && m.major == "5" && (m.minor == "" || m.minor == "1")) {
		return "fixed"
	}
	return ""
}

// bedrock collects optional platform-boundary observations. These probes do
// not contribute to the five-dimension report score or certify model identity.
func (r *runner) bedrock() {
	model := r.upstreamModel
	if model == "" {
		model = r.report.Model
	}
	for _, item := range bedrockPlan() {
		if r.ctx.Err() != nil || r.focusedStop() {
			return
		}
		body := r.body("Reply with exactly PONG. Do not call any tools.")
		body["max_tokens"] = 32
		request := Request{Body: body}
		expected := ""
		toolName, toolVersion := "", ""
		for _, tool := range bedrockToolProbes {
			if item.ID != tool.id {
				continue
			}
			toolName, toolVersion, expected = tool.name, tool.version, "server_tools"
			definition := map[string]any{"type": tool.version, "name": tool.name}
			if tool.name != "code_execution" {
				definition["max_uses"] = 1
			}
			if tool.name == "advisor" {
				request.Beta = bedrockAdvisorBeta
				definition["model"], definition["max_tokens"] = "claude-opus-5", 1024
			}
			body["tools"] = []any{definition}
		}
		switch item.ID {
		case "bedrock_role":
			expected = "message_role"
			// A deliberately nonexistent role avoids treating supported mid-turn
			// system messages on newer Opus models as invalid.
			body["messages"] = []any{map[string]any{"role": "model_check_invalid_role", "content": "PONG"}}
		case "bedrock_beta":
			expected, request.Beta = "beta", bedrockInvalidBeta
		case "bedrock_signature":
			expected = "thinking_signature"
			request.Body = r.bedrockSignatureBody()
		case "bedrock_sampling":
			expected = "sampling"
			switch bedrockSamplingRule(model) {
			case "fixed":
				body["temperature"] = 0.5
			case "exclusive":
				body["temperature"], body["top_p"] = 0.5, 0.8
			default:
				r.check(item.ID, "skipped", "bedrock_rule_unknown", map[string]any{"rule_model": model})
				continue
			}
		}
		before := len(r.report.Samples)
		_, response, valid := r.probe(item.ID, request)
		if len(r.report.Samples) == before {
			return
		}
		evidence := map[string]any{"http_status": response.Status, "expected_rejection": expected, "rule_model": model, "identity_verified": false}
		if toolName != "" {
			evidence["tool_name"], evidence["tool_type"] = toolName, toolVersion
			evidence["scope"] = "tool_declaration"
		}
		sample := r.report.Samples[len(r.report.Samples)-1]
		if response.Diagnostic != nil {
			evidence["diagnostic"] = response.Diagnostic
		}
		// A proxy may wrap a provider validation error. Match the actual reason,
		// never any 400, permission error, 5xx or successful HTTP error envelope.
		reasonText := sample.Error
		var envelope struct {
			Message string `json:"message"`
			Error   struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if common.Unmarshal(response.Body, &envelope) == nil {
			if envelope.Error.Message != "" {
				reasonText = envelope.Error.Message
			} else if envelope.Message != "" {
				reasonText = envelope.Message
			}
		}
		reason := bedrockValidationCode(reasonText)
		if response.Diagnostic != nil {
			reason = response.Diagnostic.Code
		}
		state, code := "inconclusive", "bedrock_rejection_unresolved"
		matches := reason == expected && explicitBedrockRejection(reasonText)
		if toolName != "" {
			matches = matches && bedrockToolRejection(reasonText, toolName, toolVersion)
		}
		if item.ID == "bedrock_signature" {
			// A thinking-mode/budget error or an echoed signature field does not
			// establish that the upstream validates thinking signatures.
			matches = reason == expected && bedrockSignatureRejection(reasonText)
		}
		if response.Status == 400 && matches {
			state, code = "pass", "bedrock_expected_rejection"
		} else if valid {
			// Acceptance can mean conversion, dropped fields, or gateway-provided
			// features. Preserve the difference without claiming a counterfeit model.
			state, code = "inconclusive", "bedrock_boundary_difference"
		}
		r.check(item.ID, state, code, evidence)
	}
}

// A synthetic tool-result continuation keeps the thinking block in the current
// turn, where the upstream must validate it. Completed prior turns may have
// their thinking stripped. No real reasoning or signature is collected/altered.
func (r *runner) bedrockSignatureBody() map[string]any {
	body := r.body("Call record_probe with PONG, then reply with exactly PONG.")
	body["max_tokens"] = 2048
	body["thinking"] = r.thinkingMode()
	body["tools"] = probeTools()
	body["messages"] = append(body["messages"].([]any),
		map[string]any{"role": "assistant", "content": []map[string]any{
			{"type": "thinking", "thinking": "Record the requested value.", "signature": "bW9kZWwtY2hlY2staW52YWxpZC1zaWduYXR1cmU="},
			{"type": "tool_use", "id": "toolu_model_check_signature", "name": "record_probe", "input": map[string]any{"value": "PONG"}},
		}},
		map[string]any{"role": "user", "content": []map[string]any{
			{"type": "tool_result", "tool_use_id": "toolu_model_check_signature", "content": "Recorded successfully. Reply with exactly PONG."},
		}},
	)
	return body
}

var bedrockSignatureRejections = []*regexp.Regexp{
	regexp.MustCompile(`\binvalid\s+(?:thinking\s+(?:block\s+)?)?signature\b`),
	regexp.MustCompile(`\bsignature\s+(?:verification\s+)?(?:is\s+)?(?:invalid|not valid|failed|does not match|could not be verified)\b`),
	regexp.MustCompile(`\b(?:failed|unable)\s+to\s+verify\s+(?:the\s+)?(?:thinking\s+(?:block\s+)?)?signature\b`),
}

func bedrockSignatureRejection(message string) bool {
	text := strings.NewReplacer("`", "", "'", "", "\"", "").Replace(strings.ToLower(message))
	if !strings.Contains(text, "thinking") {
		return false
	}
	for _, pattern := range bedrockSignatureRejections {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

// Only rejection of the tested tool/type establishes this boundary. A tool
// mentioned in an unrelated field, advisor model, beta or permission error is
// insufficient. Never award another tool's rejection to the current probe.
func bedrockToolRejection(message, name, version string) bool {
	text := strings.ToLower(message)
	for _, field := range []string{"permission", "not enabled", "not authorized", "advisor model", "executor model", "model pair"} {
		if strings.Contains(text, field) {
			return false
		}
	}
	tool := `(?:` + regexp.QuoteMeta(version) + `|` + regexp.QuoteMeta(name) + `)`
	patterns := []string{
		`\b` + tool + `["']?\s+(?:tool\s+)?(?:is\s+|are\s+)?(?:unsupported|not supported|not available|not allowed)\b`,
		`(?:unsupported|unrecognized|unknown|invalid)\s+(?:server[- ]side\s+)?tool(?:\s+type)?\s*[:=]?\s*["']?` + tool + `\b`,
		`(?:does not support|do not support)\s+(?:the\s+)?["']?` + tool + `\b`,
		`input tag ["']` + tool + `["'][^\n]*does not match any of the expected tags`,
	}
	for _, pattern := range patterns {
		if regexp.MustCompile(pattern).MatchString(text) {
			return true
		}
	}
	return false
}

func isBedrockProbe(id string) bool { return strings.HasPrefix(id, "bedrock_") }

// An error merely echoing the request fields is not evidence of a restriction.
func explicitBedrockRejection(message string) bool {
	text := strings.ToLower(message)
	for _, generic := range []string{"invalid request", "invalid_request_error", "model_check_invalid_role"} {
		text = strings.ReplaceAll(text, generic, "")
	}
	for _, marker := range []string{"invalid", "unsupported", "not supported", "not available", "does not support", "unknown beta", "unrecognized beta", "not permitted", "not allowed", "cannot both", "cannot be specified", "must be", "expected tags", "should be", "only supports"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
