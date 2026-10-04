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
  CapabilityBenchmark,
  ClaudeCheckReport,
  ComparisonBaseline,
  ClaudeCheckSample,
} from '../types'
import { capabilitySummary, capabilityPairs } from './capability-assessment'
import { isCountProbe } from './check-plan'
import { checkScore } from './report-score'

const PROTOCOL_CHECKS = new Set([
  'system',
  'stream',
  'tool',
  'max_tokens',
  'stop_sequence',
  'multi_turn',
  'zero_output',
  'vision',
  'pdf',
  'thinking',
  'thinking_stream',
  'signature_replay',
  'cache',
  'stream_comparison',
  'stream_stop_reason',
])

export function benchmarkAccuracy(benchmark?: CapabilityBenchmark) {
  if (benchmark && [2, 3].includes(benchmark.version)) {
    const summary = capabilitySummary(benchmark)
    return {
      assessed: summary.count,
      total: summary.total,
      score: summary.score,
    }
  }
  const assessed =
    benchmark?.items.filter((item) => typeof item.correct === 'boolean') ?? []
  return {
    assessed: assessed.length,
    total: benchmark?.items.length ?? 12,
    score: assessed.length
      ? Math.round(
          (100 * assessed.filter((item) => item.correct).length) /
            assessed.length
        )
      : null,
  }
}

// Base-2 Jensen-Shannon divergence is bounded in [0,1]. The displayed value
// is distribution similarity, never a probability that a model is authentic.
export function fingerprintSimilarity(
  a?: BehaviorFingerprint,
  b?: BehaviorFingerprint
): number | null {
  if (
    !a ||
    !b ||
    a.version !== 1 ||
    b.version !== 1 ||
    a.cells.length !== 4 ||
    b.cells.length !== 4
  )
    return null
  let divergence = 0
  for (const cell of a.cells) {
    const other = b.cells.find((item) => item.id === cell.id)
    if (
      !other ||
      !cell.profile ||
      cell.profile !== other.profile ||
      cell.valid < 10 ||
      other.valid < 10
    )
      return null
    if (
      Object.values(cell.counts).reduce((sum, n) => sum + n, 0) !==
        cell.valid ||
      Object.values(other.counts).reduce((sum, n) => sum + n, 0) !== other.valid
    )
      return null
    const keys = new Set([
      ...Object.keys(cell.counts),
      ...Object.keys(other.counts),
    ])
    for (const key of keys) {
      const p = (cell.counts[key] ?? 0) / cell.valid
      const q = (other.counts[key] ?? 0) / other.valid
      const m = (p + q) / 2
      if (p > 0) divergence += 0.5 * p * Math.log2(p / m)
      if (q > 0) divergence += 0.5 * q * Math.log2(q / m)
    }
  }
  return Math.round(
    100 * Math.max(0, Math.min(1, 1 - divergence / a.cells.length))
  )
}

function median(values: number[]): number | null {
  if (!values.length) return null
  const sorted = [...values].sort((a, b) => a - b)
  const mid = Math.floor(sorted.length / 2)
  return sorted.length % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2
}

export function performanceMetrics(samples: ClaudeCheckSample[]) {
  const valid = samples.filter(
    (sample) =>
      sample.probe.startsWith('performance_') &&
      sample.valid_response &&
      sample.request_profile &&
      sample.duration_ms > 0 &&
      (sample.usage.output_tokens ?? 0) >= 64 &&
      (sample.stream?.text_events ?? 0) >= 2
  )
  return {
    count: valid.length,
    rate: median(
      valid.map(
        (sample) => (1000 * sample.usage.output_tokens!) / sample.duration_ms
      )
    ),
    ttft: median(
      valid.flatMap((sample) =>
        sample.stream?.first_text_ms == null
          ? []
          : [sample.stream.first_text_ms]
      )
    ),
  }
}

