package openaicheck

import (
	"fmt"
	"sort"
	"strings"
)

// Spec documents one check: what it sends, what it validates, and which public
// project the idea comes from.
type Spec struct {
	Title  string // short Chinese title
	Input  string // request parameters exercised
	Output string // response fields validated
	Source string // open-source suite the check is modeled on
}

const (
	srcGPTOSS = "openai/gpt-oss compatibility-test"
	srcTester = "avelrl/openai-compatible-tester"
	srcAPI7   = "API7 AI 网关契约测试清单"
	srcLHC    = "LiveHelperChat/openai-test-api"
	srcSelf   = "本项目自研"
)

var specs = map[string]Spec{
	"models_list":              {"模型列表", "GET /v1/models", "object=list, data[].id/object, 目标模型是否在列", srcTester + " (GET /models)"},
	"chat_basic":               {"Chat 基础响应", "model, messages", "object/id/created/model, choices[].index/message.role/finish_reason, usage 三字段及加和", srcTester + " / " + srcGPTOSS},
	"chat_system":              {"system 指令遵循", "messages[role=system]", "输出遵循 system 角色指令", srcSelf},
	"chat_max_tokens":          {"token 上限", "max_completion_tokens / max_tokens", "finish_reason=length, completion_tokens ≤ 上限", srcAPI7},
	"chat_stop":                {"stop 序列", "stop", "输出不含 stop 序列及其后内容", srcSelf},
	"chat_multi_turn":          {"多轮历史", "messages 多轮 user/assistant", "能引用前文随机口令", srcTester},
	"chat_params":              {"采样参数入参", "n, seed, user, temperature, top_p, presence/frequency_penalty (推理模型为 reasoning_effort)", "请求被接受 (无 4xx)", srcAPI7},
	"chat_n":                   {"n 多候选", "n=2", "choices 数量=2", srcSelf},
	"chat_logprobs":            {"logprobs", "logprobs, top_logprobs", "choices[0].logprobs.content[].token/logprob/top_logprobs", srcSelf},
	"chat_stream":              {"流式 SSE", "stream=true", "chat.completion.chunk 帧、首帧 role、finish_reason 唯一、[DONE] 终止、id 一致", srcTester + " / " + srcGPTOSS + " (--streaming)"},
	"chat_stream_usage":        {"流式 usage 帧", "stream_options.include_usage", "末帧 choices=[] 且 usage 字段完整", srcTester + " (usage 校验)"},
	"chat_tool_call":           {"工具调用", "tools, tool_choice=auto, parallel_tool_calls=false", "tool_calls[].id/type/function.name/arguments(JSON 精确匹配), finish_reason=tool_calls", srcGPTOSS + " / " + srcLHC},
	"chat_tool_choice":         {"强制工具", "tool_choice={type:function,function.name}", "必须调用指定函数且参数含 city", srcTester + " (tool choice 模式)"},
	"chat_tool_choice_none":    {"禁用工具", "tool_choice=none", "不产生 tool_calls", srcTester + " (tool choice 模式)"},
	"chat_tool_roundtrip":      {"工具往返", "assistant.tool_calls + role=tool/tool_call_id", "第二轮基于工具结果给出精确 JSON", srcTester + " (两步工具流) / " + srcGPTOSS},
	"chat_parallel_tools":      {"并行工具", "parallel_tool_calls=true", "同轮返回 ≥2 个独立 id 的调用", srcTester + " (parallel tool calls)"},
	"chat_tool_stream":         {"流式工具调用", "stream=true + tools", "按 index 聚合 delta.tool_calls, 参数 JSON 精确匹配", srcGPTOSS + " (--streaming)"},
	"chat_json_mode":           {"JSON 模式", "response_format=json_object", "整段输出为合法 JSON 对象", srcSelf},
	"chat_json_schema":         {"结构化输出", "response_format=json_schema(strict)", "字段、类型与值精确匹配, 无 refusal", srcTester + " (structured json_schema)"},
	"chat_vision":              {"图片输入", "content[type=image_url] (data URL)", "能识别纯红图片", srcSelf},
	"responses_basic":          {"Responses 基础", "input, instructions, max_output_tokens, store", "object=response, status=completed, output[].message.output_text, usage 三字段", srcTester + " (responses.basic)"},
	"responses_max_output":     {"max_output_tokens", "max_output_tokens=16", "status=incomplete, incomplete_details.reason=max_output_tokens", srcSelf},
	"responses_stream":         {"Responses 流式", "stream=true", "response.created → output_text.delta → response.completed, sequence_number 递增", srcTester + " (responses.stream)"},
	"responses_tool_call":      {"Responses 函数调用", "tools(flat function), tool_choice, parallel_tool_calls", "output[type=function_call].call_id/name/arguments", srcTester + " (responses.tool_call) / " + srcGPTOSS},
	"responses_tool_stream":    {"Responses 流式函数调用 item_id", "stream=true + tools", "output_item.added 含 id/call_id/name; 每个 function_call_arguments.delta 与 done 都带 item_id 且指向已添加的 item, output_index 一致, done.arguments=各 delta 拼接且为合法 JSON, item_id 与最终 response 一致", srcTester + " (responses.tool_call, streaming) / " + srcGPTOSS + " (--streaming)"},
	"responses_tool_roundtrip": {"Responses 工具往返", "input + function_call_output", "第二轮基于结果给出精确 JSON", srcTester + " (second turn after tool execution)"},
	"responses_structured":     {"Responses 结构化", "text.format=json_schema(strict)", "字段、类型与值精确匹配", srcTester + " (responses.structured.json_schema)"},
	"responses_previous_id":    {"previous_response_id", "store=true, previous_response_id", "第二轮能引用第一轮口令", srcTester + " (responses.memory.prev_id)"},
	"error_shape":              {"错误响应形状", "缺少 messages 的非法请求", "4xx 且 error.message/type/code/param 键齐全", srcTester + " (error shape) / " + srcAPI7},
	"usage_fields":             {"usage 一致性", "全部成功样本", "input/output/total 齐全、非负、total=input+output", srcAPI7 + " (usage)"},
	"model_consistency":        {"模型回显一致", "全部成功样本", "响应 model 与请求一致 (允许日期快照后缀), system_fingerprint 记录", srcSelf},
	"performance":              {"延迟观测", "全部成功样本", "中位/最大耗时, 流式首帧耗时", srcAPI7 + " (timeouts)"},
}

