package openaicheck

import (
	"strconv"
	"testing"

	"model-check/common"
)

// Basic envelope, stream and error-shape checks for Chat Completions and
// Responses. Each case breaks one documented field and expects its own code.

func mutated(base string, mutate func(object)) []byte {
	doc, _ := parseObject([]byte(base))
	if mutate != nil {
		mutate(doc)
	}
	data, _ := common.Marshal(doc)
	return data
}

func expectClean(t *testing.T, what string, issues []string) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("conforming %s rejected: %v", what, issues)
	}
}

func expectIssue(t *testing.T, name string, issues []string, code string) {
	t.Helper()
	if !hasIssue(issues, code) {
		t.Errorf("%s: want %s, got %v", name, code, issues)
	}
}

func choice0(doc object) object { return arr(doc, "choices")[0].(object) }

func TestChatEnvelopeRequiresDocumentedFields(t *testing.T) {
	base := string(chat("stop", ""))
	expectClean(t, "chat.completion", func() []string { _, i := parseChat([]byte(base)); return i }())
	for code, mutate := range map[string]func(object){
		"object_not_chat_completion": func(d object) { d["object"] = "text_completion" },
		"missing_id":                 func(d object) { delete(d, "id") },
		"missing_created":            func(d object) { d["created"] = "today" },
		"missing_model":              func(d object) { delete(d, "model") },
		"missing_choices":            func(d object) { d["choices"] = []any{} },
		"bad_choice_index":           func(d object) { choice0(d)["index"] = 1 },
		"missing_message":            func(d object) { delete(choice0(d), "message") },
		"missing_finish_reason":      func(d object) { choice0(d)["finish_reason"] = nil },
		"role_not_assistant":         func(d object) { obj(choice0(d), "message")["role"] = "user" },
		"missing_message_content":    func(d object) { delete(obj(choice0(d), "message"), "content") },
		"missing_usage":              func(d object) { delete(d, "usage") },
		"missing_total_tokens":       func(d object) { delete(obj(d, "usage"), "total_tokens") },
		"negative_prompt_tokens":     func(d object) { obj(d, "usage")["prompt_tokens"] = -1 },
		"usage_total_mismatch":       func(d object) { obj(d, "usage")["total_tokens"] = 99 },
	} {
		_, issues := parseChat(mutated(base, mutate))
		expectIssue(t, code, issues, code)
	}
	if _, issues := parseChat([]byte("not json")); !hasIssue(issues, "invalid_json") {
		t.Errorf("non-JSON body accepted: %v", issues)
	}
}

func TestChatToolCallMessageAllowsNullContent(t *testing.T) {
	body := `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"gpt-x","choices":[{"index":0,"finish_reason":"tool_calls",
"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Paris\"}"}}]}}],
"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`
	v, issues := parseChat([]byte(body))
	expectClean(t, "tool call message", issues)
	if len(v.Tools) != 1 || v.Tools[0].ID != "call_1" || v.Tools[0].Name != "get_weather" || v.Tools[0].Arguments != `{"city":"Paris"}` {
		t.Errorf("tool call not read: %+v", v.Tools)
	}
}

func events(lines ...string) []SSEEvent {
	out := make([]SSEEvent, len(lines))
	for i, line := range lines {
		out[i] = SSEEvent{Data: []byte(line)}
	}
	return out
}

const (
	chunkRole   = `{"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-x","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`
	chunkText   = `{"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-x","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`
	chunkFinish = `{"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-x","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`
	chunkUsage  = `{"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-x","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`
)

