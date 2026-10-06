package geminicheck

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"model-check/common"
)

const testKey = "AIza-test-key-123456"

// fakeGemini is a small, spec-faithful Gemini API server. Knobs break one
// contract at a time so each check can be shown to catch its own violation.
type fakeGemini struct {
	model        string
	lowerFinish  bool // finishReason "stop" instead of "STOP"
	doneMarker   bool // append an OpenAI-style [DONE] frame to streams
	noUsage      bool
	badKey       bool // reply like Gemini does to an invalid key
	openAIErrors bool // OpenAI error envelope for 4xx
	stringArgs   bool // functionCall.args as a JSON string (OpenAI style)
	countOffset  int64
	sawKeyHeader atomic.Bool
	sawQueryKey  atomic.Bool
	requests     atomic.Int64
}

func (f *fakeGemini) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		if r.Header.Get("x-goog-api-key") == testKey {
			f.sawKeyHeader.Store(true)
		}
		if r.URL.Query().Get("key") != "" {
			f.sawQueryKey.Store(true)
		}
		if f.badKey {
			writeError(w, 400, "API key not valid. Please pass a valid API key.", "INVALID_ARGUMENT", `,"details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"API_KEY_INVALID"}]`)
			return
		}
		prefix := "/v1beta/models/" + f.model
		switch {
		case r.Method == http.MethodGet && r.URL.Path == prefix:
			writeJSON(w, 200, object{"name": "models/" + f.model, "baseModelId": f.model, "version": "001", "displayName": "Gemini Test", "inputTokenLimit": 1048576, "outputTokenLimit": 65536, "supportedGenerationMethods": []string{"generateContent", "countTokens"}})
		case r.URL.Path == prefix+":generateContent":
			f.generate(w, r, false)
		case r.URL.Path == prefix+":streamGenerateContent" && r.URL.Query().Get("alt") == "sse":
			f.generate(w, r, true)
		case r.URL.Path == prefix+":countTokens":
			var body object
			data, _ := io.ReadAll(r.Body)
			_ = common.Unmarshal(data, &body)
			writeJSON(w, 200, object{"totalTokens": promptTokens(body) + f.countOffset})
		default:
			writeError(w, 404, "models/x is not found", "NOT_FOUND", "")
		}
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, _ := common.Marshal(v)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func writeError(w http.ResponseWriter, status int, message, rpc, extra string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":{"code":%d,"message":%q,"status":%q%s}}`, status, message, rpc, extra)
}

func promptTokens(body object) int64 {
	n := int64(0)
	for _, raw := range arr(body, "contents") {
		content, _ := raw.(object)
		for _, p := range arr(content, "parts") {
			part, _ := p.(object)
			n += int64(len(strings.Fields(str(part, "text")))) + 2
		}
	}
	return n
}

type reply struct {
	text   string
	calls  []object
	finish string
	out    int64
	cands  int
}

