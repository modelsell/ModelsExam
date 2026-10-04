package openaicheck

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"model-check/common"
)

func TestConformingEndpointPassesFullSuite(t *testing.T) {
	f := &fakeOpenAI{model: "gpt-4o"}
	report := runAgainst(t, f, Options{Model: "gpt-4o", Suite: SuiteFull, Vision: true})
	for _, c := range report.Checks {
		if c.Status == "fail" || (c.Kind == "assertion" && c.Status == "inconclusive") {
			t.Errorf("%s: %s %s %v", c.ID, c.Status, c.Code, c.Evidence)
		}
	}
	if report.Score == nil || *report.Score != 100 {
		t.Fatalf("score = %v, want 100", report.Score)
	}
	if got, max := int(f.requests.Load()), MaxRequests(report.optionsOrZero()); got > max {
		t.Errorf("sent %d requests, bound was %d", got, max)
	}
	if report.RequestsRun != int(f.requests.Load()) {
		t.Errorf("RequestsRun = %d, server saw %d", report.RequestsRun, f.requests.Load())
	}
	for _, item := range report.Plan {
		if checkByID(report, item.ID).ID == "" {
			t.Errorf("plan item %s has no check", item.ID)
		}
	}
	if c := checkByID(report, "model_consistency"); c.Status != "pass" {
		t.Errorf("model_consistency = %s %s", c.Status, c.Code)
	}
}

func (r Report) optionsOrZero() Options {
	if r.Options == nil {
		return Options{}
	}
	return *r.Options
}

func TestStandardSuiteSkipsOptionalChecks(t *testing.T) {
	report := runAgainst(t, &fakeOpenAI{model: "gpt-4o"}, Options{Model: "gpt-4o"})
	for _, id := range []string{"chat_n", "chat_vision", "chat_parallel_tools", "responses_basic", "chat_logprobs"} {
		c := checkByID(report, id)
		if c.Status != "skipped" || c.Code != "not_requested" {
			t.Errorf("%s = %s %s, want skipped not_requested", id, c.Status, c.Code)
		}
	}
	if report.Score == nil || *report.Score != 100 {
		t.Errorf("score = %v", report.Score)
	}
}

func TestReasoningModelUsesCompatibleInputs(t *testing.T) {
	// The fake rejects temperature/top_p/penalties for reasoning models exactly
	// as OpenAI does, so a pass proves the probes adapt to the model family.
	report := runAgainst(t, &fakeOpenAI{model: "gpt-5", reasoning: true}, Options{Model: "gpt-5", Suite: SuiteFull})
	for _, id := range []string{"chat_params", "chat_basic", "chat_tool_call", "chat_stream"} {
		if c := checkByID(report, id); c.Status != "pass" {
			t.Errorf("%s = %s %s %v", id, c.Status, c.Code, c.Evidence)
		}
	}
	if c := checkByID(report, "chat_logprobs"); c.Status != "skipped" || c.Code != "unsupported_model_family" {
		t.Errorf("chat_logprobs = %s %s", c.Status, c.Code)
	}
}

func TestMissingDoneMarkerAndUsageAreDetected(t *testing.T) {
	report := runAgainst(t, &fakeOpenAI{model: "gpt-4o", noDone: true, noUsage: true}, Options{Model: "gpt-4o"})
	if c := checkByID(report, "chat_stream"); c.Status != "fail" || !strings.Contains(evidenceLine(c.Evidence), "missing_done_marker") {
		t.Errorf("chat_stream = %s %s %v", c.Status, c.Code, c.Evidence)
	}
	if c := checkByID(report, "chat_stream_usage"); c.Status != "fail" || c.Code != "missing_usage_chunk" {
		t.Errorf("chat_stream_usage = %s %s", c.Status, c.Code)
	}
	if c := checkByID(report, "usage_fields"); c.Status != "fail" || c.Code != "usage_missing" {
		t.Errorf("usage_fields = %s %s", c.Status, c.Code)
	}
	if c := checkByID(report, "chat_basic"); c.Status != "fail" {
		t.Errorf("chat_basic with no usage = %s %s, want fail", c.Status, c.Code)
	}
	if report.Score == nil || *report.Score >= 100 {
		t.Errorf("score = %v, want < 100", report.Score)
	}
}

