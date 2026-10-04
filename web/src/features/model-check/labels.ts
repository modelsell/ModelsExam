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
import { useTranslation } from 'react-i18next'
import { useBaselineLabels } from './hooks/use-baseline-labels'
import { useTokenAuditLabels } from './hooks/use-token-audit-labels'
import { useUsageTokenLabels } from './hooks/use-usage-token-labels'
import type { ClaudeCheckStatus } from './types'

export function useClaudeCheckLabels() {
  const { t } = useTranslation()
  const suiteNames = useBaselineLabels()
  const audit = useTokenAuditLabels()
  const usageTokens = useUsageTokenLabels()
  const names: Record<string, string> = {
    ...usageTokens.names,
    bedrock_role: t('Invalid message role boundary'),
    bedrock_beta: t('Unknown beta boundary'),
    bedrock_server_tools: t('Server tool availability boundary'),
    bedrock_web_search: t('Web search tool boundary'),
    bedrock_web_fetch: t('Web fetch tool boundary'),
    bedrock_code_execution: t('Code execution tool boundary'),
    bedrock_advisor: t('Advisor tool boundary'),
    bedrock_sampling: t('Model sampling parameter boundary'),
    bedrock_signature: t('Invalid thinking signature rejection'),
    cache_token_audit: t('Cache write/read token consistency'),
    behavior_fingerprint: t('Behavior fingerprint suite'),
    capability_benchmark: t('Capability collection suite'),
    performance_sampling: t('Generation performance suite'),
    basic: t('Response structure'),
    model_echo: t('Model mapping and echo'),
    model_consistency: t('Across-request model consistency'),
    cold_cache: t('Baseline cache usage'),
    error_shape: t('Error envelope'),
    vision: t('Image understanding'),
    thinking_stream: t('Streaming thinking integrity'),
    repeatability: t('Repeated sampling'),
    repeat_1: t('Repeat sample 1'),
    repeat_2: t('Repeat sample 2'),
    repeat_3: t('Repeat sample 3'),
    usage_fields: t('Usage field validity'),
    aws_usage: t('AWS header reconciliation'),
    system: t('System instruction adherence'),
    source: t('Source fingerprints'),
    token_count: t('Token count comparison'),
    stream: t('Streaming event lifecycle'),
    tool: t('Structured tool schema'),
    max_tokens: t('Output token limit'),
    zero_output: t('Zero-output request'),
    pdf: t('PDF identifier extraction'),
    stream_stop_reason: t('Stream stop reason consistency'),
    structured_tool: t('Structured tool schema'),
    stream_comparison: t('Stream and non-stream comparison'),
    comparison_nonstream: t('JSON non-stream sample'),
    comparison_stream: t('JSON stream sample'),
    reference_baseline: t('Historical baseline reference'),
    stop_sequence: t('Stop sequence'),
    multi_turn: t('Conversation context'),
    thinking: t('Thinking blocks'),
    signature_replay: t('Thinking signature replay'),
    cache: t('Prompt cache write and read'),
    cache_write: t('Cache write request'),
    cache_read_1: t('Cache read request 1'),
    cache_read_2: t('Cache read request 2'),
    billing: t('Billing verification'),
    prompt_integrity: t('Prompt integrity'),
    error_validation: t('Invalid parameter rejection'),
  }
  for (const [id, name] of Object.entries(suiteNames))
    names[`benchmark_${id}`] = name
  names.benchmark_tool_roundtrip = `${suiteNames.tool_roundtrip} · 1/2`
  names.benchmark_tool_roundtrip_result = `${suiteNames.tool_roundtrip} · 2/2`
  for (const id of ['number', 'letter', 'color', 'animal']) {
    for (let i = 1; i <= 10; i++)
      names[`fingerprint_${id}_${String(i).padStart(2, '0')}`] =
        `${suiteNames[id]} · ${i}/10`
  }
  for (const id of [
    'short',
    'system',
    'long',
    'floor_a',
    'floor_b',
    'system_canary',
  ]) {
    names[`prompt_audit_${id}`] = audit.names[id]
    names[`prompt_audit_${id}_count`] =
      `${audit.names[id]} · ${t('Request token count')}`
  }
  for (const id of ['floor_a', 'long', 'system_canary']) {
    for (let round = 1; round <= 3; round++) {
      const key = `${id}_r${round}`
      names[`prompt_audit_${key}`] = audit.names[key]
      names[`prompt_audit_${key}_count`] =
        `${audit.names[key]} · ${t('Request token count')}`
    }
  }
  for (let round = 1; round <= 3; round++)
    names[`prompt_audit_minimal_r${round}`] = audit.names[`minimal_r${round}`]
  for (let read = 1; read <= 6; read++)
    names[`cache_read_${read}`] = audit.names[`cache_read_${read}`]
  names.cache_write_2 = `${t('Cache write request')} · 2/2`
  for (let i = 1; i <= 3; i++)
    names[`performance_${i}`] = `${t('Generation performance suite')} · ${i}/3`
  const codes: Record<string, string> = {
    usage_incomplete: usageTokens.codes.usage_incomplete,
    usage_anomaly: usageTokens.codes.usage_anomaly,
    usage_consistent: usageTokens.codes.usage_consistent,
    bedrock_expected_rejection: t(
      '100 points: the rejection matches the tested parameter restriction.'
    ),
    bedrock_boundary_difference: t(
      'The request was accepted. A gateway may normalize parameters or provide extra features; this does not prove a model substitution.'
    ),
    bedrock_rejection_unresolved: t(
      'The response does not establish this restriction. Access, quota and service errors remain unscored.'
    ),
    bedrock_rule_unknown: t(
      'No documented sampling rule for this model ID; no request was sent.'
    ),
    ...audit.codes,
    performance_collected: t(
      'Three fixed streaming requests measure latency and observed throughput. This is not a load test.'
    ),
    fingerprint_collected: t(
      'Distribution samples are shown separately and do not affect the compatibility score.'
    ),
    benchmark_collected: t(
      'Capability answers are scored separately using a versioned test set.'
    ),
    model_declaration_mismatch: t(
      'A recognized model family, version or snapshot differs between the request, mapping or responses. Inspect the conflicting model names.'
    ),
    model_declaration_match: t(
      'Recognized model IDs are consistent after normalizing documented aliases and Bedrock wrappers. Rewritten fields can still conceal substitution.'
    ),
    model_declaration_unknown: t(
      'The requested alias or response model cannot be resolved reliably. Model matching is unscored.'
    ),
    request_refused: t(
      'The model declined this request. This does not establish whether the capability is supported; no score is assigned.'
    ),
    answer_truncated: t(
      'The output budget ended before the answer was complete. No capability score is assigned.'
    ),
    comparison_stop_observed: t(
      'Compare completion reasons separately from JSON correctness. Both responses should complete with end_turn.'
    ),
    probe_timeout: t(
      'The request timed out. Availability is unconfirmed; this is not a model compatibility score.'
    ),
    transport_error: t(
      'The upstream connection did not complete. Inspect the request error and network route.'
    ),
    upstream_unavailable: t(
      'The upstream is temporarily unavailable or overloaded. Inspect its response before retrying.'
    ),
    route_unavailable: t(
      'The upstream has no available channel for this model and group. Check the upstream routing configuration.'
    ),
    access_denied: t(
      'The upstream denied access. Check the API key, balance and model permissions.'
    ),
    endpoint_not_found: t(
      'The upstream endpoint was not found. Check the Base URL and API path.'
    ),
    request_rejected: t(
      'The upstream rejected the request. Inspect its response and channel overrides.'
    ),
    rate_limited: t(
      'The upstream rate limit was reached. Inspect the response and retry later.'
    ),
    invalid_response: t(
      'The response structure is invalid. Inspect the recorded validation reasons.'
    ),
    baseline_unavailable: t(
      'No usable baseline response was obtained. Dependent checks were not executed.'
    ),
    run_stopped: t(
      'This check was not executed because the run stopped early.'
    ),
    run_timeout: t(
      'The run reached its time limit. Finished results are preserved; remaining checks were skipped.'
    ),
    history_unavailable: t(
      'History saving was interrupted. Export the available report and check database access.'
    ),

    no_error_response: t(
      'No error response was produced; error envelope scoring is excluded.'
    ),
    token_limit_overridden: t(
      'The configured channel changed the requested token limit. This result is unscored.'
    ),
    token_limit_unavailable: t(
      'No usable token-limit response. Check model support, route access or rate limits.'
    ),
    token_limit_invalid_response: t(
      'The response envelope or usage is invalid. See the recorded validation reasons.'
    ),
    token_limit_exceeded: t(
      'Reported output tokens exceed the requested limit. Check upstream parameter forwarding.'
    ),
    zero_output_has_content: t(
      'A zero-output request returned content blocks. The official response requires an empty array.'
    ),
    token_limit_observed: t(
      'The response reached the requested token limit. An empty content array is valid here.'
    ),
    zero_output_observed: t(
      'Observed the official zero-output shape: empty content, max_tokens stop reason and zero output tokens.'
    ),
    token_limit_not_reached: t(
      'The response ended before demonstrating the requested limit; no score is assigned.'
    ),
    semantic_observed: t(
      'Compared the response with the expected content and schema. Token counts remain observations.'
    ),
    probe_unavailable: t(
      'The upstream did not provide a usable result for this optional probe. No score is assigned.'
    ),
    message_invalid: t(
      'The upstream returned an incomplete or invalid message or stream.'
    ),
    reference_only: t(
      'A matching historical model reference is available. Sampling and platform differences are not scored.'
    ),
    no_matching_reference: t(
      'No baseline matches this model. Shared fixtures still run; other model baselines are not substituted.'
    ),

    observed: t('See the observed response and usage below.'),
    model_mapping: t(
      'Compare requested, mapped, and returned names. Aliases can differ legitimately.'
    ),
    cold_cache_observation: t(
      'Baseline cache counters are observations; implicit caching or channel overrides may apply.'
    ),
    error_envelope: t(
      'Error formats may be normalized by AWS or proxies and do not establish origin.'
    ),
    usage_observation: t(
      'Check required input/output fields and nonnegative reported counters.'
    ),
    aws_header_comparison: t(
      'Compare available AWS counters; cached input with ambiguous normalization is excluded.'
    ),
    repeat_sample: t(
      'Compare three identical requests. Variations are clues, not proof of substitution or injection.'
    ),
    thinking_stream_observation: t(
      'Check stream completion and a signed thinking block, including omitted thinking text.'
    ),
    vision_observation: t(
      'Check recognition of one synthetic color image; this is not a vision benchmark.'
    ),
    count_difference: t(
      'CountTokens differs from reported input. Check transformations and counting semantics before judging billing.'
    ),
    fingerprint_only: t(
      'Fingerprints can be rewritten by proxies and do not prove model identity.'
    ),
    reported_usage_only: t(
      'Reported tokens are not an AWS invoice. Billing remains unverified.'
    ),
    no_trusted_baseline: t(
      'Without a trusted baseline, hidden prompt injection cannot be ruled out.'
    ),
    not_requested: t('Not selected for this run.'),
    baseline_failed: t(
      'Not evaluated because the baseline response was unavailable.'
    ),
    dependency_unavailable: t(
      'Required thinking or tool blocks were not returned.'
    ),
    count_unavailable: t(
      'CountTokens is unavailable for this model, region, permission, or proxy route.'
    ),
    missing_usage: t(
      'Required usage fields are missing; missing values are not zero.'
    ),
    same_endpoint_count: t(
      'Compared input plus cache tokens with CountTokens on the same upstream.'
    ),
    thinking_unavailable: t(
      'Thinking was rejected or no signed block was returned; support varies by model.'
    ),
    signature_present: t(
      'A signed thinking block was returned. Its signature was not locally verified.'
    ),
    replay_only: t(
      'Original blocks were replayed on the same credential. Acceptance is not an authenticity proof.'
    ),
    cache_unconfirmed: t(
      'Cache reuse was not confirmed. Check model support, minimum tokens, and upstream routing.'
    ),
    cache_observed: t(
      'Observed a cache write followed by a cache read. TTL expiry was not tested.'
    ),
    unexpected_usage: t('The upstream returned invalid token counts.'),
    cancelled: t('Skipped because the run was cancelled or timed out.'),
    interrupted: t('Skipped because the connection was interrupted.'),
  }
  const states: Record<ClaudeCheckStatus, string> = {
    pass: t('Passed'),
    fail: t('Review suggested'),
    inconclusive: t('Inconclusive'),
    skipped: t('Skipped'),
  }
  const stages: Record<string, string> = {
    connection: t('Connection and baseline'),
    protocol: t('Protocol and behavior'),
    capabilities: t('Document, vision, thinking and cache'),
    reliability: t('Consistency and usage'),
    boundaries: t('Verification boundaries'),
  }
  const stageDescriptions: Record<string, string> = {
    connection: t(
      'Confirm availability and record model, source, token, and cache observations.'
    ),
    protocol: t(
      'Validate streaming, structured tools, instructions and parameter boundaries.'
    ),
    capabilities: t(
      'Check PDF reading, image recognition, signed thinking, replay and cache reuse.'
    ),
    reliability: t(
      'Compare stream semantics, stop reasons, repeated requests and reported usage.'
    ),
    boundaries: t(
      'Model identity, hidden prompts, and supplier invoices require independent evidence.'
    ),
  }
  const actions: Record<string, string> = {
    model_echo: t(
      'Inspect channel model mapping and response model names; correlate provider invocation records when available.'
    ),
    model_consistency: t(
      'Inspect channel model mapping and response model names; correlate provider invocation records when available.'
    ),
    max_tokens: t(
      'Inspect requested and effective limits, usage and validation reasons in the request evidence.'
    ),
    zero_output: t(
      'Inspect requested and effective limits, usage and validation reasons in the request evidence.'
    ),
    basic: t(
      'Check the endpoint, credential, model access, and response format.'
    ),
    stream: t(
      'Check stream buffering, event order, and truncated connections.'
    ),
    thinking_stream: t(
      'Check thinking support and preservation of completed signature blocks.'
    ),
    tool: t('Check tool schema conversion and tool call identifiers.'),
    repeatability: t(
      'Inspect low-scoring samples for rate limits, routing changes, or timeouts.'
    ),
    aws_usage: t(
      'Inspect counter differences and the gateway usage conversion.'
    ),
  }
  return { names, codes, states, stages, stageDescriptions, actions }
}
