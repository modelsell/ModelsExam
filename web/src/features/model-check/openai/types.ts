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
export type OpenAICheckStatus = 'pass' | 'fail' | 'inconclusive' | 'skipped'
export type OpenAICheckKind = 'assertion' | 'observation'
export type OpenAISuite = 'basic' | 'standard' | 'full'
export type OpenAILimitParam = 'max_completion_tokens' | 'max_tokens'

export type OpenAICheckOptions = {
  model: string
  suite: OpenAISuite
  responses: boolean
  vision: boolean
  logprobs: boolean
  limit_param: OpenAILimitParam
}

export type OpenAICheckTarget = OpenAICheckOptions & {
  base_url: string
  key: string
}

export type OpenAICheck = {
  id: string
  stage: string
  kind: OpenAICheckKind
  status: OpenAICheckStatus
  code: string
  evidence?: Record<string, unknown>
}

export type OpenAIPlanItem = {
  id: string
  stage: string
  kind: OpenAICheckKind
  selected: boolean
}

export type OpenAIUsage = {
  input_tokens: number | null
  output_tokens: number | null
  total_tokens: number | null
}

export type OpenAISample = {
  probe: string
  kind: string
  method: string
  path: string
  http_status: number
  duration_ms: number
  first_event_ms?: number
  stream?: boolean
  valid_response?: boolean
  validation_errors?: string[]
  usage_issues?: string[]
  error_code?: string
  error?: string
  response_model?: string
  response_id?: string
  finish_reason?: string
  system_fingerprint?: string
  usage: OpenAIUsage
}

export type OpenAICheckReport = {
  version: number
  id: string
  // Native Gemini reports share this shape and view.
  provider: 'openai' | 'gemini'
  model: string
  endpoint?: string
  options?: Partial<OpenAICheckOptions>
  limits?: {
    probe_timeout_seconds: number
    run_timeout_seconds: number
    max_requests: number
  }
  plan?: OpenAIPlanItem[]
  started_at: string
  duration_ms: number
  checks: OpenAICheck[]
  samples: OpenAISample[]
  summary: Record<OpenAICheckStatus, number>
  score?: number | null
  stop_reason?: string
  stop_probe?: string
  cancelled: boolean
  requests_run: number
  remark?: string
  // Set by the server: false when the run could not be stored in history.
  history_saved?: boolean
}

export type OpenAICheckEvent =
  | { type: 'start'; report: OpenAICheckReport }
  | { type: 'done'; report: OpenAICheckReport; markdown?: string }
  | { type: 'probe_start'; probe: string }
  | { type: 'sample'; sample: OpenAISample }
  | { type: 'check'; check: OpenAICheck }

export type OpenAIRunPhase =
  | 'idle'
  | 'connecting'
  | 'running'
  | 'complete'
  | 'cancelled'
  | 'error'

export type OpenAIRunState = {
  phase: OpenAIRunPhase
  report: OpenAICheckReport | null
  markdown: string | null
  activeProbe: string | null
  error: string | null
}