export function compareBaseline(
  report: ClaudeCheckReport,
  baseline: ComparisonBaseline
) {
  // Version 7 only adds independent collection suites; v6/v7/v8 protocol
  // assertions share rules. Earlier reports are not retroactively reinterpreted.
  const compatible =
    [6, 7, 8].includes(report.version) && [6, 7, 8].includes(baseline.version)
  const protocol = compatible
    ? report.checks.flatMap((check) => {
        const reference = baseline.checks.find((item) => item.id === check.id)
        if (
          !PROTOCOL_CHECKS.has(check.id) ||
          checkScore(check) === null ||
          checkScore(reference) === null
        )
          return []
        return [
          {
            id: check.id,
            current: checkScore(check),
            reference: checkScore(reference),
            equal: check.status === reference?.status,
          },
        ]
      })
    : []
  const benchmark =
    report.benchmark?.version === 1 && baseline.benchmark?.version === 1
      ? report.benchmark.items.flatMap((item) => {
          const other = baseline.benchmark?.items.find(
            (candidate) => candidate.id === item.id
          )
          if (
            !other ||
            !item.profile ||
            item.profile !== other.profile ||
            typeof item.correct !== 'boolean' ||
            typeof other.correct !== 'boolean'
          )
            return []
          return [
            {
              id: item.id,
              current: item.correct ? 100 : 0,
              reference: other.correct ? 100 : 0,
              equal: item.correct === other.correct,
            },
          ]
        })
      : capabilityPairs(report.benchmark, baseline.benchmark)
  const latencyRatios: number[] = []
  const tokenDeltas: number[] = []
  for (const sample of report.samples) {
    if (
      !sample.valid_response ||
      !sample.request_profile ||
      isCountProbe(sample.probe)
    )
      continue
    const reference = baseline.samples.find(
      (item) =>
        item.probe === sample.probe &&
        item.valid_response &&
        item.request_profile === sample.request_profile
    )
    if (!reference) continue
    if (sample.duration_ms > 0 && reference.duration_ms > 0)
      latencyRatios.push(sample.duration_ms / reference.duration_ms)
    const currentInput = sample.usage.input_tokens
    const baselineInput = reference.usage.input_tokens
    if (currentInput !== null && baselineInput !== null)
      tokenDeltas.push(currentInput - baselineInput)
  }
  return {
    baseline,
    fingerprint: fingerprintSimilarity(
      report.fingerprint,
      baseline.fingerprint
    ),
    protocol: {
      count: protocol.length,
      score: protocol.length
        ? Math.round(
            (100 * protocol.filter((item) => item.equal).length) /
              protocol.length
          )
        : null,
      items: protocol,
    },
    benchmark: {
      items: benchmark,
      count: benchmark.length,
      score: benchmark.length
        ? Math.round(
            (100 * benchmark.filter((item) => item.equal).length) /
              benchmark.length
          )
        : null,
    },
    performance: {
      count: latencyRatios.length,
      latencyRatio: latencyRatios.length >= 3 ? median(latencyRatios) : null,
      inputTokenDelta: tokenDeltas.length >= 3 ? median(tokenDeltas) : null,
    },
  }
}

export function compareBaselines(report: ClaudeCheckReport) {
  return (report.baselines ?? [])
    .map((baseline) => compareBaseline(report, baseline))
    .sort(
      (a, b) =>
        (b.fingerprint ?? -1) - (a.fingerprint ?? -1) ||
        (b.protocol.score ?? -1) - (a.protocol.score ?? -1) ||
        a.baseline.id.localeCompare(b.baseline.id)
    )
}

export function nearestBaselineTypes(report: ClaudeCheckReport): string[] {
  const rows = compareBaselines(report).filter(
    (row) => row.fingerprint !== null
  )
  if (!rows.length) return []
  const best = rows[0].fingerprint!
  // A five-point display band retains close alternatives. It is an explicit
  // presentation heuristic, not a statistically calibrated acceptance cutoff.
  return [
    ...new Set(
      rows
        .filter((row) => best - row.fingerprint! < 5)
        .map((row) => row.baseline.type)
    ),
  ]
}
