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
  CheckEvent,
  CheckPlanItem,
  ClaudeCheck,
  ClaudeCheckReport,
} from '../types'

// Only legacy wire identifiers live here. Keep persisted scores and evidence
// intact while presenting historical runs in the current report vocabulary.
const ids: Record<string, string> = {
  veridrop_pdf: 'pdf',
  veridrop_tool: 'structured_tool',
  veridrop_integrity: 'stream_comparison',
  veridrop_integrity_nonstream: 'comparison_nonstream',
  veridrop_integrity_stream: 'comparison_stream',
  veridrop_baseline: 'reference_baseline',
}
const codes: Record<string, string> = {
  veridrop_observed: 'semantic_observed',
  veridrop_unavailable: 'probe_unavailable',
  veridrop_invalid_response: 'message_invalid',
  veridrop_reference_only: 'reference_only',
  veridrop_no_baseline: 'no_matching_reference',
}
export const presentProbe = (id: string): string => ids[id] ?? id

export function presentPlanItem(item: CheckPlanItem): CheckPlanItem {
  if (!ids[item.id] && item.stage !== 'veridrop') return item
  const id = presentProbe(item.id)
  return {
    ...item,
    id,
    stage:
      id === 'pdf'
        ? 'capabilities'
        : id === 'structured_tool'
          ? 'protocol'
          : 'reliability',
  }
}

function presentCheck(check: ClaudeCheck): ClaudeCheck {
  if (!ids[check.id] && !codes[check.code]) return check
  const id = presentProbe(check.id)
  // Historical attribution remains in the stored raw report and source notice.
  // The compatibility report shows only the applicable model reference.
  const evidence =
    id === 'reference_baseline' && check.evidence
      ? {
          mapped_model: check.evidence.mapped_model,
          matched_model: check.evidence.matched_model,
        }
      : check.evidence
  return { ...check, id, code: codes[check.code] ?? check.code, evidence }
}

export function presentReport(report: ClaudeCheckReport): ClaudeCheckReport {
  if (
    !report.reference &&
    report.options?.veridrop === undefined &&
    !report.checks.some((c) => ids[c.id] || codes[c.code]) &&
    !report.plan?.some((p) => ids[p.id] || p.stage === 'veridrop') &&
    !report.samples.some((s) => ids[s.probe])
  )
    return report
  const result = { ...report }
  delete result.reference
  if (report.options) {
    result.options = { ...report.options }
    if (report.options.veridrop) {
      result.options.pdf = true
      result.options.stream_comparison = true
    }
    delete result.options.veridrop
  }
  result.checks = report.checks.map(presentCheck)
  result.plan = report.plan?.map(presentPlanItem)
  result.samples = report.samples.map((sample) => ({
    ...sample,
    probe: presentProbe(sample.probe),
  }))
  if (result.stop_probe) result.stop_probe = presentProbe(result.stop_probe)
  return result
}

export function presentEvent(event: CheckEvent): CheckEvent {
  if (event.type === 'start' || event.type === 'done')
    return { ...event, report: presentReport(event.report) }
  if (event.type === 'probe_start')
    return { ...event, probe: presentProbe(event.probe) }
  if (event.type === 'sample')
    return {
      ...event,
      sample: { ...event.sample, probe: presentProbe(event.sample.probe) },
    }
  if (event.type === 'check')
    return { ...event, check: presentCheck(event.check) }
  return event
}
