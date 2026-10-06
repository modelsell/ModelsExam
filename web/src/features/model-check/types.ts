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
import type { ImageCheckReport } from './image/types'
import type { OpenAICheckReport } from './openai/types'

export type ClaudeCheckStatus = 'pass' | 'fail' | 'inconclusive' | 'skipped'
export type ClaudeCheckOptions = {
  bedrock?: boolean
  suite?: 'focused'
  performance_tolerance?: number
  model: string
  cache: boolean
  thinking: boolean
  repeat?: boolean
  vision?: boolean
  pdf?: boolean
  stream_comparison?: boolean
  fingerprint?: boolean
  benchmark?: boolean
  performance?: boolean
  prompt_audit?: boolean
  compare_baselines?: boolean
  baseline_id?: string
  baseline_type?: string
  /** Legacy input only; normalized for display. */
  veridrop?: boolean
}
export type PromptAssessment = {
  scoring?: string
  behavior_score?: number | null
  token_score?: number | null
  behavior_planned?: number
  token_source?: string
  score: number | null
  coverage: number
  behavior_points: number
  token_points: number
  behavior_measured: number
  token_measured: number
}
export type PromptInjectionReport = {
  allowance_tokens?: number
  estimated_extra_tokens?: number | null
  max_extra_tokens?: number | null
  extra_rounds?: number
  baseline_samples?: number
  platform?: 'anthropic' | 'aws'
  selected_reference_id?: string
  sampling?: Array<{
    outliers?: number
    id: string
    valid: number
    planned: number
    median: number | null
    min: number | null
    max: number | null
    stable: boolean
  }>
  code: string
  local_changes: number
  repeat_delta: number | null
  system_delta: number | null
  incompatible_references: number
  references: Array<{
    id: string
    report_id: string
    type: string
    code: string
    pairs: Array<{
      id: string
      expected: number
      actual: number
      difference: number
      tolerance: number
    }>
  }>
}
export type TokenComparison = {
  platform?: 'anthropic' | 'aws'
  response_model?: string
  client_profile?: string
  prompt_profile?: string
  prompt_changed?: boolean
  model?: string
  behavior?: { score: number | null; code: string }
  tolerance?: number

  id: string
  expected: number | null
  actual: number | null
  difference: number | null
  score: number | null
  code: string
  profile?: string
  request_changed?: boolean
  repeated_write?: number | null
}
export type CacheAssessment = {
  score: number | null
  measured: number
  planned: number
  coverage: number
  rounds: number
}
export type TokenAuditReport = {
  cache_assessment?: CacheAssessment
  injection?: PromptInjectionReport
  prompt_assessment?: PromptAssessment
  version: number
  prompt: TokenComparison[]
  cache: TokenComparison[]
}
export type ClaudeCheckReport = {
  token_audit?: TokenAuditReport
  baselines?: ComparisonBaseline[]
  fingerprint?: BehaviorFingerprint
  benchmark?: CapabilityBenchmark
  stop_reason?: string
  stop_probe?: string
  baseline_probe?: string
  limits?: {
    probe_timeout_seconds: number
    run_timeout_seconds: number
    max_requests: number
  }
  version: number
  id: string
  model: string
  channel_id: number
  channel_name: string
  /** Description read from the tested site's home page when the check started. */
  site_description?: string | null
  remark?: string | null
  transport: 'bedrock_runtime' | 'anthropic_proxy'
  endpoint?: string
  options?: ClaudeCheckOptions
  reference?: BaselineReference
  history_saved?: boolean
  plan?: CheckPlanItem[]
  started_at: string
  duration_ms: number
  cancelled: boolean
  summary: Record<ClaudeCheckStatus, number>
  checks: Array<{
    id: string
    status: ClaudeCheckStatus
    code: string
    evidence?: Record<string, unknown>
  }>
  samples: Array<{
    diagnostic?: {
      code: string
      exception?: string
      source: string
      retryable: boolean
      expected_status?: number
      original_status?: number
    }
    request_profile?: string
    probe: string
    error_code?: string
    http_status: number
    duration_ms: number
    first_event_ms?: number
    valid_response?: boolean
    requested_max_tokens?: number
    effective_max_tokens?: number
    content_blocks?: number
    validation_errors?: string[]
    stream?: {
      first_text_ms: number | null
      last_text_ms: number | null
      max_text_gap_ms: number | null
      text_events: number
      events: number
    }
    upstream_model?: string
    response_model?: string
    message_id?: string
    request_id?: string
    stop_reason?: string
    headers?: Record<string, string>
    error?: string
    usage: {
      input_tokens: number | null
      output_tokens: number | null
      cache_creation_input_tokens: number | null
      cache_read_input_tokens: number | null
    }
  }>
}

