package claudecheck

import (
	"context"
	"strings"
	"testing"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

// A deterministic fake upstream: its input meter includes the known injected
// prefix, while its answer still obeys the user's original marker. It is not an
// LLM accuracy benchmark; it exercises the detector against controlled truth.
func collectPromptChannel(t *testing.T, mode string, references []ComparisonBaseline) Report {
	t.Helper()
	r := runner{ctx: context.Background(), report: Report{Version: 9, ID: mode, Model: "claude-opus-5", Baselines: references,
		TokenAudit: &TokenAuditReport{Version: 3, Prompt: []TokenComparison{{ID: "floor_a"}, {ID: "floor_b"}, {ID: "system_canary"}}}}}
	calls := 0
	r.call = func(ctx context.Context, req Request) (Response, error) {
		calls++
		if req.Count && (mode == "no_count" || mode == "hidden_no_count") {
			return Response{Status: 404}, nil
		}
		body, err := common.Marshal(req.Body)
		require.NoError(t, err)
		var effective map[string]any
		require.NoError(t, common.Unmarshal(body, &effective))
		output, zero := int64(6), int64(0)
		if mode == "local_rewrite" {
			system, _ := effective["system"].(string)
			effective["system"] = strings.Repeat("x", 64) + system
		}
		profile, promptProfile := bodyProfile(effective), promptContentProfile(effective)
		visibleSystem, _ := effective["system"].(string)
		// The checker sees the request before this upstream-only mutation.
		if mode == "hidden" || mode == "hidden_no_count" || mode == "forged" {
			effective["system"] = strings.Repeat("x", 64) + visibleSystem
		}
		actualSystem, _ := effective["system"].(string)
		// A toy meter, deliberately derived from the effective body, not an
		// assertion about any real model's tokenizer.
		input := int64(34 + len(actualSystem))
		if mode == "forged" {
			input = int64(34 + len(visibleSystem))
		}
		if mode == "jitter" && calls == 3 {
			input += 12
		}
		answer := "PONG"
		if strings.Contains(actualSystem, "MAPLE-7391") {
			answer = "MAPLE-7391"
		}
		if req.Count {
			out := testResponse(t, map[string]any{"input_tokens": input})
			out.RequestProfile, out.PromptProfile = profile, promptProfile
			return out, nil
		}
		out := testResponse(t, message{ID: "msg_test", Type: "message", Role: "assistant", Model: r.report.Model, StopReason: "end_turn",
			Content: []map[string]any{{"type": "text", "text": answer}}, Usage: Usage{Input: &input, Output: &output, CacheRead: &zero, CacheWrite: &zero}})
		out.RequestProfile, out.PromptProfile = profile, promptProfile
		return out, nil
	}
	r.promptTokenAudit()
	return r.report
}

func TestHiddenPromptDetectedEvenWhenAnswersAndSameEndpointCountsMatch(t *testing.T) {
	clean := collectPromptChannel(t, "clean", nil)
	require.Equal(t, "injection_unverified", clean.TokenAudit.Injection.Code)
	require.Equal(t, 30, *clean.TokenAudit.PromptAssessment.Score)
	ref := SnapshotBaseline(clean, "trusted", "aws", 1)
	for _, mode := range []string{"matching", "hidden", "hidden_no_count", "no_count", "local_rewrite", "jitter"} {
		t.Run(mode, func(t *testing.T) {
			report := collectPromptChannel(t, mode, []ComparisonBaseline{ref})
			audit := report.TokenAudit
			switch mode {
			case "matching", "no_count":
				require.Equal(t, "reference_aligned", audit.Injection.Code)
				require.Equal(t, 100, *audit.PromptAssessment.Score)
				require.Equal(t, "saved_reference", audit.PromptAssessment.TokenSource)
			case "hidden", "hidden_no_count":
				require.Equal(t, "reference_fixed_excess", audit.Injection.Code)
				for _, pair := range audit.Injection.References[0].Pairs {
					require.Equal(t, int64(64), pair.Difference)
				}
				// The old same-endpoint test sees perfect scores and misses it.
				for _, item := range audit.Prompt {
					if mode == "hidden" {
						require.Equal(t, 100, *item.Score)
					}
					require.Equal(t, 100, *item.Behavior.Score)
				}
				require.Less(t, *audit.PromptAssessment.Score, 100)
			case "local_rewrite":
				require.Equal(t, "local_prompt_changed", audit.Injection.Code)
				require.Equal(t, 3, audit.Injection.LocalChanges)
			case "jitter":
				require.Equal(t, "reference_input_deviation", audit.Injection.Code)
			}
			bytes, err := common.Marshal(SnapshotBaseline(report, "next", "aws", 2))
			require.NoError(t, err)
			require.NotContains(t, string(bytes), "Private added prefix")
			require.Contains(t, string(bytes), "injection")
		})
	}
}

func TestPromptReferenceConflictsAndMissingEvidenceNeverBecomeClean(t *testing.T) {
	clean := SnapshotBaseline(collectPromptChannel(t, "clean", nil), "clean-ref", "aws", 1)
	hidden := SnapshotBaseline(collectPromptChannel(t, "hidden", nil), "hidden-ref", "kiro", 2)
	r := collectPromptChannel(t, "target", []ComparisonBaseline{clean, hidden})
	require.Equal(t, "references_disagree", r.TokenAudit.Injection.Code)
	require.Equal(t, 30, *r.TokenAudit.PromptAssessment.Score)
	require.Len(t, r.TokenAudit.Injection.References, 2)
	for _, mode := range []string{"model", "version", "profile", "unstable"} {
		t.Run(mode, func(t *testing.T) {
			bytes, _ := common.Marshal(clean)
			var ref ComparisonBaseline
			require.NoError(t, common.Unmarshal(bytes, &ref))
			switch mode {
			case "model":
				ref.Model = "claude-sonnet-5"
			case "version":
				ref.TokenAudit.Version = 2
			case "profile":
				ref.TokenAudit.Prompt[0].ClientProfile = "different"
			case "unstable":
				*ref.TokenAudit.Prompt[1].Actual += 20
			}
			r := collectPromptChannel(t, "target", []ComparisonBaseline{ref})
			require.Equal(t, "injection_unverified", r.TokenAudit.Injection.Code)
			require.Equal(t, 1, r.TokenAudit.Injection.Incompatible)
		})
	}
	forged := collectPromptChannel(t, "forged", []ComparisonBaseline{clean})
	require.Equal(t, "reference_aligned", forged.TokenAudit.Injection.Code) // Honest boundary: forged counters remain indistinguishable.
	r = collectPromptChannel(t, "hidden", nil)
	require.Equal(t, "injection_unverified", r.TokenAudit.Injection.Code) // Cannot prove an unseen prefix without an independent reference.
}

func TestPromptContentProfileSeparatesTextChangesFromWireNormalization(t *testing.T) {
	a := []byte(`{"model":"one","max_tokens":128,"messages":[{"role":"user","content":"PONG"}]}`)
	b := []byte(`{"model":"two","max_tokens":256,"anthropic_version":"bedrock-2023-05-31","messages":[{"role":"user","content":[{"type":"text","text":"PONG","cache_control":{"type":"ephemeral"}}]}]}`)
	require.Equal(t, PromptContentProfile(a), PromptContentProfile(b))
	require.NotEqual(t, PromptContentProfile(a), PromptContentProfile([]byte(`{"system":"Added instruction","messages":[{"role":"user","content":"PONG"}]}`)))
	require.NotEqual(t, PromptContentProfile(a), PromptContentProfile([]byte(`{"messages":[{"role":"assistant","content":"PONG"}]}`)))
	fixtures := promptChannelFixtures("claude-opus-5")
	require.Equal(t, bodyProfile(fixtures[0].body), bodyProfile(fixtures[1].body))
	require.NotContains(t, fixtures[0].body, "system")
	require.NotEqual(t, promptContentProfile(fixtures[0].body), promptContentProfile(fixtures[2].body))
}
