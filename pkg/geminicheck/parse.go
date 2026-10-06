package geminicheck

import (
	"fmt"
	"regexp"
	"strings"

	"model-check/common"
)

type object = map[string]any

func str(m object, key string) string {
	s, _ := m[key].(string)
	return s
}

func obj(m object, key string) object {
	o, _ := m[key].(object)
	return o
}

func arr(m object, key string) []any {
	a, _ := m[key].([]any)
	return a
}

func num(m object, key string) (int64, bool) {
	f, ok := m[key].(float64)
	if !ok || f != float64(int64(f)) {
		return 0, false
	}
	return int64(f), true
}

func has(m object, key string) bool {
	_, ok := m[key]
	return ok
}

func parseObject(body []byte) (object, bool) {
	var doc object
	if common.Unmarshal(body, &doc) != nil || doc == nil {
		return nil, false
	}
	return doc, true
}

type functionCall struct {
	ID   string
	Name string
	Args map[string]any
}

// view is the reading of one generateContent result (or an assembled stream).
type view struct {
	ID          string // responseId
	Model       string // modelVersion
	Text        string // non-thought text of candidate 0
	Thoughts    int    // parts marked thought:true
	Finish      string // finishReason of candidate 0
	Calls       []functionCall
	Candidates  int
	Content     object // candidate 0 content, verbatim, for multi-turn replay
	BlockReason string
	Usage       Usage
	UsageSeen   bool
	UsageIssues []string
}

// enumPattern is the proto enum spelling every documented finishReason and
// blockReason uses. The enum keeps growing (MISSING_THOUGHT_SIGNATURE,
// MALFORMED_RESPONSE, ...), so values are checked for form, not membership:
// "stop", "end_turn" or "tool_calls" are other protocols leaking through.
var enumPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// readUsage validates usageMetadata. Proto JSON omits zero counters, so only
// promptTokenCount and totalTokenCount are required; the documented total is
// prompt + candidates + thoughts (+ tool-use prompt).
func readUsage(doc object, issues *[]string) Usage {
	u := obj(doc, "usageMetadata")
	if u == nil {
		*issues = append(*issues, "missing_usage_metadata")
		return Usage{}
	}
	var usage Usage
	counter := func(key string, required bool) (int64, bool) {
		if !has(u, key) {
			if required {
				*issues = append(*issues, "missing_"+key)
			}
			return 0, false
		}
		n, ok := num(u, key)
		if !ok {
			*issues = append(*issues, "invalid_"+key)
			return 0, false
		}
		if n < 0 {
			*issues = append(*issues, "negative_"+key)
			return 0, false
		}
		return n, true
	}
	prompt, okPrompt := counter("promptTokenCount", true)
	candidates, _ := counter("candidatesTokenCount", false)
	thoughts, okThoughts := counter("thoughtsTokenCount", false)
	toolUse, _ := counter("toolUsePromptTokenCount", false)
	cached, okCached := counter("cachedContentTokenCount", false)
	total, okTotal := counter("totalTokenCount", true)
	output := candidates + thoughts
	usage.Output = &output
	if okPrompt {
		usage.Input = &prompt
	}
	if okTotal {
		usage.Total = &total
	}
	if okThoughts {
		usage.Thoughts = &thoughts
	}
	if okCached {
		usage.Cached = &cached
		if okPrompt && cached > prompt {
			*issues = append(*issues, "cached_exceeds_prompt")
		}
	}
	if okPrompt && okTotal && total != prompt+candidates+thoughts+toolUse {
		*issues = append(*issues, "usage_total_mismatch")
	}
	return usage
}

