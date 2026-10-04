package claudecheck

import (
	"reflect"

	"model-check/common"
)

func capabilityToolBody(model string, roundtrip bool) map[string]any {
	prompt := `Find inventory for SKU-204 at warehouse north, excluding reserved stock. Use the appropriate tool with include_reserved set to false. The answer is not supplied in this prompt.`
	if roundtrip {
		prompt = `Look up order ORD-731 with the provided tool. After receiving its result, return exactly one JSON object with the fields order_id, status, total, currency, using only values in the tool result.`
	}
	body := capabilityBody(model, prompt)
	// Follow the native tool loop defaults. Disabling thinking can make newer
	// Claude models write a call as plain text instead of emitting tool_use.
	delete(body, "thinking")
	body["max_tokens"] = 1024
	body["tool_choice"] = map[string]any{"type": "auto", "disable_parallel_tool_use": true}
	body["tools"] = []any{
		map[string]any{"name": "lookup_inventory", "description": "Return available stock for a SKU at a warehouse.", "input_schema": map[string]any{"type": "object", "properties": map[string]any{"sku": map[string]any{"type": "string"}, "warehouse": map[string]any{"type": "string"}, "include_reserved": map[string]any{"type": "boolean"}}, "required": []string{"sku", "warehouse", "include_reserved"}, "additionalProperties": false}},
		map[string]any{"name": "get_weather", "description": "Return a weather observation for a city.", "input_schema": map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}, "required": []string{"city"}, "additionalProperties": false}},
	}
	if roundtrip {
		body["tools"] = []any{map[string]any{"name": "lookup_order", "description": "Return the status and total of a fictional order.", "input_schema": map[string]any{"type": "object", "properties": map[string]any{"order_id": map[string]any{"type": "string"}}, "required": []string{"order_id"}, "additionalProperties": false}}}
	}
	return body
}

func capabilityToolCall(m message, roundtrip bool) (map[string]any, []BenchmarkAssertion) {
	var calls []map[string]any
	for _, block := range m.Content {
		if block["type"] == "tool_use" {
			calls = append(calls, block)
		}
	}
	var call map[string]any
	if len(calls) == 1 {
		call = calls[0]
	}
	name := "lookup_inventory"
	input := map[string]any{"sku": "SKU-204", "warehouse": "north", "include_reserved": false}
	if roundtrip {
		name = "lookup_order"
		input = map[string]any{"order_id": "ORD-731"}
	}
	id, _ := call["id"].(string)
	return call, []BenchmarkAssertion{{"one_tool_call", len(calls) == 1 && id != ""}, {"tool_name", call["name"] == name}, {"tool_arguments", reflect.DeepEqual(call["input"], input)}, {"tool_stop", m.StopReason == "tool_use"}}
}

func (r *runner) gradeCapabilityTool(item *BenchmarkItem, fixture capabilityFixture, m message) {
	roundtrip := fixture.id == "tool_roundtrip"
	call, assertions := capabilityToolCall(m, roundtrip)
	if call != nil {
		// Persist only tool name/arguments, never signatures, internal reasoning
		// or the provider-generated call id.
		data, _ := common.Marshal(map[string]any{"name": call["name"], "input": call["input"]})
		item.Actual = capabilityActual(string(data))
	}
	valid := true
	for _, a := range assertions {
		valid = valid && a.Passed
	}
	if !valid {
		// A text answer or wrong call cannot receive partial credit merely for
		// returning the tool_use envelope without using the requested tool.
		scoreCapability(item, []BenchmarkAssertion{{"correct_tool_call", false}})
		return
	}
	if !roundtrip {
		scoreCapability(item, assertions)
		return
	}
	if r.ctx.Err() != nil {
		item.Code = "cancelled"
		return
	}
	body := capabilityToolBody(r.report.Model, true)
	body["messages"] = append(body["messages"].([]any),
		map[string]any{"role": "assistant", "content": m.Content},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": call["id"], "content": capabilityToolResult}}},
	)
	profile := bodyProfile(body)
	answer, response, ok := r.probe("benchmark_tool_roundtrip_result", Request{Body: body})
	item.Code = capabilityUnavailable(answer, response, ok, profile)
	if item.Code != "" {
		return
	}
	if !capabilityFinalAnswer(answer) {
		item.Code = "unexpected_stop"
		return
	}
	item.Actual = capabilityActual(answer.text())
	assertions = append(assertions, gradeCapabilityJSON(answer.text(), map[string]any{"order_id": "ORD-731", "status": "shipped", "total": float64(42), "currency": "USD"})...)
	assertions = append(assertions, BenchmarkAssertion{"final_answer", answer.StopReason == "end_turn"})
	scoreCapability(item, assertions)
}
