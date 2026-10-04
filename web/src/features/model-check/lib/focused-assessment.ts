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
  BehaviorFingerprint,
  ClaudeCheckReport,
  ClaudeCheckSample,
  ComparisonBaseline,
} from '../types'
import {
  capabilitySummary,
  capabilityPairSummary,
  LEGACY_CAPABILITY_IDS,
} from './capability-assessment'

export const CORE_QUESTIONS = LEGACY_CAPABILITY_IDS
const CATEGORIES = ['letter', 'animal']
const PERFORMANCE_PROBES = ['performance_1', 'performance_2', 'performance_3']
const median = (values: number[]) => {
  if (!values.length) return null
  const sorted = [...values].sort((a, b) => a - b)
  const i = Math.floor(sorted.length / 2)
  return sorted.length % 2 ? sorted[i] : (sorted[i - 1] + sorted[i]) / 2
}
const meanScore = (values: boolean[]) =>
  values.length
    ? Math.round((100 * values.filter(Boolean).length) / values.length)
    : null

export function coreAccuracy(report: Pick<ClaudeCheckReport, 'benchmark'>) {
  return capabilitySummary(report.benchmark)
}

function usableCell(cell?: BehaviorFingerprint['cells'][number]) {
  return (
    !!cell &&
    cell.valid >= 10 &&
    Object.values(cell.counts).every((n) => Number.isInteger(n) && n >= 0) &&
    Object.values(cell.counts).reduce((a, b) => a + b, 0) === cell.valid
  )
}

// v2 retains the exact letter/animal fixtures from v1. An unavailable cell is
// explained individually; partial similarity never qualifies for ranking.
export function focusedFingerprint(
  a?: BehaviorFingerprint,
  b?: BehaviorFingerprint
) {
  const cells = CATEGORIES.map((id) => {
    const current = a?.cells.find((cell) => cell.id === id)
    const reference = b?.cells.find((cell) => cell.id === id)
    const result = {
      id,
      current: current?.valid ?? 0,
      reference: reference?.valid ?? 0,
      score: null as number | null,
      reason: 'missing',
    }
    if (!current || !reference) return result
    if (
      ![1, 2].includes(a!.version) ||
      ![1, 2].includes(b!.version) ||
      !current.profile ||
      current.profile !== reference.profile
    )
      return { ...result, reason: 'profile' }
    if (!usableCell(current) || !usableCell(reference))
      return { ...result, reason: 'samples' }
    let divergence = 0
    for (const key of new Set([
      ...Object.keys(current.counts),
      ...Object.keys(reference.counts),
    ])) {
      const p = (current.counts[key] ?? 0) / current.valid
      const q = (reference.counts[key] ?? 0) / reference.valid
      const m = (p + q) / 2
      if (p > 0) divergence += 0.5 * p * Math.log2(p / m)
      if (q > 0) divergence += 0.5 * q * Math.log2(q / m)
    }
    return {
      ...result,
      reason: 'comparable',
      score: 100 * Math.max(0, Math.min(1, 1 - divergence)),
    }
  })
  const scores = cells.flatMap((cell) =>
    cell.score === null ? [] : [cell.score]
  )
  return {
    cells,
    count: scores.length,
    complete: scores.length === CATEGORIES.length,
    score: scores.length
      ? Math.round(scores.reduce((a, b) => a + b, 0) / scores.length)
      : null,
  }
}

function usablePerformance(sample: ClaudeCheckSample) {
  return (
    PERFORMANCE_PROBES.includes(sample.probe) &&
    sample.valid_response &&
    !!sample.request_profile &&
    Number.isFinite(sample.duration_ms) &&
    sample.duration_ms > 0 &&
    Number.isFinite(sample.usage.output_tokens) &&
    (sample.usage.output_tokens ?? 0) >= 64 &&
    (sample.stream?.text_events ?? 0) >= 2 &&
    sample.stream?.first_text_ms != null &&
    Number.isFinite(sample.stream.first_text_ms) &&
    sample.stream.first_text_ms >= 0 &&
    sample.stream.first_text_ms <= sample.duration_ms
  )
}
function metrics(samples: ClaudeCheckSample[]) {
  return {
    count: samples.length,
    ttft: median(samples.map((s) => s.stream!.first_text_ms!)),
    rate: median(
      samples.map((s) => (1000 * s.usage.output_tokens!) / s.duration_ms)
    ),
  }
}
export function focusedPerformance(samples: ClaudeCheckSample[]) {
  return metrics(
    PERFORMANCE_PROBES.flatMap((probe) => {
      const sample = samples.find(
        (s) => s.probe === probe && usablePerformance(s)
      )
      return sample ? [sample] : []
    })
  )
}

export function focusedComparison(
  report: ClaudeCheckReport,
  baseline: ComparisonBaseline
) {
  const fingerprint = focusedFingerprint(
    report.fingerprint,
    baseline.fingerprint
  )
  const benchmark = capabilityPairSummary(report.benchmark, baseline.benchmark)
  const questions = benchmark.questions
  const pairs = PERFORMANCE_PROBES.flatMap((probe) => {
    const a = report.samples.find(
      (s) => s.probe === probe && usablePerformance(s)
    )
    const b = baseline.samples.find(
      (s) =>
        s.probe === probe &&
        usablePerformance(s) &&
        s.request_profile === a?.request_profile
    )
    return a && b ? [{ a, b }] : []
  })
  const current = metrics(pairs.map((p) => p.a)),
    reference = metrics(pairs.map((p) => p.b))
  const savedTolerance = report.options?.performance_tolerance
  const tolerance =
    savedTolerance != null &&
    Number.isFinite(savedTolerance) &&
    savedTolerance >= 0 &&
    savedTolerance <= 100
      ? savedTolerance
      : 25
  const factor = 1 + tolerance / 100
  const ttftLimit = reference.ttft === null ? null : reference.ttft * factor
  const rateLimit = reference.rate === null ? null : reference.rate / factor
  const complete = pairs.length === 3
  const ttftMet =
    complete && current.ttft !== null && ttftLimit !== null
      ? current.ttft <= ttftLimit
      : null
  const rateMet =
    complete && current.rate !== null && rateLimit !== null
      ? current.rate >= rateLimit
      : null
  const score =
    ttftMet === null || rateMet === null ? null : meanScore([ttftMet, rateMet])
  return {
    baseline,
    fingerprint,
    questions,
    benchmark,
    performance: {
      count: pairs.length,
      current,
      reference,
      tolerance,
      ttftLimit,
      rateLimit,
      ttftMet,
      rateMet,
      score,
    },
  }
}

export function focusedComparisons(report: ClaudeCheckReport) {
  return (report.baselines ?? []).map((baseline) =>
    focusedComparison(report, baseline)
  )
}
export function closestFocusedTypes(report: ClaudeCheckReport) {
  const complete = focusedComparisons(report).filter(
    (row) => row.fingerprint.complete && row.fingerprint.score !== null
  )
  if (!complete.length) return []
  const top = Math.max(...complete.map((row) => row.fingerprint.score!))
  return [
    ...new Set(
      complete
        .filter((row) => top - row.fingerprint.score! < 5)
        .map((row) => row.baseline.type)
    ),
  ]
}
