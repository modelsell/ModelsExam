// Package badge decides which check result a website may show as its
// ModelsExam badge, and renders it as JSON, an SVG image and an embeddable
// script. The badge belongs to a domain: it is built from the latest completed
// check whose endpoint host is that domain (or a subdomain), and it expires.
package badge

import (
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// FreshDays is how long a completed check counts as current. After that the
// badge reads "Check expired" until the site is checked again.
const FreshDays = 30

const freshFor = FreshDays * 24 * time.Hour

// Run is the part of a stored check run the badge needs.
type Run struct {
	ID        string
	Model     string
	Transport string
	Endpoint  string
	Score     *int
	StartedAt int64 // unix milliseconds
}

type Result struct {
	Domain    string `json:"domain"`
	Status    string `json:"status"` // ok | stale | none
	Tier      string `json:"tier,omitempty"`
	Score     *int   `json:"score,omitempty"`
	Model     string `json:"model,omitempty"`
	Protocol  string `json:"protocol,omitempty"`
	ReportID  string `json:"report_id,omitempty"`
	CheckedAt int64  `json:"checked_at,omitempty"` // unix milliseconds
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

var labelRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// NormalizeDomain turns what a person types (a URL, host:port, mixed case)
// into a bare host name. IP addresses, localhost and names without a dot are
// rejected: a badge is for a public website.
func NormalizeDomain(in string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(in))
	s = strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSuffix(s, ".")
	if s == "" || len(s) > 253 {
		return "", errors.New("enter a domain such as relay.example.com")
	}
	labels := strings.Split(s, ".")
	if len(labels) < 2 {
		return "", errors.New("enter a full domain such as relay.example.com")
	}
	for _, l := range labels {
		if !labelRe.MatchString(l) {
			return "", errors.New("the domain may contain only letters, digits and hyphens")
		}
	}
	tld := labels[len(labels)-1]
	if strings.Trim(tld, "0123456789") == "" {
		return "", errors.New("IP addresses cannot have a badge; use the domain name")
	}
	return s, nil
}

// HostMatches reports whether an endpoint URL belongs to domain: the same
// host or a subdomain of it.
func HostMatches(endpoint, domain string) bool {
	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	return within(strings.ToLower(u.Hostname()), domain)
}

// OriginAllowed reports whether a browser Origin header is a page of domain.
func OriginAllowed(origin, domain string) bool {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	return within(strings.ToLower(u.Hostname()), domain)
}

func within(host, domain string) bool {
	return host != "" && (host == domain || strings.HasSuffix(host, "."+domain))
}

func tierOf(score int) string {
	switch {
	case score >= 100:
		return "conformant"
	case score >= 80:
		return "mostly"
	default:
		return "review"
	}
}

func protocolOf(transport string) string {
	switch transport {
	case "openai_api":
		return "openai"
	case "image_api":
		return "image"
	default:
		return "claude"
	}
}

// Pick chooses the badge for domain from completed, scored runs: the newest
// one whose endpoint host matches.
func Pick(runs []Run, domain string, now time.Time) Result {
	sorted := append([]Run(nil), runs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].StartedAt > sorted[j].StartedAt })
	for _, r := range sorted {
		if r.Score == nil || !HostMatches(r.Endpoint, domain) {
			continue
		}
		expires := time.UnixMilli(r.StartedAt).Add(freshFor)
		res := Result{
			Domain: domain, Status: "ok", Tier: tierOf(*r.Score), Score: r.Score, Model: r.Model,
			Protocol: protocolOf(r.Transport), ReportID: r.ID, CheckedAt: r.StartedAt, ExpiresAt: expires.UnixMilli(),
		}
		if !now.Before(expires) {
			res.Status = "stale"
		}
		return res
	}
	return Result{Domain: domain, Status: "none"}
}
