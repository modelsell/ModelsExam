# OpenAI 模型检测

在「模型检测」中增加 OpenAI 兼容 API 的检测能力：对一个 Base URL + Key + 模型，逐项验证**入参**（端点是否接受 OpenAI 的参数）与**出参**（返回是否符合 OpenAI 的响应契约），并生成 JSON 与 Markdown 报告。

实现位于 `pkg/openaicheck`（检测引擎，无 gin/GORM 依赖）与 `controller/model_check_openai.go`（薄胶水层）。

## 调研：开源项目检测了什么

| 项目 | 可借鉴的检测点 |
|---|---|
| [openai/gpt-oss `compatibility-test`](https://developers.openai.com/cookbook/articles/gpt-oss/verifying-implementations) | 对 Responses / Chat Completions 发起一组工具调用请求，同时检查「调用了正确的工具」和「API 形状正确」；支持 `--streaming`；定位是冒烟测试，不保证推理精度 |
| [avelrl/openai-compatible-tester](https://github.com/avelrl/openai-compatible-tester) | `compat`（务实互通）与 `strict`（OpenAI 规范）双层判定；`/models`、SSE 流（含 `[DONE]`）、两步工具调用（工具执行后的第二轮）、`tool_choice` 各模式、并行工具、`json_schema` 结构化输出、usage 字段、错误响应形状、`previous_response_id` 记忆流、自定义工具 |
| [LiveHelperChat/openai-test-api](https://github.com/LiveHelperChat/openai-test-api) | 用 `cases.json` + `structure.json` 声明式描述函数调用用例（是否调用了对的函数、参数是否对），并统计 token / 缓存 |
| [API7《Test the Contract》](https://api7.ai/blog/openai-compatible-ai-gateway) | 兼容性是「受支持的接口面」而非「功能等价」；契约矩阵：鉴权、发现、字段、流（事件顺序/结束/usage/取消）、工具（调用→参数→结果→后续）、usage、错误、超时；Chat 通过不代表 Responses/embeddings 通过 |

未采用：`davlgd/openai-api-tester` 只是手工点测的 Web 工具，没有可复用的断言。`openai/gpt-oss` 仓库页面被 robots 限制无法直接读取，其测试内容以 OpenAI 官方 cookbook 文章为准。

## 设计要点

- **流式函数调用 id**：Responses 的 `response.function_call_arguments.delta/done` 必须带 `item_id`，且指向先前 `response.output_item.added` 声明的 function_call item（`output_index` 一致、`done.arguments` 等于各 delta 拼接且是合法 JSON、`item_id` 与最终 response 中的 item 一致）；Chat 流式的首个 `tool_calls` delta 必须带 `id` 与 `name`。

- **断言 vs 观测**：断言有对错（计入得分）；观测只记录（模型列表、模型回显、延迟、并行工具等），永不判失败。
- **失败分层**：凭证/限流/网络/上游 5xx →「无法判定」（说明没测成，不怪模型）；端点拒绝 OpenAI 允许的入参（4xx）或返回不合规 →「失败」，并附上游错误原文。
- **基线门控**：`chat_basic` 遇到 401/403/429/5xx/超时/网络错误即停止，避免对着同一个故障重复请求；HTTP 400 不会停（可能是 Responses-only 模型，或 `limit_param` 选错，报告会给出提示）。
- **模型系列适配**：`o1/o3/o4/gpt-5*` 视为推理模型——不发 `temperature/top_p/penalty`，改发 `reasoning_effort`，token 预算放宽以容纳隐藏推理，跳过 `logprobs`。
- **`limit_param`**：默认 `max_completion_tokens`（当前 OpenAI 名称）；老式兼容服务可选 `max_tokens`。
- **请求从不重试**；单次最长 90s，整轮 10 分钟；每个请求体 ≤128KB，token 上限 ≤4096。
- **安全**：复用 Claude 检测的 `service.NewModelCheckClient / NewUserModelCheckClient`（SSRF 防护、DNS 固定、不跟随重定向）；Key 从报告、进度流、Markdown、错误文本中统一脱敏；每用户同时只能跑一个，全局并发 2。
- **入库**：每次检测写入 `model_check_runs`（`transport = openai_api`，报告 ID 为 UUID），详见下文「历史记录」。API Key 不会入库：每次写入前都先脱敏。

## 检测目录

| 检测项 | 类型 | 套件 | 入参 | 出参校验 | 依据 |
|---|---|---|---|---|---|
| `models_list` 模型列表 | 观测 | basic | GET /v1/models | object=list, data[].id/object, 目标模型是否在列 | avelrl/openai-compatible-tester (GET /models) |
| `chat_basic` Chat 基础响应 | 断言 | basic | model, messages | object/id/created/model, choices[].index/message.role/finish_reason, usage 三字段及加和 | avelrl/openai-compatible-tester / openai/gpt-oss compatibility-test |
| `chat_system` system 指令遵循 | 断言 | standard | messages[role=system] | 输出遵循 system 角色指令 | 本项目自研 |
| `chat_max_tokens` token 上限 | 断言 | standard | max_completion_tokens / max_tokens | finish_reason=length, completion_tokens ≤ 上限 | API7 AI 网关契约测试清单 |
| `chat_stop` stop 序列 | 断言 | standard | stop | 输出不含 stop 序列及其后内容 | 本项目自研 |
| `chat_multi_turn` 多轮历史 | 断言 | standard | messages 多轮 user/assistant | 能引用前文随机口令 | avelrl/openai-compatible-tester |
| `chat_params` 采样参数入参 | 断言 | standard | n, seed, user, temperature, top_p, presence/frequency_penalty (推理模型为 reasoning_effort) | 请求被接受 (无 4xx) | API7 AI 网关契约测试清单 |
| `chat_n` n 多候选 | 观测 | full | n=2 | choices 数量=2 | 本项目自研 |
| `chat_logprobs` logprobs | 观测 | full / `logprobs` | logprobs, top_logprobs | choices[0].logprobs.content[].token/logprob/top_logprobs | 本项目自研 |
| `chat_stream` 流式 SSE | 断言 | basic | stream=true | chat.completion.chunk 帧、首帧 role、finish_reason 唯一、[DONE] 终止、id 一致 | avelrl/openai-compatible-tester / openai/gpt-oss compatibility-test (--streaming) |
| `chat_stream_usage` 流式 usage 帧 | 断言 | basic | stream_options.include_usage | 末帧 choices=[] 且 usage 字段完整 | avelrl/openai-compatible-tester (usage 校验) |
| `chat_tool_call` 工具调用 | 断言 | standard | tools, tool_choice=auto, parallel_tool_calls=false | tool_calls[].id/type/function.name/arguments(JSON 精确匹配), finish_reason=tool_calls | openai/gpt-oss compatibility-test / LiveHelperChat/openai-test-api |
| `chat_tool_choice` 强制工具 | 断言 | standard | tool_choice={type:function,function.name} | 必须调用指定函数且参数含 city | avelrl/openai-compatible-tester (tool choice 模式) |
| `chat_tool_roundtrip` 工具往返 | 断言 | standard | assistant.tool_calls + role=tool/tool_call_id | 第二轮基于工具结果给出精确 JSON | avelrl/openai-compatible-tester (两步工具流) / openai/gpt-oss compatibility-test |
| `chat_tool_choice_none` 禁用工具 | 断言 | full | tool_choice=none | 不产生 tool_calls | avelrl/openai-compatible-tester (tool choice 模式) |
| `chat_tool_stream` 流式工具调用 | 断言 | full | stream=true + tools | 按 index 聚合 delta.tool_calls, 参数 JSON 精确匹配 | openai/gpt-oss compatibility-test (--streaming) |
| `chat_parallel_tools` 并行工具 | 观测 | full | parallel_tool_calls=true | 同轮返回 ≥2 个独立 id 的调用 | avelrl/openai-compatible-tester (parallel tool calls) |
| `chat_json_mode` JSON 模式 | 断言 | standard | response_format=json_object | 整段输出为合法 JSON 对象 | 本项目自研 |
| `chat_json_schema` 结构化输出 | 断言 | standard | response_format=json_schema(strict) | 字段、类型与值精确匹配, 无 refusal | avelrl/openai-compatible-tester (structured json_schema) |
| `chat_vision` 图片输入 | 断言 | full / `vision` | content[type=image_url] (data URL) | 能识别纯红图片 | 本项目自研 |
| `responses_basic` Responses 基础 | 断言 | `responses` | input, instructions, max_output_tokens, store | object=response, status=completed, output[].message.output_text, usage 三字段 | avelrl/openai-compatible-tester (responses.basic) |
| `responses_max_output` max_output_tokens | 断言 | `responses` | max_output_tokens=16 | status=incomplete, incomplete_details.reason=max_output_tokens | 本项目自研 |
| `responses_stream` Responses 流式 | 断言 | `responses` | stream=true | response.created → output_text.delta → response.completed, sequence_number 递增 | avelrl/openai-compatible-tester (responses.stream) |
| `responses_tool_call` Responses 函数调用 | 断言 | `responses` | tools(flat function), tool_choice, parallel_tool_calls | output[type=function_call].call_id/name/arguments | avelrl/openai-compatible-tester (responses.tool_call) / openai/gpt-oss compatibility-test |
| `responses_tool_stream` Responses 流式函数调用 item_id | 断言 | `responses` | stream=true + tools | output_item.added 含 id/call_id/name; 每个 function_call_arguments.delta 与 done 都带 item_id 且指向已添加的 item, output_index 一致, done.arguments=各 delta 拼接且为合法 JSON, item_id 与最终 response 一致 | avelrl/openai-compatible-tester (responses.tool_call, streaming) / openai/gpt-oss compatibility-test (--streaming) |
| `responses_tool_roundtrip` Responses 工具往返 | 断言 | `responses` | input + function_call_output | 第二轮基于结果给出精确 JSON | avelrl/openai-compatible-tester (second turn after tool execution) |
| `responses_structured` Responses 结构化 | 断言 | `responses` | text.format=json_schema(strict) | 字段、类型与值精确匹配 | avelrl/openai-compatible-tester (responses.structured.json_schema) |
| `responses_previous_id` previous_response_id | 断言 | `responses` + full | store=true, previous_response_id | 第二轮能引用第一轮口令 | avelrl/openai-compatible-tester (responses.memory.prev_id) |
| `error_shape` 错误响应形状 | 断言 | basic | 缺少 messages 的非法请求 | 4xx 且 error.message/type/code/param 键齐全 | avelrl/openai-compatible-tester (error shape) / API7 AI 网关契约测试清单 |
| `usage_fields` usage 一致性 | 断言 | basic | 全部成功样本 | input/output/total 齐全、非负、total=input+output | API7 AI 网关契约测试清单 (usage) |
| `model_consistency` 模型回显一致 | 观测 | basic | 全部成功样本 | 响应 model 与请求一致 (允许日期快照后缀), system_fingerprint 记录 | 本项目自研 |
| `performance` 延迟观测 | 观测 | basic | 全部成功样本 | 中位/最大耗时, 流式首帧耗时 | API7 AI 网关契约测试清单 (timeouts) |

请求数上限：basic=4，standard=15，full=30（full 已含 responses 与 logprobs），再加 vision=31

`usage_fields`、`model_consistency`、`performance`、`chat_stream_usage` 由已有样本推导，不额外发请求。

## API

`POST /api/model_check/openai`（需登录；与 `POST /api/model_check` 同一权限与路由组）

```json
{
  "base_url": "https://relay.example.com/v1",
  "key": "sk-...",
  "model": "gpt-4o",
  "suite": "standard",
  "responses": false,
  "vision": false,
  "logprobs": false,
  "limit_param": "max_completion_tokens"
}
```

- `suite`：`basic` | `standard`（默认）| `full`；`responses` / `vision` / `logprobs` 可在任一套件上叠加。
- `base_url` 可写源站、`/v1` 基址或完整端点，会被规范化；含账号密码、query、fragment 的 URL 会被拒绝。
- 普通请求返回 `{"success":true,"data":<Report>,"markdown":"..."}`。
- 请求头带 `Accept: text/event-stream` 时，以 SSE 推送 `start / probe_start / sample / check / done` 事件，`done` 事件额外携带 `markdown`。

## 报告

- **得分** = 已判定断言中通过的比例（四舍五入）；观测、无法判定、跳过都不计入；没有任何断言得出结论时为 `null`。
- **Markdown 报告**按阶段列出每项的结果、代码、入参→出参校验与依据，并给出「需要关注的发现」、逐请求明细（HTTP、耗时、首帧、token、响应模型）、检测依据和局限。

### 局限（报告里也会写明）

- 衡量的是**接口兼容性**，**不能证明**上游就是名称所示的 OpenAI 模型：`model`、`system_fingerprint`、`usage` 都是端点自报的。
- 模型输出有随机性，一次通过只代表本次样本；关键结论请重复检测。
- 本检测不包含 embeddings、audio、images、batch、moderation、Realtime 等接口，且 Chat 通过不代表 Responses 通过（反之亦然）。

## 验证状态

- `pkg/openaicheck`：用符合规范的假 OpenAI 服务（Chat、SSE、工具、结构化输出、Responses 全套）做端到端测试，另有「不合规」场景（缺 `[DONE]`、缺 usage、拒绝入参、工具调用写成文本、401、断网、中途取消、重定向）与传输层测试（URL 规范化、载荷上限、密钥脱敏）；`go test -race` 通过，语句覆盖率约 88%。
- 前端：`tsc`（含 `noUnusedLocals`）、`eslint`、`prettier` 对 `features/model-check` 通过，`openai/` 与历史状态相关的 21 个单元测试通过；**尚未在浏览器里实际渲染过**，请本地 `bun run dev` 打开模型检测页，选 OpenAI 走一遍。
- `controller/model_check_openai.go` 与路由注册**尚未在完整仓库内编译**（开发环境缺少 Go 1.25 与模块代理），已按 `CheckModelClaude` 的写法逐行对照；入库相关的 `controller/model_check_openai_history.go`、历史详情分支及其测试（`model_check_openai_history_test.go`、`pkg/openaicheck/id_test.go`）同样只做过 `gofmt` 检查，未编译、未运行。合并前请先执行 `go build ./... && go test ./pkg/openaicheck/ ./router/ ./controller/`。

## 前端

入口：模型检测页（`web/default/src/features/model-check`）表单上方的 **Claude / OpenAI** 切换；代码在 `openai/` 子目录，仅小幅改动了 `components/new-check.tsx`（切换）和 `components/check-model-select.tsx`（可按厂商过滤模型建议）。

- **表单**：Base URL、API Key、模型、套件（基础 / 标准 / 完整）；高级设置里可勾选 Responses API、对数概率、图片输入，以及 Token 上限参数（`max_completion_tokens` / `max_tokens`）。完整套件固定包含 Responses 与 logprobs（开关置灰），图片输入不属于完整套件。表单下方实时显示请求数上限（基础 4 / 标准 15 / 完整 30 / 完整+图片 31）。
- **实时进度**：通过 SSE 逐项更新；报告抽屉按阶段分组，正在执行的探测高亮，实时得分按已判定断言计算（与服务端算法一致，结束后以服务端报告为准）。
- **报告**（参照 Claude 检测的报告纸，复用同一套 `.mc-report` 样式）：
  - 顶部状态卡：模型、状态、请求进度条（已用 / 上限）、耗时、当前探测、中止原因。
  - 报告纸：标题与时间 / 端点；得分环（兼容性得分）；雷达图（Chat 基础、流式、工具调用、结构化输出、Responses、协议与用量，按所选套件出现，每轴为该组已判定断言的通过率）；结论标签与覆盖率；指标卡（中位 / 最大延迟、流式首事件、上报 token）。
  - 需要关注的发现（失败 / 无法判定及原因）、模型路由线索（请求模型 / 返回模型 / `system_fingerprint`）、请求明细表（HTTP、耗时、首事件、输入输出 token、模型、备注）、按阶段的逐项结果。
  - 「All request evidence」折叠区保留每项的证据 JSON 与对应请求。
- **导出**：下载 HTML 报告、打印 / 存为 PDF（均由报告纸生成，与 Claude 报告同一实现）、导出 JSON、下载 / 复制 Markdown（服务端 `done` 事件返回）。检测进行中导出按钮置灰。
- 中止或断流时，未完成的检测会记为「跳过」，报告保持完整。
- i18n：7 个语言文件新增 155 条；中文、繁体中文已翻译，fr / ja / ru / vi 暂以英文原文占位，需要时再补译（`i18n:sync` 的 untranslated 报告可据此生成）。
- 测试：`openai/lib/openai-check.test.ts`与 `sheet.test.ts`（`node:test`，共 14 项）覆盖报告纸数据（雷达维度、结论、延迟 / 模型统计）以及请求数预算、完整套件强制项、得分取整、事件应用、中断收尾、SSE 读取（含 UTF-8 分片、注释帧、提前结束、未知事件）。

## 历史记录

- 写入：`controller/model_check_openai_history.go` 的 `openAICheckHistory` 与 Claude 的同名机制一致——首个事件创建一行 `running`，随后每个 `probe_start / sample / check` 更新检查点，`done` 时落到终态：`completed`、有失败断言 `failed`（界面显示「Review suggested」）、有中止原因 `stopped`、被取消 `cancelled`；观察者没收到 `done`（panic 等）时 `finalizeInterrupted` 记为 `interrupted`，超过 5 分钟没更新的 `running` 行由既有的清理任务改为 `interrupted`。
- 流式与非流式都入库。写库失败不会让检测失败：`done` 事件 / 响应里的报告带 `history_saved: false`，界面提示「历史记录保存失败，请在离开页面前导出这份报告」。
- 脱敏：快照在序列化后经 `Redactor.JSON` 再落库，库里不含 API Key；`Endpoint` 为规范化后的 Base URL。
- 读取：`GET /api/model_check/history` 列表对两种厂商通用（行上 `transport` 区分，`openai_api` 为 OpenAI）；`GET /api/model_check/history/:id` 与备注接口按 `transport` 解码对应报告，OpenAI 行额外返回由已存报告重新生成的 `markdown`。
- 前端：「我的检测」里 OpenAI 记录带 `OpenAI` 标记，「查看报告」用与实时检测相同的 OpenAI 报告视图打开（含 HTML / PDF / JSON / Markdown 导出）；表格里的「Export JSON」导出 OpenAI 报告 JSON。检测结束后报告抽屉里有「在历史中查看本次检测」按钮。
- 范围：OpenAI 记录暂不支持编辑备注（界面没有入口），也不参与基线对比。
- 升级注意：报告 ID 由 32 位十六进制改为 UUID v4；此前 OpenAI 检测从未入库，因此没有历史数据需要迁移。

## 尚未包含

- 渠道内检测：`POST /api/channel/:id/openai_check`（使用渠道已保存的 Key 与模型映射）。
- 基线对比。