func TestRejectedInputIsAFailureNotInconclusive(t *testing.T) {
	report := runAgainst(t, &fakeOpenAI{model: "gpt-4o", rejectParams: true}, Options{Model: "gpt-4o"})
	c := checkByID(report, "chat_params")
	if c.Status != "fail" || c.Code != "request_rejected" {
		t.Fatalf("chat_params = %s %s", c.Status, c.Code)
	}
	if !strings.Contains(evidenceLine(c.Evidence), "seed") {
		t.Errorf("upstream error missing from evidence: %v", c.Evidence)
	}
	if checkByID(report, "chat_basic").Status != "pass" {
		t.Error("an unrelated rejected parameter must not fail the baseline")
	}
}

func TestToolCallReturnedAsTextFails(t *testing.T) {
	report := runAgainst(t, &fakeOpenAI{model: "gpt-4o", toolsAsText: true}, Options{Model: "gpt-4o"})
	for _, id := range []string{"chat_tool_call", "chat_tool_choice", "chat_tool_roundtrip"} {
		if c := checkByID(report, id); c.Status != "fail" || c.Code != "tool_call_invalid" && c.Code != "forced_tool_not_called" {
			t.Errorf("%s = %s %s", id, c.Status, c.Code)
		}
	}
}

func TestUnauthorizedStopsRunAndIsInconclusive(t *testing.T) {
	f := &fakeOpenAI{model: "gpt-4o", statusOverride: 401}
	report := runAgainst(t, f, Options{Model: "gpt-4o", Suite: SuiteFull})
	if c := checkByID(report, "chat_basic"); c.Status != "inconclusive" || c.Code != "unauthorized" {
		t.Fatalf("chat_basic = %s %s", c.Status, c.Code)
	}
	if report.StopReason != "baseline_unavailable" {
		t.Errorf("StopReason = %q", report.StopReason)
	}
	if report.Score != nil {
		t.Errorf("score = %d, want nil: nothing was decided", *report.Score)
	}
	if got := f.requests.Load(); got != 2 {
		t.Errorf("sent %d requests after a 401, want 2 (models + baseline)", got)
	}
	if c := checkByID(report, "chat_stream"); c.Status != "skipped" || c.Code != "run_stopped" {
		t.Errorf("chat_stream = %s %s", c.Status, c.Code)
	}
}

func TestCancelledRunSkipsRemaining(t *testing.T) {
	server := httptest.NewServer((&fakeOpenAI{model: "gpt-4o"}).handler())
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int64
	base := httpTransport(server.URL)
	transport := func(c context.Context, req Request) (Response, error) {
		resp, err := base(c, req)
		if calls.Add(1) == 3 {
			cancel()
		}
		return resp, err
	}
	report := Run(ctx, Options{Model: "gpt-4o", Suite: SuiteFull}, transport)
	if !report.Cancelled || report.StopReason != "cancelled" {
		t.Errorf("cancelled=%v stop=%q", report.Cancelled, report.StopReason)
	}
	if report.RequestsRun != 3 {
		t.Errorf("RequestsRun = %d, want 3: no request may start after cancel", report.RequestsRun)
	}
	for _, c := range report.Checks {
		if c.Status == "fail" {
			t.Errorf("%s failed after cancel: %s", c.ID, c.Code)
		}
	}
}

func TestOptionsValidation(t *testing.T) {
	cases := []struct {
		o  Options
		ok bool
	}{
		{Options{Model: "gpt-4o"}, true},
		{Options{Model: "gpt-4o", Suite: "full", LimitParam: "max_tokens"}, true},
		{Options{Model: ""}, false},
		{Options{Model: "x", Suite: "everything"}, false},
		{Options{Model: "x", LimitParam: "max_len"}, false},
		{Options{Model: strings.Repeat("a", 201)}, false},
	}
	for _, c := range cases {
		if ValidOptions(c.o) != c.ok {
			t.Errorf("ValidOptions(%+v) = %v", c.o, !c.ok)
		}
	}
}

func TestModelMatches(t *testing.T) {
	for _, c := range []struct {
		requested, observed string
		want                bool
	}{
		{"gpt-4o", "gpt-4o", true},
		{"gpt-4o", "gpt-4o-2024-08-06", true},
		{"gpt-4o", "gpt-4o-mini-2024-07-18", false},
		{"gpt-4o-mini", "gpt-4o", false},
		{"openai/gpt-4o", "gpt-4o-2024-11-20", true},
		{"gpt-4o-2024-08-06", "gpt-4o", true},
		{"gpt-4.1", "gpt-4.1-latest", true},
		{"gpt-4.1", "claude-sonnet-4", false},
	} {
		if got := modelMatches(c.requested, c.observed); got != c.want {
			t.Errorf("modelMatches(%q,%q) = %v", c.requested, c.observed, got)
		}
	}
}

