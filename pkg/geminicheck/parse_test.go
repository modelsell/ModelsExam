package geminicheck

import (
	"strconv"
	"testing"

	"model-check/common"
)

func hasIssue(issues []string, code string) bool {
	for _, issue := range issues {
		if issue == code {
			return true
		}
	}
	return false
}

const officialResponse = `{"candidates":[{"content":{"parts":[{"text":"PONG"}],"role":"model"},"finishReason":"STOP","avgLogprobs":-0.01}],
"usageMetadata":{"promptTokenCount":6,"candidatesTokenCount":1,"totalTokenCount":27,"thoughtsTokenCount":20,"promptTokensDetails":[{"modality":"TEXT","tokenCount":6}]},
"modelVersion":"gemini-2.5-flash","responseId":"abc123"}`

func mutate(base string, change func(object)) []byte {
	doc, _ := parseObject([]byte(base))
	change(doc)
	data, _ := common.Marshal(doc)
	return data
}

func candidate0(d object) object { return arr(d, "candidates")[0].(object) }

func TestGenerateResponseFollowsTheDocumentedShape(t *testing.T) {
	v, issues := parseGenerate([]byte(officialResponse))
	if len(issues) != 0 {
		t.Fatalf("official response rejected: %v", issues)
	}
	if v.Text != "PONG" || v.Finish != "STOP" || *v.Usage.Input != 6 || *v.Usage.Output != 21 || *v.Usage.Thoughts != 20 {
		t.Fatalf("response not read: %+v", v)
	}
	for code, change := range map[string]func(object){
		"openai_envelope":          func(d object) { d["choices"] = []any{} },
		"missing_candidates":       func(d object) { delete(d, "candidates") },
		"missing_model_version":    func(d object) { delete(d, "modelVersion") },
		"missing_response_id":      func(d object) { delete(d, "responseId") },
		"invalid_finish_reason":    func(d object) { candidate0(d)["finishReason"] = "stop" },
		"missing_finish_reason":    func(d object) { delete(candidate0(d), "finishReason") },
		"missing_content":          func(d object) { delete(candidate0(d), "content") },
		"role_not_model":           func(d object) { obj(candidate0(d), "content")["role"] = "assistant" },
		"empty_parts":              func(d object) { obj(candidate0(d), "content")["parts"] = []any{} },
		"bad_candidate_index":      func(d object) { candidate0(d)["index"] = 1 },
		"text_not_string":          func(d object) { obj(candidate0(d), "content")["parts"] = []any{object{"text": 1}} },
		"missing_usage_metadata":   func(d object) { delete(d, "usageMetadata") },
		"missing_promptTokenCount": func(d object) { delete(obj(d, "usageMetadata"), "promptTokenCount") },
		"missing_totalTokenCount":  func(d object) { delete(obj(d, "usageMetadata"), "totalTokenCount") },
		"negative_candidatesTokenCount": func(d object) {
			obj(d, "usageMetadata")["candidatesTokenCount"] = -1
		},
		"usage_total_mismatch":  func(d object) { obj(d, "usageMetadata")["totalTokenCount"] = 7 },
		"cached_exceeds_prompt": func(d object) { obj(d, "usageMetadata")["cachedContentTokenCount"] = 9 },
	} {
		if _, issues := parseGenerate(mutate(officialResponse, change)); !hasIssue(issues, code) {
			t.Errorf("%s not reported: %v", code, issues)
		}
	}
}

func TestGenerateResponseToleratesDocumentedVariations(t *testing.T) {
	for name, change := range map[string]func(object){
		// Proto JSON omits zero counters, and a truncated or blocked candidate may
		// have no content.
		"zero candidates omitted": func(d object) {
			u := obj(d, "usageMetadata")
			delete(u, "candidatesTokenCount")
			u["totalTokenCount"] = 26
		},
		"max tokens without content": func(d object) {
			candidate0(d)["finishReason"] = "MAX_TOKENS"
			delete(candidate0(d), "content")
		},
		"newer finish reason": func(d object) { candidate0(d)["finishReason"] = "MISSING_THOUGHT_SIGNATURE" },
		"thought parts excluded": func(d object) {
			obj(candidate0(d), "content")["parts"] = []any{object{"text": "hmm", "thought": true}, object{"text": "PONG"}}
		},
		"blocked prompt": func(d object) { delete(d, "candidates"); d["promptFeedback"] = object{"blockReason": "SAFETY"} },
		"tool use prompt counted": func(d object) {
			u := obj(d, "usageMetadata")
			u["toolUsePromptTokenCount"] = 3
			u["totalTokenCount"] = 30
		},
	} {
		v, issues := parseGenerate(mutate(officialResponse, change))
		if len(issues) != 0 {
			t.Errorf("%s rejected: %v", name, issues)
		}
		if name == "thought parts excluded" && (v.Text != "PONG" || v.Thoughts != 1) {
			t.Errorf("thought part leaked into text: %+v", v)
		}
	}
}

