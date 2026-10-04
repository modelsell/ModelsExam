package claudecheck

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

func collectRepeatedPrompt(t *testing.T, mode string, refs []ComparisonBaseline) Report {
	t.Helper()
	r := runner{ctx: context.Background(), report: Report{Version: 10, ID: mode, Model: "claude-opus-5", Baselines: refs,
		TokenAudit: &TokenAuditReport{Version: 4, Prompt: []TokenComparison{}}}}
	for _, f := range repeatedPromptFixtures(r.report.Model) {
		r.report.TokenAudit.Prompt = append(r.report.TokenAudit.Prompt, TokenComparison{ID: f.id, Code: "pending"})
	}
	inferences, counts := 0, 0
	r.observe = func(e Event) {
		if e.Type == "token_audit" && !r.hasCheck("prompt_integrity") {
			require.Nil(t, e.TokenAudit.PromptAssessment.Score, "no final score while still collecting")
		}
	}
	r.call = func(ctx context.Context, req Request) (Response, error) {
		if req.Count {
			counts++
			if mode == "no_count" || mode == "hidden_no_count" {
				return Response{Status: 404}, nil
			}
		} else {
			inferences++
		}
		content := req.Body["messages"].([]any)[0].(map[string]any)["content"].(string)
		system, _ := req.Body["system"].(string)
		input := int64(20 + len(content)/3 + len(system)/3)
		profile, promptProfile := bodyProfile(req.Body), promptContentProfile(req.Body)
		answer, model := "PONG", r.report.Model
		if len(content) > 100 {
			answer = "701544"
		}
		if system != "" {
			answer = "MAPLE-7391"
		}
		switch mode {
		case "hidden", "hidden_no_count":
			input += 64
		case "one_outlier":
			if inferences == 4 {
				input += 60
			}
		case "small_jitter":
			if inferences == 4 {
				input++
			}
		case "mixed_model":
			if inferences == 4 {
				model = "claude-sonnet-5"
			}
		case "incomplete":
			if inferences == 4 && !req.Count {
				return Response{Status: 500}, fmt.Errorf("transient")
			}
		case "persistent_count_gap":
			if !req.Count {
				input += 8
			}
		case "single_count_gap":
			if inferences == 4 && req.Count {
				input -= 8
			}
		case "local_rewrite":
			promptProfile = "changed-content"
		case "wrong_answer":
			if inferences == 4 {
				answer = "extra boilerplate"
			}
		}
		var out Response
		if req.Count {
			out = testResponse(t, map[string]any{"input_tokens": input})
		} else {
			zero, output := int64(0), int64(6)
			out = testResponse(t, message{ID: fmt.Sprintf("msg_%d", inferences), Type: "message", Role: "assistant", Model: model, StopReason: "end_turn",
				Content: []map[string]any{{"type": "text", "text": answer}}, Usage: Usage{Input: &input, Output: &output, CacheRead: &zero, CacheWrite: &zero}})
		}
		out.RequestProfile, out.PromptProfile = profile, promptProfile
		return out, nil
	}
	r.promptTokenAudit()
	require.Equal(t, 9, inferences)
	if mode == "no_count" || mode == "hidden_no_count" {
		require.Equal(t, 1, counts)
	}
	return r.report
}

func TestRepeatedPromptUsesStableGroupsAndPreservesOutliers(t *testing.T) {
	clean := collectRepeatedPrompt(t, "clean", nil)
	ref := SnapshotBaseline(clean, "trusted", "aws", 1)
	for _, tc := range []struct {
		mode, code string
		score      int
	}{
		{"matching", "reference_aligned", 100}, {"no_count", "reference_aligned", 100},
		{"small_jitter", "reference_aligned", 100}, {"one_outlier", "prompt_samples_unstable", 30},
		{"mixed_model", "prompt_samples_unstable", 30}, {"incomplete", "prompt_samples_incomplete", 27},
		{"local_rewrite", "local_prompt_changed", 30}, {"wrong_answer", "reference_aligned", 97},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			r := collectRepeatedPrompt(t, tc.mode, []ComparisonBaseline{ref})
			require.Equal(t, tc.code, r.TokenAudit.Injection.Code)
			require.Equal(t, tc.score, *r.TokenAudit.PromptAssessment.Score)
			require.Len(t, r.TokenAudit.Injection.Sampling, 3)
			if tc.mode == "one_outlier" {
				g := r.TokenAudit.Injection.Sampling[0]
				require.Equal(t, *g.Min, *g.Median)
				require.Equal(t, int64(60), *g.Max-*g.Min)
			}
		})
	}
	for _, mode := range []string{"hidden", "hidden_no_count"} {
		r := collectRepeatedPrompt(t, mode, []ComparisonBaseline{ref})
		require.Equal(t, "reference_fixed_excess", r.TokenAudit.Injection.Code)
		require.Less(t, *r.TokenAudit.PromptAssessment.Score, 100)
		for _, pair := range r.TokenAudit.Injection.References[0].Pairs {
			require.Equal(t, int64(64), pair.Difference)
		}
	}
}

