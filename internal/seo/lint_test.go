package seo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// The template the frontend build produces: noindex until the server (or the
// static generator) fills in the real head.
const lintTemplate = `<!doctype html><html lang="en" class="dark"><head><meta charset="UTF-8" /><meta name="viewport" content="width=device-width, initial-scale=1.0" /><meta name="robots" content="noindex" /><title>ModelsExam</title></head><body><div id="root"></div></body></html>`

const lintSite = "https://modelsexam.com"

var (
	reH1      = regexp.MustCompile(`(?i)<h1[ >]`)
	reHref    = regexp.MustCompile(`href="([^"]+)"`)
	reLD      = regexp.MustCompile(`(?s)<script type="application/ld\+json">(.*?)</script>`)
	reTitle   = regexp.MustCompile(`(?s)<title>(.*?)</title>`)
	reDesc    = regexp.MustCompile(`<meta name="description" content="([^"]*)">`)
	reCanon   = regexp.MustCompile(`<link rel="canonical" href="([^"]*)">`)
	reRobots  = regexp.MustCompile(`<meta name="robots" content="([^"]*)">`)
	reHeading = regexp.MustCompile(`(?i)<h([1-6])[ >]`)
)

func lintPages(t *testing.T) []Page {
	t.Helper()
	score := 100
	named := ReportInfo{ID: "11111111-1111-4111-8111-111111111111", SiteName: "Relay One", Description: "A relay", Model: "claude-sonnet-4-5", Endpoint: "https://relay.example.com/v1", Status: "completed", Score: &score, StartedAt: 1791100000000, Requests: 20, Passed: 20}
	recs := []Record{RecordOf(named)}
	out := append([]Page{}, pages...)
	out = append(out, ReportPage(lintSite, named), SitePage(lintSite, "relay.example.com", recs),
		ModelPage(lintSite, "claude-sonnet-4-5", recs, []BoardRow{{Rank: 1, Site: "Relay One", Host: "relay.example.com", Score: 100, Verdict: "符合", Source: "其他", Date: "2026-10-01", ReportID: named.ID, Fresh: true}}),
		RecordsPage(lintSite, 1, 2, recs), RecordsPage(lintSite, 2, 2, recs))
	return out
}

func TestSEOLint(t *testing.T) {
	titles, descs := map[string]string{}, map[string]string{}
	for _, p := range lintPages(t) {
		out := string(Render([]byte(lintTemplate), lintSite, p))
		fail := func(format string, a ...any) { t.Helper(); t.Errorf("%s: "+format, append([]any{p.Path}, a...)...) }

		if !strings.Contains(out, `<html lang="zh-CN"`) {
			fail("html lang must be zh-CN for Chinese content")
		}
		if n := len(reH1.FindAllString(out, -1)); n != 1 {
			fail("want exactly one h1, got %d", n)
		}
		prev := 1
		for _, m := range reHeading.FindAllStringSubmatch(out, -1) {
			lvl := int(m[1][0] - '0')
			if lvl > prev+1 {
				fail("heading level jumps from h%d to h%d", prev, lvl)
			}
			prev = lvl
		}
		title := reTitle.FindStringSubmatch(out)
		desc := reDesc.FindStringSubmatch(out)
		robots := reRobots.FindStringSubmatch(out)
		if title == nil || desc == nil || robots == nil {
			fail("missing title, description or robots")
			continue
		}
		if n := utf8.RuneCountInString(title[1]); n < 8 || n > 70 {
			fail("title length %d out of 8..70: %q", n, title[1])
		}
		if n := utf8.RuneCountInString(desc[1]); n < 40 || n > 300 {
			fail("description length %d out of 40..300", n)
		}
		noindex := strings.Contains(robots[1], "noindex")
		if noindex != p.NoIndex {
			fail("robots %q disagrees with NoIndex=%v", robots[1], p.NoIndex)
		}
		for _, want := range []string{`property="og:image" content="` + lintSite + `/og-image.png"`, `name="twitter:image"`, `property="og:locale"`, `name="twitter:card" content="summary_large_image"`, `name="theme-color"`, `property="og:title"`, `property="og:description"`} {
			if !strings.Contains(out, want) {
				fail("missing %s", want)
			}
		}
		canon := reCanon.FindStringSubmatch(out)
		if noindex {
			if canon != nil {
				fail("noindex page must not carry a canonical")
			}
		} else {
			if canon == nil || !strings.HasPrefix(canon[1], lintSite+"/") {
				fail("indexable page needs an absolute canonical, got %v", canon)
			}
			if !strings.Contains(out, `hreflang="zh-CN"`) || !strings.Contains(out, `hreflang="x-default"`) {
				fail("indexable page needs hreflang alternates")
			}
			key := canon[1]
			if prev, ok := titles[title[1]]; ok && prev != key {
				fail("duplicate title also on %s", prev)
			}
			if prev, ok := descs[desc[1]]; ok && prev != key {
				fail("duplicate description also on %s", prev)
			}
			titles[title[1]], descs[desc[1]] = key, key
		}
		for _, m := range reLD.FindAllStringSubmatch(out, -1) {
			var v any
			if err := json.Unmarshal([]byte(strings.ReplaceAll(m[1], `<\/`, "</")), &v); err != nil {
				fail("invalid JSON-LD: %v", err)
			}
		}
		body := out[strings.Index(out, `<div id="root">`):]
		for _, m := range reHref.FindAllStringSubmatch(body, -1) {
			href := strings.ReplaceAll(m[1], "&amp;", "&")
			if !strings.HasPrefix(href, "/") {
				fail("internal link must be root-relative: %s", href)
				continue
			}
			path := href
			if i := strings.IndexAny(path, "?#"); i >= 0 {
				path = path[:i]
			}
			if _, ok := Lookup(path); !ok {
				fail("link to unknown page %s", href)
			}
		}
	}
}

func TestSitemapMatchesIndexablePages(t *testing.T) {
	sm := string(Sitemap(lintSite, nil))
	for _, p := range pages {
		in := strings.Contains(sm, "<loc>"+lintSite+p.Path+"</loc>")
		if in == p.NoIndex {
			t.Errorf("%s: sitemap presence %v, NoIndex %v", p.Path, in, p.NoIndex)
		}
	}
	if !strings.Contains(string(Robots(lintSite)), "Sitemap: "+lintSite+"/sitemap.xml") {
		t.Fatal("robots.txt must point at the sitemap")
	}
}

func TestWriteStatic(t *testing.T) {
	dist, out := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte(lintTemplate), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dist, "static", "js"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "static", "js", "a.js"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	written, err := WriteStatic(dist, out, lintSite)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != len(StaticPaths)+2 {
		t.Fatalf("written %v", written)
	}
	for _, rel := range []string{"index.html", "baselines/index.html", "get-badge/index.html", "method/index.html", "404.html", "robots.txt", "static/js/a.js"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing %s", rel)
		}
	}
	home, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if strings.Contains(string(home), `content="noindex"`) || !strings.Contains(string(home), `rel="canonical" href="`+lintSite+`/"`) {
		t.Fatal("static home must be indexable with a canonical")
	}
	nf, _ := os.ReadFile(filepath.Join(out, "404.html"))
	if !strings.Contains(string(nf), "noindex") {
		t.Fatal("404 page must be noindex")
	}
}
