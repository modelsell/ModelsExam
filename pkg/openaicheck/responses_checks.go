package openaicheck

import (
	"sort"
	"strings"

	"model-check/common"
)

func unmarshalString(s string, v any) error { return common.UnmarshalJsonStr(s, v) }

func sortedNames(names []string) []string {
	sort.Strings(names)
	return names
}

func (r *runner) responsesSuite() {
	if !r.opts.Responses || r.ctx.Err() != nil || r.report.StopReason != "" {
		return
	}
	r.responsesBasic()
	r.responsesMaxOutput()
	r.responsesStream()
	r.responsesToolCall()
	r.responsesToolStream()
	r.responsesToolRoundtrip()
	r.responsesStructured()
	r.responsesPreviousID()
}

// outputBudget keeps max_output_tokens at or above the documented minimum of 16.
func (r *runner) outputBudget(n int) int {
	n = r.budget(n)
	if n < 16 {
		return 16
	}
	return n
}

func (r *runner) responsesBody(input any, limit int) map[string]any {
	return map[string]any{"model": r.report.Model, "input": input, "max_output_tokens": r.outputBudget(limit)}
}

func (r *runner) responsesBasic() {
	if !r.live("responses_basic") {
		return
	}
	body := r.responsesBody("Reply with exactly PONG.", 256)
	body["instructions"] = "You are a terse assistant."
	body["store"] = false
	res := r.probe("responses_basic", KindResponses, r.responsesRequest(body))
	r.judge("responses_basic", res, res.v.Finish == "completed" && res.v.Text != "", "response_valid", "unexpected_status_or_empty", map[string]any{"status": res.v.Finish, "text_length": len(res.v.Text)})
}

func (r *runner) responsesMaxOutput() {
	if !r.live("responses_max_output") {
		return
	}
	body := object{"model": r.report.Model, "input": "Write a long numbered list of integers from 1 to 1000.", "max_output_tokens": 16, "store": false}
	res := r.probe("responses_max_output", KindResponses, r.responsesRequest(body))
	r.judge("responses_max_output", res, res.v.Finish == "incomplete:max_output_tokens", "limit_enforced", "limit_not_enforced", map[string]any{"status": res.v.Finish})
}

func (r *runner) responsesStream() {
	if !r.live("responses_stream") {
		return
	}
	body := r.responsesBody("Count from 1 to 5, separated by commas.", 256)
	body["stream"] = true
	body["store"] = false
	res := r.probe("responses_stream", KindResponses, r.responsesRequest(body))
	evidence := map[string]any{"events": len(res.resp.Events), "status": res.v.Finish}
	if res.resp.FirstEventMS != nil {
		evidence["first_event_ms"] = *res.resp.FirstEventMS
	}
	r.judge("responses_stream", res, res.v.Finish == "completed" && res.v.Text != "", "stream_lifecycle_valid", "stream_unexpected_output", evidence)
}

func (r *runner) responsesToolBody(prompt string, tools ...object) map[string]any {
	flat := make([]any, 0, len(tools))
	for _, tool := range tools {
		flat = append(flat, flatTool(tool))
	}
	body := r.responsesBody(prompt, 512)
	body["tools"] = flat
	body["tool_choice"] = "auto"
	body["parallel_tool_calls"] = false
	return body
}

func (r *runner) responsesToolCall() {
	if !r.live("responses_tool_call") {
		return
	}
	res := r.probe("responses_tool_call", KindResponses, r.responsesRequest(r.responsesToolBody(promptInventory, toolInventory(), toolWeather())))
	// A Responses function call completes the response; there is no finish_reason.
	assertions := gradeToolCall(res.v, "completed", "lookup_inventory", map[string]any{"sku": "SKU-204", "warehouse": "north", "include_reserved": false})
	r.judge("responses_tool_call", res, len(failed(assertions)) == 0, "function_call_valid", "function_call_invalid", map[string]any{"failed": failed(assertions), "tool_calls": len(res.v.Tools)})
}

