// Package board builds the public result boards: for one model, the latest
// completed result of each website, ordered by score. A board is a list of
// check results, not a ranking of quality or an endorsement: every row carries
// the date of the check, and results older than FreshDays are marked.
package board

import (
	"net/url"
	"sort"
	"strings"
	"time"
)

// FreshDays matches the badge: a result older than this is shown as expired.
const FreshDays = 30

// Run is the part of a stored run a board needs.
type Run struct {
	ID        string
	Site      string // site name read from the tested site, may be empty
	Model     string
	Endpoint  string
	Transport string
	Score     *int
	StartedAt int64 // unix milliseconds
}

// Entry is one row of a board.
type Entry struct {
	Rank      int    `json:"rank"`
	ReportID  string `json:"report_id"`
	Site      string `json:"site"`
	Host      string `json:"host"`
	Score     int    `json:"score"`
	Tier      string `json:"tier"`   // conformant | mostly | review
	Source    string `json:"source"` // official | aws | other
	CheckedAt int64  `json:"checked_at"`
	Fresh     bool   `json:"fresh"`
}

// Hostname returns the lower-case host name of an endpoint, without port.
func Hostname(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// Tier uses the same thresholds as the badge.
func Tier(score int) string {
	switch {
	case score >= 100:
		return "conformant"
	case score >= 80:
		return "mostly"
	default:
		return "review"
	}
}

// SourceOf says whether an endpoint is the vendor's own API, AWS Bedrock or
// something else (a relay or third party).
func SourceOf(endpoint, transport string) string {
	h := Hostname(endpoint)
	switch {
	case h == "api.anthropic.com", h == "api.openai.com":
		return "official"
	case strings.HasPrefix(h, "bedrock-mantle.") && strings.HasSuffix(h, ".api.aws"),
		strings.HasPrefix(h, "bedrock-runtime.") && strings.HasSuffix(h, ".amazonaws.com"),
		transport == "bedrock_runtime":
		return "aws"
	}
	return "other"
}

// Build keeps the newest scored run of each host, orders the hosts by score
// (higher first), then by newest check, and returns at most limit rows. runs
// may be in any order.
func Build(runs []Run, now time.Time, limit int) []Entry {
	latest := map[string]Run{}
	for _, r := range runs {
		host := Hostname(r.Endpoint)
		if host == "" || r.Score == nil {
			continue
		}
		if cur, ok := latest[host]; !ok || r.StartedAt > cur.StartedAt {
			latest[host] = r
		}
	}
	entries := make([]Entry, 0, len(latest))
	for host, r := range latest {
		name := strings.TrimSpace(r.Site)
		if name == "" {
			name = host
		}
		checked := time.UnixMilli(r.StartedAt)
		entries = append(entries, Entry{
			ReportID: r.ID, Site: name, Host: host, Score: *r.Score, Tier: Tier(*r.Score),
			Source: SourceOf(r.Endpoint, r.Transport), CheckedAt: r.StartedAt,
			Fresh: now.Sub(checked) <= FreshDays*24*time.Hour,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.CheckedAt != b.CheckedAt {
			return a.CheckedAt > b.CheckedAt
		}
		return a.Host < b.Host
	})
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	for i := range entries {
		entries[i].Rank = i + 1
	}
	return entries
}

// Site is one row of the site directory: the newest completed result of a host.
type Site struct {
	Host      string `json:"host"`
	Name      string `json:"name"`
	ReportID  string `json:"report_id"`
	Model     string `json:"model"`
	Score     *int   `json:"score"`
	Tier      string `json:"tier"`
	Source    string `json:"source"`
	CheckedAt int64  `json:"checked_at"`
	Checks    int    `json:"checks"` // completed named checks of this host
	Fresh     bool   `json:"fresh"`
}

// Directory groups runs by host, newest check first. A host appears once, with
// its newest run and how many checks it has.
func Directory(runs []Run, now time.Time) []Site {
	byHost := map[string]*Site{}
	for _, r := range runs {
		host := Hostname(r.Endpoint)
		if host == "" {
			continue
		}
		s, ok := byHost[host]
		if !ok {
			s = &Site{Host: host}
			byHost[host] = s
		}
		s.Checks++
		if s.CheckedAt != 0 && r.StartedAt <= s.CheckedAt {
			continue
		}
		name := strings.TrimSpace(r.Site)
		if name == "" {
			name = host
		}
		s.Name, s.ReportID, s.Model, s.Score = name, r.ID, r.Model, r.Score
		s.Source, s.CheckedAt = SourceOf(r.Endpoint, r.Transport), r.StartedAt
		s.Tier = ""
		if r.Score != nil {
			s.Tier = Tier(*r.Score)
		}
		s.Fresh = now.Sub(time.UnixMilli(r.StartedAt)) <= FreshDays*24*time.Hour
	}
	out := make([]Site, 0, len(byHost))
	for _, s := range byHost {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CheckedAt != out[j].CheckedAt {
			return out[i].CheckedAt > out[j].CheckedAt
		}
		return out[i].Host < out[j].Host
	})
	return out
}
