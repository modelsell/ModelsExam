package claudecheck

import (
	"strings"

	"model-check/common"
)

// ComparisonModelsMatch accepts only compatible declared model IDs or the same
// explicit alias. The comparison is not evidence of the upstream model identity.
func ComparisonModelsMatch(left, right string) bool {
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	return left != "" && right != "" && (declaredModelRelation(left, right) == 1 || strings.EqualFold(left, right))
}

// ComparisonBaseline is an immutable, operator-selected reference. Its source
// describes the observed route, not an authenticated model identity. It never
// embeds another baseline, so repeated comparisons cannot grow recursively.
type ComparisonBaseline struct {
	TokenAudit  *TokenAuditReport    `json:"token_audit,omitempty"`
	ID          string               `json:"id"`
	ReportID    string               `json:"report_id"`
	Type        string               `json:"type"`
	Model       string               `json:"model"`
	Version     int                  `json:"version"`
	ChannelID   int                  `json:"channel_id"`
	ChannelName string               `json:"channel_name"`
	Transport   string               `json:"transport"`
	Endpoint    string               `json:"endpoint,omitempty"`
	StartedAt   string               `json:"started_at"`
	CreatedAt   int64                `json:"created_at"`
	Options     *Options             `json:"options,omitempty"`
	Plan        []PlanItem           `json:"plan,omitempty"`
	Checks      []Check              `json:"checks"`
	Samples     []Sample             `json:"samples"`
	Fingerprint *BehaviorFingerprint `json:"fingerprint,omitempty"`
	Benchmark   *CapabilityBenchmark `json:"benchmark,omitempty"`
}

func SnapshotBaseline(report Report, id, baselineType string, createdAt int64) ComparisonBaseline {
	var options *Options
	if report.Options != nil {
		copy := *report.Options
		copy.CompareBaselines = false
		copy.BaselineID, copy.BaselineType = "", ""
		options = &copy
	}
	checks := make([]Check, 0, len(report.Checks))
	for _, check := range report.Checks {
		checks = append(checks, Check{ID: check.ID, Status: check.Status, Code: check.Code})
	}
	// Usage is useful comparison evidence; full headers, errors and request IDs
	// remain in the linked source report, keeping multiple snapshots bounded.
	keepFingerprintSamples := report.Version >= 14 && report.Options != nil && report.Options.Suite == "focused"
	samples := make([]Sample, 0, len(report.Samples))
	for _, sample := range report.Samples {
		if strings.HasPrefix(sample.Probe, "fingerprint_") && !keepFingerprintSamples {
			continue
		}
		if len(sample.Probe) > 64 {
			continue
		}
		samples = append(samples, baselineUsageSample(sample))
	}
	return ComparisonBaseline{ID: id, Type: baselineType, ReportID: report.ID, Model: report.Model,
		Version: report.Version, ChannelID: report.ChannelID, ChannelName: report.ChannelName,
		Transport: report.Transport, Endpoint: report.Endpoint, StartedAt: report.StartedAt,
		CreatedAt: createdAt, Options: options, Plan: report.Plan, Checks: checks,
		TokenAudit: report.TokenAudit, Samples: samples, Fingerprint: report.Fingerprint, Benchmark: baselineBenchmark(report.Benchmark)}
}

// Answer excerpts and per-assertion details stay in the source report. Baselines
// need only the versioned task, request profile and score for comparison. Clone
// items before trimming so saving a baseline never erases report evidence.
func baselineBenchmark(benchmark *CapabilityBenchmark) *CapabilityBenchmark {
	if benchmark == nil {
		return nil
	}
	copy := *benchmark
	copy.Items = append([]BenchmarkItem(nil), benchmark.Items...)
	for i := range copy.Items {
		copy.Items[i].Actual = ""
		copy.Items[i].Assertions = nil
	}
	return &copy
}

func baselineUsageSample(sample Sample) Sample {
	return Sample{Probe: sample.Probe, Status: sample.Status,
		Valid: sample.Valid, Usage: sample.Usage, RequestProfile: sample.RequestProfile,
		DurationMS: sample.DurationMS, FirstEventMS: sample.FirstEventMS, Stream: sample.Stream}
}

// RestoreBaselineFingerprintUsage only restores the compact measurements that
// historical snapshots intentionally omitted. It never substitutes questions,
// updates existing measurements or changes the stored baseline/source report.
func RestoreBaselineFingerprintUsage(snapshot ComparisonBaseline, source Report, maxBytes int) ComparisonBaseline {
	if source.ID != snapshot.ReportID || source.Version != snapshot.Version || !ComparisonModelsMatch(source.Model, snapshot.Model) || maxBytes <= 0 {
		return snapshot
	}
	seen := make(map[string]bool, len(snapshot.Samples))
	for _, sample := range snapshot.Samples {
		seen[sample.Probe] = true
	}
	copy := snapshot
	copy.Samples = append([]Sample(nil), snapshot.Samples...)
	for _, sample := range source.Samples {
		if seen[sample.Probe] || !strings.HasPrefix(sample.Probe, "fingerprint_") || len(sample.Probe) > 64 {
			continue
		}
		seen[sample.Probe] = true
		copy.Samples = append(copy.Samples, baselineUsageSample(sample))
	}
	if len(copy.Samples) == len(snapshot.Samples) {
		return snapshot
	}
	data, err := common.Marshal(copy)
	if err != nil || len(data) > maxBytes {
		return snapshot
	}
	return copy
}
