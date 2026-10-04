package imagecheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"model-check/pkg/openaicheck"
)

func TestNormalize(t *testing.T) {
	good := Input{Options: Options{Model: " gpt-image-2 "}, BaseURL: "https://relay.example/v1", Key: " sk-abcd "}
	ep, err := good.Normalize()
	if err != nil || ep != "https://relay.example" || good.Model != "gpt-image-2" || good.Key != "sk-abcd" {
		t.Fatalf("%q %v %+v", ep, err, good)
	}
	for name, in := range map[string]Input{
		"no key":          {Options: Options{Model: "m"}, BaseURL: "https://x.example"},
		"bad url":         {Options: Options{Model: "m"}, BaseURL: "ftp://x", Key: "sk-abcd"},
		"url creds":       {Options: Options{Model: "m"}, BaseURL: "https://u:p@x.example", Key: "sk-abcd"},
		"bad suite":       {Options: Options{Model: "m", Suite: "x"}, BaseURL: "https://x.example", Key: "sk-abcd"},
		"long remark":     {Options: Options{Model: "m"}, BaseURL: "https://x.example", Key: "sk-abcd", Remark: strings.Repeat("字", 201)},
		"newline key":     {Options: Options{Model: "m"}, BaseURL: "https://x.example", Key: "sk-ab\ncd"},
		"prov, no key":    {Options: Options{Model: "m", Provenance: true}, BaseURL: "https://x.example", Key: "sk-abcd"},
		"baseline no key": {Options: Options{Model: "m", Baseline: true}, BaseURL: "https://x.example", Key: "sk-abcd"},
		"bad verify key":  {Options: Options{Model: "m"}, BaseURL: "https://x.example", Key: "sk-abcd", VerifyKey: "ab"},
	} {
		in := in
		if _, err := in.Normalize(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	withKey := Input{Options: Options{Model: "m", Baseline: true}, BaseURL: "https://x.example", Key: "sk-abcd", VerifyKey: "sk-verify"}
	if _, err := withKey.Normalize(); err != nil || !withKey.Provenance {
		t.Fatalf("baseline must imply provenance: %v %+v", err, withKey.Options)
	}
}

func TestBuildWiresKeysToTheRightHosts(t *testing.T) {
	var relayAuth, officialAuth atomic.Value
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		relayAuth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(400)
	}))
	defer relay.Close()
	official := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		officialAuth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(404)
	}))
	defer official.Close()
	in := Input{Options: Options{Model: "gpt-image-2", Provenance: true, Baseline: true}, BaseURL: relay.URL, Key: "sk-relay-key", VerifyKey: "sk-official-key"}
	ep, err := in.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	red := openaicheck.NewRedactor(in.Key, in.VerifyKey)
	tr, deps := Build(Plumbing{Client: http.DefaultClient, Endpoint: ep, OfficialBase: official.URL, Input: in, Redactor: red})
	if deps.Verify == nil || deps.Baseline == nil || deps.Fetch == nil {
		t.Fatal("deps missing")
	}
	_, _ = tr(context.Background(), Request{Method: "POST", Path: "/v1/images/generations", Body: map[string]any{"model": "m"}})
	_, _ = deps.Baseline(context.Background(), Request{Method: "POST", Path: "/v1/images/generations", Body: map[string]any{"model": "m"}})
	_, _ = deps.Verify(context.Background(), []byte("x"), "image/png")
	if relayAuth.Load() != "Bearer sk-relay-key" {
		t.Errorf("relay saw %v", relayAuth.Load())
	}
	if officialAuth.Load() != "Bearer sk-official-key" {
		t.Errorf("official host saw %v", officialAuth.Load())
	}
	// Without a verify key nothing official is wired.
	in2 := Input{Options: Options{Model: "m"}, BaseURL: relay.URL, Key: "sk-relay-key"}
	ep2, _ := in2.Normalize()
	_, deps2 := Build(Plumbing{Client: http.DefaultClient, Endpoint: ep2, Input: in2})
	if deps2.Verify != nil || deps2.Baseline != nil {
		t.Fatal("official collaborators wired without a key")
	}
}

func TestVerifyErrorsAreRedacted(t *testing.T) {
	in := Input{Options: Options{Model: "m", Provenance: true}, BaseURL: "https://x.example", Key: "sk-relay-key", VerifyKey: "sk-official-key"}
	ep, _ := in.Normalize()
	_, deps := Build(Plumbing{Client: http.DefaultClient, Endpoint: ep, OfficialBase: "http://127.0.0.1:1", Input: in, Redactor: openaicheck.NewRedactor(in.Key, in.VerifyKey)})
	_, err := deps.Verify(context.Background(), []byte("x"), "image/png")
	if err == nil || strings.Contains(err.Error(), "sk-official-key") {
		t.Fatalf("err=%v", err)
	}
}

func TestFetcher(t *testing.T) {
	var sawAuth atomic.Value
	mux := http.NewServeMux()
	mux.HandleFunc("/img", func(w http.ResponseWriter, r *http.Request) {
		sawAuth.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte("PNGDATA"))
	})
	mux.HandleFunc("/r1", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/r2", 302) })
	mux.HandleFunc("/r2", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/img", 302) })
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/loop", 302) })
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, MaxImageBytes+10))
	})
	mux.HandleFunc("/missing", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	validated := 0
	p := Plumbing{Client: noRedirect, ValidateURL: func(context.Context, string) error { validated++; return nil }}
	fetch := p.fetcher()
	got, err := fetch(context.Background(), srv.URL+"/r1")
	if err != nil || string(got) != "PNGDATA" {
		t.Fatalf("%q %v", got, err)
	}
	if validated != 3 {
		t.Errorf("each hop must be validated, got %d", validated)
	}
	if sawAuth.Load() != "" {
		t.Errorf("credentials leaked to the image host: %v", sawAuth.Load())
	}
	for path, name := range map[string]string{"/loop": "loop", "/big": "big", "/missing": "404"} {
		if _, err := fetch(context.Background(), srv.URL+path); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	for _, bad := range []string{"file:///etc/passwd", "ftp://x/y", "https://user:pw@x.example/y", "::"} {
		if _, err := fetch(context.Background(), bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	blocked := Plumbing{Client: noRedirect, ValidateURL: func(context.Context, string) error { return errors.New("private address") }}.fetcher()
	if _, err := blocked(context.Background(), srv.URL+"/img"); err == nil {
		t.Fatal("policy block ignored")
	}
}

func TestSnapshotNeverCarriesThumbnails(t *testing.T) {
	r := Report{Images: []ImageRef{{Thumb: "data:image/jpeg;base64,AAAA"}}, Samples: []Sample{{}, {}}, Checks: []Check{{Kind: KindAssertion, Status: "pass"}, {Kind: KindAssertion, Status: "fail"}, {Kind: KindProvenance, Status: "inconclusive"}}}
	s := Snapshot(r)
	if s.Images[0].Thumb != "" || r.Images[0].Thumb == "" {
		t.Fatal("snapshot must strip a copy, not the live report")
	}
	if s.RequestsRun != 2 || s.Summary["pass"] != 1 || s.Summary["fail"] != 1 || s.Summary["inconclusive"] != 1 || s.Score == nil || *s.Score != 50 {
		t.Fatalf("%+v %v", s.Summary, s.Score)
	}
}
