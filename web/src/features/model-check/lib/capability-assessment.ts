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
import type { CapabilityBenchmark } from '../types'

export const LEGACY_CAPABILITY_IDS = [
  'digit_count',
  'boolean_count',
  'python_alias',
  'javascript_queue',
  'zh_constraint',
  'context_retrieval',
]
export const CAPABILITY_GROUPS = [
  { id: 'reasoning', questions: ['digit_count', 'boolean_count'] },
  { id: 'code', questions: ['python_alias', 'javascript_queue'] },
  { id: 'instruction', questions: ['zh_constraint', 'instruction_format'] },
  { id: 'structured', questions: ['json_extraction', 'json_types'] },
  { id: 'context', questions: ['context_retrieval', 'context_multihop'] },
  { id: 'tools', questions: ['tool_selection', 'tool_roundtrip'] },
]
export const CAPABILITY_IDS = CAPABILITY_GROUPS.flatMap(
  (group) => group.questions
)
export const CURRENT_CAPABILITY_IDS = [
  'instruction_format',
  'json_extraction',
  'json_types',
  'context_multihop',
  'tool_selection',
  'tool_roundtrip',
]
function capabilityIDs(version?: number): string[] {
  if (version === 1) return LEGACY_CAPABILITY_IDS
  if (version === 2) return CAPABILITY_IDS
  if (version === 3) return CURRENT_CAPABILITY_IDS
  return []
}
type Item = CapabilityBenchmark['items'][number]
const average = (values: number[]) =>
  values.length
    ? Math.round(values.reduce((sum, value) => sum + value, 0) / values.length)
    : null

export function capabilityItemScore(
  version: number,
  item?: Item
): number | null {
  if (!item) return null
  if (version === 1 && LEGACY_CAPABILITY_IDS.includes(item.id))
    return typeof item.correct === 'boolean' ? (item.correct ? 100 : 0) : null
  if ([2, 3].includes(version) && capabilityIDs(version).includes(item.id))
    return typeof item.score === 'number' &&
      Number.isFinite(item.score) &&
      item.score >= 0 &&
      item.score <= 100
      ? item.score
      : null
  return null
}

// Stable question IDs are the denominator, so sparse or interrupted reports do
// not inflate coverage. Ignore unknown and duplicate rows in saved payloads.
export function capabilitySummary(benchmark?: CapabilityBenchmark) {
  const ids = capabilityIDs(benchmark?.version)
  const rows = ids.map((id) => {
    const item = benchmark?.items.find((candidate) => candidate.id === id)
    return {
      id,
      item,
      score: capabilityItemScore(benchmark?.version ?? 0, item),
    }
  })
  const summarize = (items: typeof rows) => {
    const values = items.flatMap((row) =>
      row.score === null ? [] : [row.score]
    )
    return {
      count: values.length,
      total: items.length,
      score: average(values),
      coverage: items.length
        ? Math.round((100 * values.length) / items.length)
        : 0,
    }
  }
  return {
    ...summarize(rows),
    rows,
    groups: CAPABILITY_GROUPS.flatMap((group) => {
      const items = rows.filter((row) => group.questions.includes(row.id))
      return items.length ? [{ id: group.id, ...summarize(items) }] : []
    }),
  }
}

export function capabilityPairs(
  current?: CapabilityBenchmark,
  baseline?: CapabilityBenchmark
) {
  if (
    !current ||
    !baseline ||
    ![1, 2, 3].includes(current.version) ||
    ![1, 2, 3].includes(baseline.version)
  )
    return []
  const referenceIDs = capabilityIDs(baseline.version)
  const ids = capabilityIDs(current.version).filter((id) =>
    referenceIDs.includes(id)
  )
  return ids.flatMap((id) => {
    const a = current.items.find((item) => item.id === id)
    const b = baseline.items.find((item) => item.id === id)
    const aScore = capabilityItemScore(current.version, a)
    const bScore = capabilityItemScore(baseline.version, b)
    const aGrader = current.version === 1 ? 'exact-v1' : a?.grader
    const bGrader = baseline.version === 1 ? 'exact-v1' : b?.grader
    if (
      !a ||
      !b ||
      !a.profile ||
      a.profile !== b.profile ||
      !aGrader ||
      aGrader !== bGrader ||
      aScore === null ||
      bScore === null
    )
      return []
    return [
      { id, current: aScore, reference: bScore, equal: aScore === bScore },
    ]
  })
}

export function capabilityPairSummary(
  current?: CapabilityBenchmark,
  baseline?: CapabilityBenchmark
) {
  const questions = capabilityPairs(current, baseline)
  return {
    questions,
    count: questions.length,
    current: average(questions.map((q) => q.current)),
    reference: average(questions.map((q) => q.reference)),
    differences: questions.filter((q) => !q.equal),
  }
}
