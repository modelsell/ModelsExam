package geminicheck

import (
	"context"
	"strings"
	"testing"

	"model-check/common"
)

func TestConformingEndpointPassesFullSuite(t *testing.T) {
	f := &fakeGemini{model: "gemini-test"}
	report := runAgainst(t, f, Options{Model: "models/gemini-test", Suite: SuiteFull, Vision: true})
	for _, c := range report.Checks {
		if c.Status == "fail" || c.Status == "inconclusive" || c.Status == "skipped" {
			t.Errorf("%s: %s %s %v", c.ID, c.Status, c.Code, c.Evidence)
		}
	}
	if report.Score == nil || *report.Score != 100 {
		t.Fatalf("score = %v, want 100", report.Score)
	}
	if report.Model != "gemini-test" || report.Provider != "gemini" {
		t.Errorf("model/provider = %s/%s", report.Model, report.Provider)
	}
	if got, max := int(f.requests.Load()), MaxRequests(*report.Options); got != max {
		t.Errorf("sent %d requests, bound was %d", got, max)
	}
	if report.RequestsRun != int(f.requests.Load()) {
		t.Errorf("RequestsRun = %d, server saw %d", report.RequestsRun, f.requests.Load())
	}
	if !f.sawKeyHeader.Load() || f.sawQueryKey.Load() {
		t.Errorf("key must travel in x-goog-api-key only (header=%v query=%v)", f.sawKeyHeader.Load(), f.sawQueryKey.Load())
	}
	for _, item := range report.Plan {
		if checkByID(report, item.ID).ID == "" {
			t.Errorf("plan item %s has no check", item.ID)
		}
		if Describe(item.ID).Title == "" {
			t.Errorf("plan item %s is not documented", item.ID)
		}
	}
	md := Markdown(report)
	if strings.Contains(md, testKey) || !strings.Contains(md, "gemini_tool_roundtrip") {
		t.Error("markdown leaked the key or lost a check")
	}
}

func TestStandardSuiteSkipsOptionalChecks(t *testing.T) {
	report := runAgainst(t, &fakeGemini{model: "gemini-test"}, Options{Model: "gemini-test"})
	for _, id := range []string{"gemini_candidates", "gemini_thinking", "gemini_vision", "gemini_parallel_tools", "gemini_tool_stream"} {
		if c := checkByID(report, id); c.Status != "skipped" || c.Code != "not_requested" {
			t.Errorf("%s = %s %s, want skipped not_requested", id, c.Status, c.Code)
		}
	}
	if report.Score == nil || *report.Score != 100 {
		t.Errorf("score = %v", report.Score)
	}
}

func TestInvalidKeyStopsRunAndIsInconclusive(t *testing.T) {
	f := &fakeGemini{model: "gemini-test", badKey: true}
	report := runAgainst(t, f, Options{Model: "gemini-test"})
	if c := checkByID(report, "gemini_basic"); c.Status != "inconclusive" || c.Code != "unauthorized" {
		t.Errorf("gemini_basic = %s %s", c.Status, c.Code)
	}
	if report.StopReason != "baseline_unavailable" || report.Summary["fail"] != 0 {
		t.Errorf("stop=%s fail=%d", report.StopReason, report.Summary["fail"])
	}
	if f.requests.Load() != 2 { // models.get and the baseline only
		t.Errorf("requests = %d", f.requests.Load())
	}
}

func TestProtocolLeaksAreFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		fake  *fakeGemini
		check string
		issue string
	}{
		"lowercase finishReason": {&fakeGemini{lowerFinish: true}, "gemini_basic", "invalid_finish_reason"},
		"[DONE] terminator":      {&fakeGemini{doneMarker: true}, "gemini_stream", "unexpected_done_marker"},
		"missing usageMetadata":  {&fakeGemini{noUsage: true}, "gemini_basic", "missing_usage_metadata"},
		"string function args":   {&fakeGemini{stringArgs: true}, "gemini_tool_call", "function_call_args_not_object"},
	} {
		f := tc.fake
		f.model = "gemini-test"
		report := runAgainst(t, f, Options{Model: "gemini-test"})
		c := checkByID(report, tc.check)
		issues, _ := c.Evidence["issues"].([]string)
		if c.Status != "fail" || !hasIssue(issues, tc.issue) {
			t.Errorf("%s: %s = %s %s %v", name, tc.check, c.Status, c.Code, c.Evidence)
		}
	}
}