// readParts validates candidate parts and collects text and function calls.
func readParts(parts []any, v *view, text *strings.Builder) []string {
	var issues []string
	for _, raw := range parts {
		part, _ := raw.(object)
		if part == nil {
			issues = append(issues, "part_not_object")
			continue
		}
		if call := obj(part, "functionCall"); call != nil {
			name := str(call, "name")
			args, argsOK := call["args"].(object)
			if name == "" {
				issues = append(issues, "function_call_missing_name")
			}
			if has(call, "args") && !argsOK {
				issues = append(issues, "function_call_args_not_object")
			}
			if args == nil {
				args = object{}
			}
			v.Calls = append(v.Calls, functionCall{ID: str(call, "id"), Name: name, Args: args})
			continue
		}
		if has(part, "functionCall") {
			issues = append(issues, "function_call_not_object")
			continue
		}
		if thought, _ := part["thought"].(bool); thought {
			v.Thoughts++
			continue
		}
		if s, ok := part["text"].(string); ok {
			text.WriteString(s)
		} else if has(part, "text") {
			issues = append(issues, "text_not_string")
		}
	}
	return issues
}

// readCandidates validates the candidates array of one response or chunk.
// final requires a finishReason on every candidate (the non-streaming form).
func readCandidates(doc object, v *view, text *strings.Builder, final bool) []string {
	var issues []string
	candidates := arr(doc, "candidates")
	if has(doc, "candidates") && candidates == nil {
		issues = append(issues, "candidates_not_array")
	}
	for i, raw := range candidates {
		candidate, _ := raw.(object)
		if candidate == nil {
			issues = append(issues, "candidate_not_object")
			continue
		}
		// Proto JSON omits index 0.
		if index, ok := num(candidate, "index"); (ok && index != int64(i)) || (!ok && has(candidate, "index")) || (!ok && i > 0) {
			issues = append(issues, "bad_candidate_index")
		}
		finish, hasFinish := candidate["finishReason"].(string)
		if has(candidate, "finishReason") && (!hasFinish || !enumPattern.MatchString(finish)) {
			issues = append(issues, "invalid_finish_reason")
		}
		if final && finish == "" {
			issues = append(issues, "missing_finish_reason")
		}
		content := obj(candidate, "content")
		if content == nil && has(candidate, "content") {
			issues = append(issues, "content_not_object")
		}
		// A blocked or truncated candidate may carry no content at all.
		if content == nil && final && finish == "STOP" {
			issues = append(issues, "missing_content")
		}
		var candidateText strings.Builder
		if content != nil {
			if role := str(content, "role"); role != "model" && (role != "" || len(arr(content, "parts")) > 0) {
				issues = append(issues, "role_not_model")
			}
			if has(content, "parts") && arr(content, "parts") == nil {
				issues = append(issues, "parts_not_array")
			}
			if final && finish == "STOP" && len(arr(content, "parts")) == 0 {
				issues = append(issues, "empty_parts")
			}
		}
		if i != 0 {
			var other view
			issues = append(issues, readParts(arr(content, "parts"), &other, &candidateText)...)
			continue
		}
		if finish != "" {
			v.Finish = finish
		}
		if content != nil {
			if v.Content == nil {
				v.Content = object{"role": "model", "parts": []any{}}
			}
			v.Content["parts"] = append(v.Content["parts"].([]any), arr(content, "parts")...)
		}
		issues = append(issues, readParts(arr(content, "parts"), v, text)...)
	}
	return issues
}

// leakedOpenAI reports an OpenAI envelope returned by a Gemini endpoint, the
// usual sign of a relay that converts protocols on one side only.
func leakedOpenAI(doc object) bool {
	return has(doc, "choices") || strings.HasPrefix(str(doc, "object"), "chat.completion")
}

// parseGenerate validates a non-streaming GenerateContentResponse.
func parseGenerate(body []byte) (view, []string) {
	var v view
	doc, ok := parseObject(body)
	if !ok {
		return v, []string{"invalid_json"}
	}
	var issues []string
	if leakedOpenAI(doc) {
		issues = append(issues, "openai_envelope")
	}
	v.ID, v.Model = str(doc, "responseId"), str(doc, "modelVersion")
	if feedback := obj(doc, "promptFeedback"); feedback != nil {
		v.BlockReason = str(feedback, "blockReason")
		if v.BlockReason != "" && !enumPattern.MatchString(v.BlockReason) {
			issues = append(issues, "invalid_block_reason")
		}
	}
	v.Candidates = len(arr(doc, "candidates"))
	if v.Candidates == 0 && v.BlockReason == "" {
		issues = append(issues, "missing_candidates")
	}
	if v.Model == "" {
		issues = append(issues, "missing_model_version")
	}
	if v.ID == "" {
		issues = append(issues, "missing_response_id")
	}
	var text strings.Builder
	issues = append(issues, readCandidates(doc, &v, &text, true)...)
	v.Text = strings.TrimSpace(text.String())
	v.Usage = readUsage(doc, &issues)
	v.UsageSeen = has(doc, "usageMetadata")
	return v, dedupe(issues)
}

