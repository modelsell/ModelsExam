package geminicheck

// Stage names group checks in the report. They are distinct from the OpenAI
// stage names so one report view can label both.
const (
	StageDiscovery = "discovery"
	StageBasic     = "gemini_basic"
	StageInputs    = "gemini_inputs"
	StageStream    = "gemini_stream"
	StageTools     = "gemini_tools"
	StageFormat    = "gemini_structured"
	StageVision    = "gemini_vision"
	StageProtocol  = "protocol"
	StageReliable  = "reliability"
)

// Plan is persisted with the report so later releases cannot retroactively add
// unexecuted checks to historical reports.
func Plan(options Options) []PlanItem {
	options = options.normalized()
	var plan []PlanItem
	add := func(stage, kind string, selected bool, ids ...string) {
		for _, id := range ids {
			plan = append(plan, PlanItem{ID: id, Stage: stage, Kind: kind, Selected: selected})
		}
	}
	std, full := options.standard(), options.full()
	add(StageDiscovery, "observation", true, "gemini_model_get")
	add(StageBasic, "assertion", true, "gemini_basic")
	add(StageBasic, "assertion", std, "gemini_system")
	add(StageInputs, "assertion", std, "gemini_max_tokens", "gemini_stop", "gemini_multi_turn", "gemini_params")
	add(StageInputs, "observation", full, "gemini_candidates", "gemini_thinking")
	add(StageStream, "assertion", true, "gemini_stream", "gemini_stream_usage")
	add(StageTools, "assertion", std, "gemini_tool_call", "gemini_tool_choice", "gemini_tool_roundtrip")
	add(StageTools, "assertion", full, "gemini_tool_choice_none", "gemini_tool_stream")
	add(StageTools, "observation", full, "gemini_parallel_tools")
	add(StageFormat, "assertion", std, "gemini_json_mode", "gemini_json_schema")
	add(StageVision, "assertion", options.Vision, "gemini_vision")
	add(StageProtocol, "assertion", std, "gemini_count_tokens")
	add(StageProtocol, "assertion", true, "gemini_error_shape", "gemini_usage")
	add(StageReliable, "observation", true, "model_consistency", "performance")
	return plan
}

// MaxRequests is the exact upper bound of upstream requests for options.
func MaxRequests(options Options) int {
	total := 0
	for _, item := range Plan(options) {
		if !item.Selected {
			continue
		}
		switch item.ID {
		case "gemini_stream_usage", "gemini_usage", "model_consistency", "performance":
			// derived from earlier samples
		case "gemini_tool_roundtrip":
			total += 2
		default:
			total++
		}
	}
	return total
}