export type ClaudeCheck = ClaudeCheckReport['checks'][number]
export type ClaudeCheckSample = ClaudeCheckReport['samples'][number]
export type CheckEvent =
  | { type: 'token_audit'; token_audit: TokenAuditReport }
  | { type: 'fingerprint'; fingerprint: BehaviorFingerprint }
  | { type: 'benchmark'; benchmark: CapabilityBenchmark }
  | { type: 'start' | 'done'; report: ClaudeCheckReport }
  | { type: 'probe_start'; probe: string }
  | { type: 'sample'; sample: ClaudeCheckSample }
  | { type: 'check'; check: ClaudeCheck }

export type CheckTarget = ClaudeCheckOptions &
  ({ channel_id: number } | { base_url: string; key: string })
export type RunPhase =
  | 'idle'
  | 'connecting'
  | 'running'
  | 'complete'
  | 'cancelled'
  | 'error'
export type RunState = {
  phase: RunPhase
  report: ClaudeCheckReport | null
  activeProbe: string | null
  error: string | null
}

export type CheckHistoryStatus =
  | 'running'
  | 'completed'
  | 'stopped'
  | 'failed'
  | 'cancelled'
  | 'interrupted'
export type CheckHistoryItem = {
  id: string
  model: string
  channel_id: number
  channel_name: string
  remark?: string | null
  score?: number | null
  endpoint: string
  transport:
    | ClaudeCheckReport['transport']
    | 'openai_api'
    | 'gemini_api'
    | 'image_api'
  status: CheckHistoryStatus
  started_at: number
  updated_at: number
  duration_ms: number
  request_count: number
  pass_count: number
  fail_count: number
  active_probe: string
}
export type CheckHistoryDetail = {
  run: CheckHistoryItem
  report: ClaudeCheckReport
}
export type OpenAIHistoryDetail = {
  run: CheckHistoryItem
  report: OpenAICheckReport
  markdown?: string
}
export type ImageHistoryDetail = {
  run: CheckHistoryItem
  report: ImageCheckReport
  markdown?: string
}
// The detail route answers with the report type of the run's transport.
export type HistoryDetail =
  | CheckHistoryDetail
  | OpenAIHistoryDetail
  | ImageHistoryDetail
export type CheckHistoryPage = {
  items: CheckHistoryItem[]
  total: number
  page: number
  page_size: number
}

export type BehaviorFingerprint = {
  version: number
  repetitions: number
  cells: Array<{
    id: string
    profile: string
    attempts: number
    valid: number
    counts: Record<string, number>
  }>
}
export type CapabilityBenchmark = {
  version: number
  items: Array<{
    id: string
    category: string
    profile: string
    expected: string
    correct: boolean | null
    score?: number | null
    actual?: string
    grader?: string
    code?: string
    assertions?: Array<{ id: string; passed: boolean }>
  }>
}
export type ComparisonBaseline = Pick<
  ClaudeCheckReport,
  | 'id'
  | 'model'
  | 'version'
  | 'channel_id'
  | 'channel_name'
  | 'transport'
  | 'endpoint'
  | 'started_at'
  | 'options'
  | 'plan'
  | 'checks'
  | 'samples'
  | 'fingerprint'
  | 'benchmark'
  | 'token_audit'
> & { report_id: string; type: string; created_at: number }

export type BaselineListItem = {
  id: string
  report_id: string
  type: string
  model: string
  channel_name: string
  created_at: number
  fingerprint: boolean
}

export type CheckPlanItem = {
  id: string
  stage: string
  kind: 'assertion' | 'observation' | 'boundary'
  selected: boolean
}

export type BaselineReference = {
  source_url: string
  commit: string
  license: string
  matched_model?: string
  snapshots: Array<{
    model: string
    response_model: string
    collected_at: string
    file: string
    sha256: string
    pdf_identifier: string
    tool_name: string
    tool_city: string
    tool_unit: string
    integrity_input_tokens: number
    integrity_output_tokens: number
    integrity_stream_input_tokens: number
    integrity_stream_output_tokens: number
    consistency_runs: number
    consistency_output_tokens: number[]
    consistency_cv: number
  }>
}