// parseStream validates streamGenerateContent?alt=sse. Every data frame is a
// complete GenerateContentResponse; text arrives in pieces, a function call
// arrives whole, finishReason comes once on the last candidate chunk and the
// final chunk carries the cumulative usageMetadata. There is no [DONE] frame.
func parseStream(events []SSEEvent, done bool) (view, []string) {
	var v view
	var issues []string
	if len(events) == 0 {
		return v, []string{"empty_stream"}
	}
	if done {
		issues = append(issues, "unexpected_done_marker")
	}
	var text strings.Builder
	ids := map[string]bool{}
	finishedAt := -1
	lastUsage := -1
	for n, event := range events {
		chunk, ok := parseObject(event.Data)
		if !ok {
			issues = append(issues, "invalid_chunk_json")
			continue
		}
		if has(chunk, "error") {
			issues = append(issues, "error_event")
			continue
		}
		if leakedOpenAI(chunk) {
			issues = append(issues, "openai_envelope")
			continue
		}
		if id := str(chunk, "responseId"); id != "" {
			ids[id] = true
			v.ID = id
		}
		if model := str(chunk, "modelVersion"); model != "" && v.Model == "" {
			v.Model = model
		}
		if feedback := obj(chunk, "promptFeedback"); feedback != nil && str(feedback, "blockReason") != "" {
			v.BlockReason = str(feedback, "blockReason")
		}
		if len(arr(chunk, "candidates")) > v.Candidates {
			v.Candidates = len(arr(chunk, "candidates"))
		}
		before := v.Finish
		var parts int
		if candidates := arr(chunk, "candidates"); len(candidates) > 0 {
			if c, _ := candidates[0].(object); c != nil {
				parts = len(arr(obj(c, "content"), "parts"))
			}
		}
		if finishedAt >= 0 && parts > 0 {
			issues = append(issues, "content_after_finish_reason")
		}
		issues = append(issues, readCandidates(chunk, &v, &text, false)...)
		if v.Finish != "" && before == "" {
			finishedAt = n
		} else if before != "" && hasFinishReason(chunk) {
			issues = append(issues, "multiple_finish_reasons")
		}
		if has(chunk, "usageMetadata") {
			lastUsage = n
			v.UsageIssues = nil
			v.Usage = readUsage(chunk, &v.UsageIssues)
			v.UsageSeen = true
		}
	}
	if finishedAt < 0 && v.BlockReason == "" {
		issues = append(issues, "missing_finish_reason")
	}
	if len(ids) > 1 {
		issues = append(issues, "inconsistent_response_ids")
	}
	if v.Model == "" {
		issues = append(issues, "missing_model_version")
	}
	if !v.UsageSeen {
		v.UsageIssues = append(v.UsageIssues, "missing_usage_metadata")
	} else if lastUsage != len(events)-1 {
		v.UsageIssues = append(v.UsageIssues, "usage_not_in_final_chunk")
	}
	v.Text = strings.TrimSpace(text.String())
	return v, dedupe(issues)
}

func hasFinishReason(chunk object) bool {
	for _, raw := range arr(chunk, "candidates") {
		if c, _ := raw.(object); c != nil && str(c, "finishReason") != "" {
			return true
		}
	}
	return false
}