func TestOpenAIErrorEnvelopeFailsErrorShape(t *testing.T) {
	report := runAgainst(t, &fakeGemini{model: "gemini-test", openAIErrors: true}, Options{Model: "gemini-test", Suite: SuiteBasic})
	c := checkByID(report, "gemini_error_shape")
	issues, _ := c.Evidence["issues"].([]any)
	if c.Status != "fail" || c.Code != "error_shape_mismatch" {
		t.Errorf("gemini_error_shape = %s %s %v %v", c.Status, c.Code, c.Evidence, issues)
	}
}

func TestCountTokensMustMatchPromptUsage(t *testing.T) {
	report := runAgainst(t, &fakeGemini{model: "gemini-test", countOffset: 3}, Options{Model: "gemini-test"})
	if c := checkByID(report, "gemini_count_tokens"); c.Status != "fail" || c.Code != "count_differs_from_usage" {
		t.Errorf("gemini_count_tokens = %s %s %v", c.Status, c.Code, c.Evidence)
	}
}

func TestOptionsAndModelValidation(t *testing.T) {
	for model, ok := range map[string]bool{
		"gemini-2.5-flash": true, "models/gemini-3.8-flash": true, "gemini-2.0-flash-001": true,
		"": false, "models/": false, "../secrets": false, "gemini:generateContent": false, "a/b": false, "gemini?x=1": false, "..": false,
	} {
		if got := ValidOptions(Options{Model: model}); got != ok {
			t.Errorf("ValidOptions(%q) = %v", model, got)
		}
	}
	if ValidOptions(Options{Model: "gemini-test", Suite: "huge"}) {
		t.Error("unknown suite accepted")
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://generativelanguage.googleapis.com":                                                "https://generativelanguage.googleapis.com",
		"https://generativelanguage.googleapis.com/v1beta":                                         "https://generativelanguage.googleapis.com",
		"https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent": "https://generativelanguage.googleapis.com",
		"https://relay.example/gemini/v1beta/":                                                     "https://relay.example/gemini",
	} {
		if got, err := NormalizeBaseURL(in); err != nil || got != want {
			t.Errorf("NormalizeBaseURL(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"ftp://x", "https://x/v1beta?key=abc", "https://user:pw@x", "not a url"} {
		if _, err := NormalizeBaseURL(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestTransportRefusesForeignPathsAndOversizedLimits(t *testing.T) {
	transport := NewHTTPTransport(nil, "https://x", testKey, nil)
	if _, err := transport(context.Background(), Request{Method: "GET", Path: "/v1/models"}); err == nil {
		t.Error("non-v1beta path accepted")
	}
	body, _ := common.Marshal(map[string]any{"contents": []any{}, "generationConfig": map[string]any{"maxOutputTokens": 100000}})
	if ValidatePayload(body) == nil {
		t.Error("oversized maxOutputTokens accepted")
	}
}

func TestPlanBudgetMatchesSuites(t *testing.T) {
	for options, want := range map[Options]int{
		{Model: "m", Suite: SuiteBasic}:              4,  // model, basic, stream, error
		{Model: "m"}:                                 16, // + system, 4 inputs, 2 tools, round trip x2, 2 formats, countTokens
		{Model: "m", Suite: SuiteFull, Vision: true}: 22, // + candidates, thinking, none, tool stream, parallel, vision
	} {
		if got := MaxRequests(options); got != want {
			t.Errorf("MaxRequests(%+v) = %d, want %d", options, got, want)
		}
	}
}
