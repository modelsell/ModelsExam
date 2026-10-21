package openaicheck

import (
	"strings"
	"testing"
)

func injectionEvidence(t *testing.T, report Report) Check {
	t.Helper()
	c := checkByID(report, "prompt_injection")
	if c.ID == "" {
		t.Fatal("prompt_injection check missing")
	}
	return c
}

func TestPromptInjectionCleanEndpointPasses(t *testing.T) {
	f := &fakeOpenAI{model: "gpt-4o"}
	report := runAgainst(t, f, Options{Model: "gpt-4o", Suite: SuiteBasic})
	c := injectionEvidence(t, report)
	if c.Status != "pass" || c.Code != "no_obvious_injection" || c.Kind != "assertion" || c.Stage != StageProtocol {
		t.Fatalf("prompt_injection = %+v", c)
	}
	if got := c.Evidence["valid_samples"]; got != PromptInjectionRequests {
		t.Errorf("valid_samples = %v", got)
	}
	if got := c.Evidence["estimated_extra_tokens"]; got != int64(0) {
		t.Errorf("estimated_extra_tokens = %v", got)
	}
	if got := c.Evidence["score"]; got != 100 {
		t.Errorf("score = %v", got)
	}
	// chat_basic is reused as the first sample, so only two probes are added.
	var probes []string
	for _, s := range report.Samples {
		if strings.HasPrefix(s.Probe, "prompt_injection") {
			probes = append(probes, s.Probe)
		}
	}
	if strings.Join(probes, ",") != "prompt_injection_r2,prompt_injection_r3" {
		t.Errorf("probes = %v", probes)
	}
	if got, bound := int(f.requests.Load()), MaxRequests(Options{Model: "gpt-4o", Suite: SuiteBasic}); got != bound {
		t.Errorf("basic suite sent %d requests, bound %d", got, bound)
	}
}

func TestPromptInjectionScoresAllowanceBoundary(t *testing.T) {
	for _, tc := range []struct {
		input  int
		status string
		score  int
		extra  int64
	}{{64, "pass", 100, 0}, {65, "fail", 98, 1}, {128, "fail", 50, 64}, {2100, "fail", 3, 2036}} {
		f := &fakeOpenAI{model: "gpt-4o", chatInput: func(int64) (int, int) { return tc.input, 0 }}
		c := injectionEvidence(t, runAgainst(t, f, Options{Model: "gpt-4o", Suite: SuiteBasic}))
		if c.Status != tc.status || c.Evidence["score"] != tc.score || c.Evidence["max_extra_tokens"] != tc.extra {
			t.Errorf("input %d: %s %s %v", tc.input, c.Status, c.Code, c.Evidence)
		}
		if tc.status == "fail" && (c.Code != "extra_input_tokens" || c.Evidence["extra_rounds"] != 3) {
			t.Errorf("input %d: %s %v", tc.input, c.Code, c.Evidence)
		}
	}
}

func TestPromptInjectionKeepsSingleAnomalousRound(t *testing.T) {
	f := &fakeOpenAI{model: "gpt-4o", chatInput: func(n int64) (int, int) {
		if n == 3 {
			return 2060, 0
		}
		return 12, 0
	}}
	c := injectionEvidence(t, runAgainst(t, f, Options{Model: "gpt-4o", Suite: SuiteBasic}))
	if c.Status != "fail" || c.Evidence["extra_rounds"] != 1 || c.Evidence["max_extra_tokens"] != int64(1996) || c.Evidence["estimated_extra_tokens"] != int64(665) {
		t.Fatalf("prompt_injection = %s %s %v", c.Status, c.Code, c.Evidence)
	}
	if score := c.Evidence["score"].(int); score != 68 {
		t.Errorf("score = %d", score)
	}
}

func TestPromptInjectionCountsSeparatelyReportedCache(t *testing.T) {
	// A converted upstream reporting a cached hidden prompt outside
	// prompt_tokens must not hide it.
	f := &fakeOpenAI{model: "gpt-4o", chatInput: func(int64) (int, int) { return 12, 1800 }}
	c := injectionEvidence(t, runAgainst(t, f, Options{Model: "gpt-4o", Suite: SuiteBasic}))
	if c.Status != "fail" || c.Evidence["max_extra_tokens"] != int64(1748) {
		t.Fatalf("prompt_injection = %s %v", c.Status, c.Evidence)
	}
	// The OpenAI contract counts cached tokens inside prompt_tokens.
	f = &fakeOpenAI{model: "gpt-4o", chatInput: func(int64) (int, int) { return 40, 32 }}
	if c := injectionEvidence(t, runAgainst(t, f, Options{Model: "gpt-4o", Suite: SuiteBasic})); c.Status != "pass" {
		t.Fatalf("prompt_injection = %s %v", c.Status, c.Evidence)
	}
}

func TestPromptInjectionWithoutUsageIsInconclusive(t *testing.T) {
	c := injectionEvidence(t, runAgainst(t, &fakeOpenAI{model: "gpt-4o", noUsage: true}, Options{Model: "gpt-4o", Suite: SuiteBasic}))
	if c.Status != "inconclusive" || c.Code != "no_usable_usage" {
		t.Fatalf("prompt_injection = %s %s", c.Status, c.Code)
	}
}

func TestPromptInjectionStopsAtRateLimit(t *testing.T) {
	f := &fakeOpenAI{model: "gpt-4o", failAfterChat: 1}
	report := runAgainst(t, f, Options{Model: "gpt-4o", Suite: SuiteBasic})
	c := injectionEvidence(t, report)
	// The baseline sample still counts; the rate-limited round is not retried.
	if c.Status != "pass" || c.Evidence["valid_samples"] != 1 {
		t.Fatalf("prompt_injection = %s %s %v", c.Status, c.Code, c.Evidence)
	}
	for _, s := range report.Samples {
		if s.Probe == "prompt_injection_r3" {
			t.Fatal("kept probing after a rate limit")
		}
	}
}

func TestPromptInjectionSkippedWhenBaselineRejected(t *testing.T) {
	report := runAgainst(t, &fakeOpenAI{model: "gpt-4o", statusOverride: 401}, Options{Model: "gpt-4o", Suite: SuiteBasic})
	if c := injectionEvidence(t, report); c.Status != "skipped" {
		t.Fatalf("prompt_injection = %s %s", c.Status, c.Code)
	}
}

func TestPromptInjectionInMarkdown(t *testing.T) {
	f := &fakeOpenAI{model: "gpt-4o", chatInput: func(int64) (int, int) { return 900, 0 }}
	md := Markdown(runAgainst(t, f, Options{Model: "gpt-4o", Suite: SuiteBasic}))
	for _, want := range []string{"提示词注入 `prompt_injection`", "extra_input_tokens", "estimated_extra_tokens=836"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q", want)
		}
	}
}