func TestFunctionCallParts(t *testing.T) {
	body := `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"Paris"}},"thoughtSignature":"c2ln"}]},"finishReason":"STOP"}],
"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15},"modelVersion":"gemini-x","responseId":"r"}`
	v, issues := parseGenerate([]byte(body))
	if len(issues) != 0 || len(v.Calls) != 1 || v.Calls[0].Name != "get_weather" || v.Calls[0].Args["city"] != "Paris" {
		t.Fatalf("function call not read: %v %+v", issues, v)
	}
	// The model turn is kept verbatim so a round trip can replay the signature.
	parts := arr(v.Content, "parts")
	if len(parts) != 1 || str(parts[0].(object), "thoughtSignature") != "c2ln" {
		t.Errorf("thought signature not kept: %v", v.Content)
	}
	for code, change := range map[string]func(object){
		"function_call_missing_name": func(d object) {
			delete(obj(arr(obj(candidate0(d), "content"), "parts")[0].(object), "functionCall"), "name")
		},
		"function_call_args_not_object": func(d object) {
			obj(arr(obj(candidate0(d), "content"), "parts")[0].(object), "functionCall")["args"] = `{"city":"Paris"}`
		},
	} {
		if _, issues := parseGenerate(mutate(body, change)); !hasIssue(issues, code) {
			t.Errorf("%s not reported: %v", code, issues)
		}
	}
}

func sse(lines ...string) []SSEEvent {
	out := make([]SSEEvent, len(lines))
	for i, line := range lines {
		out[i] = SSEEvent{Data: []byte(line)}
	}
	return out
}

func streamChunk(text, finish string, usage bool, id string) string {
	c := `{"content":{"role":"model","parts":[{"text":` + strconv.Quote(text) + `}]}`
	if finish != "" {
		c += `,"finishReason":"` + finish + `"`
	}
	c += "}"
	doc := `{"candidates":[` + c + `],"modelVersion":"gemini-x","responseId":"` + id + `"`
	if usage {
		doc += `,"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":3,"totalTokenCount":7}`
	}
	return doc + "}"
}

func TestStreamChunksFollowTheDocumentedShape(t *testing.T) {
	a, b := streamChunk("1, 2", "", false, "r1"), streamChunk(", 3", "STOP", true, "r1")
	v, issues := parseStream(sse(a, b), false)
	if len(issues) != 0 || len(v.UsageIssues) != 0 || v.Text != "1, 2, 3" || v.Finish != "STOP" || !v.UsageSeen {
		t.Fatalf("stream rejected: %v %+v", issues, v)
	}
	for code, stream := range map[string][]SSEEvent{
		"missing_finish_reason":       sse(a, streamChunk(", 3", "", true, "r1")),
		"multiple_finish_reasons":     sse(streamChunk("1", "STOP", false, "r1"), b),
		"content_after_finish_reason": sse(streamChunk("1", "STOP", false, "r1"), streamChunk("2", "", true, "r1")),
		"inconsistent_response_ids":   sse(a, streamChunk(", 3", "STOP", true, "r2")),
		"invalid_finish_reason":       sse(a, streamChunk(", 3", "stop", true, "r1")),
		"openai_envelope":             sse(`{"object":"chat.completion.chunk","choices":[]}`, b),
		"error_event":                 sse(a, `{"error":{"code":500,"message":"x","status":"INTERNAL"}}`),
		"invalid_chunk_json":          sse(a, `{"candidates":`, b),
		"empty_stream":                nil,
	} {
		if _, issues := parseStream(stream, false); !hasIssue(issues, code) {
			t.Errorf("%s not reported: %v", code, issues)
		}
	}
	if _, issues := parseStream(sse(a, b), true); !hasIssue(issues, "unexpected_done_marker") {
		t.Errorf("[DONE] accepted: %v", issues)
	}
	if v, _ := parseStream(sse(a, streamChunk(", 3", "STOP", false, "r1")), false); !hasIssue(v.UsageIssues, "missing_usage_metadata") {
		t.Errorf("missing usage accepted: %v", v.UsageIssues)
	}
	usageFirst := streamChunk("1, 2", "", true, "r1")
	if v, _ := parseStream(sse(usageFirst, streamChunk(", 3", "STOP", false, "r1")), false); !hasIssue(v.UsageIssues, "usage_not_in_final_chunk") {
		t.Errorf("early usage accepted: %v", v.UsageIssues)
	}
}

