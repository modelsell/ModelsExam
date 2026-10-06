package geminicheck

import (
	"reflect"
	"strings"
	"unicode"
)

func (r *runner) suite() {
	r.system()
	r.maxTokens()
	r.stop()
	r.multiTurn()
	r.params()
	r.candidates()
	r.thinking()
	r.streamText()
	r.toolCall()
	r.toolChoice()
	r.toolChoiceNone()
	r.toolRoundtrip()
	r.parallelTools()
	r.toolStream()
	r.jsonMode()
	r.jsonSchema()
	r.vision()
	r.countTokens()
}

func trimWord(s string) string {
	return strings.ToUpper(strings.TrimFunc(strings.TrimSpace(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }))
}

func (r *runner) system() {
	if !r.live("gemini_system") {
		return
	}
	b := body(defaultBudget, userText("Hello"))
	b["systemInstruction"] = object{"parts": []any{object{"text": "You answer every message with exactly one word: ORCHID. Never add punctuation or anything else."}}}
	res := r.probe("gemini_system", KindGenerate, r.generate(b))
	r.judge("gemini_system", res, trimWord(res.v.Text) == "ORCHID", "system_followed", "system_ignored", map[string]any{"text": truncate(res.v.Text, 80)})
}

func (r *runner) maxTokens() {
	if !r.live("gemini_max_tokens") {
		return
	}
	const limit = 16
	b := body(limit, userText("Write a long numbered list of integers from 1 to 1000."))
	res := r.probe("gemini_max_tokens", KindGenerate, r.generate(b))
	// Visible output (candidatesTokenCount) must fit the limit; thinking
	// models may also spend part of it on thoughts.
	within := true
	evidence := map[string]any{"limit": limit, "finish_reason": res.v.Finish}
	if u := res.v.Usage; u.Output != nil {
		candidates := *u.Output
		if u.Thoughts != nil {
			candidates -= *u.Thoughts
		}
		within = candidates <= limit
		evidence["candidates_tokens"] = candidates
	}
	r.judge("gemini_max_tokens", res, res.v.Finish == "MAX_TOKENS" && within, "max_tokens_enforced", "limit_not_enforced", evidence)
}

func (r *runner) stop() {
	if !r.live("gemini_stop") {
		return
	}
	b := body(defaultBudget, userText("Output exactly: ALPHA CHECK_STOP OMEGA"))
	config(b)["stopSequences"] = []string{"CHECK_STOP"}
	res := r.probe("gemini_stop", KindGenerate, r.generate(b))
	applied := !strings.Contains(res.v.Text, "CHECK_STOP") && !strings.Contains(res.v.Text, "OMEGA")
	r.judge("gemini_stop", res, applied && res.v.Finish == "STOP", "stop_sequence_applied", "stop_sequence_ignored", map[string]any{"text": truncate(res.v.Text, 80), "finish_reason": res.v.Finish})
}

func (r *runner) multiTurn() {
	if !r.live("gemini_multi_turn") {
		return
	}
	needle := newID()[:12]
	res := r.probe("gemini_multi_turn", KindGenerate, r.generate(body(defaultBudget,
		userText("Remember this code: "+needle), modelText("I will remember the code."), userText("Return only the exact code I gave you."))))
	r.judge("gemini_multi_turn", res, strings.Contains(res.v.Text, needle), "history_used", "history_lost", nil)
}

// gemini_params checks input acceptance of the documented sampling fields.
func (r *runner) params() {
	if !r.live("gemini_params") {
		return
	}
	b := body(defaultBudget, userText("Reply with exactly PONG."))
	c := config(b)
	c["temperature"], c["topP"], c["topK"], c["candidateCount"], c["seed"], c["responseMimeType"] = 0, 1, 40, 1, 7, "text/plain"
	b["safetySettings"] = []any{object{"category": "HARM_CATEGORY_DANGEROUS_CONTENT", "threshold": "BLOCK_ONLY_HIGH"}}
	res := r.probe("gemini_params", KindGenerate, r.generate(b))
	r.judge("gemini_params", res, true, "params_accepted", "params_rejected", map[string]any{"parameters": []string{"candidateCount", "responseMimeType", "safetySettings", "seed", "temperature", "topK", "topP"}})
}

