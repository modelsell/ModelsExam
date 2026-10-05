package seo

import (
	"net/http/httptest"
	"strings"
	"testing"
)

const tpl = `<!doctype html><html lang="en"><head><meta charset="UTF-8"><meta name="robots" content="noindex"><title>Old</title><meta name="description" content="old"></head><body><div id="root"></div></body></html>`

func TestLookup(t *testing.T) {
	for _, path := range []string{"/", "/get-badge", "/baselines", "/method", "/integrate"} {
		p, ok := Lookup(path)
		if !ok || p.NoIndex {
			t.Fatalf("%s: ok=%v noindex=%v", path, ok, p.NoIndex)
		}
	}
	// Check records are private to each browser: the page exists but is never indexed.
	if p, ok := Lookup("/records"); !ok || !p.NoIndex {
		t.Fatalf("/records: ok=%v noindex=%v", ok, p.NoIndex)
	}
	p, ok := Lookup("/reports/abc-123")
	if !ok || !p.NoIndex || p.Path != "/reports/abc-123" {
		t.Fatalf("report page: %+v ok=%v", p, ok)
	}
	for _, path := range []string{"/nope", "/reports/", "/reports/a/b", "/api/x"} {
		p, ok := Lookup(path)
		if ok || !p.NoIndex {
			t.Fatalf("%s should be not found and noindex", path)
		}
	}
}

func TestRenderReplacesTemplateTags(t *testing.T) {
	p, _ := Lookup("/baselines")
	out := string(Render([]byte(tpl), "https://modelsexam.com", p))
	if strings.Count(out, "<title>") != 1 || strings.Contains(out, "Old") {
		t.Fatalf("title not replaced once: %s", out)
	}
	if strings.Count(out, `name="description"`) != 1 || strings.Count(out, `name="robots"`) != 1 {
		t.Fatalf("duplicate description/robots: %s", out)
	}
	for _, want := range []string{
		`<link rel="canonical" href="https://modelsexam.com/baselines">`,
		`content="index, follow"`,
		`<h1>官方基线</h1>`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %s", want, out)
		}
	}
	if strings.Contains(out, "ld+json") {
		t.Fatal("structured data belongs on the home page only")
	}
}

func TestRenderNoIndexPageHasNoCanonical(t *testing.T) {
	p, _ := Lookup("/reports/abc")
	out := string(Render([]byte(tpl), "https://modelsexam.com", p))
	if !strings.Contains(out, `content="noindex, nofollow"`) || strings.Contains(out, "canonical") {
		t.Fatalf("report page must be noindex without canonical: %s", out)
	}
}

func TestRenderHomeStructuredData(t *testing.T) {
	p, _ := Lookup("/")
	out := string(Render([]byte(tpl), "https://modelsexam.com", p))
	if !strings.Contains(out, `application/ld+json`) || !strings.Contains(out, `"SoftwareApplication"`) {
		t.Fatalf("missing structured data: %s", out)
	}
	if strings.Count(out, "</script>") != strings.Count(out, "<script") {
		t.Fatal("structured data closed the script tag more than once")
	}
}

func TestRenderHandlesMinifiedRoot(t *testing.T) {
	min := `<html><head><title>x</title></head><body><div id=root></div></body></html>`
	p, _ := Lookup("/")
	if out := string(Render([]byte(min), "https://a.example", p)); !strings.Contains(out, `<div id="root"><div class="seo-fallback">`) {
		t.Fatalf("root not filled: %s", out)
	}
}

func TestSiteURL(t *testing.T) {
	r := httptest.NewRequest("GET", "http://internal/", nil)
	r.Host = "modelsexam.com"
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := SiteURL("", r); got != "https://modelsexam.com" {
		t.Fatalf("derived: %s", got)
	}
	if got := SiteURL("https://example.org/", r); got != "https://example.org" {
		t.Fatalf("configured: %s", got)
	}
	r.Host = `evil"><script>`
	if got := SiteURL("", r); got != "https://localhost" {
		t.Fatalf("bad host must not be echoed: %s", got)
	}
	if got := SiteURL("javascript:alert(1)", httptest.NewRequest("GET", "http://x.test/", nil)); strings.Contains(got, "javascript") {
		t.Fatalf("bad configured value: %s", got)
	}
}

