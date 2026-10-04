// Package siteinfo reads the public name and description of a tested website
// from its home page, so check records can show what was tested and search
// engines can index it. Everything it returns is untrusted text from the
// remote site: it is cleaned and length-limited here, and must still be
// escaped wherever it is rendered.
package siteinfo

import (
	"context"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	MaxNameRunes = 80
	MaxDescRunes = 300
	maxBody      = 256 << 10
	maxHops      = 4
	fetchTimeout = 6 * time.Second
)

type Info struct {
	Name        string
	Description string
}

// Validator rejects URLs the server must not fetch (private addresses and so on).
type Validator func(ctx context.Context, rawURL string) error

// Fetch reads the home page of the endpoint's origin. It sends no credentials,
// follows at most a few redirects and validates every hop. Any failure returns
// an empty Info: the check itself never depends on this.
func Fetch(ctx context.Context, hc *http.Client, validate Validator, endpoint string) Info {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return Info{}
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	next := (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/"}).String()
	for hop := 0; hop <= maxHops; hop++ {
		if validate != nil && validate(ctx, next) != nil {
			return Info{}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return Info{}
		}
		req.Header.Set("User-Agent", "ModelsExam-site-info/1.0")
		req.Header.Set("Accept", "text/html")
		resp, err := hc.Do(req)
		if err != nil {
			return Info{}
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			resp.Body.Close()
			target, err := url.Parse(next)
			if err != nil || loc == "" {
				return Info{}
			}
			target, err = target.Parse(loc)
			if err != nil || (target.Scheme != "http" && target.Scheme != "https") {
				return Info{}
			}
			next = target.String()
			continue
		}
		defer resp.Body.Close()
		ct := strings.ToLower(resp.Header.Get("Content-Type"))
		if resp.StatusCode != http.StatusOK || (ct != "" && !strings.Contains(ct, "html")) {
			return Info{}
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		if err != nil && len(body) == 0 {
			return Info{}
		}
		return Parse(body)
	}
	return Info{}
}

var (
	titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	metaRe  = regexp.MustCompile(`(?is)<meta\s[^>]*>`)
	attrRe  = regexp.MustCompile(`(?is)([a-zA-Z][a-zA-Z0-9:_-]*)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
)

// Parse extracts the name and description from home-page HTML.
func Parse(body []byte) Info {
	doc := string(body)
	meta := map[string]string{}
	for _, tag := range metaRe.FindAllString(doc, -1) {
		attrs := map[string]string{}
		for _, m := range attrRe.FindAllStringSubmatch(tag, -1) {
			v := m[2]
			if v == "" {
				v = m[3]
			}
			attrs[strings.ToLower(m[1])] = v
		}
		key := strings.ToLower(attrs["property"])
		if key == "" {
			key = strings.ToLower(attrs["name"])
		}
		if key != "" && attrs["content"] != "" {
			if _, seen := meta[key]; !seen {
				meta[key] = attrs["content"]
			}
		}
	}
	title := ""
	if m := titleRe.FindStringSubmatch(doc); m != nil {
		title = m[1]
	}
	name := first(meta["og:site_name"], meta["application-name"], title, meta["og:title"])
	desc := first(meta["description"], meta["og:description"], meta["twitter:description"])
	return Info{Name: Clean(name, MaxNameRunes), Description: Clean(desc, MaxDescRunes)}
}

func first(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// Clean decodes entities, drops control characters, collapses whitespace and
// cuts the text to max runes.
func Clean(s string, max int) string {
	s = html.UnescapeString(s)
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			space = true
		case unicode.IsControl(r) || r == '�':
		default:
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		}
	}
	out := b.String()
	if utf8.RuneCountInString(out) > max {
		out = strings.TrimSpace(string([]rune(out)[:max])) + "…"
	}
	return out
}
