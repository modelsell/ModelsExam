package claudecheck

import (
	"context"
	"strings"
	"testing"

	"model-check/common"
	"github.com/stretchr/testify/require"
)

func TestComparisonModelsMatch(t *testing.T) {
	for _, tc := range []struct {
		left, right string
		matched     bool
	}{
		{"claude-opus-5", "global.anthropic.claude-opus-5", true},
		{"claude-sonnet-4-5", "us.anthropic.claude-sonnet-4-5-20250929-v1:0", true},
		{" MY-ALIAS ", "my-alias", true},
		{"claude-opus-5", "claude-sonnet-5", false},
		{"claude-opus-4-7", "claude-opus-5", false},
		{"my-alias", "claude-opus-5", false},
		{"", "", false},
		{"claude-sonnet-4-5-20250929", "claude-sonnet-4-5-20251029", false},
	} {
		require.Equal(t, tc.matched, ComparisonModelsMatch(tc.left, tc.right), tc.left+" / "+tc.right)
	}
}

func TestFocusedBaselineRetainsBoundedTokenLedger(t *testing.T) {
	base := focusedTransport(t)
	options := Options{Suite: "focused", Model: "claude-opus-5", Cache: true, Fingerprint: true, PromptAudit: true,
		PDF: true, Vision: true, Bedrock: true, CompareBaselines: true, BaselineID: "prior-reference", BaselineType: "ccmax"}
	report := Run(context.Background(), options, func(ctx context.Context, request Request) (Response, error) {
		if request.Body["thinking"] != nil && request.Body["system"] != fingerprintSystem && request.Body["system"] != capabilitySystem && request.Body["system"] != usageTokensSystem {
			input, output := int64(43), int64(1)
			response := testResponse(t, message{ID: "msg_test", Type: "message", Role: "assistant", Model: "claude-opus-5", StopReason: "end_turn",
				Content: []map[string]any{{"type": "text", "text": "PONG"}}, Usage: Usage{Input: &input, Output: &output}})
			response.RequestProfile, response.PromptProfile = bodyProfile(request.Body), promptContentProfile(request.Body)
			return response, nil
		}
		return base(ctx, request)
	})
	require.Len(t, report.Samples, 36)
	for i := range report.Samples {
		report.Samples[i].Headers = map[string]string{"request-id": "private-evidence"}
		report.Samples[i].Error, report.Samples[i].MessageID = "private-error", "private-message"
	}
	for i := range report.Benchmark.Items {
		report.Benchmark.Items[i].Actual = strings.Repeat("长", 512)
	}
	snapshot := SnapshotBaseline(report, "reference-id", "ccmax", 123)
	require.Empty(t, snapshot.Options.BaselineID)
	require.Empty(t, snapshot.Options.BaselineType)
	require.False(t, snapshot.Options.CompareBaselines)
	require.Equal(t, "prior-reference", report.Options.BaselineID)
	require.Len(t, snapshot.Samples, 36)
	require.Equal(t, 3, snapshot.Benchmark.Version)
	require.Len(t, snapshot.Benchmark.Items, 6)
	for i, item := range snapshot.Benchmark.Items {
		require.Empty(t, item.Actual)
		require.Empty(t, item.Assertions)
		require.Equal(t, report.Benchmark.Items[i].Score, item.Score)
		require.Equal(t, report.Benchmark.Items[i].Profile, item.Profile)
		require.Equal(t, report.Benchmark.Items[i].Grader, item.Grader)
		require.NotEmpty(t, report.Benchmark.Items[i].Actual)
		require.NotEmpty(t, report.Benchmark.Items[i].Assertions)
	}
	fingerprints := 0
	for _, sample := range snapshot.Samples {
		if strings.HasPrefix(sample.Probe, "fingerprint_") {
			fingerprints++
			require.NotNil(t, sample.Usage.Input)
			require.NotEmpty(t, sample.RequestProfile)
		}
	}
	require.Zero(t, fingerprints)
	data, err := common.Marshal(snapshot)
	require.NoError(t, err)
	t.Logf("full focused snapshot: %d bytes", len(data))
	require.LessOrEqual(t, len(data), 32<<10)
	require.NotContains(t, string(data), "private-evidence")
	require.NotContains(t, string(data), "private-error")
	require.NotContains(t, string(data), "private-message")

	// Existing fingerprint usage remains available on historical focused reports.
	// Old compact snapshots are not implicitly backfilled by SnapshotBaseline.
	valid, input := true, int64(30)
	report.Samples = append(report.Samples, Sample{Probe: "fingerprint_letter_01", Status: 200, Valid: &valid,
		Usage: Usage{Input: &input}, RequestProfile: "historical-profile"})
	report.Version = 16
	report.Options.Fingerprint, report.Options.Bedrock = true, true
	historical := SnapshotBaseline(report, "historical-reference", "ccmax", 123)
	require.Len(t, historical.Samples, 37)
	require.True(t, historical.Options.Fingerprint)
	require.True(t, historical.Options.Bedrock)
	report.Version = 13
	require.Len(t, SnapshotBaseline(report, "old-reference", "ccmax", 123).Samples, 36)
	report.Version, report.Options.Suite = 14, ""
	require.Len(t, SnapshotBaseline(report, "other-suite", "ccmax", 123).Samples, 36)
}

func TestRestoreBaselineFingerprintUsageBoundedAndExactSource(t *testing.T) {
	valid, oldInput, newInput := true, int64(10), int64(20)
	snapshot := ComparisonBaseline{ID: "baseline", ReportID: "source", Version: 7, Model: "claude-opus-5",
		Samples: []Sample{{Probe: "fingerprint_existing", Status: 200, Valid: &valid, Usage: Usage{Input: &oldInput}}}}
	source := Report{ID: "source", Version: 7, Model: "global.anthropic.claude-opus-5", Samples: []Sample{
		{Probe: "fingerprint_existing", Status: 200, Valid: &valid, Usage: Usage{Input: &newInput}},
		{Probe: "fingerprint_restored", Status: 200, Valid: &valid, Usage: Usage{Input: &newInput}, RequestProfile: "profile", Error: "private-error"},
		{Probe: "prompt_audit_minimal_r1", Status: 200, Valid: &valid, Usage: Usage{Input: &newInput}},
	}}
	restored := RestoreBaselineFingerprintUsage(snapshot, source, 32<<10)
	require.Len(t, restored.Samples, 2)
	require.Equal(t, oldInput, *restored.Samples[0].Usage.Input)
	require.Equal(t, newInput, *restored.Samples[1].Usage.Input)
	require.Equal(t, "profile", restored.Samples[1].RequestProfile)
	require.Empty(t, restored.Samples[1].Error)
	require.Len(t, snapshot.Samples, 1)
	require.Equal(t, snapshot, RestoreBaselineFingerprintUsage(snapshot, source, 1))
	for _, kind := range []string{"id", "version", "model"} {
		bad := source
		switch kind {
		case "id":
			bad.ID = "another-source"
		case "version":
			bad.Version = 8
		case "model":
			bad.Model = "claude-sonnet-5"
		}
		require.Equal(t, snapshot, RestoreBaselineFingerprintUsage(snapshot, bad, 32<<10))
	}
}
