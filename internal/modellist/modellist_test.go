package modellist

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestModelsURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://api.example.com":                           "https://api.example.com/v1/models",
		"https://api.example.com/":                          "https://api.example.com/v1/models",
		"https://api.example.com/v1":                        "https://api.example.com/v1/models",
		"https://api.example.com/v1/messages":               "https://api.example.com/v1/models",
		"https://api.example.com/proxy/v1/chat/completions": "https://api.example.com/proxy/v1/models",
		"http://localhost:3000/v1/models":                   "http://localhost:3000/v1/models",
	} {
		got, err := ModelsURL(in)
		if err != nil || got != want {
			t.Fatalf("%q -> %q %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ftp://x.com", "https://u:p@x.com", "https://x.com/?a=1", "https://x.com/#f", "x.com"} {
		if _, err := ModelsURL(bad); err == nil {
			t.Fatalf("%q must be refused", bad)
		}
	}
}

func TestParseShapes(t *testing.T) {
	for _, body := range []string{
		`{"data":[{"id":"a"},{"id":"b"},{"id":"a"}]}`,
		`{"models":["a","models/b"]}`,
		`[{"name":"a"},{"id":"b"}]`,
	} {
		ids, err := Parse([]byte(body))
		if err != nil || !reflect.DeepEqual(ids, []string{"a", "b"}) {
			t.Fatalf("%s -> %v %v", body, ids, err)
		}
	}
	if _, err := Parse([]byte(`<html>`)); err != ErrFormat {
		t.Fatal("html must be a format error")
	}
	if _, err := Parse([]byte(`{"x":1}`)); err != ErrFormat {
		t.Fatal("unknown shape must be a format error")
	}
}

func TestSelect(t *testing.T) {
	all := []string{"claude-sonnet-4-5", "gpt-4o", "text-embedding-3-small", "gpt-image-1", "claude-haiku-4-5", "o3"}
	if r := Select(all, KindClaude); !r.Filtered || !reflect.DeepEqual(r.Models, []string{"claude-sonnet-4-5", "claude-haiku-4-5"}) {
		t.Fatalf("%+v", r)
	}
	if r := Select(all, KindOpenAI); !reflect.DeepEqual(r.Models, []string{"o3", "gpt-4o"}) {
		t.Fatalf("%+v", r)
	}
	if r := Select(all, KindImage); !reflect.DeepEqual(r.Models, []string{"gpt-image-1"}) {
		t.Fatalf("%+v", r)
	}
	r := Select([]string{"foo", "bar"}, KindClaude)
	if r.Filtered || len(r.Models) != 2 || r.Total != 2 {
		t.Fatalf("unmatched must fall back to all: %+v", r)
	}
}

func TestFetchSendsKeyAndMapsErrors(t *testing.T) {
	var gotAuth, gotX string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotX = r.Header.Get("Authorization"), r.Header.Get("x-api-key")
		switch r.URL.Path {
		case "/v1/models":
			if gotAuth != "Bearer sk-good" {
				w.WriteHeader(401)
				return
			}
			w.Write([]byte(`{"data":[{"id":"claude-opus-4-1"},{"id":"gpt-4o"}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	hc := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := Fetch(context.Background(), hc, nil, srv.URL, "sk-good", KindClaude)
	if err != nil || !reflect.DeepEqual(res.Models, []string{"claude-opus-4-1"}) || gotX != "sk-good" {
		t.Fatalf("%+v %v x=%q", res, err, gotX)
	}
	if _, err := Fetch(context.Background(), hc, nil, srv.URL, "sk-bad", KindOpenAI); Code(err) != "auth" {
		t.Fatalf("want auth, got %v", err)
	}
	if _, err := Fetch(context.Background(), hc, nil, srv.URL+"/nope", "sk-good", KindOpenAI); Code(err) != "not_found" {
		t.Fatalf("want not_found, got %v", err)
	}
	if _, err := Fetch(context.Background(), hc, nil, "https://x.com/?k=1", "k", KindOpenAI); Code(err) != "url" {
		t.Fatal("want url")
	}
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.invalid/", 302)
	}))
	defer redir.Close()
	if _, err := Fetch(context.Background(), hc, nil, redir.URL, "sk-good", KindOpenAI); err == nil {
		t.Fatal("a redirect must not be followed")
	}
}
