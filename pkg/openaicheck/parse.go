package openaicheck

import (
	"fmt"
	"strings"

	"model-check/common"
)

type toolCall struct {
	ID        string
	Type      string
	Name      string
	Arguments string
	Index     int
	ItemID    string // Responses output item id ("fc_..."), distinct from the call_id
}

// streamedCall tracks one function_call item across Responses stream events.
type streamedCall struct {
	index int64
	args  strings.Builder
}

// view is the API-neutral reading of one Chat Completions or Responses result.
type view struct {
	ID          string
	Model       string
	Fingerprint string
	Text        string
	Finish      string // finish_reason, or the Responses status / incomplete reason
	Refusal     string
	Tools       []toolCall
	Usage       Usage
	Choices     int
	Raw         map[string]any // assistant message (chat) or the response object (responses)
	Output      []any          // Responses output items, verbatim
	UsageSeen   bool           // a streamed usage chunk arrived
	UsageIssues []string       // problems in a streamed usage chunk, kept apart from stream structure
}

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

func readUsage(doc object, inKey, outKey string, issues *[]string) Usage {
	u := obj(doc, "usage")
	if u == nil {
		*issues = append(*issues, "missing_usage")
		return Usage{}
	}
	var usage Usage
	read := func(key string, dst **int64) {
		n, ok := num(u, key)
		if !ok {
			*issues = append(*issues, "missing_"+key)
			return
		}
		if n < 0 {
			*issues = append(*issues, "negative_"+key)
			return
		}
		*dst = &n
	}
	read(inKey, &usage.Input)
	read(outKey, &usage.Output)
	read("total_tokens", &usage.Total)
	// Optional detail objects: when present their counters are non-negative
	// integers, and reasoning tokens are part of (never more than) the output.
	detail := func(group, key string) *int64 {
		d := obj(u, group)
		if d == nil || !has(d, key) {
			return nil
		}
		n, ok := num(d, key)
		if !ok || n < 0 {
			*issues = append(*issues, "invalid_"+group+"_"+key)
			return nil
		}
		return &n
	}
	detail(inKey+"_details", "cached_tokens")
	if reasoning := detail(outKey+"_details", "reasoning_tokens"); reasoning != nil && usage.Output != nil && *reasoning > *usage.Output {
		*issues = append(*issues, "reasoning_tokens_exceed_output")
	}
	if usage.Input != nil && usage.Output != nil && usage.Total != nil && *usage.Total != *usage.Input+*usage.Output {
		*issues = append(*issues, "usage_total_mismatch")
	}
	return usage
}

// chatFinishReasons are the finish_reason values the Chat Completions API
// documents; any other string is not an OpenAI-conformant response.
var chatFinishReasons = map[string]bool{"stop": true, "length": true, "tool_calls": true, "content_filter": true, "function_call": true}

// responseStatuses are the documented Response.status values.
var responseStatuses = map[string]bool{"completed": true, "failed": true, "in_progress": true, "cancelled": true, "queued": true, "incomplete": true}

func parseToolCallItem(item object, fallbackIndex int) toolCall {
	call := toolCall{Index: fallbackIndex}
	call.ID, call.Type = str(item, "id"), str(item, "type")
	if n, ok := num(item, "index"); ok {
		call.Index = int(n)
	}
	if fn := obj(item, "function"); fn != nil {
		call.Name, call.Arguments = str(fn, "name"), str(fn, "arguments")
	}
	return call
}

