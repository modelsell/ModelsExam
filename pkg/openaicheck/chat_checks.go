package openaicheck

import (
	"strings"
	"unicode"
)

func (r *runner) chatSuite() {
	r.chatSystem()
	r.chatMaxTokens()
	r.chatStop()
	r.chatMultiTurn()
	r.chatParams()
	r.chatN()
	r.chatLogprobs()
	r.chatStream()
	r.chatToolCall()
	r.chatToolChoice()
	r.chatToolChoiceNone()
	r.chatToolRoundtrip()
	r.chatParallelTools()
	r.chatToolStream()
	r.chatJSONMode()
	r.chatJSONSchema()
	r.chatVision()
}

func trimWord(s string) string {
	return strings.ToUpper(strings.TrimFunc(strings.TrimSpace(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }))
}

func (r *runner) chatSystem() {
	if !r.live("chat_system") {
		return
	}
	res := r.probe("chat_system", KindChat, r.chatRequest(r.chatBody(r.budget(128),
		system("You answer every message with exactly one word: ORCHID. Never add punctuation or anything else."), user("Hello"))))
	r.judge("chat_system", res, trimWord(res.v.Text) == "ORCHID", "system_followed", "system_ignored", map[string]any{"text": truncate(res.v.Text, 80)})
}

func (r *runner) chatMaxTokens() {
	if !r.live("chat_max_tokens") {
		return
	}
	const limit = 8
	res := r.probe("chat_max_tokens", KindChat, r.chatRequest(r.chatBody(limit, user("Write a long numbered list of integers from 1 to 1000."))))
	within := res.v.Usage.Output == nil || *res.v.Usage.Output <= limit
	evidence := map[string]any{"limit_param": r.opts.LimitParam, "limit": limit, "finish_reason": res.v.Finish}
	if res.v.Usage.Output != nil {
		evidence["completion_tokens"] = *res.v.Usage.Output
	}
	r.judge("chat_max_tokens", res, res.v.Finish == "length" && within, "length_enforced", "limit_not_enforced", evidence)
}

func (r *runner) chatStop() {
	if !r.live("chat_stop") {
		return
	}
	body := r.chatBody(r.budget(128), user("Output exactly: ALPHA CHECK_STOP OMEGA"))
	body["stop"] = []string{"CHECK_STOP"}
	res := r.probe("chat_stop", KindChat, r.chatRequest(body))
	applied := !strings.Contains(res.v.Text, "CHECK_STOP") && !strings.Contains(res.v.Text, "OMEGA")
	r.judge("chat_stop", res, applied, "stop_sequence_applied", "stop_sequence_ignored", map[string]any{"text": truncate(res.v.Text, 80)})
}

func (r *runner) chatMultiTurn() {
	if !r.live("chat_multi_turn") {
		return
	}
	needle := newID()[:12]
	res := r.probe("chat_multi_turn", KindChat, r.chatRequest(r.chatBody(r.budget(128),
		user("Remember this code: "+needle), assistant("I will remember the code."), user("Return only the exact code I gave you."))))
	r.judge("chat_multi_turn", res, strings.Contains(res.v.Text, needle), "history_used", "history_lost", nil)
}

// chat_params checks input acceptance: a conforming endpoint must take the
// documented sampling and bookkeeping parameters without a 4xx.
func (r *runner) chatParams() {
	if !r.live("chat_params") {
		return
	}
	body := r.chatBody(r.budget(64), user("Reply with exactly PONG."))
	body["n"] = 1
	body["seed"] = 7
	body["user"] = "openai-model-check"
	if r.reasoning {
		body["reasoning_effort"] = "low"
	} else {
		body["temperature"] = 0
		body["top_p"] = 1
		body["presence_penalty"] = 0
		body["frequency_penalty"] = 0
	}
	res := r.probe("chat_params", KindChat, r.chatRequest(body))
	names := make([]string, 0, len(body))
	for key := range body {
		if key != "model" && key != "messages" {
			names = append(names, key)
		}
	}
	r.judge("chat_params", res, true, "params_accepted", "params_rejected", map[string]any{"parameters": sortedNames(names)})
}

func (r *runner) chatN() {
	if !r.live("chat_n") {
		return
	}
	body := r.chatBody(r.budget(64), user("Name a color."))
	body["n"] = 2
	res := r.probe("chat_n", KindChat, r.chatRequest(body))
	r.judge("chat_n", res, res.v.Choices == 2, "n_choices_returned", "n_ignored", map[string]any{"choices": res.v.Choices})
}