func TestChatStreamChunksFollowTheDocumentedShape(t *testing.T) {
	v, issues := parseChatStream(events(chunkRole, chunkText, chunkFinish, chunkUsage), true)
	expectClean(t, "chunk stream", issues)
	if len(v.UsageIssues) != 0 || !v.UsageSeen || v.Text != "Hello" || v.Finish != "stop" || *v.Usage.Total != 6 {
		t.Fatalf("stream not assembled: %+v", v)
	}
	usageWithChoice := `{"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-x","choices":[{"index":0,"delta":{},"finish_reason":null}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`
	for code, stream := range map[string][]SSEEvent{
		"object_not_chunk":             events(chunkRole, `{"id":"c1","object":"chat.completion","choices":[{"index":0,"delta":{"content":"x"}}]}`, chunkFinish),
		"inconsistent_chunk_ids":       events(chunkRole, `{"id":"c2","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"x"}}]}`, chunkFinish),
		"multiple_finish_reasons":      events(chunkRole, chunkFinish, chunkFinish),
		"missing_finish_reason":        events(chunkRole, chunkText),
		"missing_assistant_role_delta": events(chunkText, chunkFinish),
		"unknown_finish_reason":        events(chunkRole, `{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"end_turn"}]}`),
		"invalid_chunk_json":           events(chunkRole, `{"id":`, chunkFinish),
		"error_event":                  events(chunkRole, `{"error":{"message":"overloaded","type":"server_error"}}`),
		"empty_stream":                 nil,
	} {
		_, issues := parseChatStream(stream, true)
		expectIssue(t, code, issues, code)
	}
	if _, issues := parseChatStream(events(chunkRole, chunkFinish), false); !hasIssue(issues, "missing_done_marker") {
		t.Errorf("missing [DONE] accepted: %v", issues)
	}
	for code, stream := range map[string][]SSEEvent{
		"usage_chunk_has_choices": events(chunkRole, chunkFinish, usageWithChoice),
		"usage_chunk_not_last":    events(chunkRole, chunkUsage, chunkFinish),
	} {
		v, _ := parseChatStream(stream, true)
		expectIssue(t, code, v.UsageIssues, code)
	}
}

func TestChatStreamToolCallDeltas(t *testing.T) {
	first := `{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`
	argsA := `{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}`
	argsB := `{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Paris\"}"}}]}}]}`
	finish := `{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`
	v, issues := parseChatStream(events(first, argsA, argsB, finish), true)
	expectClean(t, "tool call stream", issues)
	if len(v.Tools) != 1 || v.Tools[0].ID != "call_1" || v.Tools[0].Arguments != `{"city":"Paris"}` {
		t.Fatalf("tool call deltas not assembled: %+v", v.Tools)
	}
	anonymous := `{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"function":{"arguments":"{}"}}]}}]}`
	_, issues = parseChatStream(events(anonymous, finish), true)
	expectIssue(t, "first delta without id/name", issues, "tool_call_first_delta_incomplete")
}

func TestErrorEnvelopeFields(t *testing.T) {
	official := `{"error":{"message":"Invalid value for 'temperature'","type":"invalid_request_error","param":"temperature","code":null}}`
	message, issues := parseErrorShape(400, []byte(official))
	expectClean(t, "error envelope", issues)
	if message == "" {
		t.Error("error message not read")
	}
	for code, tc := range map[string]struct {
		status int
		body   string
	}{
		"missing_error_code_key":  {400, `{"error":{"message":"bad","type":"invalid_request_error","param":null}}`},
		"missing_error_param_key": {400, `{"error":{"message":"bad","type":"invalid_request_error","code":null}}`},
		"missing_error_type":      {400, `{"error":{"message":"bad","param":null,"code":null}}`},
		"missing_error_message":   {400, `{"error":{"type":"invalid_request_error","param":null,"code":null}}`},
		"missing_error_object":    {400, `{"message":"bad"}`},
		"error_body_not_json":     {400, `<html>Bad Request</html>`},
		"unexpected_status_200":   {200, official},
		"unexpected_status_500":   {500, official},
	} {
		_, issues := parseErrorShape(tc.status, []byte(tc.body))
		expectIssue(t, code, issues, code)
	}
}

const responseBody = `{"id":"resp_1","object":"response","created_at":1,"model":"gpt-x","status":"completed",
"output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello","annotations":[]}]}],
"usage":{"input_tokens":4,"output_tokens":1,"total_tokens":5}}`