// parseChat validates a non-streaming chat.completion envelope.
func parseChat(body []byte) (view, []string) {
	var v view
	doc, ok := parseObject(body)
	if !ok {
		return v, []string{"invalid_json"}
	}
	var issues []string
	add := func(ok bool, code string) {
		if !ok {
			issues = append(issues, code)
		}
	}
	add(str(doc, "object") == "chat.completion", "object_not_chat_completion")
	add(str(doc, "id") != "", "missing_id")
	_, hasCreated := num(doc, "created")
	add(hasCreated, "missing_created")
	add(str(doc, "model") != "", "missing_model")
	v.ID, v.Model, v.Fingerprint = str(doc, "id"), str(doc, "model"), str(doc, "system_fingerprint")
	choices := arr(doc, "choices")
	v.Choices = len(choices)
	add(len(choices) > 0, "missing_choices")
	for i, c := range choices {
		choice, _ := c.(object)
		if choice == nil {
			issues = append(issues, "choice_not_object")
			continue
		}
		if index, ok := num(choice, "index"); !ok || index != int64(i) {
			issues = append(issues, "bad_choice_index")
		}
		message := obj(choice, "message")
		finish, hasFinish := choice["finish_reason"].(string)
		add(message != nil, "missing_message")
		add(hasFinish && finish != "", "missing_finish_reason")
		add(!hasFinish || finish == "" || chatFinishReasons[finish], "unknown_finish_reason")
		if message == nil {
			continue
		}
		add(str(message, "role") == "assistant", "role_not_assistant")
		add(has(message, "content"), "missing_message_content")
		if i == 0 {
			v.Finish, v.Raw = finish, message
			v.Text, v.Refusal = strings.TrimSpace(str(message, "content")), str(message, "refusal")
			for n, raw := range arr(message, "tool_calls") {
				if item, _ := raw.(object); item != nil {
					v.Tools = append(v.Tools, parseToolCallItem(item, n))
				}
			}
		}
	}
	// prompt/completion tokens are stored in the shared input/output Usage fields.
	v.Usage = readUsage(doc, "prompt_tokens", "completion_tokens", &issues)
	return v, issues
}

// parseChatStream validates chat.completion.chunk frames and assembles them.
// Usage-chunk findings go to view.UsageIssues so stream structure and the
// include_usage contract are judged separately.
func parseChatStream(events []SSEEvent, done bool) (view, []string) {
	var v view
	var issues []string
	add := func(ok bool, code string) {
		if !ok {
			issues = append(issues, code)
		}
	}
	add(len(events) > 0, "empty_stream")
	add(done, "missing_done_marker")
	var text strings.Builder
	calls := map[int]*toolCall{}
	var order []int
	sawRole, finished := false, false
	ids := map[string]bool{}
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
		add(str(chunk, "object") == "chat.completion.chunk", "object_not_chunk")
		if id := str(chunk, "id"); id != "" {
			ids[id] = true
			if v.ID == "" {
				v.ID = id
			}
		}
		if m := str(chunk, "model"); m != "" && v.Model == "" {
			v.Model = m
		}
		if fp := str(chunk, "system_fingerprint"); fp != "" {
			v.Fingerprint = fp
		}
		if has(chunk, "usage") && chunk["usage"] != nil {
			v.UsageSeen = true
			v.Usage = readUsage(chunk, "prompt_tokens", "completion_tokens", &v.UsageIssues)
			if len(arr(chunk, "choices")) != 0 {
				// OpenAI emits the final usage chunk with an empty choices array.
				v.UsageIssues = append(v.UsageIssues, "usage_chunk_has_choices")
			}
			if n != len(events)-1 {
				v.UsageIssues = append(v.UsageIssues, "usage_chunk_not_last")
			}
		}
		for _, c := range arr(chunk, "choices") {
			choice, _ := c.(object)
			if choice == nil {
				continue
			}
			if index, _ := num(choice, "index"); index != 0 {
				continue
			}
			delta := obj(choice, "delta")
			if delta != nil {
				if str(delta, "role") == "assistant" {
					sawRole = true
				}
				if content := str(delta, "content"); content != "" {
					text.WriteString(content)
				}
				if r := str(delta, "refusal"); r != "" {
					v.Refusal += r
				}
				for _, raw := range arr(delta, "tool_calls") {
					item, _ := raw.(object)
					if item == nil {
						continue
					}
					call := parseToolCallItem(item, 0)
					entry := calls[call.Index]
					if entry == nil {
						entry = &toolCall{Index: call.Index, ID: call.ID, Type: call.Type, Name: call.Name}
						calls[call.Index] = entry
						order = append(order, call.Index)
						if call.ID == "" || call.Name == "" {
							issues = append(issues, "tool_call_first_delta_incomplete")
						}
					} else if call.Name != "" && entry.Name == "" {
						entry.Name = call.Name
					}
					entry.Arguments += call.Arguments
				}
			}
			if finish, ok := choice["finish_reason"].(string); ok && finish != "" {
				if finished {
					issues = append(issues, "multiple_finish_reasons")
				}
				finished, v.Finish = true, finish
				if !chatFinishReasons[finish] {
					issues = append(issues, "unknown_finish_reason")
				}
			}
		}
	}
	add(finished, "missing_finish_reason")
	add(sawRole, "missing_assistant_role_delta")
	add(len(ids) <= 1, "inconsistent_chunk_ids")
	v.Text = strings.TrimSpace(text.String())
	for _, index := range order {
		v.Tools = append(v.Tools, *calls[index])
	}
	return v, issues
}

