package provenance

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const trustedBody = `{"object":"content_provenance_check","created_at":1790000000,"results":[
 {"type":"c2pa","outcome":"detected","validation_state":"trusted","issuer":"OpenAI","model":"gpt-image-2-2026-04-21","generated_at":"2026-10-01T00:00:00Z"},
 {"type":"synthid","outcome":"detected","model":null,"generated_at":null}]}`

func fake(t *testing.T, status int, body string, hdr map[string]string) (*Client, *struct{ got, auth, ctype, filename, mime string }) {
	t.Helper()
	seen := &struct{ got, auth, ctype, filename, mime string }{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.got, seen.auth, seen.ctype = r.Method+" "+r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err == nil {
			f, h, err := r.FormFile("file")
			if err == nil {
				b, _ := io.ReadAll(f)
				seen.filename, seen.mime = h.Filename, h.Header.Get("Content-Type")
				if len(b) == 0 {
					t.Error("empty upload")
				}
			}
		}
		for k, v := range hdr {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.Client(), "sk-verify-secret")
	c.BaseURL = srv.URL
	return c, seen
}

func TestCheckTrusted(t *testing.T) {
	c, seen := fake(t, 200, trustedBody, nil)
	res, err := c.Check(t.Context(), []byte("\x89PNG\r\n\x1a\nxxxx"), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if seen.got != "POST /v1/content_provenance_checks" || seen.auth != "Bearer sk-verify-secret" || !strings.HasPrefix(seen.ctype, "multipart/form-data") || seen.mime != "image/png" || seen.filename != "image.png" {
		t.Fatalf("request shape: %+v", seen)
	}
	v := Classify(res, "gpt-image-2")
	if v.Level != LevelTrusted || !v.SynthID || v.ModelMatch == nil || !*v.ModelMatch || v.Issuer != "OpenAI" {
		t.Fatalf("verdict %+v", v)
	}
	if v := Classify(res, "gpt-image-1"); v.ModelMatch == nil || *v.ModelMatch {
		t.Fatal("model mismatch not flagged")
	}
}

func TestClassifyLevels(t *testing.T) {
	none := &Result{C2PA: &C2PA{Outcome: "not_detected", ValidationState: "not_present"}, SynthID: &SynthID{Outcome: "not_detected"}}
	if v := Classify(none, "m"); v.Level != LevelNone || v.SynthID {
		t.Fatalf("none: %+v", v)
	}
	watermarkOnly := &Result{C2PA: &C2PA{Outcome: "not_detected", ValidationState: "not_present"}, SynthID: &SynthID{Outcome: "detected"}}
	if v := Classify(watermarkOnly, "m"); v.Level != LevelSynthID || v.ModelMatch != nil {
		t.Fatalf("synthid: %+v", v)
	}
	valid := &Result{C2PA: &C2PA{Outcome: "detected", ValidationState: "valid", Issuer: "Someone", Model: "x"}, SynthID: &SynthID{Outcome: "not_detected"}}
	if v := Classify(valid, "x"); v.Level != LevelUntrusted {
		t.Fatalf("valid but untrusted: %+v", v)
	}
	foreign := &Result{C2PA: &C2PA{Outcome: "detected", ValidationState: "trusted", Issuer: "Acme Corp"}}
	if v := Classify(foreign, "x"); v.Level != LevelUntrusted {
		t.Fatalf("non-OpenAI issuer must not be trusted: %+v", v)
	}
	if v := Classify(nil, "x"); v.Level != LevelNone {
		t.Fatal("nil result")
	}
}

func TestModelsMatch(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"gpt-image-2", "gpt-image-2-2026-04-21", true},
		{"openai/GPT-Image-2", "gpt-image-2", true},
		{"gpt-image-1", "gpt-image-1-mini", false},
		{"gpt-image-1", "gpt-image-2", false},
		{"", "", false},
	}
	for _, c := range cases {
		if ModelsMatch(c.a, c.b) != c.want {
			t.Errorf("%q vs %q", c.a, c.b)
		}
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		status int
		kind   string
		hdr    map[string]string
	}{
		{404, "no_access", nil},
		{429, "rate_limited", map[string]string{"Retry-After": "7"}},
		{400, "rejected", nil},
		{401, "unauthorized", nil},
		{503, "upstream_error", nil},
	}
	for _, tc := range cases {
		c, _ := fake(t, tc.status, `{"error":{"code":"rate_limit_exceeded","message":"slow down"}}`, tc.hdr)
		_, err := c.Check(t.Context(), []byte("x"), "image/png")
		var api *APIError
		if !errors.As(err, &api) || api.Kind() != tc.kind {
			t.Fatalf("status %d: %v", tc.status, err)
		}
		if tc.status == 429 && (api.RetryAfter != "7" || api.Code != "rate_limit_exceeded") {
			t.Fatalf("429 detail %+v", api)
		}
	}
}

func TestBadResponsesAndInputs(t *testing.T) {
	for _, body := range []string{`not json`, `{"object":"other","results":[]}`, `{"object":"content_provenance_check","results":[{"type":"c2pa","outcome":"maybe"}]}`} {
		c, _ := fake(t, 200, body, nil)
		if _, err := c.Check(t.Context(), []byte("x"), "image/png"); err == nil {
			t.Fatalf("accepted %q", body)
		}
	}
	c, _ := fake(t, 200, trustedBody, nil)
	if _, err := c.Check(t.Context(), []byte("x"), "image/gif"); err == nil {
		t.Fatal("gif accepted")
	}
	if _, err := c.Check(t.Context(), nil, "image/png"); err == nil {
		t.Fatal("empty accepted")
	}
	c.Key = ""
	if _, err := c.Check(t.Context(), []byte("x"), "image/png"); err == nil {
		t.Fatal("missing key accepted")
	}
}

func TestRedirectNotFollowedAndKeyRedacted(t *testing.T) {
	hit := false
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer other.Close()
	c, _ := fake(t, http.StatusFound, ``, map[string]string{"Location": other.URL})
	if _, err := c.Check(t.Context(), []byte("x"), "image/png"); err == nil {
		t.Fatal("redirect treated as success")
	}
	if hit {
		t.Fatal("redirect was followed")
	}
	dead := NewClient(http.DefaultClient, "sk-verify-secret")
	dead.BaseURL = "http://127.0.0.1:1"
	if _, err := dead.Check(t.Context(), []byte("x"), "image/png"); err == nil || strings.Contains(err.Error(), "sk-verify-secret") {
		t.Fatalf("err=%v", err)
	}
}
