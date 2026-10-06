/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
// Display metadata for the OpenAI and native Gemini checks (their check ids
// never collide, so one table labels both report kinds). Values are i18n keys (English source
// text) and must be rendered through t(); they are registered in
// src/i18n/static-keys.ts because they are looked up dynamically.

export const OPENAI_STAGES: ReadonlyArray<{ id: string; title: string }> = [
  { id: 'discovery', title: 'Model discovery' },
  { id: 'chat_basic', title: 'Chat Completions basics' },
  { id: 'chat_inputs', title: 'Chat input parameters' },
  { id: 'chat_stream', title: 'Chat streaming' },
  { id: 'chat_tools', title: 'Chat tool calling' },
  { id: 'chat_structured', title: 'Chat structured output' },
  { id: 'chat_vision', title: 'Chat vision' },
  { id: 'responses', title: 'Responses API' },
  { id: 'gemini_basic', title: 'generateContent basics' },
  { id: 'gemini_inputs', title: 'Gemini input parameters' },
  { id: 'gemini_stream', title: 'Gemini streaming' },
  { id: 'gemini_tools', title: 'Gemini function calling' },
  { id: 'gemini_structured', title: 'Gemini structured output' },
  { id: 'gemini_vision', title: 'Gemini vision' },
  { id: 'protocol', title: 'Protocol and usage' },
  { id: 'reliability', title: 'Reliability observations' },
]

export const OPENAI_CHECK_TITLES: Record<string, string> = {
  models_list: 'Model list',
  chat_basic: 'Chat response envelope',
  chat_system: 'System instruction',
  chat_max_tokens: 'Token limit',
  chat_stop: 'Stop sequence',
  chat_multi_turn: 'Multi-turn history',
  chat_params: 'Sampling parameters accepted',
  chat_n: 'Multiple choices (n)',
  chat_logprobs: 'Log probabilities',
  chat_stream: 'SSE streaming',
  chat_stream_usage: 'Streaming usage chunk',
  chat_tool_call: 'Tool call',
  chat_tool_choice: 'Forced tool choice',
  chat_tool_choice_none: 'Tool choice none',
  chat_tool_roundtrip: 'Tool call round trip',
  chat_parallel_tools: 'Parallel tool calls',
  chat_tool_stream: 'Streaming tool call',
  chat_json_mode: 'JSON mode',
  chat_json_schema: 'JSON schema output',
  chat_vision: 'Image input',
  responses_basic: 'Responses envelope',
  responses_max_output: 'Max output tokens',
  responses_stream: 'Responses streaming',
  responses_tool_call: 'Responses function call',
  responses_tool_stream: 'Streaming function call item_id',
  responses_tool_roundtrip: 'Responses tool round trip',
  responses_structured: 'Responses structured output',
  responses_previous_id: 'previous_response_id',
  error_shape: 'Error response shape',
  usage_fields: 'Usage consistency',
  gemini_model_get: 'Model resource',
  gemini_basic: 'generateContent envelope',
  gemini_system: 'System instruction',
  gemini_max_tokens: 'maxOutputTokens',
  gemini_stop: 'Stop sequences',
  gemini_multi_turn: 'Multi-turn history',
  gemini_params: 'Generation config accepted',
  gemini_candidates: 'Multiple candidates',
  gemini_thinking: 'Thought summaries',
  gemini_stream: 'SSE streaming',
  gemini_stream_usage: 'Streaming usageMetadata',
  gemini_tool_call: 'Function call',
  gemini_tool_choice: 'Forced function (ANY)',
  gemini_tool_choice_none: 'Function calling NONE',
  gemini_tool_roundtrip: 'Function call round trip',
  gemini_parallel_tools: 'Parallel function calls',
  gemini_tool_stream: 'Streaming function call',
  gemini_json_mode: 'JSON MIME type',
  gemini_json_schema: 'Response schema',
  gemini_vision: 'Image input',
  gemini_count_tokens: 'countTokens',
  gemini_error_shape: 'Error response shape',
  gemini_usage: 'usageMetadata consistency',
  model_consistency: 'Model name echo',
  performance: 'Latency',
}