func (r *runner) candidates() {
	if !r.live("gemini_candidates") {
		return
	}
	b := body(defaultBudget, userText("Name a color."))
	config(b)["candidateCount"] = 2
	res := r.probe("gemini_candidates", KindGenerate, r.generate(b))
	r.judge("gemini_candidates", res, res.v.Candidates == 2, "candidates_returned", "candidate_count_ignored", map[string]any{"candidates": res.v.Candidates})
}

func (r *runner) thinking() {
	if !r.live("gemini_thinking") {
		return
	}
	b := body(defaultBudget, userText("What is 17 * 23? Answer with the number only."))
	config(b)["thinkingConfig"] = object{"includeThoughts": true}
	res := r.probe("gemini_thinking", KindGenerate, r.generate(b))
	thoughts := int64(0)
	if res.v.Usage.Thoughts != nil {
		thoughts = *res.v.Usage.Thoughts
	}
	pass := strings.Contains(res.v.Text, "391") && (res.v.Thoughts > 0 || thoughts > 0)
	r.judge("gemini_thinking", res, pass, "thoughts_returned", "thoughts_missing", map[string]any{"thought_parts": res.v.Thoughts, "thoughts_tokens": thoughts})
}

func (r *runner) streamText() {
	if !r.live("gemini_stream") {
		return
	}
	res := r.probe("gemini_stream", KindGenerate, r.stream(body(defaultBudget, userText("Count from 1 to 5, separated by commas."))))
	evidence := map[string]any{"chunks": len(res.resp.Events), "text_length": len(res.v.Text)}
	if res.resp.FirstEventMS != nil {
		evidence["first_event_ms"] = *res.resp.FirstEventMS
	}
	r.judge("gemini_stream", res, res.v.Text != "" && res.v.Finish == "STOP", "stream_valid", "stream_unexpected_output", evidence)
	if !r.selected("gemini_stream_usage") {
		return
	}
	if res.sample.Status != 200 || len(res.resp.Events) == 0 {
		r.check("gemini_stream_usage", "inconclusive", "dependency_unavailable", map[string]any{"http_status": res.sample.Status})
		return
	}
	switch {
	case !res.v.UsageSeen:
		r.check("gemini_stream_usage", "fail", "missing_usage_metadata", nil)
	case len(res.v.UsageIssues) > 0:
		r.check("gemini_stream_usage", "fail", "usage_chunk_invalid", map[string]any{"issues": res.v.UsageIssues})
	default:
		r.check("gemini_stream_usage", "pass", "usage_chunk_valid", nil)
	}
}

func toolBody(prompt string, mode string, declarations ...any) map[string]any {
	b := body(defaultBudget, userText(prompt))
	b["tools"] = []any{object{"functionDeclarations": declarations}}
	b["toolConfig"] = object{"functionCallingConfig": object{"mode": mode}}
	return b
}

func (r *runner) toolCall() {
	if !r.live("gemini_tool_call") {
		return
	}
	res := r.probe("gemini_tool_call", KindGenerate, r.generate(toolBody(promptInventory, "AUTO", declInventory(), declWeather())))
	assertions := gradeCall(res.v, "lookup_inventory", inventoryArgs())
	r.judge("gemini_tool_call", res, len(failed(assertions)) == 0, "function_call_valid", "function_call_invalid", map[string]any{"failed": failed(assertions), "function_calls": len(res.v.Calls), "finish_reason": res.v.Finish})
}

