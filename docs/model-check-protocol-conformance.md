# 模型检测：官方协议一致性

2026-10-04 对照官方文档复核了 Claude 与 OpenAI 检测的入参与返回校验。已覆盖的检查没有重复添加；下表只记录本次**新增或修正**的项，以及仍未覆盖的项。

## Claude（Messages API）

依据：[Errors](https://platform.claude.com/docs/en/api/errors)、[Define tools](https://platform.claude.com/docs/en/agents-and-tools/tool-use/define-tools)、[Handling stop reasons](https://platform.claude.com/docs/en/api/handling-stop-reasons)、[Streaming](https://platform.claude.com/docs/en/build-with-claude/streaming)。

| 项 | 官方要求 | 检测处理 |
| --- | --- | --- |
| 强制工具选择 | Opus 5.5、Sonnet 5.5、Fable 5.1、Mythos 5.1 对 `tool_choice` 的 `any` / `tool` 返回 400；`auto`、`none` 可用 | **修正**：`tool` 探测对这四个型号改发 `tool_choice:{"type":"auto"}`（`supportsForcedToolChoice`，按型号名匹配，含 Bedrock 前缀与 `5.5` 写法），其余型号仍强制 `get_weather`。证据里记录 `tool_choice`。不增加请求数 |
| 错误信封 | 顶层 `type:"error"`、`error.type` + `error.message`、顶层 `request_id`，且 `error.type` 与 HTTP 状态一致（400 invalid_request_error、401 authentication_error、402 billing_error、403 permission_error、404 not_found_error、409 conflict_error、413 request_too_large、429 rate_limit_error、500 api_error、504 timeout_error、529 overloaded_error；其他 4xx 用 invalid_request_error；文档没有列出的 5xx 按 api_error 接受，这是本实现的推断） | **增强**：`error_shape` 满足全部条件记为 pass，否则仍为 inconclusive（网关可合法改写错误体，所以从不记失败）。证据新增 `error_type`、`type_matches_status`、`request_id_in_body`、`request_id_header`（响应头 `request-id`）。仍取自 `zero_output`（`max_tokens:0`）触发的真实 400 |

已有且与官方一致、本次未改动：消息信封（`type/role/id/model/content/stop_reason/usage`）、SSE 事件顺序（`message_start` → 内容块 → `message_delta` → `message_stop`）、`ping` 与未知事件容忍、`thinking/signature/input_json` 增量、`stop_sequences`、`max_tokens`、`system`、工具往返、`count_tokens`、缓存与 usage 字段。

**未新增**（会改变请求预算、报告版本与 7 种语言的文案，需要单独评审）：`GET /v1/models`（含 `capabilities`、`max_input_tokens`）、结构化输出 `output_config.format`（`json_schema`）、严格工具 `strict:true`、`tool_choice:none`。`stop_reason` 的取值集合（官方列出 `end_turn / max_tokens / stop_sequence / tool_use / pause_turn / refusal / model_context_window_exceeded`）也没有做白名单校验：官方说明取值可能扩展，白名单会把新增的合法值误判为失败。

## OpenAI（Chat Completions / Responses）

| 项 | 官方要求 | 检测处理 |
| --- | --- | --- |
| Chat `finish_reason` | `stop`、`length`、`tool_calls`、`content_filter`、`function_call` | **新增**：其他取值（如 `end_turn`）记 `unknown_finish_reason`；流式最后一帧同样校验 |
| usage 明细 | `prompt_tokens_details.cached_tokens`、`completion_tokens_details.reasoning_tokens`（Responses 为 `input_tokens_details` / `output_tokens_details`）为非负整数，推理 token 不超过输出 token | **新增**：存在时校验，违规记 `invalid_*` / `reasoning_tokens_exceed_output` |
| Responses `status` | `completed`、`failed`、`in_progress`、`cancelled`、`queued`、`incomplete` | **新增**：其他取值记 `unknown_status` |
| 模型列表 | `GET /v1/models` 的每项含 `id`、`object:"model"`、`created`、`owned_by` | **新增**：缺 `created` / `owned_by` 记 `model_missing_created` / `model_missing_owned_by`（该检查是观察项，不影响得分） |
| 报告 ID | — | 改为 UUID v4（与历史接口一致） |

已有且本次未改动：`chat.completion` / `chat.completion.chunk` 信封、`[DONE]`、`stream_options.include_usage` 末帧、工具调用（含流式增量里的 `item_id`）、结构化输出、Responses 事件顺序（`sequence_number` 递增、事件名与 `type` 一致、`response.created` 在先、终止事件）、错误体 `{error:{message,type,param,code}}`。

## 2026-10-06 基础协议覆盖对照

对照三家官方的请求参数与返回结构复核。解析器里已有的校验没有改动；本次只为**已实现但没有单测**的基础校验补测试（`pkg/claudecheck/protocol_basics_test.go`、`pkg/openaicheck/protocol_basics_test.go`），每个用例只破坏一个字段并断言对应的问题码。

| 协议 | 方面 | 结论 |
| --- | --- | --- |
| Claude | 请求参数（`max_tokens`、`system`、`stop_sequences`、`tools`/`tool_choice`、`thinking`、`count_tokens`） | 已有（runner 测试覆盖） |
| Claude | 错误信封、`error.type` 与状态码 | 已有 |
| Claude | 消息信封：`type`、`role`、`id`、`model`、`content`、`stop_reason`、`usage.input/output_tokens`（含负数、拒答可为空） | **新增测试** |
| Claude | 流式：文本/工具 `input_json_delta` 拼装、`ping` 穿插、usage 由 `message_start` 与 `message_delta` 合并（含缓存读） | **新增测试** |
| Claude | 流式生命周期违规：重复 `message_start`、块早于 `message_start`、重复块序号、块关闭后增量、未开先关、块未关就 `message_delta`、`message_delta` 后再开块、缺 `message_delta`、`message_stop` 后仍有事件、工具参数非 JSON、事件非 JSON | **新增测试** |
| Claude | `GET /v1/models`、`output_config.format`、`strict` 工具、`tool_choice:none`、`stop_reason` 白名单 | 暂不做（理由见上文） |
| OpenAI | 请求参数（`max_tokens`/`max_completion_tokens`、`stop`、`n`、`logprobs`、`tools`/`tool_choice`、`response_format`、`stream_options`、Responses `max_output_tokens`/`text.format`/`previous_response_id`） | 已有（runner 测试覆盖） |
| OpenAI | `finish_reason`、Responses `status`、usage 明细、模型列表 | 已有 |
| OpenAI | `chat.completion` 信封：`object`、`id`、`created`、`model`、`choices[].index`、`message`、`role`、`content` 键（工具调用时可为 `null`）、`finish_reason`、`usage` 缺失/负数/`total_tokens` 不一致 | **新增测试** |
| OpenAI | `chat.completion.chunk` 流：`object`、ID 一致、首帧 `role`、`finish_reason` 唯一且合法、`[DONE]`、错误帧、usage 帧须为末帧且 `choices` 为空、工具调用增量拼装与首帧需带 `id`/`name` | **新增测试** |
| OpenAI | 错误体 `{error:{message,type,param,code}}` 各字段、非 JSON、非 4xx 状态 | **新增测试** |
| OpenAI | Responses 对象：`object`、`id`、`model`、`created_at`、`status`、`output`、完成态需 usage、`incomplete_details.reason`、`refusal` 内容 | **新增测试** |
| OpenAI | Responses 流：`response.created` 在先、终止事件、终止后无事件、`sequence_number` 递增、SSE 事件名与 `type` 一致、增量文本与最终一致、终止事件需带 `response`、`response.failed` | **新增测试** |
| Gemini | `generateContent` / `streamGenerateContent`（`contents`、`generationConfig`、`candidates[].finishReason`、`usageMetadata`、`{error:{code,message,status}}`） | 暂不做：仓库没有 Gemini 原生检测器，Gemini 模型目前只走 OpenAI 兼容路径（`internal/modellist`）。新增协议需要新的检测器、报告与前端入口，应单独立项 |

验证：`go vet` 与 `go test -race ./pkg/claudecheck/ ./pkg/openaicheck/` 在完整仓库（Go 1.26）通过。

## 验证

- `pkg/openaicheck`：`go test -race` 通过（含新增的 `conformance_test.go`）。
- `pkg/claudecheck`：全部测试通过（含新增的 `protocol_conformance_test.go`），在沙箱中用 `common` 的最小替身以及 `testify/require`、`google/uuid` 的最小替身运行，因为沙箱没有 Go 模块代理，也没有 Go 1.25。替身的断言语义是按 testify 手写的，**不等同于**真实依赖；合并前请在完整仓库里重跑 `go test ./pkg/claudecheck/ ./pkg/openaicheck/`。
- 官方文档读取日期 2026-10-04；文档会变，型号限制尤其需要在新型号发布后复核。