func (r *runner) chatLogprobs() {
	if !r.live("chat_logprobs") {
		return
	}
	if r.reasoning {
		r.check("chat_logprobs", "skipped", "unsupported_model_family", nil)
		return
	}
	body := r.chatBody(8, user("Reply with exactly PONG."))
	body["logprobs"] = true
	body["top_logprobs"] = 2
	res := r.probe("chat_logprobs", KindChat, r.chatRequest(body))
	present := false
	if choices := arr(res.doc, "choices"); len(choices) > 0 {
		choice, _ := choices[0].(object)
		if lp := obj(choice, "logprobs"); lp != nil {
			if content := arr(lp, "content"); len(content) > 0 {
				entry, _ := content[0].(object)
				_, hasProb := entry["logprob"].(float64)
				present = str(entry, "token") != "" && hasProb && len(arr(entry, "top_logprobs")) > 0
			}
		}
	}
	r.judge("chat_logprobs", res, present, "logprobs_returned", "logprobs_missing", nil)
}

func (r *runner) streamBody(limit int, messages ...any) map[string]any {
	body := r.chatBody(limit, messages...)
	body["stream"] = true
	body["stream_options"] = object{"include_usage": true}
	return body
}

func (r *runner) chatStream() {
	if !r.live("chat_stream") {
		return
	}
	res := r.probe("chat_stream", KindChat, r.chatRequest(r.streamBody(r.budget(256), user("Count from 1 to 5, separated by commas."))))
	evidence := map[string]any{"chunks": len(res.resp.Events), "done_marker": res.resp.Done, "text_length": len(res.v.Text)}
	if res.resp.FirstEventMS != nil {
		evidence["first_event_ms"] = *res.resp.FirstEventMS
	}
	r.judge("chat_stream", res, res.v.Text != "" && res.v.Finish == "stop", "stream_valid", "stream_unexpected_output", evidence)
	if !r.selected("chat_stream_usage") {
		return
	}
	if res.sample.Status != 200 || len(res.resp.Events) == 0 {
		r.check("chat_stream_usage", "inconclusive", "dependency_unavailable", map[string]any{"http_status": res.sample.Status})
		return
	}
	switch {
	case !res.v.UsageSeen:
		r.check("chat_stream_usage", "fail", "missing_usage_chunk", nil)
	case len(res.v.UsageIssues) > 0:
		r.check("chat_stream_usage", "fail", "usage_chunk_invalid", map[string]any{"issues": res.v.UsageIssues})
	default:
		r.check("chat_stream_usage", "pass", "usage_chunk_valid", nil)
	}
}

func (r *runner) toolBody(prompt string, tools ...any) map[string]any {
	body := r.chatBody(r.budget(512), user(prompt))
	body["tools"] = tools
	body["tool_choice"] = "auto"
	body["parallel_tool_calls"] = false
	return body
}

func (r *runner) chatToolCall() {
	if !r.live("chat_tool_call") {
		return
	}
	res := r.probe("chat_tool_call", KindChat, r.chatRequest(r.toolBody(promptInventory, toolInventory(), toolWeather())))
	assertions := gradeToolCall(res.v, "tool_calls", "lookup_inventory", map[string]any{"sku": "SKU-204", "warehouse": "north", "include_reserved": false})
	r.judge("chat_tool_call", res, len(failed(assertions)) == 0, "tool_call_valid", "tool_call_invalid", map[string]any{"failed": failed(assertions), "tool_calls": len(res.v.Tools), "finish_reason": res.v.Finish})
}

func (r *runner) chatToolChoice() {
	if !r.live("chat_tool_choice") {
		return
	}
	body := r.toolBody("Say hello.", toolInventory(), toolWeather())
	body["tool_choice"] = object{"type": "function", "function": object{"name": "get_weather"}}
	res := r.probe("chat_tool_choice", KindChat, r.chatRequest(body))
	assertions := gradeToolCall(res.v, "tool_calls", "get_weather", nil)
	var city string
	if len(res.v.Tools) == 1 {
		var args map[string]any
		_ = unmarshalString(res.v.Tools[0].Arguments, &args)
		city, _ = args["city"].(string)
	}
	assertions = append(assertions, assertion{"city_argument", city != ""})
	r.judge("chat_tool_choice", res, len(failed(assertions)) == 0, "forced_tool_called", "forced_tool_not_called", map[string]any{"failed": failed(assertions), "tool_calls": len(res.v.Tools)})
}

func (r *runner) chatToolChoiceNone() {
	if !r.live("chat_tool_choice_none") {
		return
	}
	body := r.toolBody(promptInventory, toolInventory(), toolWeather())
	body["tool_choice"] = "none"
	res := r.probe("chat_tool_choice_none", KindChat, r.chatRequest(body))
	r.judge("chat_tool_choice_none", res, len(res.v.Tools) == 0 && res.v.Finish != "tool_calls", "tools_suppressed", "tool_called_despite_none", map[string]any{"tool_calls": len(res.v.Tools), "finish_reason": res.v.Finish})
}

