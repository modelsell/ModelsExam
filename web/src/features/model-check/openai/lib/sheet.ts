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
import type { OpenAICheck, OpenAICheckReport, OpenAISample } from '../types'
import { checkIdForProbe } from './catalog'
import { openAIScore } from './run-state'

// Radar axes. Each groups report stages; the score is the pass share of the
// decided assertions in the group, or null when none was decided.
export const OPENAI_DIMENSIONS: ReadonlyArray<{
  id: string
  label: string
  stages: readonly string[]
}> = [
  {
    id: 'chat',
    label: 'Chat basics',
    stages: ['chat_basic', 'chat_inputs', 'chat_vision'],
  },
  {
    id: 'generate',
    label: 'generateContent basics',
    stages: ['gemini_basic', 'gemini_inputs', 'gemini_vision'],
  },
  { id: 'stream', label: 'Streaming', stages: ['chat_stream', 'gemini_stream'] },
  { id: 'tools', label: 'Tool calling', stages: ['chat_tools', 'gemini_tools'] },
  {
    id: 'structured',
    label: 'Structured output',
    stages: ['chat_structured', 'gemini_structured'],
  },
  { id: 'responses', label: 'Responses API', stages: ['responses'] },
  // Model discovery is an observation, so it never moves a score.
  {
    id: 'protocol',
    label: 'Protocol and usage',
    stages: ['discovery', 'protocol', 'reliability'],
  },
]

export type SheetDimension = {
  id: string
  label: string
  score: number | null
}

export type SheetVerdict =
  | 'pending'
  | 'review'
  | 'partial'
  | 'clean'
  | 'unknown'

export type OpenAISheetData = {
  score: number | null
  dimensions: SheetDimension[]
  verdict: SheetVerdict
  assertions: number
  decided: number
  findings: OpenAICheck[]
  latency: { median: number | null; max: number | null; count: number }
  firstEvent: { median: number | null; count: number }
  totalTokens: number | null
  returnedModels: string[]
  fingerprints: string[]
}

export function median(values: number[]): number | null {
  if (values.length === 0) return null
  const sorted = [...values].sort((a, b) => a - b)
  const mid = Math.floor(sorted.length / 2)
  return sorted.length % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2
}

function unique(values: Array<string | undefined>): string[] {
  return [...new Set(values.filter((value): value is string => !!value))]
}

// Only answered requests describe the endpoint's speed; transport failures
// and rejected calls would distort the figures.
function answered(samples: OpenAISample[]): OpenAISample[] {
  return samples.filter(
    (sample) =>
      sample.http_status >= 200 &&
      sample.http_status < 300 &&
      !sample.error_code
  )
}

export function sheetData(
  report: OpenAICheckReport,
  running: boolean
): OpenAISheetData {
  const plan = report.plan ?? []
  const selectedStages = new Set(
    plan.filter((item) => item.selected).map((item) => item.stage)
  )
  const dimensions = OPENAI_DIMENSIONS.filter((dimension) =>
    report.plan && report.plan.length > 0
      ? dimension.stages.some((stage) => selectedStages.has(stage))
      : report.checks.some((check) => dimension.stages.includes(check.stage))
  ).map((dimension) => ({
    id: dimension.id,
    label: dimension.label,
    score: openAIScore(
      report.checks.filter((check) => dimension.stages.includes(check.stage))
    ),
  }))
  const assertions = report.checks.filter((c) => c.kind === 'assertion')
  const decided = assertions.filter(
    (c) => c.status === 'pass' || c.status === 'fail'
  ).length
  const failed = assertions.filter((c) => c.status === 'fail').length
  const inconclusive = assertions.filter(
    (c) => c.status === 'inconclusive'
  ).length
  let verdict: SheetVerdict = 'unknown'
  if (running) verdict = 'pending'
  else if (failed > 0) verdict = 'review'
  else if (decided > 0 && inconclusive > 0) verdict = 'partial'
  else if (decided > 0) verdict = 'clean'
  const ok = answered(report.samples)
  const rank = { fail: 0, inconclusive: 1, pass: 2, skipped: 3 }
  const findings = report.checks
    .filter((c) => c.status === 'fail' || c.status === 'inconclusive')
    .sort((a, b) => rank[a.status] - rank[b.status])
  const tokens = report.samples
    .map((sample) => sample.usage?.total_tokens)
    .filter((value): value is number => typeof value === 'number')
  return {
    score: report.score ?? null,
    dimensions,
    verdict,
    assertions: assertions.length,
    decided,
    findings,
    latency: {
      median: median(ok.map((s) => s.duration_ms)),
      max: ok.length ? Math.max(...ok.map((s) => s.duration_ms)) : null,
      count: ok.length,
    },
    firstEvent: {
      median: median(
        ok
          .map((s) => s.first_event_ms)
          .filter((value): value is number => !!value && value > 0)
      ),
      count: ok.filter((s) => !!s.first_event_ms && s.first_event_ms > 0)
        .length,
    },
    totalTokens: tokens.length ? tokens.reduce((a, b) => a + b, 0) : null,
    returnedModels: unique(ok.map((s) => s.response_model)),
    fingerprints: unique(ok.map((s) => s.system_fingerprint)),
  }
}

// Evidence for one check: samples of its probes, including "<id>_result".
export function samplesFor(
  report: OpenAICheckReport,
  id: string
): OpenAISample[] {
  return report.samples.filter((sample) => checkIdForProbe(sample.probe) === id)
}