func TestSitemapAndRobots(t *testing.T) {
	sm := string(Sitemap("https://modelsexam.com", nil))
	if strings.Count(sm, "<loc>") != indexablePages() || strings.Contains(sm, "/reports") || strings.Contains(sm, "/records") {
		t.Fatalf("sitemap: %s", sm)
	}
	rb := string(Robots("https://modelsexam.com"))
	if !strings.Contains(rb, "Disallow: /api/") || strings.Contains(rb, "/reports") ||
		!strings.Contains(rb, "Sitemap: https://modelsexam.com/sitemap.xml") {
		t.Fatalf("robots: %s", rb)
	}
}

func TestReportPage(t *testing.T) {
	score := 100
	p := ReportPage("https://modelsexam.com", ReportInfo{ID: "abc", SiteName: "Relay Co", Description: "Fast relay.", Model: "claude-x", Endpoint: "https://relay.example/v1", Status: "completed", Score: &score,
		Transport: "anthropic_proxy", StartedAt: 1791100000000, Requests: 23, Passed: 20, Failed: 3, DurationMS: 42000})
	if p.NoIndex || p.Title != "Relay Co claude-x 模型评测 | ModelsExam" || !strings.Contains(p.Description, "Fast relay.") ||
		!strings.Contains(p.Description, "符合（100/100）") || !strings.Contains(p.Description, "符合（100/100）") || p.Path != "/reports/abc" {
		t.Fatalf("%+v", p)
	}
	out := string(Render([]byte(tpl), "https://modelsexam.com", p))
	if !strings.Contains(out, `rel="canonical" href="https://modelsexam.com/reports/abc"`) || !strings.Contains(out, "Tested endpoint: https://relay.example/v1") ||
		!strings.Contains(out, "Requests sent: 23; checks passed: 20; checks failed: 3") || !strings.Contains(out, "Source: Other") ||
		!strings.Contains(out, `href="/sites/relay.example"`) || !strings.Contains(out, `href="/models/claude-x"`) || !strings.Contains(out, "BreadcrumbList") {
		t.Fatalf("%s", out)
	}
}

func TestReportPageIndexRules(t *testing.T) {
	score := 50
	if !ReportPage("https://x.test", ReportInfo{ID: "a", SiteName: "X", Model: "m", Status: "running"}).NoIndex {
		t.Fatal("running run must be noindex")
	}
	if !ReportPage("https://x.test", ReportInfo{ID: "a", Model: "m", Endpoint: "https://h.example/v1", Status: "completed", Score: &score}).NoIndex {
		t.Fatal("run without a site name must be noindex")
	}
	p := ReportPage("https://x.test", ReportInfo{ID: "a", Model: "m", Endpoint: "https://h.example/v1", Status: "completed"})
	if !strings.HasPrefix(p.H1, "h.example m 模型评测") {
		t.Fatalf("host fallback: %q", p.H1)
	}
}

func TestSitemapIncludesReports(t *testing.T) {
	sm := string(Sitemap("https://modelsexam.com", []Entry{{Path: "/reports/r1", LastMod: 1791100000000}, {Path: "/reports/r%202"}}))
	if !strings.Contains(sm, "<loc>https://modelsexam.com/reports/r1</loc>") || !strings.Contains(sm, "/reports/r%202") || !strings.Contains(sm, "<lastmod>2026-") || strings.Count(sm, "<loc>") != indexablePages()+2 {
		t.Fatal(sm)
	}
}

