package openaicheck

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"model-check/common"
)

// fakeOpenAI is a small, spec-faithful OpenAI server. Knobs break one contract
// at a time so each check can be shown to catch exactly its own violation.
type fakeOpenAI struct {
	model          string
	reasoning      bool
	noDone         bool
	noUsage        bool
	rejectParams   bool
	toolsAsText    bool
	noToolItemID   bool // function_call_arguments events omit item_id
	wrongToolItem  bool // function_call_arguments events name an unknown item
	statusOverride int
	requests       atomic.Int64
}

func (f *fakeOpenAI) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, object{"object": "list", "data": []any{object{"id": f.model, "object": "model", "created": 1, "owned_by": "openai"}}})
	})
	mux.HandleFunc("/v1/chat/completions", f.chat)
	mux.HandleFunc("/v1/responses", f.responses)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		if f.statusOverride != 0 {
			writeJSON(w, f.statusOverride, object{"error": object{"message": "Incorrect API key provided", "type": "invalid_request_error", "param": nil, "code": "invalid_api_key"}})
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, _ := common.Marshal(v)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-request-id", "req_test")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func badRequest(w http.ResponseWriter, message, param string) {
	writeJSON(w, 400, object{"error": object{"message": message, "type": "invalid_request_error", "param": param, "code": nil}})
}

func lastUser(messages []any) string {
	for i := len(messages) - 1; i >= 0; i-- {
		m, _ := messages[i].(object)
		if str(m, "role") == "user" {
			if s, ok := m["content"].(string); ok {
				return s
			}
			return "<parts>"
		}
	}
	return ""
}

func usageOf(in, out int) object {
	return object{"prompt_tokens": in, "completion_tokens": out, "total_tokens": in + out}
}

type scripted struct {
	text     string
	calls    []object // {name, args}
	finish   string
	outToken int
}

func (f *fakeOpenAI) script(body object) scripted {
	messages := arr(body, "messages")
	prompt := lastUser(messages)
	hasTools := len(arr(body, "tools")) > 0
	choice := body["tool_choice"]
	lastRole := ""
	if len(messages) > 0 {
		m, _ := messages[len(messages)-1].(object)
		lastRole = str(m, "role")
	}
	limit := int64(0)
	for _, key := range []string{"max_completion_tokens", "max_tokens"} {
		if n, ok := num(body, key); ok {
			limit = n
		}
	}
	if rf := obj(body, "response_format"); rf != nil {
		if str(rf, "type") == "json_object" {
			return scripted{text: `{"ok": true}`, finish: "stop", outToken: 5}
		}
		return scripted{text: `{"order_id":"ORD-246","paid":true,"quantity":3,"total":37.5}`, finish: "stop", outToken: 20}
	}
	switch {
	case lastRole == "tool":
		return scripted{text: `{"order_id":"ORD-731","status":"shipped","total":42,"currency":"USD"}`, finish: "stop", outToken: 20}
	case hasTools && choice == "none":
		return scripted{text: "I cannot call tools.", finish: "stop", outToken: 6}
	case hasTools && choice != "auto" && choice != nil:
		return scripted{calls: []object{{"name": "get_weather", "args": `{"city":"Paris"}`}}, finish: "tool_calls", outToken: 12}
	case hasTools && strings.Contains(prompt, "Paris and in Tokyo"):
		return scripted{calls: []object{{"name": "get_weather", "args": `{"city":"Paris"}`}, {"name": "get_weather", "args": `{"city":"Tokyo"}`}}, finish: "tool_calls", outToken: 24}
	case hasTools && strings.Contains(prompt, "SKU-204"):
		return scripted{calls: []object{{"name": "lookup_inventory", "args": `{"sku":"SKU-204","warehouse":"north","include_reserved":false}`}}, finish: "tool_calls", outToken: 20}
	case hasTools && strings.Contains(prompt, "ORD-731"):
		return scripted{calls: []object{{"name": "lookup_order", "args": `{"order_id":"ORD-731"}`}}, finish: "tool_calls", outToken: 14}
	case strings.Contains(prompt, "CHECK_STOP"):
		return scripted{text: "ALPHA ", finish: "stop", outToken: 2}
	case strings.Contains(prompt, "long numbered list") && limit > 0 && limit <= 16:
		return scripted{text: "1, 2, 3", finish: "length", outToken: int(limit)}
	case strings.Contains(prompt, "Return only the exact code"):
		for _, raw := range messages {
			m, _ := raw.(object)
			if s, _ := m["content"].(string); strings.HasPrefix(s, "Remember this code: ") {
				return scripted{text: strings.TrimPrefix(s, "Remember this code: "), finish: "stop", outToken: 4}
			}
		}
	case strings.Contains(prompt, "dominant color") || prompt == "<parts>":
		return scripted{text: "red", finish: "stop", outToken: 1}
	case strings.Contains(prompt, "Count from 1 to 5"):
		return scripted{text: "1, 2, 3, 4, 5", finish: "stop", outToken: 9}
	case strings.Contains(prompt, "Name a color"):
		return scripted{text: "Blue", finish: "stop", outToken: 1}
	}
	for _, raw := range messages {
		m, _ := raw.(object)
		if str(m, "role") == "system" && strings.Contains(str(m, "content"), "ORCHID") {
			return scripted{text: "ORCHID", finish: "stop", outToken: 2}
		}
	}
	return scripted{text: "PONG", finish: "stop", outToken: 2}
}

