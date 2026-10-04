package claudecheck

// The plan is persisted with the report so later releases cannot retroactively
// add unexecuted checks to historical reports.
type PlanItem struct {
	ID       string `json:"id"`
	Stage    string `json:"stage"`
	Kind     string `json:"kind"`
	Selected bool   `json:"selected"`
}

func Plan(options Options) []PlanItem {
	options = options.normalized()
	if options.Suite == "focused" {
		return focusedPlan(options)
	}
	var plan []PlanItem
	add := func(stage, kind string, selected bool, ids ...string) {
		for _, id := range ids {
			plan = append(plan, PlanItem{ID: id, Stage: stage, Kind: kind, Selected: selected})
		}
	}
	add("connection", "assertion", true, "basic")
	add("connection", "observation", true, "model_echo", "source", "token_count", "cold_cache")
	add("protocol", "assertion", true, "system", "stream", "tool", "max_tokens", "stop_sequence", "multi_turn", "zero_output")
	add("protocol", "observation", true, "error_shape")
	add("capabilities", "assertion", options.Vision, "vision")
	add("capabilities", "assertion", options.PDF, "pdf")
	add("capabilities", "assertion", options.Thinking, "thinking", "thinking_stream", "signature_replay")
	add("capabilities", "observation", options.Cache, "cache")
	add("reliability", "boundary", options.Cache, "cache_token_audit")
	add("reliability", "observation", options.Repeat, "repeatability")
	add("reliability", "assertion", true, "usage_fields", "aws_usage", "model_consistency")
	add("reliability", "assertion", options.StreamComparison, "stream_comparison", "stream_stop_reason")
	add("boundaries", "boundary", true, "billing", "prompt_integrity")
	if options.Benchmark {
		add("reliability", "boundary", true, "capability_benchmark")
	}
	if options.Performance {
		add("reliability", "boundary", true, "performance_sampling")
	}
	if options.Bedrock {
		plan = append(plan, bedrockPlan()...)
	}
	return plan
}
