package badge

import (
	"strings"
	"testing"
	"time"
)

func ip(n int) *int { return &n }

func TestNormalizeDomain(t *testing.T) {
	good := map[string]string{
		"Relay.Example.com":                   "relay.example.com",
		"https://relay.example.com/v1/x?y=1":  "relay.example.com",
		"relay.example.com:8443":              "relay.example.com",
		"  http://user:pw@a-b.example.org/  ": "a-b.example.org",
		"relay.example.com.":                  "relay.example.com",
	}
	for in, want := range good {
		got, err := NormalizeDomain(in)
		if err != nil || got != want {
			t.Fatalf("%q: %q %v", in, got, err)
		}
	}
	for _, in := range []string{"", "localhost", "127.0.0.1", "[::1]", "a_b.example.com", "-x.example.com", "exa mple.com", "http://", strings.Repeat("a", 64) + ".com"} {
		if got, err := NormalizeDomain(in); err == nil {
			t.Fatalf("%q should be rejected, got %q", in, got)
		}
	}
}

func TestHostAndOriginMatching(t *testing.T) {
	if !HostMatches("https://api.relay.example.com:8443/v1", "relay.example.com") || !HostMatches("https://relay.example.com/v1", "relay.example.com") {
		t.Fatal("subdomain and exact host must match")
	}
	for _, e := range []string{"https://evilrelay.example.com/v1", "https://relay.example.com.evil.net/v1", "not a url", ""} {
		if HostMatches(e, "relay.example.com") {
			t.Fatalf("%q must not match", e)
		}
	}
	if !OriginAllowed("https://www.relay.example.com", "relay.example.com") || OriginAllowed("https://evil.net", "relay.example.com") ||
		OriginAllowed("null", "relay.example.com") || OriginAllowed("ftp://relay.example.com", "relay.example.com") {
		t.Fatal("origin matching")
	}
}

func TestPick(t *testing.T) {
	now := time.UnixMilli(2_000_000_000_000)
	day := int64(24 * time.Hour / time.Millisecond)
	runs := []Run{
		{ID: "old", Endpoint: "https://relay.example.com/v1", Score: ip(60), StartedAt: now.UnixMilli() - 40*day},
		{ID: "other", Endpoint: "https://other.net/v1", Score: ip(100), StartedAt: now.UnixMilli() - day},
		{ID: "unscored", Endpoint: "https://relay.example.com/v1", StartedAt: now.UnixMilli() - day},
		{ID: "new", Endpoint: "https://api.relay.example.com/v1", Transport: "openai_api", Score: ip(92), StartedAt: now.UnixMilli() - 2*day},
	}
	got := Pick(runs, "relay.example.com", now)
	if got.ReportID != "new" || got.Status != "ok" || got.Tier != "mostly" || got.Protocol != "openai" {
		t.Fatalf("%+v", got)
	}
	if st := Pick(runs[:1], "relay.example.com", now); st.Status != "stale" || st.Tier != "review" {
		t.Fatalf("stale: %+v", st)
	}
	if none := Pick(runs, "nobody.com", now); none.Status != "none" || none.ReportID != "" {
		t.Fatalf("none: %+v", none)
	}
	if tierOf(100) != "conformant" || tierOf(80) != "mostly" || tierOf(79) != "review" {
		t.Fatal("tier thresholds must match the web UI")
	}
}

func TestSVG(t *testing.T) {
	out := string(SVG(Result{Domain: "relay.example.com", Status: "ok", Tier: "conformant", Score: ip(100)}, "auto"))
	for _, want := range []string{"<svg", "Conformant 100", "ModelsExam", "relay.example.com"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q: %s", want, out)
		}
	}
	if s := string(SVG(Result{Domain: "a.com", Status: "stale"}, "dark")); !strings.Contains(s, "Check expired") {
		t.Fatal(s)
	}
	if s := string(MismatchSVG(`x"><script>`, "light")); strings.Contains(s, "<script>") {
		t.Fatalf("domain must be escaped: %s", s)
	}
}

func TestScriptEmbedsDomainSafely(t *testing.T) {
	js := string(Script("relay.example.com", "https://modelsexam.com"))
	for _, want := range []string{`DOMAIN = "relay.example.com"`, `API = "https://modelsexam.com"`, "attachShadow", "/api/badge/"} {
		if !strings.Contains(js, want) {
			t.Fatalf("missing %q", want)
		}
	}
	evil := string(Script(`a"; alert(1); //`, "https://x"))
	if strings.Contains(evil, `DOMAIN = "a"; alert`) {
		t.Fatalf("domain broke out of the string: %s", evil[:120])
	}
}

func TestSVGThemes(t *testing.T) {
	r := Result{Domain: "a.com", Status: "ok", Tier: "mostly", Score: ip(92)}
	dark := string(SVG(r, "dark"))
	light := string(SVG(r, "light"))
	auto := string(SVG(r, "bogus"))
	if !strings.Contains(dark, "#0a0e16") || strings.Contains(dark, "prefers-color-scheme") {
		t.Fatal("dark must use only the dark palette")
	}
	if !strings.Contains(light, "#ffffff") || strings.Contains(light, "prefers-color-scheme") {
		t.Fatal("light must use only the light palette")
	}
	if !strings.Contains(auto, "prefers-color-scheme:dark") || !strings.Contains(auto, "#ffffff") || !strings.Contains(auto, "#0a0e16") {
		t.Fatal("auto must carry both palettes")
	}
	if ThemeOf("dark") != "dark" || ThemeOf("x") != "auto" {
		t.Fatal("ThemeOf")
	}
}
