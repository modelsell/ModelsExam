// Package seo makes the single-page app readable to search engines and link
// previews. The server renders the per-page <head> tags and a plain-HTML
// fallback for each public route into index.html; React replaces the fallback
// when it starts. It also serves robots.txt and sitemap.xml.
//
// The route list mirrors web/src/lib/route.ts; keep the two in sync.
package seo

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type Link struct{ Path, Label string }

// Section is a heading with plain-text paragraphs and an optional list. The
// fallback renders these so a crawler (or "view source") reads the same
// substance the app shows.
type Section struct {
	Heading string
	Body    []string
	Items   []string
}

// Record is one public check record, listed as a link to its report page.
type Record struct {
	ID, Site, Host, Model, Verdict, Description, Date string
}

// LinkGroup is a headed list of internal links, rendered in the fallback so
// crawlers can reach model and site pages from the home page.
type LinkGroup struct {
	Heading string
	Links   []Link
}

// BoardRow is one line of a result board.
type BoardRow struct {
	Rank                                        int
	Site, Host, Verdict, Source, Date, ReportID string
	Score                                       int
	Fresh                                       bool
}

// BoardBlock is one model's board; Path links to the model page when it has one.
type BoardBlock struct {
	Model, Path string
	Rows        []BoardRow
}

type Page struct {
	Path        string
	Title       string
	Description string
	H1          string
	Lead        string
	NoIndex     bool
	Home        bool   // carries the WebSite / SoftwareApplication structured data
	Detail      string // extra plain-text line in the fallback (the tested URL)
	Sections    []Section
	Records     []Record // recent public records, filled by the server on / and /records
	MaxRecords  int      // how many of Records this page lists
	RecordsHead string   // heading above Records; "Recent check records" when empty
	Canonical   string   // path (and query) to use instead of Path, e.g. /records?page=2
	Prev, Next  string   // paging links, as paths
	JSONLD      []string // extra <script type="application/ld+json"> blocks
	LinkGroups  []LinkGroup
	Stats       string // one plain sentence of live totals, shown under the lead
	Boards      []BoardBlock
	FAQ         []QA
}

const siteName = "ModelsExam"

var reportPage = Page{
	Title:       "Check report | " + siteName,
	Description: "A single ModelsExam check report.",
	H1:          "Check report",
	Lead:        "A single ModelsExam check report.",
	NoIndex:     true,
}

var siteBadgePage = Page{
	Title:       "Site badge | " + siteName,
	Description: "The latest ModelsExam check result for a website.",
	H1:          "Site badge",
	Lead:        "The latest ModelsExam check result for a website.",
	NoIndex:     true,
}

var notFoundPage = Page{
	Title:       "Page not found | " + siteName,
	Description: "This page does not exist.",
	H1:          "Page not found",
	Lead:        "This page does not exist.",
	NoIndex:     true,
}

var reportPath = regexp.MustCompile(`^/reports/[^/]+$`)
var sitePath = regexp.MustCompile(`^/sites/[A-Za-z0-9.-]+$`)
var modelPathRe = regexp.MustCompile(`^/models/[A-Za-z0-9][A-Za-z0-9._:-]{0,199}$`)

// Lookup returns the page for a URL path. ok is false for unknown paths; the
// returned page is then the not-found page and the caller should answer 404.
func Lookup(urlPath string) (page Page, ok bool) {
	p := strings.TrimRight(urlPath, "/")
	if p == "" {
		p = "/"
	}
	for _, candidate := range pages {
		if candidate.Path == p {
			return candidate, true
		}
	}
	if reportPath.MatchString(p) {
		page = reportPage
		page.Path = p
		return page, true
	}
	if modelPathRe.MatchString(p) {
		page = modelPlaceholder
		page.Path = p
		return page, true
	}
	if sitePath.MatchString(p) {
		page = siteBadgePage
		page.Path = p
		return page, true
	}
	return notFoundPage, false
}

var hostPattern = regexp.MustCompile(`^[A-Za-z0-9.\-]+(:[0-9]{1,5})?$`)