func (f *fakeOpenAI) validateInput(w http.ResponseWriter, body object) bool {
	if f.rejectParams && (has(body, "seed") || has(body, "user")) {
		badRequest(w, "Unrecognized request argument supplied: seed", "seed")
		return false
	}
	if f.reasoning && (has(body, "temperature") || has(body, "top_p") || has(body, "presence_penalty") || has(body, "frequency_penalty")) {
		badRequest(w, "Unsupported parameter: 'temperature' is not supported with this model.", "temperature")
		return false
	}
	if f.reasoning && has(body, "logprobs") {
		badRequest(w, "logprobs is not supported", "logprobs")
		return false
	}
	return true
}

func (f *fakeOpenAI) chat(w http.ResponseWriter, r *http.Request) {
	var body object
	data, _ := io.ReadAll(r.Body)
	if common.Unmarshal(data, &body) != nil {
		badRequest(w, "invalid json", "")
		return
	}
	if !has(body, "messages") {
		badRequest(w, "Missing required parameter: 'messages'.", "messages")
		return
	}
	if !f.validateInput(w, body) {
		return
	}
	s := f.script(body)
	if f.toolsAsText && len(s.calls) > 0 {
		s = scripted{text: "I would call " + str(s.calls[0], "name"), finish: "stop", outToken: 8}
	}
	if stream, _ := body["stream"].(bool); stream {
		f.chatStream(w, body, s)
		return
	}
	message := object{"role": "assistant", "content": s.text, "refusal": nil}
	if len(s.calls) > 0 {
		message["content"] = nil
		var calls []any
		for i, c := range s.calls {
			calls = append(calls, object{"id": fmt.Sprintf("call_%d", i), "type": "function", "function": object{"name": c["name"], "arguments": c["args"]}})
		}
		message["tool_calls"] = calls
	}
	choice := object{"index": 0, "message": message, "finish_reason": s.finish}
	if has(body, "logprobs") {
		choice["logprobs"] = object{"content": []any{object{"token": "PONG", "logprob": -0.1, "top_logprobs": []any{object{"token": "PONG", "logprob": -0.1}}}}}
	}
	choices := []any{choice}
	if n, _ := num(body, "n"); n == 2 {
		second := object{"index": 1, "message": object{"role": "assistant", "content": "Green", "refusal": nil}, "finish_reason": "stop"}
		choices = append(choices, second)
	}
	resp := object{"id": "chatcmpl-1", "object": "chat.completion", "created": 1, "model": f.model + "-2024-08-06", "system_fingerprint": "fp_test", "choices": choices}
	if !f.noUsage {
		resp["usage"] = usageOf(12, s.outToken)
	}
	writeJSON(w, 200, resp)
}

