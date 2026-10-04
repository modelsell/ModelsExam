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
import type {
  ClaudeCheckReport,
  ClaudeCheckSample,
  TokenComparison,
} from '../types'
import { getCheckPlan, isCountProbe } from './check-plan'
import {
  coreAccuracy,
  focusedComparisons,
  focusedPerformance,
} from './focused-assessment'
import { modelIdentityState } from './model-identity'
import { median } from './report-metrics'
import { checkScore } from './report-score'
import {
  assessUsageTokens,
  isUsageTokenProbe,
  usageTokenCounters,
} from './usage-token-integrity'

export const measurable = (n: number | null | undefined): n is number =>
  typeof n === 'number' && Number.isFinite(n) && n >= 0
const average = (values: Array<number | null | undefined>) => {
  const valid = values.filter(measurable)
  return valid.length
    ? Math.round(valid.reduce((a, b) => a + b, 0) / valid.length)
    : null
}
export function auditSummary(items: TokenComparison[]) {
  const scored = items.filter((i) => measurable(i.score))
  return {
    score: average(scored.map((i) => i.score)),
    measured: scored.length,
    total: items.length,
    equal: scored.filter((i) => i.difference === 0).length,
  }
}
export function distribution(values: number[]) {
  const usable = values.filter(measurable)
  const mean = usable.reduce((a, b) => a + b, 0) / usable.length
  return {
    count: usable.length,
    median: median(usable),
    deviation:
      usable.length > 1
        ? Math.sqrt(
            usable.reduce((sum, n) => sum + (n - mean) ** 2, 0) /
              (usable.length - 1)
          )
        : null,
  }
}
export function totalInput(sample: ClaudeCheckSample): number | null {
  if (isUsageTokenProbe(sample.probe))
    return usageTokenCounters(sample).total_input
  if (isCountProbe(sample.probe) || sample.valid_response !== true) return null
  const values = [
    sample.usage.input_tokens,
    sample.usage.cache_creation_input_tokens,
    sample.usage.cache_read_input_tokens,
  ]
  return values.every(measurable)
    ? values.reduce<number>((sum, n) => sum + n!, 0)
    : null
}
const HEADER_NAMES = new Set([
  'server',
  'via',
  'cf-ray',
  'x-new-api-version',
  'x-amzn-requestid',
  'request-id',
  'x-request-id',
  'x-amzn-errortype',
  'x-amzn-bedrock-input-token-count',
  'x-amzn-bedrock-output-token-count',
])
export function reportHeaders(sample: ClaudeCheckSample) {
  return Object.entries(sample.headers ?? {}).filter(([key]) =>
    HEADER_NAMES.has(key.toLowerCase())
  )
}
function header(sample: ClaudeCheckSample, name: string) {
  return reportHeaders(sample).find(([key]) => key.toLowerCase() === name)?.[1]
}
// Compare only explicit uncached input counters. Cached header semantics vary.
export function awsCounterComparison(sample: ClaudeCheckSample) {
  if (isCountProbe(sample.probe) || sample.valid_response !== true)
    return { compared: 0, differences: 0 }
  const fields: Array<[string, number | null]> = [
    ['output', sample.usage.output_tokens],
  ]
  if (
    sample.usage.cache_creation_input_tokens === 0 &&
    sample.usage.cache_read_input_tokens === 0
  )
    fields.push(['input', sample.usage.input_tokens])
  let compared = 0,
    differences = 0
  for (const [field, value] of fields) {
    const raw = header(sample, `x-amzn-bedrock-${field}-token-count`)
    if (
      raw == null ||
      !/^\d+$/.test(raw) ||
      !measurable(value) ||
      !Number.isSafeInteger(Number(raw))
    )
      continue
    compared++
    if (Number(raw) !== value) differences++
  }
  return { compared, differences }
}
export function safeReportEndpoint(endpoint?: string) {
  if (!endpoint) return ''
  try {
    const url = new URL(endpoint)
    return ['http:', 'https:'].includes(url.protocol)
      ? url.origin + url.pathname
      : ''
  } catch {
    return ''
  }
}
export function sheetAssessment(report: ClaudeCheckReport, running = false) {
  const prompt = {
    ...auditSummary(report.token_audit?.prompt ?? []),
    assessment:
      (report.token_audit?.version ?? 0) >= 2
        ? report.token_audit?.prompt_assessment
        : undefined,
  }
  if (prompt.assessment) prompt.score = prompt.assessment.score
  const cache = auditSummary(report.token_audit?.cache ?? [])
  if ((report.token_audit?.version ?? 0) >= 4)
    cache.score = report.token_audit?.cache_assessment?.score ?? null
  const identity = modelIdentityState(report, running)
  const protocolIDs = new Set([
    'basic',
    'stream',
    'tool',
    'structured_tool',
    'vision',
    'pdf',
    'max_tokens',
    'stop_sequence',
  ])
  let modelScore: number | null = null
  if (identity === 'consistent') modelScore = 100
  if (identity === 'mismatch') modelScore = 0
  const usageTokens = assessUsageTokens(report)
  const dimensions = [
    {
      id: 'model',
      score: modelScore,
    },
    { id: 'capability', score: coreAccuracy(report).score },
    { id: 'prompt', score: prompt.score },
    { id: 'cache', score: cache.score },
    {
      id: 'protocol',
      score: average([
        ...report.checks.filter((c) => protocolIDs.has(c.id)).map(checkScore),
        usageTokens.score,
      ]),
    },
  ]
  // Performance distributions use exactly the same qualified samples as the
  // baseline comparison. Tiny prompts, cache and intentional rejections stay out.
  const performanceSamples = [
    'performance_1',
    'performance_2',
    'performance_3',
  ].flatMap((probe) => {
    const sample = report.samples.find(
      (s) => s.probe === probe && focusedPerformance([s]).count === 1
    )
    return sample ? [sample] : []
  })
  const warm = report.samples.filter((s) => /^cache_read_\d+$/.test(s.probe))
  const measuredWarm = warm.filter(
    (s) =>
      s.valid_response === true && measurable(s.usage.cache_read_input_tokens)
  )
  const hits = measuredWarm.filter(
    (s) => s.usage.cache_read_input_tokens! > 0
  ).length
  const base = report.samples.find(
    (s) => s.probe === (report.baseline_probe || 'basic')
  )
  const comparisons = focusedComparisons(report)
  const headerCounts = report.samples.map(awsCounterComparison)
  const modelNames = (field: 'upstream_model' | 'response_model') => [
    ...new Set(
      report.samples
        .filter((s) => s.valid_response === true)
        .flatMap((s) => (s[field] ? [s[field]!] : []))
    ),
  ]
  const evidence = [
    ...new Set(
      report.samples.flatMap((s) => [
        ...(s.message_id?.startsWith('msg_bdrk_') ? ['msg_bdrk_'] : []),
        ...(header(s, 'x-amzn-requestid') ? ['x-amzn-requestid'] : []),
        ...(header(s, 'x-new-api-version') ? ['x-new-api-version'] : []),
        ...(header(s, 'server') ? [`server: ${header(s, 'server')}`] : []),
        ...(header(s, 'via') ? [`via: ${header(s, 'via')}`] : []),
      ])
    ),
  ]
  const selected = getCheckPlan(report).filter((p) => p.selected)
  return {
    dimensions,
    usageTokens,
    score: average(dimensions.map((d) => d.score)),
    measured: dimensions.filter((d) => d.score !== null).length,
    identity,
    prompt,
    cache,
    performance: focusedPerformance(report.samples),
    timing: distribution(
      performanceSamples.map((s) => s.stream!.first_text_ms!)
    ),
    firstEvent: distribution(
      performanceSamples.flatMap((s) =>
        measurable(s.first_event_ms) &&
        s.first_event_ms <= s.stream!.first_text_ms!
          ? [s.first_event_ms]
          : []
      )
    ),
    warm: {
      attempts: warm.length,
      measured: measuredWarm.length,
      hits,
      rate: measuredWarm.length
        ? Math.round((100 * hits) / measuredWarm.length)
        : null,
    },
    cold:
      base?.valid_response === true &&
      base.usage.cache_creation_input_tokens === 0 &&
      base.usage.cache_read_input_tokens === 0,
    aws: {
      compared: headerCounts.reduce((a, b) => a + b.compared, 0),
      differences: headerCounts.reduce((a, b) => a + b.differences, 0),
    },
    comparisons,
    upstreamModels: modelNames('upstream_model'),
    returnedModels: modelNames('response_model'),
    evidence,
    selected,
    executed: selected.filter((p) =>
      report.checks.some((c) => c.id === p.id && c.status !== 'skipped')
    ).length,
  }
}