func (r *runner) toolChoice() {
	if !r.live("gemini_tool_choice") {
		return
	}
	b := toolBody("Say hello.", "ANY", declInventory(), declWeather())
	b["toolConfig"] = object{"functionCallingConfig": object{"mode": "ANY", "allowedFunctionNames": []string{"get_weather"}}}
	res := r.probe("gemini_tool_choice", KindGenerate, r.generate(b))
	assertions := gradeCall(res.v, "get_weather", nil)
	city := ""
	if len(res.v.Calls) == 1 {
		city, _ = res.v.Calls[0].Args["city"].(string)
	}
	assertions = append(assertions, assertion{"city_argument", city != ""})
	r.judge("gemini_tool_choice", res, len(failed(assertions)) == 0, "forced_tool_called", "forced_tool_not_called", map[string]any{"failed": failed(assertions), "function_calls": len(res.v.Calls)})
}

func (r *runner) toolChoiceNone() {
	if !r.live("gemini_tool_choice_none") {
		return
	}
	res := r.probe("gemini_tool_choice_none", KindGenerate, r.generate(toolBody(promptInventory, "NONE", declInventory(), declWeather())))
	r.judge("gemini_tool_choice_none", res, len(res.v.Calls) == 0 && res.v.Text != "", "tools_suppressed", "tool_called_despite_none", map[string]any{"function_calls": len(res.v.Calls), "finish_reason": res.v.Finish})
}

// toolRoundtrip replays the model turn verbatim (thought signatures included,
// which Gemini 3 requires) followed by a functionResponse part.
func (r *runner) toolRoundtrip() {
	if !r.live("gemini_tool_roundtrip") {
		return
	}
	first := r.probe("gemini_tool_roundtrip", KindGenerate, r.generate(toolBody(promptOrder, "AUTO", declOrder())))
	assertions := gradeCall(first.v, "lookup_order", map[string]any{"order_id": "ORD-731"})
	if !first.ok || len(failed(assertions)) > 0 || first.v.Content == nil {
		r.judge("gemini_tool_roundtrip", first, false, "", "function_call_invalid", map[string]any{"failed": failed(assertions), "step": "function_call"})
		return
	}
	call := first.v.Calls[0]
	response := object{"name": call.Name, "response": toolResultOrder()}
	if call.ID != "" {
		response["id"] = call.ID
	}
	follow := toolBody(promptOrder, "AUTO", declOrder())
	follow["contents"] = []any{userText(promptOrder), first.v.Content, object{"role": "user", "parts": []any{object{"functionResponse": response}}}}
	second := r.probe("gemini_tool_roundtrip_result", KindGenerate, r.generate(follow))
	graded, fenced := gradeJSON(second.v.Text, expectedOrder(), true)
	pass := len(failed(graded)) == 0 && second.v.Finish == "STOP" && len(second.v.Calls) == 0
	r.judge("gemini_tool_roundtrip", second, pass, "roundtrip_valid", "roundtrip_answer_invalid", map[string]any{"failed": failed(graded), "finish_reason": second.v.Finish, "fenced_json": fenced})
}

func (r *runner) parallelTools() {
	if !r.live("gemini_parallel_tools") {
		return
	}
	res := r.probe("gemini_parallel_tools", KindGenerate, r.generate(toolBody("What is the weather in Paris and in Tokyo? Call the weather tool once for each city in the same turn.", "AUTO", declWeather())))
	cities := map[string]bool{}
	for _, call := range res.v.Calls {
		if city, _ := call.Args["city"].(string); city != "" {
			cities[strings.ToLower(city)] = true
		}
	}
	pass := len(res.v.Calls) >= 2 && cities["paris"] && cities["tokyo"]
	r.judge("gemini_parallel_tools", res, pass, "parallel_calls_returned", "single_tool_call", map[string]any{"function_calls": len(res.v.Calls)})
}