func TestIsReasoningModel(t *testing.T) {
	for name, want := range map[string]bool{"gpt-5": true, "gpt-5-mini": true, "o3-mini": true, "o4-mini": true, "o1": true, "openai/gpt-5": true, "gpt-5-chat-latest": false, "gpt-4o": false, "gpt-4.1": false, "omni-moderation": false} {
		if IsReasoningModel(name) != want {
			t.Errorf("IsReasoningModel(%q) != %v", name, want)
		}
	}
}

func TestScoreIgnoresObservationsAndUndecided(t *testing.T) {
	report := Report{Checks: []Check{
		{ID: "a", Kind: "assertion", Status: "pass"}, {ID: "b", Kind: "assertion", Status: "pass"},
		{ID: "c", Kind: "assertion", Status: "fail"}, {ID: "d", Kind: "assertion", Status: "inconclusive"},
		{ID: "e", Kind: "observation", Status: "pass"}, {ID: "f", Kind: "assertion", Status: "skipped"},
	}}
	if s := Score(report); s == nil || *s != 67 {
		t.Errorf("score = %v, want 67", s)
	}
	if Score(Report{Checks: []Check{{Kind: "assertion", Status: "inconclusive"}}}) != nil {
		t.Error("score must be nil when nothing is decided")
	}
}

func TestReadSSE(t *testing.T) {
	stream := ": keep-alive\n\nevent: response.created\ndata: {\"type\":\"response.created\"}\n\ndata: {\"a\":1,\ndata: \"b\":2}\n\ndata: [DONE]\n\n"
	events, done, first, err := ReadSSE(strings.NewReader(stream), time.Now())
	if err != nil || !done || first == nil || len(events) != 2 {
		t.Fatalf("events=%d done=%v first=%v err=%v", len(events), done, first, err)
	}
	if events[0].Name != "response.created" || string(events[1].Data) != "{\"a\":1,\n\"b\":2}" {
		t.Errorf("unexpected frames: %+v", events)
	}
	if _, _, _, err := ReadSSE(strings.NewReader("data: {\"a\":1}"), time.Now()); err == nil {
		t.Error("an unterminated frame must be an error")
	}
	if _, done, _, err := ReadSSE(strings.NewReader("data: [DONE]"), time.Now()); err != nil || !done {
		t.Errorf("trailing [DONE] without blank line: done=%v err=%v", done, err)
	}
}

func TestFailureCode(t *testing.T) {
	for status, want := range map[int]string{401: "unauthorized", 403: "forbidden", 429: "rate_limited", 502: "upstream_error", 400: "request_rejected", 404: "request_rejected", 200: ""} {
		if got := FailureCode(status, nil); got != want {
			t.Errorf("FailureCode(%d) = %q, want %q", status, got, want)
		}
	}
}

func TestMarkdownReport(t *testing.T) {
	report := runAgainst(t, &fakeOpenAI{model: "gpt-4o", noDone: true}, Options{Model: "gpt-4o", Suite: SuiteStandard, Responses: true})
	report.Endpoint = "https://relay.example.com"
	md := Markdown(report)
	for _, want := range []string{"# OpenAI 模型检测报告", "兼容性得分", "Chat 流式", "Responses API", "chat_stream", "missing_done_marker", "需要关注的发现", "请求明细", "不能证明", "openai/gpt-oss", "relay.example.com"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q", want)
		}
	}
	for id := range specs {
		if _, ok := stageOf(id); !ok {
			t.Errorf("spec %s is not in any plan", id)
		}
	}
}

func stageOf(id string) (string, bool) {
	for _, item := range Plan(Options{Model: "x", Suite: SuiteFull, Responses: true, Vision: true, Logprobs: true}) {
		if item.ID == id {
			return item.Stage, true
		}
	}
	return "", false
}

func TestEveryPlannedCheckIsDocumented(t *testing.T) {
	for _, item := range Plan(Options{Model: "x", Suite: SuiteFull, Responses: true, Vision: true, Logprobs: true}) {
		if Describe(item.ID).Title == "" || Describe(item.ID).Source == "" {
			t.Errorf("check %s has no documentation", item.ID)
		}
	}
}

