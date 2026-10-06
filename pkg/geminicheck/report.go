package geminicheck

import (
	"fmt"
	"sort"
	"strings"
)

// Spec documents one check: what it sends and what it validates.
type Spec struct {
	Title  string
	Input  string
	Output string
}

var specs = map[string]Spec{
	"gemini_model_get":        {"模型信息", "GET /v1beta/models/{model}", "name=models/{model}, version, inputTokenLimit/outputTokenLimit, supportedGenerationMethods 含 generateContent"},
	"gemini_basic":            {"generateContent 基础响应", "contents[role=user].parts[].text", "candidates[].content.role=model/parts, finishReason=STOP, usageMetadata, modelVersion, responseId"},
	"gemini_system":           {"systemInstruction 遵循", "systemInstruction.parts", "输出遵循系统指令"},
	"gemini_max_tokens":       {"maxOutputTokens", "generationConfig.maxOutputTokens=16", "finishReason=MAX_TOKENS, candidatesTokenCount ≤ 上限"},
	"gemini_stop":             {"stopSequences", "generationConfig.stopSequences", "输出不含停止序列及其后内容, finishReason=STOP"},
	"gemini_multi_turn":       {"多轮历史", "contents 多轮 user/model", "能引用前文随机口令"},
	"gemini_params":           {"采样参数入参", "temperature, topP, topK, candidateCount, seed, responseMimeType, safetySettings", "请求被接受 (无 4xx)"},
	"gemini_candidates":       {"多候选", "candidateCount=2", "candidates 数量=2 且 index 连续"},
	"gemini_thinking":         {"思考输出", "thinkingConfig.includeThoughts=true", "parts[thought=true] 或 thoughtsTokenCount>0, 答案正确"},
	"gemini_stream":           {"流式 SSE", "streamGenerateContent?alt=sse", "每帧为完整 GenerateContentResponse, finishReason 只在末帧出现一次, responseId 一致, 无 [DONE]"},
	"gemini_stream_usage":     {"流式 usageMetadata", "streamGenerateContent?alt=sse", "末帧携带完整 usageMetadata"},
	"gemini_tool_call":        {"函数调用", "tools[].functionDeclarations, toolConfig.mode=AUTO", "parts[].functionCall.name/args(对象, 精确匹配), finishReason=STOP"},
	"gemini_tool_choice":      {"强制函数", "mode=ANY, allowedFunctionNames", "必须调用指定函数且参数含 city"},
	"gemini_tool_choice_none": {"禁用函数", "mode=NONE", "不产生 functionCall, 返回文本"},
	"gemini_tool_roundtrip":   {"函数往返", "原样回放 model 轮 (含 thoughtSignature) + functionResponse", "第二轮基于结果给出精确 JSON"},
	"gemini_parallel_tools":   {"并行函数调用", "一轮请求两个城市", "同轮返回 ≥2 个 functionCall"},
	"gemini_tool_stream":      {"流式函数调用", "alt=sse + tools", "functionCall 整体出现在某一帧, args 精确匹配"},
	"gemini_json_mode":        {"JSON 输出", "responseMimeType=application/json", "整段输出为合法 JSON 对象"},
	"gemini_json_schema":      {"结构化输出", "responseMimeType + responseSchema", "字段、类型与值精确匹配"},
	"gemini_vision":           {"图片输入", "parts[].inlineData(image/png base64)", "能识别纯红图片"},
	"gemini_count_tokens":     {"countTokens", "POST :countTokens (与基础探测相同 contents)", "totalTokens 为正整数且等于 promptTokenCount"},
	"gemini_error_shape":      {"错误响应形状", "contents 为空的非法请求", "4xx, error.code=HTTP 状态, error.message, error.status 为对应的 google.rpc 状态名"},
	"gemini_usage":            {"usageMetadata 一致性", "全部成功样本", "promptTokenCount/totalTokenCount 齐全且非负, total = prompt + candidates + thoughts + toolUsePrompt"},
	"model_consistency":       {"模型回显一致", "全部成功样本", "modelVersion 与请求一致 (允许版本/日期/preview 后缀)"},
	"performance":             {"延迟观测", "全部成功样本", "中位/最大耗时, 流式首帧耗时"},
}

