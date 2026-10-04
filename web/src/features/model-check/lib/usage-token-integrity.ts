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
import type { ClaudeCheckReport, ClaudeCheckSample } from '../types'
import { selectedReportBaseline } from './baseline-selection'
import { median } from './report-metrics'

export const USAGE_TOKEN_CASES = [
  { probe: 'usage_tokens_en_30', words: 30 },
  { probe: 'usage_tokens_en_300', words: 300 },
  { probe: 'usage_tokens_en_1500', words: 1500 },
  { probe: 'usage_tokens_zh_1000', characters: 1000 },
] as const

export function isUsageTokenProbe(probe: string): boolean {
  return USAGE_TOKEN_CASES.some((item) => item.probe === probe)
}

export function usageTokenCounters(sample?: ClaudeCheckSample) {
  const empty = (code: string) => ({
    code,
    input: null,
    cache_creation: null,
    cache_read: null,
    total_input: null,
  })
  if (!sample) return empty('not_collected')
  if (sample.http_status !== 200 || sample.valid_response !== true)
    return empty('invalid_response')
  if (sample.usage?.input_tokens == null) return empty('missing_input')
  const input = sample.usage.input_tokens
  const cache_creation = sample.usage.cache_creation_input_tokens ?? 0
  const cache_read = sample.usage.cache_read_input_tokens ?? 0
  if (
    ![input, cache_creation, cache_read].every(
      (value) => Number.isSafeInteger(value) && value >= 0
    )
  )
    return empty('invalid_usage')
  const total_input = input + cache_creation + cache_read
  if (!Number.isSafeInteger(total_input)) return empty('usage_overflow')
  if (total_input <= 0) return empty('nonpositive_input')
  return {
    code: 'observed',
    input,
    cache_creation: sample.usage.cache_creation_input_tokens ?? null,
    cache_read: sample.usage.cache_read_input_tokens ?? null,
    total_input,
  }
}

export function usageTokenBaseline(report: ClaudeCheckReport) {
  const baselineID = report.options?.baseline_id
  const candidates =
    report.baselines?.filter((item) => item.id === baselineID) ?? []
  if (!report.options?.baseline_type?.trim() || candidates.length !== 1)
    return undefined
  return selectedReportBaseline({ ...report, baselines: candidates })
}

export function assessUsageTokens(report: ClaudeCheckReport) {
  const active =
    report.version >= 19 &&
    report.options?.suite === 'focused' &&
    (report.plan?.some(
      (item) => item.id === 'usage_token_integrity' && item.selected
    ) || report.samples.some((sample) => isUsageTokenProbe(sample.probe))) ===
      true
  const baselineID = report.options?.baseline_id
  const baseline = usageTokenBaseline(report)
  const rows = USAGE_TOKEN_CASES.map((definition) => {
    const matches = report.samples.filter(
      (sample) => sample.probe === definition.probe
    )
    const sample = matches.length === 1 ? matches[0] : undefined
    const counters = usageTokenCounters(sample)
    if (matches.length > 1) counters.code = 'ambiguous_sample'
    const candidates =
      baseline?.samples.filter((item) => item.probe === definition.probe) ?? []
    const references = candidates
      .filter(
        (item) =>
          counters.total_input != null &&
          !!sample?.request_profile?.trim() &&
          item.request_profile === sample.request_profile
      )
      .flatMap((item) => {
        const value = usageTokenCounters(item).total_input
        return value != null ? [value] : []
      })
    const reference_input = median(references)
    const diff =
      reference_input != null && counters.total_input != null
        ? counters.total_input - reference_input
        : null
    const diff_percent =
      diff != null && reference_input != null && reference_input > 0
        ? (100 * diff) / reference_input
        : null
    let baseline_score: number | null = null
    if (
      diff != null &&
      reference_input != null &&
      counters.total_input != null
    ) {
      baseline_score =
        Math.abs(diff) <= Math.max(4, 0.05 * reference_input)
          ? 100
          : Math.round(
              (100 * Math.min(counters.total_input, reference_input)) /
                Math.max(counters.total_input, reference_input)
            )
    }
    let reference_code = 'matched'
    if (!baselineID) reference_code = 'not_selected'
    else if (!baseline) reference_code = 'baseline_unavailable'
    else if (!candidates.length) reference_code = 'no_sample'
    else if (
      !sample?.request_profile?.trim() ||
      !candidates.some(
        (item) => item.request_profile === sample.request_profile
      )
    )
      reference_code = 'parameters_differ'
    else if (reference_input == null)
      reference_code = 'reference_usage_unavailable'
    return {
      ...definition,
      ...counters,
      reference_input,
      diff,
      diff_percent,
      baseline_score,
      reference_code,
      reference_samples: references.length,
    }
  })
  const measured = rows.filter((row) => row.total_input != null).length
  let growth_score: number | null = null
  let slope_ratio: number | null = null
  let long_short_ratio: number | null = null
  const [short, medium, long] = rows.map((row) => row.total_input)
  if (measured === 4 && short != null && long != null && short > 0)
    long_short_ratio = long / short
  if (measured === 4 && short != null && medium != null && long != null) {
    if (short >= medium || medium >= long) growth_score = 0
    else {
      const firstSlope = (medium - short) / 270
      const secondSlope = (long - medium) / 1200
      slope_ratio =
        Math.min(firstSlope, secondSlope) / Math.max(firstSlope, secondSlope)
      growth_score =
        slope_ratio >= 0.8 ? 100 : Math.round((100 * slope_ratio) / 0.8)
    }
  }
  let baseline_code = 'baseline_incomplete'
  if (!baselineID) baseline_code = 'not_selected'
  else if (!baseline) baseline_code = 'baseline_unavailable'
  const matched = rows.flatMap((row) =>
    row.baseline_score == null ? [] : [row.baseline_score]
  )
  let baseline_score: number | null = null
  if (matched.length === 4) {
    baseline_code = 'baseline_complete'
    baseline_score = Math.round(
      matched.reduce((sum, score) => sum + score, 0) / 4
    )
  }
  let score = growth_score
  if (score != null && baseline_score != null)
    score = Math.min(score, baseline_score)
  let code = 'usage_incomplete'
  if (!active) code = 'not_applicable'
  else if (score != null)
    code = score === 100 ? 'usage_consistent' : 'usage_anomaly'
  return {
    active,
    code,
    score: active ? score : null,
    growth_score: active ? growth_score : null,
    baseline_score: active ? baseline_score : null,
    measured: active ? measured : 0,
    total: 4,
    slope_ratio: active ? slope_ratio : null,
    long_short_ratio: active ? long_short_ratio : null,
    baseline_code: active ? baseline_code : 'not_selected',
    rows: active ? rows : [],
  }
}
