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

## 验证

- `pkg/openaicheck`：`go test -race` 通过（含新增的 `conformance_test.go`）。
- `pkg/claudecheck`：全部测试通过（含新增的 `protocol_conformance_test.go`），在沙箱中用 `common` 的最小替身以及 `testify/require`、`google/uuid` 的最小替身运行，因为沙箱没有 Go 模块代理，也没有 Go 1.25。替身的断言语义是按 testify 手写的，**不等同于**真实依赖；合并前请在完整仓库里重跑 `go test ./pkg/claudecheck/ ./pkg/openaicheck/`。
- 官方文档读取日期 2026-10-04；文档会变，型号限制尤其需要在新型号发布后复核。
