package openaicheck

// Stage names group checks in the report. Each check is annotated in the report
// with the open-source suite that inspired it (see Describe).
const (
	StageDiscovery  = "discovery"
	StageChatBasic  = "chat_basic"
	StageChatInput  = "chat_inputs"
	StageChatStream = "chat_stream"
	StageChatTools  = "chat_tools"
	StageChatFormat = "chat_structured"
	StageChatVision = "chat_vision"
	StageResponses  = "responses"
	StageProtocol   = "protocol"
	StageReliable   = "reliability"
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
	add(StageDiscovery, "observation", true, "models_list")
	add(StageChatBasic, "assertion", true, "chat_basic")
	add(StageChatBasic, "assertion", std, "chat_system")
	add(StageChatInput, "assertion", std, "chat_max_tokens", "chat_stop", "chat_multi_turn", "chat_params")
	add(StageChatInput, "observation", full, "chat_n")
	add(StageChatInput, "observation", options.Logprobs, "chat_logprobs")
	add(StageChatStream, "assertion", true, "chat_stream", "chat_stream_usage")
	add(StageChatTools, "assertion", std, "chat_tool_call", "chat_tool_choice", "chat_tool_roundtrip")
	add(StageChatTools, "assertion", full, "chat_tool_choice_none", "chat_tool_stream")
	add(StageChatTools, "observation", full, "chat_parallel_tools")
	add(StageChatFormat, "assertion", std, "chat_json_mode", "chat_json_schema")
	add(StageChatVision, "assertion", options.Vision, "chat_vision")
	add(StageResponses, "assertion", options.Responses, "responses_basic", "responses_max_output", "responses_stream", "responses_tool_call", "responses_tool_stream", "responses_tool_roundtrip", "responses_structured")
	add(StageResponses, "assertion", options.Responses && full, "responses_previous_id")
	add(StageProtocol, "assertion", true, "error_shape", "usage_fields")
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
		case "chat_stream_usage", "usage_fields", "model_consistency", "performance":
			// derived from earlier samples
		case "chat_tool_roundtrip", "responses_tool_roundtrip", "responses_previous_id":
			total += 2
		default:
			total++
		}
	}
	return total
}
