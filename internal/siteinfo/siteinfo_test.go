package siteinfo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParsePrefersSiteNameAndDescription(t *testing.T) {
	got := Parse([]byte(`<html><head><title>Home | Fallback</title>
<meta property="og:site_name" content="Relay &amp; Co">
<meta name='description' content="Cheap   and
fast  API relay">
</head></html>`))
	if got.Name != "Relay & Co" || got.Description != "Cheap and fast API relay" {
		t.Fatalf("%+v", got)
	}
}

func TestParseFallsBackToTitleAndOG(t *testing.T) {
	got := Parse([]byte(`<title> My Relay </title><meta property="og:description" content="About us">`))
	if got.Name != "My Relay" || got.Description != "About us" {
		t.Fatalf("%+v", got)
	}
}

func TestCleanLimitsAndStripsControl(t *testing.T) {
	got := Clean("a\x00b‮"+strings.Repeat("字", 400), 10)
	if strings.ContainsAny(got, "\x00") || len([]rune(got)) > 11 {
		t.Fatalf("%q", got)
	}
	if Parse([]byte("")).Name != "" {
		t.Fatal("empty page should give empty info")
	}
}

func TestFetchReadsOriginHomePageAndSendsNoCredentials(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<title>Site</title><meta name="description" content="Desc">`))
	}))
	defer srv.Close()
	info := Fetch(context.Background(), srv.Client(), nil, srv.URL+"/v1/messages?x=1")
	if info.Name != "Site" || info.Description != "Desc" || gotPath != "/" || gotAuth != "" {
		t.Fatalf("%+v path=%q auth=%q", info, gotPath, gotAuth)
	}
}

func TestFetchFollowsRedirectsButValidatesEveryHop(t *testing.T) {
	var final *httptest.Server
	final = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<title>Final</title>`))
	}))
	defer final.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+"/home", http.StatusMovedPermanently)
	}))
	defer first.Close()
	hc := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if got := Fetch(context.Background(), hc, nil, first.URL); got.Name != "Final" {
		t.Fatalf("redirect not followed: %+v", got)
	}
	var seen []string
	deny := func(_ context.Context, raw string) error {
		seen = append(seen, raw)
		if strings.HasPrefix(raw, final.URL) {
			return errors.New("blocked")
		}
		return nil
	}
	if got := Fetch(context.Background(), hc, deny, first.URL); got.Name != "" || len(seen) != 2 {
		t.Fatalf("blocked hop must give empty info: %+v seen=%v", got, seen)
	}
}

func TestFetchIgnoresNonHTMLAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"title":"x"}`))
	}))
	defer srv.Close()
	if got := Fetch(context.Background(), srv.Client(), nil, srv.URL); got != (Info{}) {
		t.Fatalf("%+v", got)
	}
	if got := Fetch(context.Background(), srv.Client(), nil, "ftp://x"); got != (Info{}) {
		t.Fatalf("%+v", got)
	}
}
