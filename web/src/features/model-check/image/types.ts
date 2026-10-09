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
export type ImageCheckStatus = 'pass' | 'fail' | 'inconclusive' | 'skipped'
export type ImageCheckKind = 'assertion' | 'observation' | 'provenance'
export type ImageSuite = 'basic' | 'standard' | 'full'

export type ImageCheckOptions = {
  model: string
  suite: ImageSuite
  provenance: boolean
  baseline: boolean
}

export type ImageCheckTarget = ImageCheckOptions & {
  base_url: string
  key?: string
  /** A saved key used instead of key (signed-in accounts). */
  credential_id?: string
  // Official OpenAI key: only ever sent to api.openai.com by the server.
  verify_key: string
}

export type ImageCheck = {
  id: string
  stage: string
  kind: ImageCheckKind
  status: ImageCheckStatus
  code: string
  evidence?: Record<string, unknown>
}

export type ImagePlanItem = {
  id: string
  stage: string
  kind: ImageCheckKind
  selected: boolean
}

export type ImageSample = {
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
  error_code?: string
  error?: string
  response_model?: string
  usage: {
    input_tokens: number | null
    output_tokens: number | null
    total_tokens: number | null
  }
}

export type ImageRef = {
  probe: string
  index: number
  format: string
  width: number
  height: number
  bytes: number
  sha256: string
  // Small JPEG data URL; present only in the live stream, never in history.
  thumb?: string
}

export type ProvenanceVerdict = {
  level: string
  c2pa_state: string
  issuer?: string
  model?: string
  generated_at?: string
  synthid: boolean
  model_match?: boolean
}

export type ProvenanceSummary = {
  enabled: boolean
  // trusted | synthid | untrusted | none | unavailable | off
  level: string
  unavailable_code?: string
  control_ok?: boolean
  verdict?: ProvenanceVerdict
  baseline?: { level: string; verdict?: ProvenanceVerdict }
  formats?: Record<string, string>
  verify_calls: number
}

export type ImageCheckReport = {
  version: number
  id: string
  provider: string
  model: string
  endpoint?: string
  options?: Partial<ImageCheckOptions>
  limits?: {
    probe_timeout_seconds: number
    run_timeout_seconds: number
    max_requests: number
    max_images: number
    max_verify_calls: number
  }
  plan?: ImagePlanItem[]
  started_at: string
  duration_ms: number
  checks: ImageCheck[]
  samples: ImageSample[]
  images: ImageRef[]
  summary: Record<ImageCheckStatus, number>
  score?: number | null
  provenance?: ProvenanceSummary
  stop_reason?: string
  stop_probe?: string
  cancelled: boolean
  requests_run: number
  images_requested?: number
  remark?: string
  history_saved?: boolean
}

export type ImageCheckEvent =
  | { type: 'start'; report: ImageCheckReport }
  | { type: 'done'; report: ImageCheckReport; markdown?: string }
  | { type: 'probe_start'; probe: string }
  | { type: 'sample'; sample: ImageSample }
  | { type: 'check'; check: ImageCheck }

export type ImageRunPhase =
  | 'idle'
  | 'connecting'
  | 'running'
  | 'complete'
  | 'cancelled'
  | 'error'

export type ImageRunState = {
  phase: ImageRunPhase
  report: ImageCheckReport | null
  markdown: string | null
  activeProbe: string | null
  error: string | null
}