func TestResponsesToolStreamRequiresItemID(t *testing.T) {
	opts := Options{Model: "gpt-4o", Responses: true}
	good := runAgainst(t, &fakeOpenAI{model: "gpt-4o"}, opts)
	c := checkByID(good, "responses_tool_stream")
	if c.Status != "pass" || c.Code != "function_call_deltas_valid" {
		t.Fatalf("conforming stream: %s %s %v", c.Status, c.Code, c.Evidence)
	}
	if c.Evidence["item_id"] != "fc_1" {
		t.Errorf("evidence item_id = %v, want fc_1", c.Evidence["item_id"])
	}

	cases := []struct {
		name  string
		fake  *fakeOpenAI
		issue string
	}{
		{"missing item_id", &fakeOpenAI{model: "gpt-4o", noToolItemID: true}, "tool_delta_missing_item_id"},
		{"unknown item_id", &fakeOpenAI{model: "gpt-4o", wrongToolItem: true}, "tool_delta_unknown_item_id"},
	}
	for _, tc := range cases {
		report := runAgainst(t, tc.fake, opts)
		c := checkByID(report, "responses_tool_stream")
		if c.Status != "fail" || c.Code != "invalid_response" {
			t.Errorf("%s: %s %s", tc.name, c.Status, c.Code)
			continue
		}
		if issues, _ := c.Evidence["issues"].([]string); !contains(issues, tc.issue) {
			t.Errorf("%s: issues = %v, want %s", tc.name, issues, tc.issue)
		}
		if !contains(c.Evidence["issues"].([]string), "tool_args_done_"+strings.TrimPrefix(tc.issue, "tool_delta_")) {
			t.Errorf("%s: the done event must be checked too: %v", tc.name, c.Evidence["issues"])
		}
		// The text-only stream shares no function-call events and must be unaffected.
		if s := checkByID(report, "responses_stream"); s.Status != "pass" {
			t.Errorf("%s: responses_stream = %s", tc.name, s.Status)
		}
		if !strings.Contains(Markdown(report), tc.issue) {
			t.Errorf("%s: markdown does not surface %s", tc.name, tc.issue)
		}
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestResponsesStreamToolEventRules(t *testing.T) {
	frame := func(kind string, extra object) SSEEvent {
		extra["type"] = kind
		data, _ := common.Marshal(extra)
		return SSEEvent{Name: kind, Data: data}
	}
	item := object{"id": "fc_1", "type": "function_call", "call_id": "call_1", "name": "f", "arguments": `{"a":1}`, "status": "completed"}
	completed := frame("response.completed", object{"response": object{"id": "r", "object": "response", "created_at": 1, "status": "completed", "model": "m", "output": []any{item}, "usage": object{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2}}})
	created := frame("response.created", object{"response": object{"id": "r"}})
	added := frame("response.output_item.added", object{"output_index": 0, "item": object{"id": "fc_1", "type": "function_call", "call_id": "call_1", "name": "f", "arguments": ""}})
	d1 := frame("response.function_call_arguments.delta", object{"item_id": "fc_1", "output_index": 0, "delta": `{"a":`})
	d2 := frame("response.function_call_arguments.delta", object{"item_id": "fc_1", "output_index": 0, "delta": `1}`})
	done := frame("response.function_call_arguments.done", object{"item_id": "fc_1", "output_index": 0, "arguments": `{"a":1}`})

	if _, issues := parseResponsesStream([]SSEEvent{created, added, d1, d2, done, completed}); len(issues) != 0 {
		t.Fatalf("valid stream reported %v", issues)
	}
	for name, tc := range map[string]struct {
		events []SSEEvent
		issue  string
	}{
		"delta before item added": {[]SSEEvent{created, d1, d2, done, completed}, "tool_delta_unknown_item_id"},
		"wrong output_index":      {[]SSEEvent{created, added, frame("response.function_call_arguments.delta", object{"item_id": "fc_1", "output_index": 3, "delta": "x"}), completed}, "tool_delta_output_index_mismatch"},
		"done args differ":        {[]SSEEvent{created, added, d1, d2, frame("response.function_call_arguments.done", object{"item_id": "fc_1", "output_index": 0, "arguments": `{"a":2}`}), completed}, "tool_args_done_mismatch"},
		"incomplete added item":   {[]SSEEvent{created, frame("response.output_item.added", object{"output_index": 0, "item": object{"type": "function_call", "name": "f"}}), completed}, "tool_item_incomplete"},
		"no stream events":        {[]SSEEvent{created, completed}, "missing_function_call_stream_events"},
		"id differs from final":   {[]SSEEvent{created, frame("response.output_item.added", object{"output_index": 0, "item": object{"id": "fc_other", "type": "function_call", "call_id": "c", "name": "f"}}), completed}, "tool_item_id_differs_from_final"},
	} {
		if _, issues := parseResponsesStream(tc.events); !contains(issues, tc.issue) {
			t.Errorf("%s: issues = %v, want %s", name, issues, tc.issue)
		}
	}
}
