package seo

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var modelPlaceholder = Page{
	Title:       "Model results | " + siteName,
	Description: "ModelsExam check results for one model.",
	H1:          "Model results",
	Lead:        "ModelsExam check results for one model.",
	NoIndex:     true,
}

func dateOf(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format("2006-01-02")
}

// SitePath returns /sites/<host> for an endpoint's host name, or false when it
// cannot be a site page (empty, or characters outside a plain host name).
func SitePath(endpoint string) (string, bool) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", false
	}
	p := "/sites/" + strings.ToLower(u.Hostname())
	return p, sitePath.MatchString(p)
}

// ModelPath returns /models/<name>, or false when the name cannot be a URL
// path segment (such names simply get no model page).
func ModelPath(model string) (string, bool) {
	p := "/models/" + model
	return p, modelPathRe.MatchString(p)
}

func protocolOf(transport string) string {
	switch transport {
	case "openai_api":
		return "OpenAI-compatible (Chat Completions and Responses)"
	case "image_api":
		return "OpenAI Images"
	case "gemini_api":
		return "Gemini API (generateContent)"
	default:
		return "Anthropic Messages (Claude)"
	}
}

func sourceOf(endpoint, transport string) string {
	h := strings.ToLower(hostOf(endpoint))
	switch {
	case transport == "openai_api" && h == "api.openai.com", h == "api.anthropic.com",
		transport == "gemini_api" && h == "generativelanguage.googleapis.com":
		return "Official API"
	case strings.HasPrefix(h, "bedrock-mantle.") && strings.HasSuffix(h, ".api.aws"),
		strings.HasPrefix(h, "bedrock-runtime.") && strings.HasSuffix(h, ".amazonaws.com"),
		transport == "bedrock_runtime":
		return "AWS Bedrock"
	}
	return "Other (relay or third-party endpoint)"
}

func ld(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	// "</" cannot appear inside a script element.
	return `<script type="application/ld+json">` + strings.ReplaceAll(string(b), "</", `<\/`) + `</script>`
}

func breadcrumbs(site string, items ...Link) string {
	list := make([]map[string]any, 0, len(items))
	for i, it := range items {
		list = append(list, map[string]any{"@type": "ListItem", "position": i + 1, "name": it.Label, "item": site + it.Path})
	}
	return ld(map[string]any{"@context": "https://schema.org", "@type": "BreadcrumbList", "itemListElement": list})
}

// ReportPage builds the page for one stored report. It is indexable only when
// the run completed and the site's name could be read, so half-finished runs
// and unnamed endpoints never reach a search engine. The body states the facts
// of the run; the full evidence stays in the app.
func ReportPage(site string, r ReportInfo) Page {
	name := r.SiteName
	if name == "" {
		name = hostOf(r.Endpoint)
	}
	if name == "" {
		name = "Unnamed site"
	}
	host := hostOf(r.Endpoint)
	v := ""
	if r.Status == "completed" && r.Score != nil {
		v = fmt.Sprintf("%s (%d/100)", verdict(r.Score), *r.Score)
	}
	date := dateOf(r.StartedAt)
	zh := name + " " + r.Model + " 模型评测"
	if v != "" {
		zh += "：" + zhVerdict(r.Score) + fmt.Sprintf("（%d/100）", *r.Score)
	}
	if date != "" {
		zh += "，检测于 " + date
	}
	en := "ModelsExam check of " + r.Model
	if host != "" {
		en += " on " + host
	}
	if v != "" {
		en += ": " + v
	}
	en += "."
	desc := zh + "。" + en
	if r.Description != "" {
		desc = cut(r.Description, 110) + " | " + desc
	}
	items := []string{
		"Site: " + name,
		"Tested endpoint: " + r.Endpoint,
		"Source: " + sourceOf(r.Endpoint, r.Transport),
		"Protocol: " + protocolOf(r.Transport),
		"Model: " + r.Model,
	}
	if v != "" {
		items = append(items, "Result: "+v)
	} else {
		items = append(items, "Result: no verdict (the run did not complete)")
	}
	if date != "" {
		items = append(items, "Checked on: "+date+" (UTC)")
	}
	if r.Requests > 0 {
		items = append(items, fmt.Sprintf("Requests sent: %d; checks passed: %d; checks failed: %d", r.Requests, r.Passed, r.Failed))
	}
	if r.DurationMS > 0 {
		items = append(items, fmt.Sprintf("Duration: %.1f seconds", float64(r.DurationMS)/1000))
	}
	p := Page{
		Path:        "/reports/" + r.ID,
		Title:       cut(name+" "+r.Model+" 模型评测", 60) + " | " + siteName,
		Description: cut(desc, 300),
		H1:          name + " " + r.Model + " 模型评测 / check report",
		Lead:        cut(desc, 300),
		NoIndex:     r.Status != "completed" || r.SiteName == "",
		Sections: []Section{
			{Heading: "Test summary", Items: items},
			{Heading: "About this result", Body: []string{
				"This page describes one check of one endpoint and model at the time shown. The score counts scored assertions only; observations such as latency never change it. It is evidence about that run, not a ranking, a certification or an endorsement.",
				"本页只描述该地址和模型在检测时刻的一次表现，分数只统计计分断言，不是排名、认证或背书。",
			}},
		},
		JSONLD: []string{breadcrumbs(site, Link{"/", siteName}, Link{"/records", "Check records"}, Link{"/reports/" + r.ID, name + " " + r.Model})},
	}
	var rel []Link
	if sp, ok := SitePath(r.Endpoint); ok {
		rel = append(rel, Link{sp, "All checks of " + name + " (" + host + ")"})
	}
	if mp, ok := ModelPath(r.Model); ok {
		rel = append(rel, Link{mp, "Results for " + r.Model + " on other sites"})
	}
	rel = append(rel, Link{"/records", "All check records"})
	p.LinkGroups = []LinkGroup{{Heading: "Related", Links: rel}}
	return p
}