// SiteURL is the public origin used for canonical links and the sitemap. The
// configured value wins; without one it is taken from the request, which is
// only safe when no cache sits in front, so production should set it.
func SiteURL(configured string, r *http.Request) string {
	if u, err := url.Parse(strings.TrimSpace(configured)); err == nil && configured != "" &&
		(u.Scheme == "https" || u.Scheme == "http") && hostPattern.MatchString(u.Host) {
		return u.Scheme + "://" + u.Host
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if !hostPattern.MatchString(host) {
		host = "localhost"
	}
	return scheme + "://" + host
}

var (
	titleTag  = regexp.MustCompile(`(?is)<title[^>]*>.*?</title>`)
	descTag   = regexp.MustCompile(`(?is)<meta\s+name=["']?description["']?[^>]*>`)
	robotsTag = regexp.MustCompile(`(?is)<meta\s+name=["']?robots["']?[^>]*>`)
	canonTag  = regexp.MustCompile(`(?is)<link\s+rel=["']?canonical["']?[^>]*>`)
	headClose = regexp.MustCompile(`(?i)</head>`)
	htmlLang  = regexp.MustCompile(`(?is)(<html\b[^>]*?)\slang=["'][^"']*["']`)
	rootDiv   = regexp.MustCompile(`(?i)<div id=["']?root["']?>\s*</div>`)
)

func esc(s string) string { return html.EscapeString(s) }

func structuredData(site string) string {
	graph := map[string]any{
		"@context": "https://schema.org",
		"@graph": []any{
			map[string]any{"@type": "WebSite", "name": siteName, "url": site + "/", "inLanguage": "zh-CN"},
			map[string]any{
				"@type": "Organization", "name": siteName, "url": site + "/",
				"logo": site + "/logo.png",
			},
			map[string]any{
				"@type":               "SoftwareApplication",
				"name":                siteName,
				"applicationCategory": "DeveloperApplication",
				"operatingSystem":     "Web",
				"license":             "https://www.gnu.org/licenses/agpl-3.0.html",
				"url":                 site + "/",
			},
		},
	}
	b, _ := json.Marshal(graph) // escapes <, > and & so it cannot close the script tag
	return `<script type="application/ld+json">` + string(b) + `</script>`
}

// OGImagePath is the 1200x630 share image served from web/public.
const OGImagePath = "/og-image.png"

func head(site string, p Page) string {
	var b strings.Builder
	url := site + p.Path
	if p.Canonical != "" {
		url = site + p.Canonical
	}
	fmt.Fprintf(&b, "<title>%s</title>\n", esc(p.Title))
	fmt.Fprintf(&b, "<meta name=\"description\" content=\"%s\">\n", esc(p.Description))
	if p.NoIndex {
		b.WriteString("<meta name=\"robots\" content=\"noindex, nofollow\">\n")
	} else {
		b.WriteString("<meta name=\"robots\" content=\"index, follow\">\n")
		fmt.Fprintf(&b, "<link rel=\"canonical\" href=\"%s\">\n", esc(url))
		fmt.Fprintf(&b, "<meta property=\"og:url\" content=\"%s\">\n", esc(url))
		fmt.Fprintf(&b, "<link rel=\"alternate\" hreflang=\"zh-CN\" href=\"%s\">\n", esc(url))
		fmt.Fprintf(&b, "<link rel=\"alternate\" hreflang=\"x-default\" href=\"%s\">\n", esc(url))
	}
	b.WriteString("<meta name=\"theme-color\" content=\"#0a0e16\">\n")
	b.WriteString("<meta property=\"og:locale\" content=\"zh_CN\">\n")
	fmt.Fprintf(&b, "<meta property=\"og:image\" content=\"%s\">\n", esc(site+OGImagePath))
	b.WriteString("<meta property=\"og:image:width\" content=\"1200\">\n<meta property=\"og:image:height\" content=\"630\">\n")
	fmt.Fprintf(&b, "<meta property=\"og:image:alt\" content=\"%s 大模型 API 真伪与协议一致性检测\">\n", siteName)
	fmt.Fprintf(&b, "<meta name=\"twitter:image\" content=\"%s\">\n", esc(site+OGImagePath))
	fmt.Fprintf(&b, "<meta property=\"og:site_name\" content=\"%s\">\n", siteName)
	b.WriteString("<meta property=\"og:type\" content=\"website\">\n")
	fmt.Fprintf(&b, "<meta property=\"og:title\" content=\"%s\">\n", esc(p.Title))
	fmt.Fprintf(&b, "<meta property=\"og:description\" content=\"%s\">\n", esc(p.Description))
	b.WriteString("<meta name=\"twitter:card\" content=\"summary_large_image\">\n")
	if !p.NoIndex {
		if p.Prev != "" {
			fmt.Fprintf(&b, "<link rel=\"prev\" href=\"%s\">\n", esc(site+p.Prev))
		}
		if p.Next != "" {
			fmt.Fprintf(&b, "<link rel=\"next\" href=\"%s\">\n", esc(site+p.Next))
		}
	}
	for _, ld := range p.JSONLD {
		b.WriteString(ld + "\n")
	}
	if len(p.FAQ) > 0 {
		b.WriteString(faqLD(p.FAQ) + "\n")
	}
	if p.Home {
		b.WriteString(structuredData(site) + "\n")
	}
	return b.String()
}

func fallback(p Page) string {
	var b strings.Builder
	b.WriteString(`<div class="seo-fallback">`)
	fmt.Fprintf(&b, "<h1>%s</h1><p>%s</p>", esc(p.H1), esc(p.Lead))
	if p.Stats != "" {
		fmt.Fprintf(&b, "<p>%s</p>", esc(p.Stats))
	}
	if p.Detail != "" {
		fmt.Fprintf(&b, "<p>%s</p>", esc(p.Detail))
	}
	for _, sec := range p.Sections {
		fmt.Fprintf(&b, "<section><h2>%s</h2>", esc(sec.Heading))
		for _, para := range sec.Body {
			fmt.Fprintf(&b, "<p>%s</p>", esc(para))
		}
		if len(sec.Items) > 0 {
			b.WriteString("<ul>")
			for _, it := range sec.Items {
				fmt.Fprintf(&b, "<li>%s</li>", esc(it))
			}
			b.WriteString("</ul>")
		}
		b.WriteString("</section>")
	}
	for _, bb := range p.Boards {
		if len(bb.Rows) == 0 {
			continue
		}
		if bb.Path != "" {
			fmt.Fprintf(&b, `<section><h2><a href="%s">%s 检测榜单</a></h2><ol>`, esc(bb.Path), esc(bb.Model))
		} else {
			fmt.Fprintf(&b, "<section><h2>%s 检测榜单</h2><ol>", esc(bb.Model))
		}
		for _, row := range bb.Rows {
			fmt.Fprintf(&b, `<li><a href="/reports/%s">%s</a>`, esc(url.PathEscape(row.ReportID)), esc(row.Site))
			if row.Host != "" && row.Host != row.Site {
				fmt.Fprintf(&b, "（%s）", esc(row.Host))
			}
			fmt.Fprintf(&b, "：%d 分，%s，%s，检测于 %s", row.Score, esc(row.Verdict), esc(row.Source), esc(row.Date))
			if !row.Fresh {
				b.WriteString("（已过期）")
			}
			b.WriteString("</li>")
		}
		b.WriteString("</ol></section>")
	}
	if len(p.FAQ) > 0 {
		b.WriteString("<section><h2>常见问题</h2><dl>")
		for _, qa := range p.FAQ {
			fmt.Fprintf(&b, "<dt>%s</dt><dd>%s</dd>", esc(qa.Q), esc(qa.A))
		}
		b.WriteString("</dl></section>")
	}
	if p.MaxRecords > 0 && len(p.Records) > 0 {
		head := p.RecordsHead
		if head == "" {
			head = "最近的检测记录"
		}
		fmt.Fprintf(&b, "<section><h2>%s</h2><ul>", esc(head))
		for i, rec := range p.Records {
			if i >= p.MaxRecords {
				break
			}
			fmt.Fprintf(&b, `<li><a href="/reports/%s">%s</a>`, esc(url.PathEscape(rec.ID)), esc(rec.Site))
			if rec.Host != "" && rec.Host != rec.Site {
				fmt.Fprintf(&b, " (%s)", esc(rec.Host))
			}
			if rec.Model != "" {
				fmt.Fprintf(&b, "，模型 %s", esc(rec.Model))
			}
			if rec.Verdict != "" {
				fmt.Fprintf(&b, ": %s", esc(rec.Verdict))
			}
			if rec.Date != "" {
				fmt.Fprintf(&b, "（检测于 %s）", esc(rec.Date))
			}
			if rec.Description != "" {
				fmt.Fprintf(&b, ". %s", esc(cut(rec.Description, 160)))
			}
			b.WriteString("</li>")
		}
		b.WriteString("</ul></section>")
	}
	for _, g := range p.LinkGroups {
		if len(g.Links) == 0 {
			continue
		}
		fmt.Fprintf(&b, "<section><h2>%s</h2><ul>", esc(g.Heading))
		for _, l := range g.Links {
			fmt.Fprintf(&b, `<li><a href="%s">%s</a></li>`, esc(l.Path), esc(l.Label))
		}
		b.WriteString("</ul></section>")
	}
	if p.Prev != "" || p.Next != "" {
		b.WriteString("<p>")
		if p.Prev != "" {
			fmt.Fprintf(&b, `<a href="%s" rel="prev">较新的记录</a> `, esc(p.Prev))
		}
		if p.Next != "" {
			fmt.Fprintf(&b, `<a href="%s" rel="next">较早的记录</a>`, esc(p.Next))
		}
		b.WriteString("</p>")
	}
	b.WriteString("<nav aria-label=\"Main\"><ul>")
	for _, l := range nav {
		fmt.Fprintf(&b, "<li><a href=\"%s\">%s</a></li>", esc(l.Path), esc(l.Label))
	}
	b.WriteString("</ul></nav></div>")
	return b.String()
}

// RecordOf turns a stored run into a listing entry.
func RecordOf(r ReportInfo) Record {
	name := r.SiteName
	if name == "" {
		name = hostOf(r.Endpoint)
	}
	v := ""
	if r.Score != nil {
		v = fmt.Sprintf("%s（%d/100）", zhVerdict(r.Score), *r.Score)
	}
	return Record{ID: r.ID, Site: name, Host: hostOf(r.Endpoint), Model: r.Model, Verdict: v, Description: r.Description, Date: dateOf(r.StartedAt)}
}

// Render fills the page-specific head tags and fallback content into the built
// index.html. It removes any title, description, robots or canonical tag the
// template already has so none appears twice.
func Render(index []byte, site string, p Page) []byte {
	out := htmlLang.ReplaceAllString(string(index), `${1} lang="zh-CN"`)
	for _, re := range []*regexp.Regexp{titleTag, descTag, robotsTag, canonTag} {
		out = re.ReplaceAllString(out, "")
	}
	out = headClose.ReplaceAllLiteralString(out, head(site, p)+"</head>")
	out = rootDiv.ReplaceAllLiteralString(out, `<div id="root">`+fallback(p)+`</div>`)
	return []byte(out)
}

// ReportInfo is what a stored check run tells a search engine.
type ReportInfo struct {
	ID          string
	SiteName    string // read from the tested site's home page
	Description string // likewise
	Model       string
	Endpoint    string
	Status      string
	Score       *int
	Transport   string
	StartedAt   int64 // unix milliseconds
	Requests    int
	Passed      int
	Failed      int
	DurationMS  int64
}

// HostOf returns the host of an endpoint URL, or "".
func HostOf(endpoint string) string { return hostOf(endpoint) }

func hostOf(endpoint string) string {
	if u, err := url.Parse(endpoint); err == nil {
		return u.Host
	}
	return ""
}

func cut(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max-1])) + "…"
}

