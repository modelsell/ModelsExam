package httpgzip

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(ct, body string, hdr map[string]string) *httptest.ResponseRecorder {
	h := Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Length", "999")
		_, _ = io.WriteString(w, body)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCompressesHTML(t *testing.T) {
	body := strings.Repeat("<p>hello</p>", 100)
	rec := serve("text/html; charset=utf-8", body, map[string]string{"Accept-Encoding": "gzip, br"})
	if rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("Content-Length") != "" || !strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("headers: %v", rec.Header())
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(zr)
	if string(got) != body {
		t.Fatal("round trip failed")
	}
}

func TestSkips(t *testing.T) {
	for name, c := range map[string]struct {
		ct  string
		hdr map[string]string
	}{
		"no accept": {"text/html", nil},
		"sse":       {"text/event-stream", map[string]string{"Accept-Encoding": "gzip"}},
		"png":       {"image/png", map[string]string{"Accept-Encoding": "gzip"}},
		"range":     {"text/html", map[string]string{"Accept-Encoding": "gzip", "Range": "bytes=0-5"}},
	} {
		rec := serve(c.ct, "data", c.hdr)
		if rec.Header().Get("Content-Encoding") != "" || rec.Body.String() != "data" {
			t.Fatalf("%s: must pass through, got %v", name, rec.Header())
		}
	}
}