func lastUserText(contents []any) string {
	for i := len(contents) - 1; i >= 0; i-- {
		c, _ := contents[i].(object)
		if str(c, "role") == "user" {
			for _, p := range arr(c, "parts") {
				part, _ := p.(object)
				if obj(part, "inlineData") != nil {
					return "<image>"
				}
				if obj(part, "functionResponse") != nil {
					return "<function_response>"
				}
			}
			for _, p := range arr(c, "parts") {
				part, _ := p.(object)
				if s := str(part, "text"); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func (f *fakeGemini) script(body object) reply {
	contents := arr(body, "contents")
	prompt := lastUserText(contents)
	cfg := obj(body, "generationConfig")
	limit, _ := num(cfg, "maxOutputTokens")
	mode := str(obj(obj(body, "toolConfig"), "functionCallingConfig"), "mode")
	tools := len(arr(body, "tools")) > 0
	if str(cfg, "responseMimeType") == "application/json" {
		if obj(cfg, "responseSchema") != nil {
			return reply{text: `{"order_id":"ORD-246","paid":true,"quantity":3,"total":37.5}`, finish: "STOP", out: 20}
		}
		return reply{text: `{"ok": true}`, finish: "STOP", out: 5}
	}
	switch {
	case prompt == "<function_response>":
		return reply{text: `{"order_id":"ORD-731","status":"shipped","total":42,"currency":"USD"}`, finish: "STOP", out: 20}
	case tools && mode == "NONE":
		return reply{text: "I cannot call tools.", finish: "STOP", out: 6}
	case tools && mode == "ANY":
		return reply{calls: []object{{"name": "get_weather", "args": object{"city": "Paris"}}}, finish: "STOP", out: 12}
	case tools && strings.Contains(prompt, "Paris and in Tokyo"):
		return reply{calls: []object{{"name": "get_weather", "args": object{"city": "Paris"}}, {"name": "get_weather", "args": object{"city": "Tokyo"}}}, finish: "STOP", out: 24}
	case tools && strings.Contains(prompt, "SKU-204"):
		return reply{calls: []object{{"name": "lookup_inventory", "args": object{"sku": "SKU-204", "warehouse": "north", "include_reserved": false}}}, finish: "STOP", out: 20}
	case tools && strings.Contains(prompt, "ORD-731"):
		return reply{calls: []object{{"name": "lookup_order", "args": object{"order_id": "ORD-731"}}}, finish: "STOP", out: 14}
	case strings.Contains(prompt, "CHECK_STOP"):
		return reply{text: "ALPHA ", finish: "STOP", out: 2}
	case strings.Contains(prompt, "long numbered list") && limit > 0 && limit <= 16:
		return reply{text: "1, 2, 3", finish: "MAX_TOKENS", out: limit}
	case strings.HasPrefix(prompt, "Return only the exact code"):
		for _, raw := range contents {
			c, _ := raw.(object)
			for _, p := range arr(c, "parts") {
				if s := str(p.(object), "text"); strings.HasPrefix(s, "Remember this code: ") {
					return reply{text: strings.TrimPrefix(s, "Remember this code: "), finish: "STOP", out: 4}
				}
			}
		}
	case prompt == "<image>":
		return reply{text: "red", finish: "STOP", out: 1}
	case strings.Contains(prompt, "Count from 1 to 5"):
		return reply{text: "1, 2, 3, 4, 5", finish: "STOP", out: 9}
	case strings.Contains(prompt, "17 * 23"):
		return reply{text: "391", finish: "STOP", out: 1}
	case strings.Contains(prompt, "Name a color"):
		n, _ := num(cfg, "candidateCount")
		return reply{text: "Blue", finish: "STOP", out: 1, cands: int(n)}
	}
	if si := obj(body, "systemInstruction"); si != nil && strings.Contains(fmt.Sprint(si), "ORCHID") {
		return reply{text: "ORCHID", finish: "STOP", out: 2}
	}
	return reply{text: "PONG", finish: "STOP", out: 2}
}

func (f *fakeGemini) generate(w http.ResponseWriter, r *http.Request, stream bool) {
	var body object
	data, _ := io.ReadAll(r.Body)
	if common.Unmarshal(data, &body) != nil {
		writeError(w, 400, "Invalid JSON payload received.", "INVALID_ARGUMENT", "")
		return
	}
	if len(arr(body, "contents")) == 0 {
		if f.openAIErrors {
			writeJSON(w, 400, object{"error": object{"message": "contents is required", "type": "invalid_request_error", "param": "contents", "code": nil}})
			return
		}
		writeError(w, 400, "* GenerateContentRequest.contents: contents is not specified", "INVALID_ARGUMENT", "")
		return
	}
	s := f.script(body)
	finish := s.finish
	if f.lowerFinish {
		finish = strings.ToLower(finish)
	}
	parts := []any{}
	if strings.Contains(fmt.Sprint(obj(body, "generationConfig")["thinkingConfig"]), "includeThoughts:true") {
		parts = append(parts, object{"text": "Multiply 17 by 23.", "thought": true})
	}
	for _, c := range s.calls {
		args := any(c["args"])
		if f.stringArgs {
			encoded, _ := common.Marshal(args)
			args = string(encoded)
		}
		parts = append(parts, object{"functionCall": object{"name": c["name"], "args": args}, "thoughtSignature": "c2lnbmF0dXJl"})
	}
	prompt := promptTokens(body)
	thoughts := int64(0)
	if len(parts) > 0 && parts[0].(object)["thought"] == true {
		thoughts = 5
	}
	usage := object{"promptTokenCount": prompt, "candidatesTokenCount": s.out, "totalTokenCount": prompt + s.out + thoughts, "promptTokensDetails": []any{object{"modality": "TEXT", "tokenCount": prompt}}}
	if thoughts > 0 {
		usage["thoughtsTokenCount"] = thoughts
	}
	if !stream {
		if s.text != "" {
			parts = append(parts, object{"text": s.text})
		}
		n := s.cands
		if n < 1 {
			n = 1
		}
		var candidates []any
		for i := 0; i < n; i++ {
			c := object{"content": object{"role": "model", "parts": parts}, "finishReason": finish}
			if i > 0 {
				c["index"] = i
			}
			candidates = append(candidates, c)
		}
		doc := object{"candidates": candidates, "modelVersion": f.model, "responseId": "resp-1"}
		if !f.noUsage {
			doc["usageMetadata"] = usage
		}
		writeJSON(w, 200, doc)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)
	frame := func(doc object) {
		data, _ := common.Marshal(doc)
		_, _ = fmt.Fprintf(w, "data: %s\r\n\r\n", data)
	}
	chunk := func(parts []any, finish string, final bool) {
		candidate := object{"content": object{"role": "model", "parts": parts}}
		if finish != "" {
			candidate["finishReason"] = finish
		}
		doc := object{"candidates": []any{candidate}, "modelVersion": f.model, "responseId": "resp-s"}
		if final && !f.noUsage {
			doc["usageMetadata"] = usage
		}
		frame(doc)
	}
	if s.text != "" {
		half := len(s.text) / 2
		chunk(append(parts, object{"text": s.text[:half]}), "", false)
		chunk([]any{object{"text": s.text[half:]}}, finish, true)
	} else {
		chunk(parts, finish, true)
	}
	if f.doneMarker {
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}
}

func runAgainst(t *testing.T, f *fakeGemini, options Options) Report {
	t.Helper()
	server := httptest.NewServer(f.handler())
	t.Cleanup(server.Close)
	transport := NewHTTPTransport(server.Client(), server.URL, testKey, nil)
	return Run(context.Background(), options, transport)
}

func checkByID(report Report, id string) Check {
	for _, c := range report.Checks {
		if c.ID == id {
			return c
		}
	}
	return Check{}
}