// parseResponses validates a non-streaming Responses API object.
func parseResponses(body []byte) (view, []string) {
	var v view
	doc, ok := parseObject(body)
	if !ok {
		return v, []string{"invalid_json"}
	}
	return readResponseObject(doc)
}

func readResponseObject(doc object) (view, []string) {
	var v view
	var issues []string
	add := func(ok bool, code string) {
		if !ok {
			issues = append(issues, code)
		}
	}
	add(str(doc, "object") == "response", "object_not_response")
	add(str(doc, "id") != "", "missing_id")
	add(str(doc, "model") != "", "missing_model")
	_, hasCreated := num(doc, "created_at")
	add(hasCreated, "missing_created_at")
	status := str(doc, "status")
	add(status != "", "missing_status")
	add(status == "" || responseStatuses[status], "unknown_status")
	v.ID, v.Model, v.Finish, v.Raw = str(doc, "id"), str(doc, "model"), status, doc
	if status == "incomplete" {
		if details := obj(doc, "incomplete_details"); details != nil && str(details, "reason") != "" {
			v.Finish = "incomplete:" + str(details, "reason")
		}
	}
	output := arr(doc, "output")
	add(doc["output"] != nil && output != nil, "missing_output")
	v.Output = output
	var text strings.Builder
	for n, raw := range output {
		item, _ := raw.(object)
		if item == nil {
			issues = append(issues, "output_item_not_object")
			continue
		}
		switch str(item, "type") {
		case "message":
			add(str(item, "role") == "assistant", "role_not_assistant")
			for _, part := range arr(item, "content") {
				p, _ := part.(object)
				switch str(p, "type") {
				case "output_text":
					text.WriteString(str(p, "text"))
				case "refusal":
					v.Refusal += str(p, "refusal")
				}
			}
		case "function_call":
			v.Tools = append(v.Tools, toolCall{ID: str(item, "call_id"), ItemID: str(item, "id"), Type: "function", Name: str(item, "name"), Arguments: str(item, "arguments"), Index: n})
		}
	}
	v.Text = strings.TrimSpace(text.String())
	if status == "completed" || status == "incomplete" {
		v.Usage = readUsage(doc, "input_tokens", "output_tokens", &issues)
	} else if has(doc, "usage") {
		v.Usage = readUsage(doc, "input_tokens", "output_tokens", &issues)
	}
	return v, issues
}