// Describe returns the documentation for a check ID.
func Describe(id string) Spec { return specs[id] }

var stageTitles = []struct{ id, title string }{
	{StageDiscovery, "模型发现"},
	{StageChatBasic, "Chat Completions 基础"},
	{StageChatInput, "Chat 入参能力"},
	{StageChatStream, "Chat 流式"},
	{StageChatTools, "Chat 工具调用"},
	{StageChatFormat, "Chat 结构化输出"},
	{StageChatVision, "Chat 多模态"},
	{StageResponses, "Responses API"},
	{StageProtocol, "协议与计量"},
	{StageReliable, "可靠性观测"},
}

var statusLabel = map[string]string{"pass": "✅ 通过", "fail": "❌ 失败", "inconclusive": "⚠️ 无法判定", "skipped": "⏭ 跳过"}

var codeNotes = map[string]string{
	"unauthorized": "凭证无效 (401)", "forbidden": "无权限 (403)", "rate_limited": "被限流 (429)",
	"upstream_error": "上游 5xx", "timeout": "请求超时", "network_error": "网络错误",
	"request_rejected": "端点拒绝了 OpenAI 允许的入参 (4xx)", "invalid_response": "响应不符合 OpenAI 契约",
	"not_requested": "未选择该检测", "run_stopped": "基线不可用, 已停止", "baseline_failed": "Chat 基线请求被拒绝, 跳过后续 Chat 检测", "cancelled": "已取消",
	"model_not_listed": "目标模型未出现在 /v1/models (中转站常隐藏或别名)", "model_mismatch": "响应 model 与请求不一致 (可能是中转映射)",
	"unsupported_model_family": "该模型系列不支持", "dependency_unavailable": "依赖的前置请求不可用",
	"single_tool_call": "只返回了一个工具调用", "limit_not_enforced": "token 上限未被遵守",
	"missing_usage_chunk": "流式未返回 usage 帧", "streamed_function_call_invalid": "流式函数调用事件不合规 (见 issues, 如 tool_delta_missing_item_id)", "usage_missing": "部分响应缺少 usage", "usage_inconsistent": "usage 数值不自洽",
}