func TestErrorEnvelope(t *testing.T) {
	official := `{"error":{"code":400,"message":"* GenerateContentRequest.contents: contents is not specified","status":"INVALID_ARGUMENT"}}`
	if _, status, issues := parseErrorShape(400, []byte(official)); len(issues) != 0 || status != "INVALID_ARGUMENT" {
		t.Fatalf("official error rejected: %v", issues)
	}
	for code, tc := range map[string]struct {
		status int
		body   string
	}{
		"error_code_differs_from_http_status":   {400, `{"error":{"code":500,"message":"x","status":"INVALID_ARGUMENT"}}`},
		"error_code_not_integer":                {400, `{"error":{"code":"invalid_request","message":"x","status":"INVALID_ARGUMENT"}}`},
		"error_status_differs_from_http_status": {400, `{"error":{"code":400,"message":"x","status":"NOT_FOUND"}}`},
		"missing_error_status":                  {400, `{"error":{"code":400,"message":"x"}}`},
		"invalid_error_status":                  {400, `{"error":{"code":400,"message":"x","status":"invalid_argument"}}`},
		"missing_error_message":                 {400, `{"error":{"code":400,"status":"INVALID_ARGUMENT"}}`},
		"openai_error_fields":                   {400, `{"error":{"code":400,"message":"x","status":"INVALID_ARGUMENT","type":"invalid_request_error","param":null}}`},
		"missing_error_object":                  {400, `{"message":"x"}`},
		"error_body_not_json":                   {400, `Bad Request`},
		"unexpected_status_500":                 {500, `{"error":{"code":500,"message":"x","status":"INTERNAL"}}`},
	} {
		if _, _, issues := parseErrorShape(tc.status, []byte(tc.body)); !hasIssue(issues, code) {
			t.Errorf("%s not reported: %v", code, issues)
		}
	}
}

func TestModelResourceAndCountTokens(t *testing.T) {
	model := `{"name":"models/gemini-x","version":"001","inputTokenLimit":1048576,"outputTokenLimit":65536,"supportedGenerationMethods":["generateContent","countTokens"]}`
	if methods, issues := parseModel([]byte(model), "gemini-x"); len(issues) != 0 || len(methods) != 2 {
		t.Fatalf("model rejected: %v", issues)
	}
	if _, issues := parseModel([]byte(model), "gemini-y"); !hasIssue(issues, "model_name_mismatch") {
		t.Errorf("wrong name accepted: %v", issues)
	}
	if _, issues := parseModel([]byte(`{"name":"models/gemini-x"}`), "gemini-x"); !hasIssue(issues, "missing_supported_generation_methods") || !hasIssue(issues, "invalid_inputTokenLimit") {
		t.Errorf("bare model accepted: %v", issues)
	}
	if n, issues := parseCount([]byte(`{"totalTokens":6,"promptTokensDetails":[{"modality":"TEXT","tokenCount":6}]}`)); n != 6 || len(issues) != 0 {
		t.Errorf("count rejected: %d %v", n, issues)
	}
	if _, issues := parseCount([]byte(`{"total_tokens":6}`)); !hasIssue(issues, "missing_totalTokens") {
		t.Errorf("wrong count field accepted: %v", issues)
	}
}

func TestFailureCodeRecognisesInvalidKey(t *testing.T) {
	body := []byte(`{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.","status":"INVALID_ARGUMENT","details":[{"reason":"API_KEY_INVALID"}]}}`)
	if got := FailureCode(400, body, nil); got != "unauthorized" {
		t.Errorf("invalid key = %s", got)
	}
	if got := FailureCode(400, []byte(`{"error":{"code":400}}`), nil); got != "request_rejected" {
		t.Errorf("plain 400 = %s", got)
	}
}