// parseResponsesStream validates the Responses event lifecycle. The terminal
// response.completed / response.incomplete event carries the full object, which
// is validated with the same rules as the non-streaming form.
func parseResponsesStream(events []SSEEvent) (view, []string) {
	var v view
	var issues []string
	if len(events) == 0 {
		return v, []string{"empty_stream"}
	}
	seen := map[string]int{}
	var text strings.Builder
	lastSequence := int64(-1)
	terminal := false
	calls := map[string]*streamedCall{}
	for _, event := range events {
		e, ok := parseObject(event.Data)
		if !ok {
			issues = append(issues, "invalid_event_json")
			continue
		}
		kind := str(e, "type")
		if kind == "" {
			issues = append(issues, "missing_event_type")
			continue
		}
		if event.Name != "" && event.Name != kind {
			issues = append(issues, "event_name_type_mismatch")
		}
		if terminal {
			issues = append(issues, "event_after_terminal")
		}
		seen[kind]++
		if seq, ok := num(e, "sequence_number"); ok {
			if seq <= lastSequence {
				issues = append(issues, "sequence_not_increasing")
			}
			lastSequence = seq
		}
		switch kind {
		case "response.output_text.delta":
			text.WriteString(str(e, "delta"))
		case "response.output_item.added", "response.output_item.done":
			issues = append(issues, readFunctionCallItemEvent(kind, e, calls)...)
		case "response.function_call_arguments.delta", "response.function_call_arguments.done":
			issues = append(issues, readFunctionCallArgumentsEvent(kind, e, calls)...)
		case "response.completed", "response.incomplete":
			terminal = true
			response := obj(e, "response")
			if response == nil {
				issues = append(issues, "terminal_event_missing_response")
				continue
			}
			var inner []string
			v, inner = readResponseObject(response)
			issues = append(issues, inner...)
		case "response.failed", "error":
			issues = append(issues, "error_event")
			terminal = true
		}
	}
	if seen["response.created"] == 0 {
		issues = append(issues, "missing_response_created")
	}
	// The streamed item ids must be the ids of the items in the final response.
	for _, tool := range v.Tools {
		if len(calls) == 0 {
			issues = append(issues, "missing_function_call_stream_events")
			break
		}
		if tool.ItemID == "" {
			issues = append(issues, "final_function_call_missing_item_id")
		} else if calls[tool.ItemID] == nil {
			issues = append(issues, "tool_item_id_differs_from_final")
		}
	}
	if !terminal {
		issues = append(issues, "missing_terminal_event")
	}
	if v.Text == "" && text.Len() > 0 {
		v.Text = strings.TrimSpace(text.String())
	}
	if len(v.Tools) == 0 && v.Text != "" && seen["response.output_text.delta"] == 0 {
		issues = append(issues, "missing_text_delta_events")
	}
	if v.Text != "" && strings.TrimSpace(text.String()) != v.Text && seen["response.output_text.delta"] > 0 {
		issues = append(issues, "delta_text_differs_from_final")
	}
	return v, issues
}

// parseModels validates GET /v1/models and reports whether model is listed.
func parseModels(body []byte, model string) (listed bool, count int, issues []string) {
	doc, ok := parseObject(body)
	if !ok {
		return false, 0, []string{"invalid_json"}
	}
	if str(doc, "object") != "list" {
		issues = append(issues, "object_not_list")
	}
	data := arr(doc, "data")
	if data == nil {
		return false, 0, append(issues, "missing_data")
	}
	for _, raw := range data {
		item, _ := raw.(object)
		id := str(item, "id")
		if id == "" {
			issues = append(issues, "model_missing_id")
			continue
		}
		if str(item, "object") != "model" {
			issues = append(issues, "model_object_not_model")
		}
		if _, ok := num(item, "created"); !ok {
			issues = append(issues, "model_missing_created")
		}
		if str(item, "owned_by") == "" {
			issues = append(issues, "model_missing_owned_by")
		}
		if id == model {
			listed = true
		}
	}
	return listed, len(data), dedupe(issues)
}