func TestRepeatedPromptRejectsWeakReferencesAndSingleCountAnomalies(t *testing.T) {
	for _, mode := range []string{"one_outlier", "mixed_model", "incomplete", "local_rewrite"} {
		ref := SnapshotBaseline(collectRepeatedPrompt(t, mode, nil), "ref", "aws", 1)
		r := collectRepeatedPrompt(t, "target", []ComparisonBaseline{ref})
		require.Equal(t, 1, r.TokenAudit.Injection.Incompatible)
		require.Equal(t, 30, *r.TokenAudit.PromptAssessment.Score)
	}
	legacy := SnapshotBaseline(collectPromptChannel(t, "legacy", nil), "v3", "aws", 1)
	r := collectRepeatedPrompt(t, "target", []ComparisonBaseline{legacy})
	require.Equal(t, 1, r.TokenAudit.Injection.Incompatible)
	r = collectRepeatedPrompt(t, "single_count_gap", nil)
	require.Equal(t, "injection_unverified", r.TokenAudit.Injection.Code)
	r = collectRepeatedPrompt(t, "persistent_count_gap", nil)
	require.Equal(t, "same_endpoint_extra_input", r.TokenAudit.Injection.Code)
	clean := SnapshotBaseline(collectRepeatedPrompt(t, "clean", nil), "clean", "aws", 1)
	hidden := SnapshotBaseline(collectRepeatedPrompt(t, "hidden", nil), "hidden", "kiro", 2)
	r = collectRepeatedPrompt(t, "target", []ComparisonBaseline{clean, hidden})
	require.Equal(t, "references_disagree", r.TokenAudit.Injection.Code)
	require.Equal(t, 30, *r.TokenAudit.PromptAssessment.Score)
	// Persist the raw rounds, not just the median, so a saved baseline can be
	// independently revalidated without inventing old samples.
	data, err := common.Marshal(SnapshotBaseline(r, "next", "aws", 3))
	require.NoError(t, err)
	var saved ComparisonBaseline
	require.NoError(t, common.Unmarshal(data, &saved))
	require.Len(t, saved.TokenAudit.Prompt, 9)
}

func TestRepeatedCacheFreshRoundsIncludeMissesRewritesAndTransientErrors(t *testing.T) {
	for _, mode := range []string{"clean", "miss", "rewrite", "transient", "write_missing", "cold_hit", "profile_change", "rate_limit"} {
		t.Run(mode, func(t *testing.T) {
			r := runner{ctx: context.Background(), report: Report{Version: 10, Model: "claude-opus-5", TokenAudit: &TokenAuditReport{Version: 4}}}
			for _, id := range repeatedCacheReadIDs() {
				r.report.TokenAudit.Cache = append(r.report.TokenAudit.Cache, TokenComparison{ID: id, Code: "pending"})
			}
			var prefixes []string
			calls := 0
			r.call = func(ctx context.Context, req Request) (Response, error) {
				calls++
				data, err := common.Marshal(req.Body)
				require.NoError(t, err)
				prefixes = append(prefixes, string(data))
				if mode == "transient" && calls == 2 {
					return Response{Status: 500}, fmt.Errorf("transient")
				}
				if mode == "rate_limit" && calls == 2 {
					return Response{Status: 429}, fmt.Errorf("rate limited")
				}
				input, output, write, read := int64(10), int64(1), int64(0), int64(5000)
				if calls == 1 || calls == 5 {
					write, read = 5000, 0
				}
				if mode == "miss" && calls == 2 {
					write, read = 5000, 0
				}
				if mode == "rewrite" && calls == 2 {
					write = 5000
				}
				if mode == "cold_hit" && calls == 1 {
					read = 5000
				}
				var writePtr = &write
				if mode == "write_missing" && calls == 1 {
					writePtr = nil
				}
				out := testResponse(t, message{ID: fmt.Sprintf("msg_%d", calls), Type: "message", Role: "assistant", Model: r.report.Model, StopReason: "end_turn",
					Content: []map[string]any{{"type": "text", "text": "amber"}}, Usage: Usage{Input: &input, Output: &output, CacheRead: &read, CacheWrite: writePtr}})
				out.RequestProfile = bodyProfile(req.Body)
				if mode == "profile_change" && calls == 2 {
					out.RequestProfile = "other"
				}
				return out, nil
			}
			r.repeatedCache()
			if mode == "rate_limit" {
				require.Equal(t, 2, calls)
				require.Nil(t, r.report.TokenAudit.CacheAssessment.Score)
				return
			}
			require.Equal(t, RepeatedCacheRequests, calls)
			require.NotEqual(t, prefixes[0], prefixes[4])
			for _, group := range [][]string{prefixes[:4], prefixes[4:]} {
				for _, value := range group {
					require.Equal(t, group[0], value)
				}
			}
			a := r.report.TokenAudit.CacheAssessment
			expected := map[string]int{"clean": 100, "miss": 83, "rewrite": 92, "transient": 83, "write_missing": 50, "cold_hit": 50, "profile_change": 83}[mode]
			require.Equal(t, expected, *a.Score)
			require.NotContains(t, strings.Join(prefixes, ""), "private")
		})
	}
}