func TestResponsesObjectRequiresDocumentedFields(t *testing.T) {
	v, issues := parseResponses([]byte(responseBody))
	expectClean(t, "response", issues)
	if v.Text != "Hello" || v.Finish != "completed" {
		t.Fatalf("response not read: %+v", v)
	}
	for code, mutate := range map[string]func(object){
		"object_not_response": func(d object) { d["object"] = "chat.completion" },
		"missing_id":          func(d object) { delete(d, "id") },
		"missing_model":       func(d object) { delete(d, "model") },
		"missing_created_at":  func(d object) { delete(d, "created_at") },
		"missing_status":      func(d object) { delete(d, "status") },
		"missing_output":      func(d object) { d["output"] = nil },
		"missing_usage":       func(d object) { delete(d, "usage") },
		"role_not_assistant":  func(d object) { arr(d, "output")[0].(object)["role"] = "user" },
	} {
		_, issues := parseResponses(mutated(responseBody, mutate))
		expectIssue(t, code, issues, code)
	}
	// Usage is only required once the response has finished.
	_, issues = parseResponses(mutated(responseBody, func(d object) { d["status"] = "in_progress"; delete(d, "usage") }))
	expectClean(t, "in_progress response without usage", issues)
	incomplete := mutated(responseBody, func(d object) {
		d["status"], d["incomplete_details"] = "incomplete", object{"reason": "max_output_tokens"}
	})
	if v, _ := parseResponses(incomplete); v.Finish != "incomplete:max_output_tokens" {
		t.Errorf("incomplete reason not surfaced: %q", v.Finish)
	}
	refusal := mutated(responseBody, func(d object) {
		arr(d, "output")[0].(object)["content"] = []any{object{"type": "refusal", "refusal": "I can't help with that."}}
	})
	if v, _ := parseResponses(refusal); v.Refusal == "" || v.Text != "" {
		t.Errorf("refusal part not read: %+v", v)
	}
}

func responseEvent(kind string, seq int, extra string) SSEEvent {
	return SSEEvent{Name: kind, Data: []byte(`{"type":"` + kind + `","sequence_number":` + strconv.Itoa(seq) + extra + `}`)}
}

func TestResponsesStreamLifecycle(t *testing.T) {
	created := responseEvent("response.created", 0, `,"response":{"id":"resp_1","object":"response","status":"in_progress"}`)
	deltaA := responseEvent("response.output_text.delta", 1, `,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"Hel"`)
	deltaB := responseEvent("response.output_text.delta", 2, `,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"lo"`)
	completed := responseEvent("response.completed", 3, `,"response":`+responseBody)
	v, issues := parseResponsesStream([]SSEEvent{created, deltaA, deltaB, completed})
	expectClean(t, "response stream", issues)
	if v.Text != "Hello" {
		t.Fatalf("stream text = %q", v.Text)
	}

	renamed := deltaA
	renamed.Name = "response.output_text.done"
	wrongText := responseEvent("response.output_text.delta", 2, `,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"p!"`)
	for code, stream := range map[string][]SSEEvent{
		"missing_response_created":        {deltaA, deltaB, completed},
		"missing_terminal_event":          {created, deltaA, deltaB},
		"event_after_terminal":            {created, deltaA, deltaB, completed, deltaB},
		"sequence_not_increasing":         {created, deltaB, deltaA, completed},
		"event_name_type_mismatch":        {created, renamed, deltaB, completed},
		"delta_text_differs_from_final":   {created, deltaA, wrongText, completed},
		"missing_text_delta_events":       {created, completed},
		"terminal_event_missing_response": {created, deltaA, deltaB, responseEvent("response.completed", 3, "")},
		"error_event":                     {created, responseEvent("response.failed", 1, `,"response":{"status":"failed"}`)},
		"missing_event_type":              {created, {Data: []byte(`{"sequence_number":1}`)}, completed},
		"invalid_event_json":              {created, {Data: []byte(`{"type":`)}, deltaA, deltaB, completed},
		"empty_stream":                    nil,
	} {
		_, issues := parseResponsesStream(stream)
		expectIssue(t, code, issues, code)
	}
}