// Describe returns the documentation for a check ID.
func Describe(id string) Spec { return specs[id] }

var stageTitles = []struct{ id, title string }{
	{StageDiscovery, "模型发现"},
	{StageBasic, "generateContent 基础"},
	{StageInputs, "入参能力"},
	{StageStream, "流式"},
	{StageTools, "函数调用"},
	{StageFormat, "结构化输出"},
	{StageVision, "多模态"},
	{StageProtocol, "协议与计量"},
	{StageReliable, "可靠性观测"},
}

var statusLabel = map[string]string{"pass": "✅ 通过", "fail": "❌ 失败", "inconclusive": "⚠️ 无法判定", "skipped": "⏭ 跳过"}

var codeNotes = map[string]string{
	"unauthorized": "凭证无效", "forbidden": "无权限或余额不足", "rate_limited": "被限流 (429)",
	"upstream_error": "上游 5xx", "timeout": "请求超时", "network_error": "网络错误",
	"request_rejected": "端点拒绝了 Gemini 允许的入参 (4xx)", "invalid_response": "响应不符合 Gemini 契约",
	"not_requested": "未选择该检测", "run_stopped": "基线不可用, 已停止", "baseline_failed": "基础请求被拒绝, 跳过后续检测", "cancelled": "已取消",
	"model_mismatch": "modelVersion 与请求不一致 (可能是中转映射)", "dependency_unavailable": "依赖的前置请求不可用",
	"usage_missing": "部分响应缺少 usageMetadata", "usage_inconsistent": "usageMetadata 数值不自洽",
	"count_differs_from_usage": "countTokens 与 promptTokenCount 不一致", "error_shape_mismatch": "错误体不是 google.rpc.Status 形状",
}

// Markdown renders a human-readable report.
func Markdown(report Report) string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	w("# Gemini 原生协议检测报告\n\n")
	w("- **模型**：`%s`\n", report.Model)
	if report.Endpoint != "" {
		w("- **端点**：%s\n", report.Endpoint)
	}
	suite := ""
	if report.Options != nil {
		suite = report.Options.Suite
		if report.Options.Vision {
			suite += " + vision"
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
			w("- **%s** `%s` → `%s`", specs[c.ID].Title, c.ID, c.Code)
			if ev := evidenceLine(c.Evidence); ev != "" {
				w("：%s", ev)
			}
			w("\n")
		}
		w("\n")
	}

	w("## 请求明细\n\n| 探测 | 方法 路径 | HTTP | 耗时(ms) | 首帧(ms) | 结束 | 输入/输出 token | modelVersion |\n|---|---|---|---|---|---|---|---|\n")
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

	w("\n## 检测依据\n\n")
	w("- Gemini API 参考：`models.generateContent`、`models.streamGenerateContent`、`models.countTokens`、`models.get`（ai.google.dev/api）\n")
	w("- `finishReason` 只校验 proto 枚举写法（大写下划线），不做白名单：官方枚举持续新增取值\n")
	w("- 错误体按 google.rpc.Status：`error.code` 与 HTTP 状态一致，`error.status` 为对应的规范状态名\n\n")
	w("## 局限\n\n")
	w("- 本报告衡量的是**接口兼容性**：端点是否接受 Gemini 原生入参、返回是否符合 Gemini 原生出参契约。\n")
	w("- 它**不能证明**上游就是名称所示的 Google 模型。`modelVersion`、`responseId`、`usageMetadata` 都是端点自报的观测值。\n")
	w("- 「无法判定」表示凭证、限流、网络或上游故障挡住了该项；请在恢复后重测。\n")
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
