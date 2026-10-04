package claudecheck

import (
	"context"
	"testing"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

func TestDeclaredModelRelations(t *testing.T) {
	for _, tc := range []struct {
		left, right string
		relation    int
	}{
		{"claude-opus-5", "claude-sonnet-5", -1},
		{"claude-opus-5", "global.anthropic.claude-opus-5", 1},
		{"claude-opus-5", "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-opus-5", 1},
		{"claude-opus-4-6", "anthropic.claude-opus-4-6-v1", 1},
		{"claude-sonnet-4-5", "anthropic.claude-sonnet-4-5-20250929-v1:0", 1},
		{"claude-opus-4-7", "claude-opus-4-8", -1},
		{"claude-sonnet-4-5-20250929", "claude-sonnet-4-5-20251001", -1},
		{"claude-opus-5", "claude-opus-5-custom", 0},
		{"claude-opus-5", "alias", 0},
		{"claude-opus-5", "claude-opus-5-v1", 0},
		{"alias", "alias", 0},
		{"claude-opus-5", "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/abc123", 0},
		{"claude-opus-5", "arn:aws:other:us-east-1:123456789012:foundation-model/anthropic.claude-opus-5", 0},
		{"claude-opus-5", "claude-opus-5-20261201", 0},
		{"claude-sonnet-4-5", "claude-sonnet-4-5-20261301", 0},
		{"claude-3-5-sonnet-20241022", "anthropic.claude-3-5-sonnet-20241022-v2:0", 1},
	} {
		require.Equal(t, tc.relation, declaredModelRelation(tc.left, tc.right), tc.left+" vs "+tc.right)
	}
}

func TestDeclaredModelReplacementAndUnknownAliases(t *testing.T) {
	for _, tc := range []struct{ name, requested, mapped, returned, state string }{
		{"both fields replaced", "claude-opus-5", "global.anthropic.claude-sonnet-5", "claude-sonnet-5", "fail"},
		{"returned field replaced", "claude-opus-5", "claude-opus-5", "claude-sonnet-5", "fail"},
		{"mapping replaced but echo concealed", "claude-opus-5", "claude-sonnet-5", "claude-opus-5", "fail"},
		{"documented wrapper", "claude-opus-5", "global.anthropic.claude-opus-5", "claude-opus-5", "pass"},
		{"unknown client alias", "custom-claude", "claude-sonnet-5", "claude-sonnet-5", "inconclusive"},
		{"unknown response alias", "claude-opus-5", "claude-opus-5", "custom-claude", "inconclusive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Run(context.Background(), Options{Model: tc.requested}, rewriteModelTransport(t, tc.mapped, tc.returned, ""))
			for _, id := range []string{"model_echo", "model_consistency"} {
				check := resultCheck(t, r, id)
				require.Equal(t, tc.state, check.Status, id)
				require.Equal(t, false, check.Evidence["identity_verified"])
			}
			require.Len(t, r.Samples, 8, "model comparisons must not add upstream requests")
		})
	}
}

func rewriteModelTransport(t *testing.T, mapped, returned, late string) Transport {
	base := happyTransport(t, true)
	return func(ctx context.Context, req Request) (Response, error) {
		res, err := base(ctx, req)
		res.Model = mapped
		if req.Count {
			return res, err
		}
		name := returned
		if late != "" && req.Body["tools"] != nil {
			name = late
		}
		if len(res.Events) > 0 {
			var start map[string]any
			require.NoError(t, common.Unmarshal(res.Events[0], &start))
			start["message"].(map[string]any)["model"] = name
			res.Events[0], err = common.Marshal(start)
			require.NoError(t, err)
		} else {
			var m message
			require.NoError(t, common.Unmarshal(res.Body, &m))
			m.Model = name
			res.Body, err = common.Marshal(m)
			require.NoError(t, err)
		}
		return res, err
	}
}

func TestModelChangeAfterGoodBaseline(t *testing.T) {
	for _, requested := range []string{"claude-opus-5", "custom-alias"} {
		r := Run(context.Background(), Options{Model: requested}, rewriteModelTransport(t, "claude-opus-5", "claude-opus-5", "claude-sonnet-5"))
		check := resultCheck(t, r, "model_consistency")
		require.Equal(t, "fail", check.Status)
		require.Contains(t, check.Evidence["returned_models"], "claude-sonnet-5")
		conflicts := check.Evidence["conflicts"].([]modelConflict)
		require.NotEmpty(t, conflicts)
		found := false
		for _, conflict := range conflicts {
			if conflict.Probe == "tool" {
				found = true
			}
		}
		require.True(t, found)
	}
}

func TestModelMismatchEvidenceSurvivesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	base := rewriteModelTransport(t, "claude-opus-5", "claude-opus-5", "claude-sonnet-5")
	r := Run(ctx, Options{Model: "claude-opus-5"}, func(ctx context.Context, req Request) (Response, error) {
		res, err := base(ctx, req)
		if req.Body["max_tokens"] == 1 {
			cancel()
		}
		return res, err
	})
	require.True(t, r.Cancelled)
	require.Equal(t, "fail", resultCheck(t, r, "model_consistency").Status)
}

func TestMatchingEchoIsNeverAuthentication(t *testing.T) {
	r := Run(context.Background(), Options{Model: "claude-opus-5"}, rewriteModelTransport(t, "claude-opus-5", "claude-opus-5", ""))
	check := resultCheck(t, r, "model_consistency")
	require.Equal(t, "pass", check.Status)
	require.Equal(t, false, check.Evidence["identity_verified"])
	require.Equal(t, "declared_model_ids", check.Evidence["evidence_type"])
}
