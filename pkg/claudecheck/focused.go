package claudecheck

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
)

func ValidOptions(o Options) bool {
	if o.Suite != "" && o.Suite != "focused" {
		return false
	}
	if o.PerformanceTolerance != nil {
		v := *o.PerformanceTolerance
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 100 {
			return false
		}
	}
	return true
}

func focusedPlan(o Options) []PlanItem {
	plan := []PlanItem{{"basic", "identity", "assertion", true}, {"model_consistency", "identity", "observation", true},
		{"performance_sampling", "performance", "boundary", true}, {"usage_token_integrity", "integrity", "boundary", true}, {"capability_benchmark", "identity", "boundary", true}}
	if o.Cache {
		plan = append(plan, PlanItem{"cache_token_audit", "integrity", "boundary", true})
	}
	if o.PromptAudit {
		plan = append(plan, PlanItem{"prompt_integrity", "integrity", "boundary", true})
	}
	if o.Vision {
		plan = append(plan, PlanItem{"vision", "identity", "assertion", true})
	}
	if o.PDF {
		plan = append(plan, PlanItem{"pdf", "identity", "assertion", true})
	}
	if o.Bedrock {
		plan = append(plan, bedrockPlan()...)
	}
	return plan
}

func runFocused(ctx context.Context, options Options, call Transport, observe func(Event)) Report {
	ctx, cancel := context.WithTimeout(ctx, RunTimeout)
	defer cancel()
	start := time.Now()
	r := &runner{ctx: ctx, call: call, observe: observe, report: Report{Version: 20, ID: uuid.NewString(), Model: options.Model,
		StartedAt: start.UTC().Format(time.RFC3339), Options: &options, Plan: focusedPlan(options), Checks: []Check{}, Samples: []Sample{}, Summary: map[string]int{},
		Limits: &RunLimits{int(ProbeTimeout / time.Second), int(RunTimeout / time.Second), MaxRequests(options)}}}
	r.report.Benchmark = newCapabilityBenchmark(options.Model)
	if options.Cache || options.PromptAudit {
		r.report.TokenAudit = &TokenAuditReport{Version: 7, Prompt: []TokenComparison{}, Cache: []TokenComparison{}}
		if options.Cache {
			for _, id := range repeatedCacheReadIDs() {
				r.report.TokenAudit.Cache = append(r.report.TokenAudit.Cache, TokenComparison{ID: id, Code: "pending"})
			}
		}
		if options.PromptAudit {
			for _, f := range simplePromptFixtures(options.Model) {
				r.report.TokenAudit.Prompt = append(r.report.TokenAudit.Prompt, TokenComparison{ID: f.id, Code: "pending"})
			}
		}
	}
	if r.report.TokenAudit != nil {
		r.report.TokenAudit.assessPrompt()
		r.assessPromptInjection()
	}
	r.emit(Event{Type: "start", Report: &r.report})
	body := r.body("Reply with exactly PONG.")
	body["system"] = "Respond with exactly PONG and no other text."
	_, response, ok := r.probe("basic", Request{Body: body})
	r.upstreamModel = response.Model
	r.baselineResult("basic", ok, response)
	r.report.BaselineProbe = "basic"
	if !ok && baselineRecoverable(response.ErrorCode) && response.ErrorCode != "rate_limited" && ctx.Err() == nil {
		// Reuse the first planned performance request as the recovery path.
		_, response, ok = r.probe("performance_1", Request{Body: performanceBody(options.Model)})
		r.upstreamModel, r.report.BaselineProbe = response.Model, "performance_1"
	}
	if ok {
		steps := []struct {
			enabled bool
			run     func()
		}{{true, r.performance}, {true, r.usageTokens}, {true, r.benchmark},
			{options.Cache, r.cache}, {options.PromptAudit, r.promptTokenAudit}, {options.Vision, r.vision}, {options.PDF, r.document}, {options.Bedrock, r.bedrock}}
		for _, step := range steps {
			if ctx.Err() != nil {
				break
			}
			if r.focusedStop() {
				break
			}
			if step.enabled {
				step.run()
			}
		}
		r.focusedStop()
	} else {
		r.report.StopReason = response.ErrorCode
		if r.report.StopReason == "" {
			r.report.StopReason = "baseline_unavailable"
		}
	}
	r.modelConsistency()
	if ctx.Err() != nil {
		r.report.Cancelled = true
		r.report.StopReason = "cancelled"
		if ctx.Err() == context.DeadlineExceeded {
			r.report.StopReason = "run_timeout"
		}
	}
	for _, item := range r.report.Plan {
		if !r.hasCheck(item.ID) {
			r.check(item.ID, "skipped", "run_stopped", nil)
		}
	}
	if r.report.StopReason != "" && len(r.report.Samples) > 0 {
		r.report.StopProbe = r.report.Samples[len(r.report.Samples)-1].Probe
	}
	if r.report.TokenAudit != nil {
		for _, items := range [][]TokenComparison{r.report.TokenAudit.Prompt, r.report.TokenAudit.Cache} {
			for i := range items {
				if items[i].Code == "pending" {
					items[i].Code = "not_collected"
				}
			}
		}
	}
	if r.report.TokenAudit != nil {
		r.emitTokenAudit()
	}
	r.report.DurationMS = time.Since(start).Milliseconds()
	finishCapability(r.report.Benchmark)
	r.report.Summary = map[string]int{"pass": 0, "fail": 0, "inconclusive": 0, "skipped": 0}
	for _, check := range r.report.Checks {
		r.report.Summary[check.Status]++
	}
	r.emit(Event{Type: "done", Report: &r.report})
	return r.report
}

// Authentication and rate limits apply to the whole target. Keep accumulated
// evidence and do not start another collection suite against the same rejection.
func (r *runner) focusedStop() bool {
	for _, sample := range r.report.Samples {
		// CountTokens permission is separate from permission to invoke a model.
		if r.report.TokenAudit != nil && r.report.TokenAudit.Version >= 2 && IsCountProbe(sample.Probe) && sample.Status == 403 {
			continue
		}
		if sample.ErrorCode == "access_denied" || sample.ErrorCode == "rate_limited" {
			r.report.StopReason, r.report.StopProbe = sample.ErrorCode, sample.Probe
			return true
		}
	}
	return false
}