func (r *runner) toolStream() {
	if !r.live("gemini_tool_stream") {
		return
	}
	res := r.probe("gemini_tool_stream", KindGenerate, r.stream(toolBody(promptInventory, "AUTO", declInventory(), declWeather())))
	assertions := gradeCall(res.v, "lookup_inventory", inventoryArgs())
	r.judge("gemini_tool_stream", res, len(failed(assertions)) == 0, "streamed_function_call_valid", "streamed_function_call_invalid", map[string]any{"failed": failed(assertions), "chunks": len(res.resp.Events)})
}

func (r *runner) jsonMode() {
	if !r.live("gemini_json_mode") {
		return
	}
	b := body(defaultBudget, userText(`Return a JSON object with a single key "ok" set to true.`))
	config(b)["responseMimeType"] = "application/json"
	res := r.probe("gemini_json_mode", KindGenerate, r.generate(b))
	value, _, valid := parseJSONObject(res.v.Text, false)
	ok, _ := value["ok"].(bool)
	r.judge("gemini_json_mode", res, valid && ok, "json_mime_type_honored", "json_mime_type_not_honored", map[string]any{"valid_json": valid})
}

func (r *runner) jsonSchema() {
	if !r.live("gemini_json_schema") {
		return
	}
	b := body(defaultBudget, userText(promptExtract))
	c := config(b)
	c["responseMimeType"], c["responseSchema"] = "application/json", schemaOrder()
	res := r.probe("gemini_json_schema", KindGenerate, r.generate(b))
	graded, _ := gradeJSON(res.v.Text, expectedExtract(), false)
	r.judge("gemini_json_schema", res, len(failed(graded)) == 0, "response_schema_honored", "response_schema_not_honored", map[string]any{"failed": failed(graded)})
}

func (r *runner) vision() {
	if !r.live("gemini_vision") {
		return
	}
	content := object{"role": "user", "parts": []any{
		object{"text": "What is the dominant color of this image? Answer with one lowercase word."},
		object{"inlineData": object{"mimeType": "image/png", "data": redImageBase64()}},
	}}
	res := r.probe("gemini_vision", KindGenerate, r.generate(body(defaultBudget, content)))
	r.judge("gemini_vision", res, strings.Contains(strings.ToLower(res.v.Text), "red"), "image_understood", "image_not_understood", map[string]any{"text": truncate(res.v.Text, 80)})
}

// countTokens counts the baseline prompt; for identical contents the count
// must equal the promptTokenCount generateContent reported.
func (r *runner) countTokens() {
	if !r.live("gemini_count_tokens") {
		return
	}
	res := r.probe("gemini_count_tokens", KindCount, Request{Method: "POST", Path: r.path("countTokens"), Body: map[string]any{"contents": []any{userText("Reply with exactly PONG.")}}})
	total, _ := res.doc["total"].(int64)
	evidence := map[string]any{"total_tokens": total}
	var prompt *int64
	for _, s := range r.report.Samples {
		if s.Probe == "gemini_basic" && s.Usage.Input != nil {
			prompt = s.Usage.Input
		}
	}
	matches := true
	if prompt != nil {
		evidence["generate_prompt_tokens"] = *prompt
		matches = *prompt == total
	}
	r.judge("gemini_count_tokens", res, matches, "count_matches_usage", "count_differs_from_usage", evidence)
}

type assertion struct {
	Name   string
	Passed bool
}

func failed(assertions []assertion) []string {
	var names []string
	for _, a := range assertions {
		if !a.Passed {
			names = append(names, a.Name)
		}
	}
	return names
}

// gradeCall checks one function call: Gemini ends a function-calling turn
// with finishReason STOP and carries args as a JSON object, not a string.
func gradeCall(v view, name string, args map[string]any) []assertion {
	var call functionCall
	if len(v.Calls) == 1 {
		call = v.Calls[0]
	}
	assertions := []assertion{
		{"one_function_call", len(v.Calls) == 1},
		{"function_name", call.Name == name},
		{"finish_reason", v.Finish == "STOP"},
	}
	if args != nil {
		assertions = append(assertions, assertion{"function_args", reflect.DeepEqual(call.Args, args)})
	}
	return assertions
}