// parseModel validates a models.get Model resource.
func parseModel(body []byte, model string) (methods []string, issues []string) {
	doc, ok := parseObject(body)
	if !ok {
		return nil, []string{"invalid_json"}
	}
	if str(doc, "name") != "models/"+model {
		issues = append(issues, "model_name_mismatch")
	}
	if str(doc, "version") == "" {
		issues = append(issues, "missing_version")
	}
	for _, key := range []string{"inputTokenLimit", "outputTokenLimit"} {
		if n, ok := num(doc, key); !ok || n <= 0 {
			issues = append(issues, "invalid_"+key)
		}
	}
	for _, raw := range arr(doc, "supportedGenerationMethods") {
		if s, _ := raw.(string); s != "" {
			methods = append(methods, s)
		}
	}
	if len(methods) == 0 {
		issues = append(issues, "missing_supported_generation_methods")
	}
	return methods, issues
}

// parseCount validates a countTokens response.
func parseCount(body []byte) (total int64, issues []string) {
	doc, ok := parseObject(body)
	if !ok {
		return 0, []string{"invalid_json"}
	}
	n, ok := num(doc, "totalTokens")
	if !ok {
		return 0, []string{"missing_totalTokens"}
	}
	if n <= 0 {
		issues = append(issues, "non_positive_totalTokens")
	}
	return n, issues
}

// canonicalStatus maps HTTP status to the google.rpc.Code names Gemini puts
// in error.status.
var canonicalStatus = map[int][]string{
	400: {"INVALID_ARGUMENT", "FAILED_PRECONDITION", "OUT_OF_RANGE"},
	401: {"UNAUTHENTICATED"},
	403: {"PERMISSION_DENIED"},
	404: {"NOT_FOUND"},
	409: {"ALREADY_EXISTS", "ABORTED"},
	429: {"RESOURCE_EXHAUSTED"},
	499: {"CANCELLED"},
	500: {"INTERNAL", "UNKNOWN", "DATA_LOSS"},
	501: {"UNIMPLEMENTED"},
	503: {"UNAVAILABLE"},
	504: {"DEADLINE_EXCEEDED"},
}

// parseErrorShape validates the google.rpc.Status error envelope:
// {"error":{"code":<http status>,"message":"...","status":"INVALID_ARGUMENT"}}.
func parseErrorShape(status int, body []byte) (message, rpcStatus string, issues []string) {
	if status < 400 || status >= 500 {
		issues = append(issues, fmt.Sprintf("unexpected_status_%d", status))
	}
	doc, ok := parseObject(body)
	if !ok {
		return "", "", append(issues, "error_body_not_json")
	}
	e := obj(doc, "error")
	if e == nil {
		return "", "", append(issues, "missing_error_object")
	}
	message, rpcStatus = str(e, "message"), str(e, "status")
	if message == "" {
		issues = append(issues, "missing_error_message")
	}
	if code, ok := num(e, "code"); !ok {
		issues = append(issues, "error_code_not_integer")
	} else if code != int64(status) {
		issues = append(issues, "error_code_differs_from_http_status")
	}
	switch {
	case rpcStatus == "":
		issues = append(issues, "missing_error_status")
	case !enumPattern.MatchString(rpcStatus):
		issues = append(issues, "invalid_error_status")
	case canonicalStatus[status] != nil && !contains(canonicalStatus[status], rpcStatus):
		issues = append(issues, "error_status_differs_from_http_status")
	}
	if has(e, "type") || has(e, "param") {
		issues = append(issues, "openai_error_fields")
	}
	return message, rpcStatus, issues
}

// upstreamError extracts a short, bounded description from any error body.
func upstreamError(body []byte) string {
	doc, ok := parseObject(body)
	if !ok {
		return ""
	}
	e := obj(doc, "error")
	if e == nil {
		return truncate(str(doc, "message"), 240)
	}
	parts := []string{}
	if s := str(e, "status"); s != "" {
		parts = append(parts, "status="+s)
	}
	if m := str(e, "message"); m != "" {
		parts = append(parts, truncate(m, 240))
	}
	return strings.Join(parts, " ")
}

func contains(items []string, item string) bool {
	for _, s := range items {
		if s == item {
			return true
		}
	}
	return false
}

func dedupe(items []string) []string {
	seen := map[string]bool{}
	out := items[:0:0]
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	return out
}