func (r *runner) chatToolRoundtrip() {
	if !r.live("chat_tool_roundtrip") {
		return
	}
	body := r.toolBody(promptOrder, toolOrder())
	first := r.probe("chat_tool_roundtrip", KindChat, r.chatRequest(body))
	assertions := gradeToolCall(first.v, "tool_calls", "lookup_order", map[string]any{"order_id": "ORD-731"})
	if !first.ok || len(failed(assertions)) > 0 {
		r.judge("chat_tool_roundtrip", first, false, "", "tool_call_invalid", map[string]any{"failed": failed(assertions), "step": "tool_call"})
		return
	}
	follow := r.toolBody(promptOrder, toolOrder())
	follow["messages"] = []any{user(promptOrder), toolMessageCalls(first.v.Tools), object{"role": "tool", "tool_call_id": first.v.Tools[0].ID, "content": toolResultOrder}}
	second := r.probe("chat_tool_roundtrip_result", KindChat, r.chatRequest(follow))
	graded, fenced := gradeJSON(second.v.Text, expectedOrder(), true)
	pass := len(failed(graded)) == 0 && second.v.Finish == "stop" && len(second.v.Tools) == 0
	r.judge("chat_tool_roundtrip", second, pass, "roundtrip_valid", "roundtrip_answer_invalid", map[string]any{"failed": failed(graded), "finish_reason": second.v.Finish, "fenced_json": fenced})
}

func (r *runner) chatParallelTools() {
	if !r.live("chat_parallel_tools") {
		return
	}
	body := r.toolBody("What is the weather in Paris and in Tokyo? Call the weather tool once for each city in the same turn.", toolWeather())
	body["parallel_tool_calls"] = true
	res := r.probe("chat_parallel_tools", KindChat, r.chatRequest(body))
	ids, cities := map[string]bool{}, map[string]bool{}
	for _, call := range res.v.Tools {
		ids[call.ID] = true
		var args map[string]any
		_ = unmarshalString(call.Arguments, &args)
		if city, _ := args["city"].(string); city != "" {
			cities[strings.ToLower(city)] = true
		}
	}
	pass := len(res.v.Tools) >= 2 && len(ids) == len(res.v.Tools) && cities["paris"] && cities["tokyo"]
	r.judge("chat_parallel_tools", res, pass, "parallel_calls_returned", "single_tool_call", map[string]any{"tool_calls": len(res.v.Tools)})
}

func (r *runner) chatToolStream() {
	if !r.live("chat_tool_stream") {
		return
	}
	body := r.streamBody(r.budget(512), user(promptInventory))
	body["tools"] = []any{toolInventory(), toolWeather()}
	body["tool_choice"] = "auto"
	body["parallel_tool_calls"] = false
	res := r.probe("chat_tool_stream", KindChat, r.chatRequest(body))
	assertions := gradeToolCall(res.v, "tool_calls", "lookup_inventory", map[string]any{"sku": "SKU-204", "warehouse": "north", "include_reserved": false})
	r.judge("chat_tool_stream", res, len(failed(assertions)) == 0, "streamed_tool_call_valid", "streamed_tool_call_invalid", map[string]any{"failed": failed(assertions), "chunks": len(res.resp.Events)})
}

func (r *runner) chatJSONMode() {
	if !r.live("chat_json_mode") {
		return
	}
	body := r.chatBody(r.budget(256), user(`Return a JSON object with a single key "ok" set to true.`))
	body["response_format"] = object{"type": "json_object"}
	res := r.probe("chat_json_mode", KindChat, r.chatRequest(body))
	value, _, valid := parseJSONObject(res.v.Text, false)
	ok, _ := value["ok"].(bool)
	r.judge("chat_json_mode", res, valid && ok, "json_object_honored", "json_object_not_honored", map[string]any{"valid_json": valid})
}

func (r *runner) chatJSONSchema() {
	if !r.live("chat_json_schema") {
		return
	}
	body := r.chatBody(r.budget(512), user(promptExtract))
	body["response_format"] = object{"type": "json_schema", "json_schema": object{"name": "order", "strict": true, "schema": schemaOrder()}}
	res := r.probe("chat_json_schema", KindChat, r.chatRequest(body))
	graded, _ := gradeJSON(res.v.Text, expectedExtract(), false)
	r.judge("chat_json_schema", res, len(failed(graded)) == 0 && res.v.Refusal == "", "json_schema_honored", "json_schema_not_honored", map[string]any{"failed": failed(graded)})
}

func (r *runner) chatVision() {
	if !r.live("chat_vision") {
		return
	}
	content := []any{
		object{"type": "text", "text": "What is the dominant color of this image? Answer with one lowercase word."},
		object{"type": "image_url", "image_url": object{"url": redImageDataURL()}},
	}
	res := r.probe("chat_vision", KindChat, r.chatRequest(r.chatBody(r.budget(64), object{"role": "user", "content": content})))
	r.judge("chat_vision", res, strings.Contains(strings.ToLower(res.v.Text), "red"), "image_understood", "image_not_understood", map[string]any{"text": truncate(res.v.Text, 80)})
}