// parseErrorShape validates the OpenAI error envelope.
func parseErrorShape(status int, body []byte) (message string, issues []string) {
	if status < 400 || status >= 500 {
		issues = append(issues, fmt.Sprintf("unexpected_status_%d", status))
	}
	doc, ok := parseObject(body)
	if !ok {
		return "", append(issues, "error_body_not_json")
	}
	e := obj(doc, "error")
	if e == nil {
		return "", append(issues, "missing_error_object")
	}
	message = str(e, "message")
	if message == "" {
		issues = append(issues, "missing_error_message")
	}
	if str(e, "type") == "" {
		issues = append(issues, "missing_error_type")
	}
	if !has(e, "code") {
		issues = append(issues, "missing_error_code_key")
	}
	if !has(e, "param") {
		issues = append(issues, "missing_error_param_key")
	}
	return message, issues
}

// upstreamError extracts a short, bounded description from any error body.
func upstreamError(body []byte) string {
	doc, ok := parseObject(body)
	if !ok {
		return ""
	}
	e := obj(doc, "error")
	if e == nil {
		if m := str(doc, "message"); m != "" {
			return truncate(m, 240)
		}
		return ""
	}
	parts := []string{}
	for _, key := range []string{"type", "code", "param"} {
		if s := str(e, key); s != "" {
			parts = append(parts, key+"="+s)
		}
	}
	if m := str(e, "message"); m != "" {
		parts = append(parts, truncate(m, 240))
	}
	return strings.Join(parts, " ")
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) > n {
		return string(runes[:n])
	}
	return s
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

// readFunctionCallItemEvent validates response.output_item.added/done for a
// function_call item: it must carry the item id, call_id and name.
func readFunctionCallItemEvent(kind string, e object, calls map[string]*streamedCall) []string {
	item := obj(e, "item")
	if str(item, "type") != "function_call" {
		return nil
	}
	var issues []string
	id := str(item, "id")
	index, _ := num(e, "output_index")
	if kind == "response.output_item.added" {
		if id == "" || str(item, "call_id") == "" || str(item, "name") == "" {
			issues = append(issues, "tool_item_incomplete")
		}
		if id != "" {
			calls[id] = &streamedCall{index: index}
		}
		return issues
	}
	call := calls[id]
	switch {
	case id == "":
		issues = append(issues, "tool_item_done_missing_id")
	case call == nil:
		issues = append(issues, "tool_item_done_without_added")
	case call.args.Len() > 0 && str(item, "arguments") != call.args.String():
		issues = append(issues, "tool_item_args_differ_from_deltas")
	}
	return issues
}

// readFunctionCallArgumentsEvent validates the argument stream. Every delta and
// the done event must carry item_id, and it must name an item that was added.
func readFunctionCallArgumentsEvent(kind string, e object, calls map[string]*streamedCall) []string {
	prefix := "tool_delta_"
	if kind == "response.function_call_arguments.done" {
		prefix = "tool_args_done_"
	}
	id := str(e, "item_id")
	if id == "" {
		return []string{prefix + "missing_item_id"}
	}
	call := calls[id]
	if call == nil {
		// Either the id is unknown or the delta arrived before output_item.added.
		return []string{prefix + "unknown_item_id"}
	}
	var issues []string
	if index, ok := num(e, "output_index"); ok && index != call.index {
		issues = append(issues, prefix+"output_index_mismatch")
	}
	if kind == "response.function_call_arguments.delta" {
		delta, isString := e["delta"].(string)
		if !isString {
			return append(issues, "tool_delta_missing_delta")
		}
		call.args.WriteString(delta)
		return issues
	}
	arguments := str(e, "arguments")
	if arguments != call.args.String() {
		issues = append(issues, "tool_args_done_mismatch")
	}
	var parsed map[string]any
	if common.UnmarshalJsonStr(arguments, &parsed) != nil || parsed == nil {
		issues = append(issues, "tool_args_not_json")
	}
	return issues
}