func (f *fakeOpenAI) chatStream(w http.ResponseWriter, body object, s scripted) {
	w.Header().Set("Content-Type", "text/event-stream")
	flush := w.(http.Flusher)
	send := func(v any) {
		data, _ := common.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flush.Flush()
	}
	chunk := func(delta object, finish any) object {
		return object{"id": "chatcmpl-1", "object": "chat.completion.chunk", "created": 1, "model": f.model + "-2024-08-06", "choices": []any{object{"index": 0, "delta": delta, "finish_reason": finish}}}
	}
	send(chunk(object{"role": "assistant", "content": ""}, nil))
	if len(s.calls) > 0 {
		for i, c := range s.calls {
			send(chunk(object{"tool_calls": []any{object{"index": i, "id": fmt.Sprintf("call_%d", i), "type": "function", "function": object{"name": c["name"], "arguments": ""}}}}, nil))
			args := str(c, "args")
			half := len(args) / 2
			for _, part := range []string{args[:half], args[half:]} {
				send(chunk(object{"tool_calls": []any{object{"index": i, "function": object{"arguments": part}}}}, nil))
			}
		}
	} else {
		for _, part := range strings.SplitAfter(s.text, ",") {
			send(chunk(object{"content": part}, nil))
		}
	}
	send(chunk(object{}, s.finish))
	if include := obj(body, "stream_options"); include != nil && include["include_usage"] == true && !f.noUsage {
		send(object{"id": "chatcmpl-1", "object": "chat.completion.chunk", "created": 1, "model": f.model, "choices": []any{}, "usage": usageOf(12, s.outToken)})
	}
	if !f.noDone {
		fmt.Fprint(w, "data: [DONE]\n\n")
	}
}