func verdict(score *int) string {
	switch {
	case score == nil:
		return ""
	case *score >= 100:
		return "conformant"
	case *score >= 80:
		return "mostly conformant"
	default:
		return "review suggested"
	}
}

// Entry is one extra sitemap URL: a report, a site page or a model page.
type Entry struct {
	Path    string
	LastMod int64 // unix milliseconds; 0 when unknown
}

// Sitemap lists the public, indexable pages plus the given entries.
func Sitemap(site string, extra []Entry) []byte {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")
	for _, p := range pages {
		if p.NoIndex {
			continue
		}
		fmt.Fprintf(&b, "  <url><loc>%s</loc></url>\n", esc(site+p.Path))
	}
	for _, e := range extra {
		if e.LastMod > 0 {
			fmt.Fprintf(&b, "  <url><loc>%s</loc><lastmod>%s</lastmod></url>\n", esc(site+e.Path), dateOf(e.LastMod))
		} else {
			fmt.Fprintf(&b, "  <url><loc>%s</loc></url>\n", esc(site+e.Path))
		}
	}
	b.WriteString("</urlset>\n")
	return []byte(b.String())
}

// Robots allows crawling everywhere except the API. Report pages stay
// crawlable on purpose: they carry a noindex tag, which a crawler can only
// see if it is allowed to fetch the page.
func Robots(site string) []byte {
	return []byte("User-agent: *\nAllow: /\nDisallow: /api/\nDisallow: /badge/\nDisallow: /embed/\n\nSitemap: " + site + "/sitemap.xml\n")
}
