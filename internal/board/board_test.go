package board

import (
	"testing"
	"time"
)

func sc(n int) *int { return &n }

func TestBuildKeepsNewestPerHostAndSortsByScore(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	day := int64(24 * 3600 * 1000)
	runs := []Run{
		{ID: "old-a", Site: "A", Endpoint: "https://a.example/v1", Score: sc(100), StartedAt: now.UnixMilli() - 40*day},
		{ID: "new-a", Site: "A", Endpoint: "https://A.example:8443/v1", Score: sc(85), StartedAt: now.UnixMilli() - 2*day},
		{ID: "b", Site: "", Endpoint: "https://b.example", Score: sc(100), StartedAt: now.UnixMilli() - 5*day},
		{ID: "c", Site: "C", Endpoint: "https://api.anthropic.com", Score: sc(100), StartedAt: now.UnixMilli() - 1*day},
		{ID: "noscore", Site: "D", Endpoint: "https://d.example", StartedAt: now.UnixMilli()},
		{ID: "nohost", Site: "E", Endpoint: "not a url", Score: sc(100), StartedAt: now.UnixMilli()},
	}
	got := Build(runs, now, 10)
	if len(got) != 3 {
		t.Fatalf("%+v", got)
	}
	// 100s first, newest check first among them; then the 85.
	if got[0].ReportID != "c" || got[1].ReportID != "b" || got[2].ReportID != "new-a" {
		t.Fatalf("order: %+v", got)
	}
	if got[0].Rank != 1 || got[2].Rank != 3 || got[0].Source != "official" || got[1].Site != "b.example" || got[2].Tier != "mostly" {
		t.Fatalf("fields: %+v", got)
	}
	if !got[0].Fresh || !got[2].Fresh {
		t.Fatal("recent results are fresh")
	}
	if len(Build(runs, now, 2)) != 2 {
		t.Fatal("limit")
	}
}

func TestStaleResultIsMarked(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	old := now.UnixMilli() - 31*24*3600*1000
	got := Build([]Run{{ID: "x", Endpoint: "https://x.example", Score: sc(100), StartedAt: old}}, now, 5)
	if len(got) != 1 || got[0].Fresh {
		t.Fatalf("%+v", got)
	}
}

func TestSourceOf(t *testing.T) {
	for in, want := range map[string]string{
		"https://api.anthropic.com":                       "official",
		"https://api.openai.com/v1":                       "official",
		"https://bedrock-mantle.us-east-1.api.aws":        "aws",
		"https://bedrock-runtime.us-west-2.amazonaws.com": "aws",
		"https://relay.example.com":                       "other",
		"https://api.anthropic.com.evil.example":          "other",
	} {
		if got := SourceOf(in, ""); got != want {
			t.Fatalf("%s: %s want %s", in, got, want)
		}
	}
	if SourceOf("https://x.example", "bedrock_runtime") != "aws" {
		t.Fatal("transport")
	}
}

func TestDirectory(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	runs := []Run{
		{ID: "1", Site: "A", Model: "m1", Endpoint: "https://a.example", Score: sc(90), StartedAt: now.UnixMilli() - 1000},
		{ID: "2", Site: "A2", Model: "m2", Endpoint: "https://a.example/v1", Score: sc(100), StartedAt: now.UnixMilli() - 10},
		{ID: "3", Site: "B", Model: "m1", Endpoint: "https://b.example", Score: sc(50), StartedAt: now.UnixMilli() - 500},
	}
	d := Directory(runs, now)
	if len(d) != 2 || d[0].Host != "a.example" || d[0].Checks != 2 || d[0].ReportID != "2" || d[0].Name != "A2" || d[0].Tier != "conformant" || d[1].Tier != "review" {
		t.Fatalf("%+v", d)
	}
}