func (f *fakeOpenAI) responses(w http.ResponseWriter, r *http.Request) {
	var body object
	data, _ := io.ReadAll(r.Body)
	_ = common.Unmarshal(data, &body)
	if !has(body, "input") {
		badRequest(w, "Missing required parameter: 'input'.", "input")
		return
	}
	input := body["input"]
	prompt, _ := input.(string)
	var items []any
	if list, ok := input.([]any); ok {
		items = list
		prompt = lastUser(list)
		for _, raw := range list {
			if m, _ := raw.(object); str(m, "type") == "function_call_output" {
				prompt = "TOOLRESULT"
			}
		}
	}
	_ = items
	var output []any
	text, status := "PONG", "completed"
	var incomplete any
	switch {
	case prompt == "TOOLRESULT":
		text = `{"order_id":"ORD-731","status":"shipped","total":42,"currency":"USD"}`
	case len(arr(body, "tools")) > 0 && strings.Contains(prompt, "SKU-204"):
		output = []any{object{"type": "function_call", "id": "fc_1", "call_id": "call_1", "name": "lookup_inventory", "arguments": `{"sku":"SKU-204","warehouse":"north","include_reserved":false}`, "status": "completed"}}
	case len(arr(body, "tools")) > 0 && strings.Contains(prompt, "ORD-731"):
		output = []any{object{"type": "function_call", "id": "fc_1", "call_id": "call_1", "name": "lookup_order", "arguments": `{"order_id":"ORD-731"}`, "status": "completed"}}
	case strings.Contains(prompt, "long numbered list"):
		text, status, incomplete = "1, 2, 3", "incomplete", object{"reason": "max_output_tokens"}
	case obj(obj(body, "text"), "format") != nil:
		text = `{"order_id":"ORD-246","paid":true,"quantity":3,"total":37.5}`
	case strings.Contains(prompt, "Count from 1 to 5"):
		text = "1, 2, 3, 4, 5"
	case strings.Contains(prompt, "Remember this code: "):
		text = "OK"
		f.remember(strings.TrimSuffix(strings.TrimPrefix(prompt, "Remember this code: "), ". Reply with OK."))
	case has(body, "previous_response_id"):
		text = f.remembered()
	}
	if output == nil {
		output = []any{object{"type": "message", "id": "msg_1", "status": "completed", "role": "assistant", "content": []any{object{"type": "output_text", "text": text, "annotations": []any{}}}}}
	}
	response := object{"id": "resp_1", "object": "response", "created_at": 1, "status": status, "incomplete_details": incomplete, "model": f.model + "-2024-08-06", "output": output, "usage": object{"input_tokens": 12, "output_tokens": 5, "total_tokens": 17}}
	if stream, _ := body["stream"].(bool); stream {
		w.Header().Set("Content-Type", "text/event-stream")
		flush := w.(http.Flusher)
		seq := 0
		send := func(kind string, extra object) {
			extra["type"] = kind
			extra["sequence_number"] = seq
			seq++
			data, _ := common.Marshal(extra)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data)
			flush.Flush()
		}
		send("response.created", object{"response": object{"id": "resp_1", "object": "response", "status": "in_progress"}})
		if call, _ := output[0].(object); str(call, "type") == "function_call" {
			itemID := func(extra object) object {
				switch {
				case f.noToolItemID:
				case f.wrongToolItem:
					extra["item_id"] = "fc_unknown"
				default:
					extra["item_id"] = str(call, "id")
				}
				return extra
			}
			added := object{"id": call["id"], "type": "function_call", "call_id": call["call_id"], "name": call["name"], "arguments": "", "status": "in_progress"}
			send("response.output_item.added", object{"output_index": 0, "item": added})
			args := str(call, "arguments")
			for _, part := range []string{args[:len(args)/2], args[len(args)/2:]} {
				send("response.function_call_arguments.delta", itemID(object{"output_index": 0, "delta": part}))
			}
			send("response.function_call_arguments.done", itemID(object{"output_index": 0, "arguments": args}))
			send("response.output_item.done", object{"output_index": 0, "item": call})
			send("response.completed", object{"response": response})
			return
		}
		for _, part := range strings.SplitAfter(text, ",") {
			send("response.output_text.delta", object{"item_id": "msg_1", "output_index": 0, "content_index": 0, "delta": part})
		}
		send("response.completed", object{"response": response})
		return
	}
	writeJSON(w, 200, response)
}

var remembered atomic.Value

func (f *fakeOpenAI) remember(code string) { remembered.Store(code) }
func (f *fakeOpenAI) remembered() string {
	v, _ := remembered.Load().(string)
	return v
}

// httpTransport is a minimal Transport for tests, mirroring the controller's.
func httpTransport(base string) Transport {
	client := &http.Client{Timeout: 10 * time.Second}
	return func(ctx context.Context, req Request) (Response, error) {
		var reader io.Reader
		if req.Body != nil {
			data, _ := common.Marshal(req.Body)
			reader = bytes.NewReader(data)
		}
		httpReq, err := http.NewRequestWithContext(ctx, req.Method, base+req.Path, reader)
		if err != nil {
			return Response{}, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		start := time.Now()
		resp, err := client.Do(httpReq)
		if err != nil {
			return Response{}, err
		}
		defer resp.Body.Close()
		out := Response{Status: resp.StatusCode, Header: resp.Header}
		if req.Stream && resp.StatusCode == 200 {
			out.Events, out.Done, out.FirstEventMS, err = ReadSSE(resp.Body, start)
			return out, err
		}
		out.Body, err = io.ReadAll(resp.Body)
		return out, err
	}
}

func runAgainst(t *testing.T, f *fakeOpenAI, options Options) Report {
	t.Helper()
	server := httptest.NewServer(f.handler())
	t.Cleanup(server.Close)
	return Run(context.Background(), options, httpTransport(server.URL))
}

func checkByID(report Report, id string) Check {
	for _, c := range report.Checks {
		if c.ID == id {
			return c
		}
	}
	return Check{}
}
