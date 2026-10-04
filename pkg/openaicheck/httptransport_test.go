package openaicheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"model-check/common"
)

func TestNormalizeBaseURL(t *testing.T) {
	good := map[string]string{
		"https://api.openai.com":                        "https://api.openai.com",
		"https://api.openai.com/v1":                     "https://api.openai.com",
		"https://api.openai.com/v1/":                    "https://api.openai.com",
		"https://relay.example.com/v1/chat/completions": "https://relay.example.com",
		"https://relay.example.com/openai/v1/responses": "https://relay.example.com/openai",
		"http://127.0.0.1:3000/":                        "http://127.0.0.1:3000",
	}
	for in, want := range good {
		if got, err := NormalizeBaseURL(in); err != nil || got != want {
			t.Errorf("NormalizeBaseURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "ftp://x.com", "https://user:pw@x.com", "https://x.com?key=1", "https://x.com#f", "not a url", "https://" + strings.Repeat("a", 2100)} {
		if _, err := NormalizeBaseURL(in); err == nil {
			t.Errorf("NormalizeBaseURL(%q) must fail", in)
		}
	}
}

func TestValidatePayload(t *testing.T) {
	ok := []string{`{"max_tokens":8}`, `{"max_completion_tokens":4096}`, `{"max_output_tokens":16}`, `{"model":"x"}`}
	bad := []string{`{"max_tokens":4097}`, `{"max_completion_tokens":-1}`, `{"max_output_tokens":1.5}`, `{"max_tokens":"8"}`, `{"x":"` + strings.Repeat("a", MaxRequestBytes) + `"}`}
	for _, body := range ok {
		if err := ValidatePayload([]byte(body)); err != nil {
			t.Errorf("%s rejected: %v", body, err)
		}
	}
	for _, body := range bad {
		if err := ValidatePayload([]byte(body)); err == nil {
			t.Errorf("%.40s accepted", body)
		}
	}
}

func TestRedactor(t *testing.T) {
	r := NewRedactor("sk-secret-key-123", "Bearer tok-abcdef")
	if got := r.String("bad key sk-secret-key-123 and tok-abcdef"); strings.Contains(got, "secret") || strings.Contains(got, "abcdef") {
		t.Errorf("not redacted: %s", got)
	}
	if got := string(r.JSON([]byte(`{"e":"key sk-secret-key-123"}`))); strings.Contains(got, "secret") {
		t.Errorf("json not redacted: %s", got)
	}
	r.Add("ab") // too short to be a credential; must not blank out ordinary text
	if r.String("about") != "about" {
		t.Error("short secrets must be ignored")
	}
}

func TestHTTPTransportEndToEndAndRedaction(t *testing.T) {
	const key = "sk-live-credential-9876"
	var sawAuth, sawAccept string
	fake := &fakeOpenAI{model: "gpt-4o"}
	inner := fake.handler()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		if r.URL.Path == "/v1/chat/completions" && strings.Contains(r.Header.Get("Accept"), "event-stream") {
			sawAccept = r.Header.Get("Accept")
		}
		if r.URL.Path == "/v1/models" {
			// A hostile or sloppy upstream that echoes the credential.
			writeJSON(w, 401, object{"error": object{"message": "Incorrect API key provided: " + key, "type": "invalid_request_error", "param": nil, "code": "invalid_api_key"}})
			return
		}
		inner.ServeHTTP(w, r)
	}))
	defer server.Close()

	redactor := NewRedactor()
	transport := NewHTTPTransport(server.Client(), server.URL, key, redactor)
	report := Run(context.Background(), Options{Model: "gpt-4o"}, transport)
	if sawAuth != "Bearer "+key {
		t.Errorf("Authorization = %q", sawAuth)
	}
	if !strings.Contains(sawAccept, "text/event-stream") {
		t.Error("streaming probes must ask for text/event-stream")
	}
	if c := checkByID(report, "models_list"); c.Status != "inconclusive" || c.Code != "unauthorized" {
		t.Errorf("models_list = %s %s", c.Status, c.Code)
	}
	data, err := common.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), key) {
		t.Fatal("test precondition: the upstream echo should reach the raw report")
	}
	safe := redactor.JSON(data)
	if strings.Contains(string(safe), key) {
		t.Error("credential leaked into the redacted report")
	}
	var parsed Report
	if err := common.Unmarshal(safe, &parsed); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(Markdown(parsed), key) {
		t.Error("credential leaked into the markdown")
	}
	if report.Score == nil || *report.Score != 100 {
		t.Errorf("score = %v; the 401 on /v1/models is an observation and must not move it", report.Score)
	}
}

func TestHTTPTransportDoesNotFollowRedirects(t *testing.T) {
	var followed bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	transport := NewHTTPTransport(server.Client(), server.URL, "sk-redirect-test", nil)
	resp, err := transport(context.Background(), Request{Method: http.MethodGet, Path: "/v1/models"})
	if err != nil || resp.Status != http.StatusTemporaryRedirect {
		t.Fatalf("status=%d err=%v", resp.Status, err)
	}
	if followed {
		t.Error("the redirect target was contacted; the bearer key could have been replayed")
	}
}

func TestHTTPTransportRejectsOversizedAndUnknownPaths(t *testing.T) {
	transport := NewHTTPTransport(http.DefaultClient, "http://127.0.0.1:1", "sk-test-key", nil)
	if _, err := transport(context.Background(), Request{Method: http.MethodPost, Path: "/v1/chat/completions", Body: object{"max_tokens": 99999}}); err == nil {
		t.Error("an oversized token limit must be refused before any request")
	}
	if _, err := transport(context.Background(), Request{Method: http.MethodGet, Path: "/admin/keys"}); err == nil {
		t.Error("only /v1/ probe paths may be requested")
	}
}

func TestNetworkFailureIsInconclusiveAndStopsRun(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close()
	report := Run(context.Background(), Options{Model: "gpt-4o"}, NewHTTPTransport(http.DefaultClient, url, "sk-test-key", nil))
	if c := checkByID(report, "chat_basic"); c.Status != "inconclusive" || c.Code != "network_error" {
		t.Errorf("chat_basic = %s %s", c.Status, c.Code)
	}
	if report.StopReason != "baseline_unavailable" || report.Score != nil {
		t.Errorf("stop=%q score=%v", report.StopReason, report.Score)
	}
}