// responsesToolStream checks the function-call delta events. Each
// response.function_call_arguments.delta (and the done event) must include the
// item_id of the function_call item announced by response.output_item.added,
// and that id must match the item in the final response.
func (r *runner) responsesToolStream() {
	if !r.live("responses_tool_stream") {
		return
	}
	body := r.responsesToolBody(promptInventory, toolInventory(), toolWeather())
	body["stream"] = true
	res := r.probe("responses_tool_stream", KindResponses, r.responsesRequest(body))
	assertions := gradeToolCall(res.v, "completed", "lookup_inventory", map[string]any{"sku": "SKU-204", "warehouse": "north", "include_reserved": false})
	evidence := map[string]any{"failed": failed(assertions), "events": len(res.resp.Events)}
	if len(res.v.Tools) == 1 {
		evidence["item_id"] = res.v.Tools[0].ItemID
	}
	r.judge("responses_tool_stream", res, len(failed(assertions)) == 0, "function_call_deltas_valid", "streamed_function_call_invalid", evidence)
}

func (r *runner) responsesToolRoundtrip() {
	if !r.live("responses_tool_roundtrip") {
		return
	}
	first := r.probe("responses_tool_roundtrip", KindResponses, r.responsesRequest(r.responsesToolBody(promptOrder, toolOrder())))
	assertions := gradeToolCall(first.v, "completed", "lookup_order", map[string]any{"order_id": "ORD-731"})
	if !first.ok || len(failed(assertions)) > 0 {
		r.judge("responses_tool_roundtrip", first, false, "", "function_call_invalid", map[string]any{"failed": failed(assertions), "step": "function_call"})
		return
	}
	// Pass every output item back verbatim (including reasoning items) so the
	// stateless follow-up is valid for reasoning models too.
	input := []any{object{"role": "user", "content": promptOrder}}
	input = append(input, first.v.Output...)
	input = append(input, object{"type": "function_call_output", "call_id": first.v.Tools[0].ID, "output": toolResultOrder})
	body := r.responsesToolBody(promptOrder, toolOrder())
	body["input"] = input
	second := r.probe("responses_tool_roundtrip_result", KindResponses, r.responsesRequest(body))
	graded, fenced := gradeJSON(second.v.Text, expectedOrder(), true)
	pass := len(failed(graded)) == 0 && second.v.Finish == "completed" && len(second.v.Tools) == 0
	r.judge("responses_tool_roundtrip", second, pass, "roundtrip_valid", "roundtrip_answer_invalid", map[string]any{"failed": failed(graded), "status": second.v.Finish, "fenced_json": fenced})
}

func (r *runner) responsesStructured() {
	if !r.live("responses_structured") {
		return
	}
	body := r.responsesBody(promptExtract, 512)
	body["text"] = object{"format": object{"type": "json_schema", "name": "order", "strict": true, "schema": schemaOrder()}}
	body["store"] = false
	res := r.probe("responses_structured", KindResponses, r.responsesRequest(body))
	graded, _ := gradeJSON(res.v.Text, expectedExtract(), false)
	r.judge("responses_structured", res, len(failed(graded)) == 0 && res.v.Refusal == "", "json_schema_honored", "json_schema_not_honored", map[string]any{"failed": failed(graded)})
}

func (r *runner) responsesPreviousID() {
	if !r.live("responses_previous_id") {
		return
	}
	needle := newID()[:12]
	first := r.probe("responses_previous_id", KindResponses, r.responsesRequest(r.responsesBody("Remember this code: "+needle+". Reply with OK.", 128)))
	if !first.ok || first.v.ID == "" {
		r.judge("responses_previous_id", first, false, "", "first_request_invalid", nil)
		return
	}
	body := r.responsesBody("Return only the exact code I gave you.", 128)
	body["previous_response_id"] = first.v.ID
	body["store"] = false
	second := r.probe("responses_previous_id_result", KindResponses, r.responsesRequest(body))
	r.judge("responses_previous_id", second, strings.Contains(second.v.Text, needle), "conversation_state_kept", "conversation_state_lost", nil)
}