// What each check sends and validates, shown beside the result.
export const OPENAI_CHECK_DETAILS: Record<string, string> = {
  models_list: 'GET /v1/models: list envelope, model ids, target model listed.',
  chat_basic:
    'Chat envelope: object, id, created, model, choices, finish_reason and usage totals.',
  chat_system: 'A system message must steer the reply.',
  chat_max_tokens:
    'The token limit must end the reply with finish_reason length.',
  chat_stop: 'Output must stop before the stop sequence.',
  chat_multi_turn: 'An earlier turn must be remembered.',
  chat_params:
    'n, seed, user and sampling parameters (reasoning_effort for reasoning models) must be accepted.',
  chat_n: 'n=2 must return two choices.',
  chat_logprobs: 'logprobs and top_logprobs must be returned.',
  chat_stream:
    'chat.completion.chunk frames, assistant role first, one finish_reason, [DONE] terminator.',
  chat_stream_usage:
    'stream_options.include_usage must add a final usage chunk with empty choices.',
  chat_tool_call:
    'tool_calls id, type, function name and exact JSON arguments, finish_reason tool_calls.',
  chat_tool_choice: 'A forced tool_choice must call the named function.',
  chat_tool_choice_none: 'tool_choice none must not produce tool calls.',
  chat_tool_roundtrip:
    'A role tool message must let the model answer from the tool result.',
  chat_parallel_tools:
    'parallel_tool_calls may return several calls with distinct ids.',
  chat_tool_stream:
    'Streamed tool_calls deltas carry id and name first, and arguments assemble into the exact JSON.',
  chat_json_mode: 'response_format json_object must return a JSON object.',
  chat_json_schema:
    'response_format json_schema (strict) must match fields, types and values.',
  chat_vision: 'An image_url data URL must be understood.',
  responses_basic:
    'Responses envelope: object, status, output message and usage totals.',
  responses_max_output:
    'max_output_tokens must end with status incomplete and reason max_output_tokens.',
  responses_stream:
    'response.created, output_text.delta and response.completed with increasing sequence_number.',
  responses_tool_call:
    'output function_call item with call_id, name and exact JSON arguments.',
  responses_tool_stream:
    'output_item.added carries id, call_id and name; every function_call_arguments delta and done event carries the matching item_id; the final item id matches.',
  responses_tool_roundtrip:
    'function_call_output must let the model answer from the tool result.',
  responses_structured:
    'text.format json_schema (strict) must match fields, types and values.',
  responses_previous_id: 'previous_response_id must keep the conversation.',
  error_shape:
    'An invalid request must return a 4xx with error message, type, code and param.',
  usage_fields:
    'Usage must be present, non-negative and total = input + output.',
  model_consistency:
    'The reported model must match the request, allowing dated snapshot suffixes.',
  gemini_model_get:
    'GET /v1beta/models/{model}: name, version, token limits and generateContent support.',
  gemini_basic:
    'Candidates with role model and parts, finishReason STOP, usageMetadata, modelVersion and responseId.',
  gemini_system: 'systemInstruction must steer the reply.',
  gemini_max_tokens:
    'generationConfig.maxOutputTokens must end the reply with finishReason MAX_TOKENS.',
  gemini_stop: 'Output must stop before the stop sequence.',
  gemini_multi_turn: 'An earlier user / model turn must be remembered.',
  gemini_params:
    'temperature, topP, topK, candidateCount, seed, responseMimeType and safetySettings must be accepted.',
  gemini_candidates: 'candidateCount=2 must return two indexed candidates.',
  gemini_thinking:
    'thinkingConfig.includeThoughts must return thought parts or thoughtsTokenCount.',
  gemini_stream:
    'streamGenerateContent?alt=sse: complete response chunks, one finishReason on the last, consistent responseId, no [DONE].',
  gemini_stream_usage: 'The last streamed chunk must carry usageMetadata.',
  gemini_tool_call:
    'functionCall part with the exact name and object args, finishReason STOP.',
  gemini_tool_choice:
    'functionCallingConfig mode ANY with allowedFunctionNames must call that function.',
  gemini_tool_choice_none: 'Mode NONE must answer in text without a functionCall.',
  gemini_tool_roundtrip:
    'Replaying the model turn (with thought signatures) and a functionResponse part must produce the answer.',
  gemini_parallel_tools: 'One turn may return several functionCall parts.',
  gemini_tool_stream: 'A streamed functionCall must arrive whole with exact args.',
  gemini_json_mode: 'responseMimeType application/json must return a JSON object.',
  gemini_json_schema:
    'responseSchema must be matched in fields, types and values.',
  gemini_vision: 'An inlineData PNG must be understood.',
  gemini_count_tokens:
    'countTokens totalTokens must equal promptTokenCount for the same contents.',
  gemini_error_shape:
    'An invalid request must return a 4xx with error.code, error.message and the matching error.status.',
  gemini_usage:
    'usageMetadata must be present and total = prompt + candidates + thoughts + tool-use prompt.',
  performance: 'Median and maximum latency, and streaming first-event time.',
}