// Markdown renders a human-readable report. It states what was observed and, at
// the end, what the observations cannot prove.
func Markdown(report Report) string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	w("# OpenAI 模型检测报告\n\n")
	w("- **模型**：`%s`\n", report.Model)
	if report.Endpoint != "" {
		w("- **端点**：%s\n", report.Endpoint)
	}
	suite := ""
	if report.Options != nil {
		suite = report.Options.Suite
		if report.Options.Responses {
			suite += " + responses"
		}
		if report.Options.Vision {
			suite += " + vision"
		}
		if report.Options.Logprobs {
			suite += " + logprobs"
		}
	}
	w("- **套件**：%s\n- **开始时间**：%s　**耗时**：%.1fs　**上游请求数**：%d\n", suite, report.StartedAt, float64(report.DurationMS)/1000, report.RequestsRun)
	if report.Score != nil {
		w("- **兼容性得分**：**%d / 100**（已判定断言中通过的比例；观测项与无法判定项不计分）\n", *report.Score)
	} else {
		w("- **兼容性得分**：无（没有任何断言得出明确结论）\n")
	}
	w("- **结果汇总**：通过 %d · 失败 %d · 无法判定 %d · 跳过 %d\n", report.Summary["pass"], report.Summary["fail"], report.Summary["inconclusive"], report.Summary["skipped"])
	if report.StopReason != "" {
		w("- **提前结束**：`%s`（探测：`%s`）\n", report.StopReason, report.StopProbe)
	}
	w("\n")

	byStage := map[string][]Check{}
	for _, c := range report.Checks {
		byStage[c.Stage] = append(byStage[c.Stage], c)
	}
	for _, stage := range stageTitles {
		checks := byStage[stage.id]
		if len(checks) == 0 {
			continue
		}
		w("## %s\n\n| 检测项 | 类型 | 结果 | 代码 | 入参 → 出参校验 |\n|---|---|---|---|---|\n", stage.title)
		for _, c := range checks {
			spec := specs[c.ID]
			title := spec.Title
			if title == "" {
				title = c.ID
			}
			kind := "断言"
			if c.Kind == "observation" {
				kind = "观测"
			}
			detail := ""
			if spec.Input != "" {
				detail = fmt.Sprintf("`%s` → %s", cell(spec.Input), cell(spec.Output))
			}
			code := c.Code
			if note := codeNotes[c.Code]; note != "" && c.Status != "pass" {
				code += "（" + note + "）"
			}
			// Surface the concrete contract violations, not just the generic code.
			if issues, ok := c.Evidence["issues"].([]string); ok && len(issues) > 0 && c.Status != "pass" {
				code += " [" + strings.Join(issues, ", ") + "]"
			}
			w("| %s `%s` | %s | %s | %s | %s |\n", title, c.ID, kind, statusLabel[c.Status], cell(code), detail)
		}
		w("\n")
	}

	var findings []Check
	for _, c := range report.Checks {
		if c.Status == "fail" || (c.Status == "inconclusive" && len(c.Evidence) > 0 && c.Kind == "assertion") {
			findings = append(findings, c)
		}
	}
	if len(findings) > 0 {
		w("## 需要关注的发现\n\n")
		for _, c := range findings {
			title := specs[c.ID].Title
			w("- **%s** `%s` → `%s`", title, c.ID, c.Code)
			if ev := evidenceLine(c.Evidence); ev != "" {
				w("：%s", ev)
			}
			w("\n")
		}
		w("\n")
	}

	w("## 请求明细\n\n| 探测 | 方法 路径 | HTTP | 耗时(ms) | 首帧(ms) | 结束 | 输入/输出 token | 响应模型 |\n|---|---|---|---|---|---|---|---|\n")
	for _, s := range report.Samples {
		first, tokens := "-", "-"
		if s.FirstEventMS != nil {
			first = fmt.Sprint(*s.FirstEventMS)
		}
		if s.Usage.Input != nil && s.Usage.Output != nil {
			tokens = fmt.Sprintf("%d / %d", *s.Usage.Input, *s.Usage.Output)
		}
		w("| `%s` | %s %s | %d | %d | %s | %s | %s | %s |\n", s.Probe, s.Method, s.Path, s.Status, s.DurationMS, first, cell(s.FinishReason), tokens, cell(s.ResponseModel))
	}

	sources := map[string]bool{}
	for _, c := range report.Checks {
		if spec, ok := specs[c.ID]; ok {
			sources[spec.Source] = true
		}
	}
	names := make([]string, 0, len(sources))
	for s := range sources {
		names = append(names, s)
	}
	sort.Strings(names)
	w("\n## 检测依据\n\n检测项参考了公开的 OpenAI 兼容性测试项目的思路（逐项来源见各检测项定义）：\n\n")
	w("- openai/gpt-oss `compatibility-test`：Responses / Chat Completions 的工具调用与 API 形状冒烟测试（含 `--streaming`）\n")
	w("- avelrl/openai-compatible-tester：compat / strict 双层判定；SSE、两步工具调用、结构化输出、usage 与错误形状\n")
	w("- LiveHelperChat/openai-test-api：函数调用用例与 token 统计\n")
	w("- API7《OpenAI-Compatible AI Gateway: Test the Contract》：鉴权、发现、字段、流、工具、usage、错误、超时的契约矩阵\n\n")

	w("## 局限\n\n")
	w("- 本报告衡量的是**接口兼容性**：端点是否接受 OpenAI 的入参、返回是否符合 OpenAI 的出参契约。\n")
	w("- 它**不能证明**上游就是名称所示的 OpenAI 模型。响应里的 `model`、`system_fingerprint`、`usage` 都是端点自报的观测值，中转站可以伪造或映射。\n")
	w("- 「无法判定」表示凭证、限流、网络或上游故障挡住了该项，不代表模型不兼容；请在恢复后重测。\n")
	w("- 一次通过只说明本次样本通过；模型输出有随机性，关键结论建议重复检测。\n")
	return b.String()
}

func cell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.ReplaceAll(s, "\n", " ")
}

func evidenceLine(evidence map[string]any) string {
	if len(evidence) == 0 {
		return ""
	}
	keys := make([]string, 0, len(evidence))
	for k := range evidence {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, evidence[k]))
	}
	return truncate(strings.Join(parts, "，"), 300)
}
