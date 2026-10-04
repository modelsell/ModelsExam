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
import type { BaselineListItem, ClaudeCheckReport } from '../types'

function declaredModel(raw: string) {
  let model = raw.trim().toLowerCase()
  if (model.startsWith('arn:')) {
    const arn =
      /^arn:[^:]+:bedrock:[^:]*:[^:]*:(?:foundation-model|inference-profile)\/(.+)$/.exec(
        model
      )
    if (!arn) return null
    model = arn[1]
  }
  model = model.replace(
    /^(?:global|us|eu|apac|au|jp)\.anthropic\./,
    'anthropic.'
  )
  let revision = ''
  if (model.startsWith('anthropic.')) {
    model = model.slice('anthropic.'.length)
    const suffix = /-v(\d+(?::\d+)?)$/.exec(model)
    if (suffix) {
      revision = suffix[1]
      model = model.slice(0, -suffix[0].length)
    }
  }
  let parts: string[] | null =
    /^claude-(opus|sonnet|haiku|fable|mythos)-(\d+)(?:-(\d{1,2}))?(?:-(\d{8}))?$/.exec(
      model
    )
  if (!parts) {
    const old =
      /^claude-(\d+)(?:-(\d{1,2}))?-(opus|sonnet|haiku)(?:-(\d{8}))?$/.exec(
        model
      )
    if (old) parts = [old[0], old[3], old[1], old[2], old[4]]
  }
  if (!parts) return null
  const [, name, major, minor = '', date = ''] = parts
  if (date) {
    const value = `${date.slice(0, 4)}-${date.slice(4, 6)}-${date.slice(6, 8)}`
    const parsed = new Date(`${value}T00:00:00Z`)
    if (
      Number.isNaN(parsed.valueOf()) ||
      parsed.toISOString().slice(0, 10) !== value
    )
      return null
  }
  return { name, major, minor, date, revision }
}

// Match the server's declared-ID rules, without treating name compatibility
// as proof of model identity. Unknown aliases require exact matching.
export function comparisonModelsMatch(left: string, right: string): boolean {
  if (left.trim() && left.trim().toLowerCase() === right.trim().toLowerCase())
    return true
  const a = declaredModel(left),
    b = declaredModel(right)
  if (
    !a ||
    !b ||
    a.name !== b.name ||
    a.major !== b.major ||
    a.minor !== b.minor
  )
    return false
  if (a.revision && b.revision && a.revision !== b.revision) return false
  if (a.date !== b.date) {
    if (a.date && b.date) return false
    return (
      a.major === '3' ||
      (a.major === '4' && a.minor !== '' && Number(a.minor) <= 5)
    )
  }
  return true
}

export function baselineCandidates(
  items: BaselineListItem[],
  model: string,
  type?: string
): BaselineListItem[] {
  return items
    .filter(
      (item) =>
        comparisonModelsMatch(model, item.model) &&
        (!type || item.type === type)
    )
    .sort((a, b) => b.created_at - a.created_at || a.id.localeCompare(b.id))
}

export function baselineSelectionOptions(item?: BaselineListItem) {
  if (!item)
    return { compare_baselines: false, baseline_id: '', baseline_type: '' }
  return {
    compare_baselines: true,
    baseline_id: item.id,
    baseline_type: item.type,
  }
}

export function selectedReportBaseline(report: ClaudeCheckReport) {
  const id = report.options?.baseline_id
  if (!id) return undefined
  const baseline = report.baselines?.find((item) => item.id === id)
  if (!baseline || !comparisonModelsMatch(report.model, baseline.model))
    return undefined
  const type = report.options?.baseline_type?.trim().toLowerCase()
  if (type && type !== baseline.type.trim().toLowerCase()) return undefined
  return baseline
}