export const OPENAI_CODE_NOTES: Record<string, string> = {
  unauthorized: 'Credentials were rejected (401).',
  forbidden: 'Access was forbidden (403).',
  rate_limited: 'The request was rate limited (429).',
  upstream_error: 'The upstream server returned an error (5xx).',
  timeout: 'The request timed out.',
  network_error: 'A network error stopped the request.',
  request_rejected: 'The endpoint rejected an input that the official API accepts.',
  invalid_response: 'The response does not match the official API contract.',
  accepted_invalid_request: 'An invalid request was accepted.',
  error_shape_mismatch: 'The error body does not match the official error shape.',
  not_requested: 'Not selected for this run.',
  run_stopped: 'Skipped because the baseline request was unavailable.',
  baseline_failed: 'Skipped because the Chat baseline request was rejected.',
  baseline_unavailable: 'The baseline request could not be completed.',
  cancelled: 'The run was cancelled.',
  interrupted: 'The run was interrupted.',
  model_not_listed: 'The model is not listed by /v1/models.',
  model_mismatch: 'The reported model differs from the requested one.',
  model_changed_between_requests:
    'The reported model changed between requests.',
  no_model_reported: 'No model name was reported.',
  no_successful_samples: 'No successful responses to inspect.',
  unsupported_model_family: 'Not supported by this model family.',
  dependency_unavailable: 'A prerequisite request was unavailable.',
  single_tool_call: 'Only one tool call was returned.',
  limit_not_enforced: 'The token limit was not enforced.',
  missing_usage_chunk: 'No usage chunk was streamed.',
  usage_chunk_invalid: 'The streamed usage chunk is invalid.',
  usage_missing: 'Some responses have no usage.',
  usage_inconsistent: 'Usage numbers are inconsistent.',
  streamed_function_call_invalid: 'Streamed function-call events are invalid.',
  streamed_tool_call_invalid: 'The streamed tool call is invalid.',
  tool_call_invalid: 'The tool call does not match the request.',
  forced_tool_not_called: 'The forced tool was not called.',
  tool_called_despite_none: 'A tool was called although tool_choice is none.',
  roundtrip_answer_invalid: 'The answer after the tool result is wrong.',
  function_call_invalid: 'The function call does not match the request.',
  params_rejected: 'The endpoint rejected the parameters.',
  system_ignored: 'The system message was ignored.',
  stop_sequence_ignored: 'The stop sequence was ignored.',
  history_lost: 'Earlier turns were not used.',
  n_ignored: 'n was ignored.',
  logprobs_missing: 'No log probabilities were returned.',
  json_object_not_honored: 'json_object mode was not honored.',
  json_schema_not_honored: 'The JSON schema was not honored.',
  image_not_understood: 'The image was not understood.',
  unexpected_finish_or_empty: 'Unexpected finish reason or empty reply.',
  unexpected_status_or_empty: 'Unexpected status or empty reply.',
  stream_unexpected_output: 'The stream produced unexpected output.',
  conversation_state_lost: 'The conversation state was not kept.',
  first_request_invalid: 'The first request was invalid.',
  generate_content_not_supported:
    'The model does not list generateContent as supported.',
  candidate_count_ignored: 'candidateCount was ignored.',
  thoughts_missing: 'No thoughts were returned.',
  json_mime_type_not_honored: 'The JSON MIME type was not honored.',
  response_schema_not_honored: 'The response schema was not honored.',
  missing_usage_metadata: 'No usageMetadata was streamed.',
  count_differs_from_usage:
    'countTokens differs from the reported promptTokenCount.',
}

export const OPENAI_CODE_CHIPS_LIMIT = 6

// Probes named "<check>_result" are the second request of a two-step check.
export function checkIdForProbe(probe: string): string {
  return probe.replace(/_result$/, '')
}