func zhVerdict(score *int) string {
	switch {
	case score == nil:
		return ""
	case *score >= 100:
		return "符合"
	case *score >= 80:
		return "基本符合"
	default:
		return "建议核对"
	}
}

func itemList(site string, recs []Record) string {
	list := make([]map[string]any, 0, len(recs))
	for i, rec := range recs {
		list = append(list, map[string]any{"@type": "ListItem", "position": i + 1, "url": site + "/reports/" + rec.ID, "name": rec.Site + " " + rec.Model})
	}
	return ld(map[string]any{"@context": "https://schema.org", "@type": "ItemList", "itemListElement": list})
}

// SitePage lists every public check of one website. It is the page a search
// for the site's name should land on, so it is indexable as soon as the site
// has one completed, named check.
func SitePage(site, domain string, recs []Record) Page {
	name := domain
	if len(recs) > 0 && recs[0].Site != "" {
		name = recs[0].Site
	}
	latest := ""
	if len(recs) > 0 && recs[0].Verdict != "" {
		latest = " 最近结果：" + recs[0].Verdict + "。"
	}
	desc := fmt.Sprintf("%s（%s）的模型评测与真伪检测记录，共 %d 次公开检测。%s", name, domain, len(recs), latest)
	if len(recs) > 0 && recs[0].Description != "" {
		desc = cut(recs[0].Description, 110) + " | " + desc
	}
	p := Page{
		Path:        "/sites/" + domain,
		Title:       cut(name+" ("+domain+") 模型评测", 60) + " | " + siteName,
		Description: cut(desc, 300),
		H1:          name + "（" + domain + "）模型评测",
		Lead:        cut(desc, 300),
		NoIndex:     len(recs) == 0,
		Records:     recs,
		MaxRecords:  len(recs),
		RecordsHead: name + " 的公开检测",
		JSONLD:      []string{breadcrumbs(site, Link{"/", siteName}, Link{"/sites/" + domain, name})},
		Sections: []Section{{Heading: "如何理解这些结果", Body: []string{
			"每条记录描述某个接口、某个模型在检测当时的表现，只是一次运行的证据，不是排名，也不是对该站点的背书。",
		}}},
	}
	if len(recs) > 0 {
		p.JSONLD = append(p.JSONLD, itemList(site, recs))
	}
	return p
}

// ModelPage shows the board and the full records for one model name. It
// targets searches such as "<model> 模型评测" or "<model> 真伪检测". rows are
// the newest scored result per site, best first; they are results, not
// endorsements.
func ModelPage(site, model string, recs []Record, rows []BoardRow) Page {
	desc := fmt.Sprintf("%s 模型评测：各中转站与官方接口的公开真伪检测结果，共 %d 条记录。榜单按最近一次检测的得分排序，只代表检测当时的表现，不是推荐。", model, len(recs))
	path, _ := ModelPath(model)
	p := Page{
		Path:        path,
		Title:       cut(model+" 模型评测与真伪检测", 60) + " | " + siteName,
		Description: cut(desc, 300),
		H1:          model + " 模型评测",
		Lead:        cut(desc, 300),
		NoIndex:     len(recs) == 0,
		Records:     recs,
		MaxRecords:  len(recs),
		RecordsHead: model + " 的全部检测记录",
		JSONLD:      []string{breadcrumbs(site, Link{"/", siteName}, Link{path, model}), itemList(site, recs)},
		Boards:      []BoardBlock{{Model: model, Rows: rows}},
		Sections: []Section{{Heading: "如何理解这些结果", Body: []string{
			"每条记录是某个模型名在某个接口、某一时刻的一次检测。中转站上的模型名只是声明，检测看的是接口实际的行为。超过 30 天的结果会标记为已过期。",
		}}},
	}
	return p
}

// BoardsIndex is /models: one board per model, most-checked models first.
func BoardsIndex(site string, blocks []BoardBlock) Page {
	p, _ := Lookup("/models")
	p.Boards = blocks
	return p
}

// faqLD is the FAQPage structured data for the visible questions.
func faqLD(qs []QA) string {
	items := make([]map[string]any, 0, len(qs))
	for _, q := range qs {
		items = append(items, map[string]any{
			"@type": "Question", "name": q.Q,
			"acceptedAnswer": map[string]any{"@type": "Answer", "text": q.A},
		})
	}
	return ld(map[string]any{"@context": "https://schema.org", "@type": "FAQPage", "mainEntity": items})
}