func TestSiteAndModelPages(t *testing.T) {
	recs := []Record{{ID: "a", Site: "Relay Co", Host: "relay.example", Model: "claude-x", Verdict: "符合（100/100）", Description: "Fast relay", Date: "2026-10-04"}}
	sp := SitePage("https://modelsexam.com", "relay.example", recs)
	if sp.NoIndex || !strings.Contains(sp.Title, "Relay Co (relay.example) 模型评测") || !strings.Contains(string(Render([]byte(tpl), "https://modelsexam.com", sp)), `href="/reports/a"`) {
		t.Fatalf("%+v", sp)
	}
	if !SitePage("https://modelsexam.com", "none.example", nil).NoIndex {
		t.Fatal("a site without records must be noindex")
	}
	mp := ModelPage("https://modelsexam.com", "claude-x", recs, nil)
	if mp.NoIndex || mp.Path != "/models/claude-x" || !strings.Contains(mp.Title, "claude-x 模型评测") {
		t.Fatalf("%+v", mp)
	}
	if _, ok := Lookup("/models/claude-sonnet-4-5"); !ok {
		t.Fatal("model path must be known")
	}
	if _, ok := Lookup("/models/a/b"); ok {
		t.Fatal("nested model path must be not found")
	}
	if _, ok := ModelPath("bad name/x"); ok {
		t.Fatal("model names that are not path-safe get no page")
	}
}

func TestSiteBadgePageIsNoIndex(t *testing.T) {
	p, ok := Lookup("/sites/relay.example.com")
	if !ok || !p.NoIndex || p.Path != "/sites/relay.example.com" {
		t.Fatalf("%+v %v", p, ok)
	}
	if _, ok := Lookup("/sites/bad/path"); ok {
		t.Fatal("nested site path must be not found")
	}
}

func TestFallbackCarriesRealContent(t *testing.T) {
	p, _ := Lookup("/")
	score := 100
	p.Records = []Record{
		RecordOf(ReportInfo{ID: "abc", SiteName: `Relay <b>One</b>`, Description: "Fast relay", Model: "claude-sonnet-4-5", Endpoint: "https://relay.example.com/v1", Status: "completed", Score: &score}),
	}
	out := string(Render([]byte(`<html><head><title>x</title></head><body><div id="root"></div></body></html>`), "https://modelsexam.com", p))
	for _, want := range []string{"ModelsExam 徽章代表什么", "官方基线", "基本符合（80 到 99 分）", "最近的检测记录", `href="/reports/abc"`, "符合（100/100）", "relay.example.com"} {
		if !strings.Contains(out, want) {
			t.Fatalf("home fallback is missing %q", want)
		}
	}
	if strings.Contains(out, "<b>One</b>") {
		t.Fatal("record text must be escaped")
	}
	for _, path := range []string{"/get-badge", "/baselines", "/method", "/integrate"} {
		pg, _ := Lookup(path)
		if len(pg.Sections) == 0 {
			t.Fatalf("%s has no body content", path)
		}
	}
}

func TestSitePath(t *testing.T) {
	if p, ok := SitePath("https://Relay.Example.com:8443/v1"); !ok || p != "/sites/relay.example.com" {
		t.Fatalf("%q %v", p, ok)
	}
	for _, bad := range []string{"", "not a url", "https://[::1]/v1"} {
		if _, ok := SitePath(bad); ok {
			t.Fatalf("%q must have no site page", bad)
		}
	}
}

func TestBoardsAndFAQ(t *testing.T) {
	home, _ := Lookup("/")
	home.FAQ = FAQ
	home.Stats = "已检测 3 个站点"
	home.Boards = []BoardBlock{{Model: "claude-x", Path: "/models/claude-x", Rows: []BoardRow{{Rank: 1, Site: "Relay <i>", Host: "r.example", Score: 100, Verdict: "符合", Source: "其他", Date: "2026-10-01", ReportID: "r1", Fresh: false}}}}
	out := string(Render([]byte(`<html><head><title>x</title></head><body><div id="root"></div></body></html>`), "https://modelsexam.com", home))
	for _, want := range []string{`"@type":"FAQPage"`, "常见问题", "已检测 3 个站点", `href="/models/claude-x"`, "已过期", `href="/reports/r1"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(out, "<i>") {
		t.Fatal("board text must be escaped")
	}
	mp := ModelPage("https://modelsexam.com", "claude-x", []Record{{Site: "A"}}, []BoardRow{{Rank: 1, Site: "A", ReportID: "r"}})
	if len(mp.Boards) != 1 || mp.NoIndex {
		t.Fatal("model page board")
	}
}

func indexablePages() int {
	n := 0
	for _, p := range pages {
		if !p.NoIndex {
			n++
		}
	}
	return n
}
