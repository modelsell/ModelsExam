package claudecheck

import (
	"encoding/base64"
	"fmt"
	"strings"

	"model-check/common"
	"github.com/google/uuid"
)

// Structured-tool and paired-stream methods adapted from canarybyte/veridrop
// (AGPL-3.0). See testdata/veridrop/NOTICE.md for attribution and modifications.
// Assertions use local semantic expectations, never historical token thresholds.
func (r *runner) semanticResult(id string, m message, response Response, ok, matched bool, evidence map[string]any) {
	evidence["http_status"], evidence["stop_reason"] = response.Status, m.StopReason
	state, code := status(matched), "semantic_observed"
	switch {
	case !ok:
		state, code = "inconclusive", response.ErrorCode
		if code == "" {
			code = "probe_unavailable"
		}
		if code == "invalid_response" {
			state = "fail"
		}
	case m.StopReason == "refusal":
		state, code = "inconclusive", "request_refused"
	case !matched && m.StopReason == "max_tokens":
		state, code = "inconclusive", "answer_truncated"
	}
	if r.ctx.Err() != nil {
		state, code = "skipped", "cancelled"
	}
	r.check(id, state, code, evidence)
}

// A neutral one-page PDF with a fresh identifier avoids relying on a memorized
// fixture. Only that identifier enters evidence; no external document is fetched.
func documentFixture(identifier string) []byte {
	identifier = strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)").Replace(identifier)
	stream := "BT /F1 16 Tf 72 740 Td (Sample order) Tj 0 -32 Td (Order reference: " + identifier + ") Tj 0 -32 Td (Item: notebook. Quantity: 3.) Tj ET\n"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream),
	}
	var pdf strings.Builder
	pdf.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, object := range objects {
		offsets = append(offsets, pdf.Len())
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return []byte(pdf.String())
}

func (r *runner) document() {
	identifier := "ORDER-" + strings.ToUpper(uuid.NewString()[:8])
	body := r.body("")
	body["max_tokens"] = 512
	body["messages"] = []any{map[string]any{"role": "user", "content": []any{
		map[string]any{"type": "document", "source": map[string]any{"type": "base64", "media_type": "application/pdf", "data": base64.StdEncoding.EncodeToString(documentFixture(identifier))}},
		map[string]any{"type": "text", "text": "Read the order reference in this document. Reply with only the order reference."},
	}}}
	m, response, ok := r.probe("pdf", Request{Body: body})
	matched := strings.Trim(m.text(), "`\"' \n\r\t") == identifier
	r.semanticResult("pdf", m, response, ok, matched, map[string]any{"identifier_found": matched, "expected_identifier": identifier})
}

func (r *runner) structuredTool() {
	body := r.body("What's the current weather in Tokyo? Use celsius.")
	body["max_tokens"] = 512
	body["tools"] = []any{map[string]any{"name": "get_weather", "description": "Get the current weather for a city.", "input_schema": map[string]any{
		"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}, "unit": map[string]any{"type": "string", "enum": []string{"celsius", "fahrenheit"}}}, "required": []string{"city", "unit"}, "additionalProperties": false,
	}}}
	// Claude Opus 5.5, Sonnet 5.5, Fable 5.1 and Mythos 5.1 reject forced tool
	// use with a 400, so those models are asked with the default "auto".
	toolChoice := map[string]any{"type": "tool", "name": "get_weather"}
	if !supportsForcedToolChoice(r.report.Model) {
		toolChoice = map[string]any{"type": "auto"}
	}
	body["tool_choice"] = toolChoice
	m, response, ok := r.probe("tool", Request{Body: body})
	matched, calls := true, 0
	for _, b := range m.Content {
		if b["type"] != "tool_use" {
			continue
		}
		calls++
		input, isObject := b["input"].(map[string]any)
		id, _ := b["id"].(string)
		matched = matched && b["name"] == "get_weather" && id != "" && isObject && len(input) == 2 && input["city"] == "Tokyo" && input["unit"] == "celsius"
	}
	matched = matched && calls == 1
	r.semanticResult("tool", m, response, ok, matched && m.StopReason == "tool_use", map[string]any{
		"schema_matched": matched, "tool_calls": calls, "expected_tool": "get_weather", "tool_choice": toolChoice["type"], "expected_input": map[string]string{"city": "Tokyo", "unit": "celsius"},
	})
}

const comparisonPrompt = `Reply with exactly this JSON literal and nothing else: {"verify":"abc123","n":42}`

func comparisonJSONMatched(text string) bool {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```json\n") && strings.HasSuffix(text, "```") {
		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text, "```json\n"), "```"))
	}
	var value map[string]any
	return common.UnmarshalJsonStr(text, &value) == nil && len(value) == 2 && value["verify"] == "abc123" && value["n"] == float64(42)
}

func (r *runner) streamComparison() {
	body := r.body(comparisonPrompt)
	body["max_tokens"] = 512
	nonstream, nsResponse, nsOK := r.probe("comparison_nonstream", Request{Body: body})
	body["stream"] = true
	stream, response, ok := r.probe("comparison_stream", Request{Body: body})
	nsMatched, streamMatched := comparisonJSONMatched(nonstream.text()), comparisonJSONMatched(stream.text())
	evidence := map[string]any{
		"nonstream_json_matched": nsMatched, "stream_json_matched": streamMatched,
		"text_equal": nonstream.text() == stream.text(), "nonstream_usage": nonstream.Usage, "stream_usage": stream.Usage,
		"nonstream_stop_reason": nonstream.StopReason, "stream_stop_reason": stream.StopReason,
	}
	if nsInput, valid := totalInput(nonstream.Usage); valid {
		if sInput, valid := totalInput(stream.Usage); valid {
			evidence["input_plus_cache_difference"] = sInput - nsInput
		}
	}
	// Keep both responses in the ledger; the first unavailable/refused response
	// explains why a pair cannot establish a semantic comparison.
	resultMessage, resultResponse, resultOK := stream, response, ok
	if !nsOK || (ok && !nsMatched && nonstream.StopReason == "max_tokens") {
		resultMessage, resultResponse, resultOK = nonstream, nsResponse, nsOK
		evidence["phase"] = "nonstream"
	}
	r.semanticResult("stream_comparison", resultMessage, resultResponse, resultOK, nsMatched && streamMatched, evidence)
	metadata := map[string]any{"nonstream_stop_reason": nonstream.StopReason, "stream_stop_reason": evidence["stream_stop_reason"]}
	if !nsOK || !ok || nonstream.StopReason == "refusal" || stream.StopReason == "refusal" {
		r.semanticResult("stream_stop_reason", resultMessage, resultResponse, resultOK, false, metadata)
		return
	}
	r.check("stream_stop_reason", status(nonstream.StopReason == "end_turn" && evidence["stream_stop_reason"] == "end_turn"), "comparison_stop_observed", metadata)
}

// supportsForcedToolChoice reports whether tool_choice "any" / "tool" is
// accepted. Per the Claude API docs these models return a 400
// invalid_request_error for forced tool use; "auto" and "none" still work.
func supportsForcedToolChoice(model string) bool {
	name := strings.ReplaceAll(strings.ToLower(model), ".", "-")
	for _, family := range []string{"opus-5-5", "sonnet-5-5", "fable-5-1", "mythos-5-1"} {
		if strings.Contains(name, family) {
			return false
		}
	}
	return true
}
