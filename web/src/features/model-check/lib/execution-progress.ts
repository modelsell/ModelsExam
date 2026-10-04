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
import type { CheckPlanItem, ClaudeCheck, ClaudeCheckReport } from '../types'

export function executionProgress(
  plan: CheckPlanItem[],
  checks: ClaudeCheck[]
) {
  const selected = plan.filter(
    (item) => item.selected && item.kind !== 'boundary'
  )
  const results = checks.filter((check) =>
    selected.some((item) => item.id === check.id)
  )
  const skipped = results.filter((check) => check.status === 'skipped').length
  return { total: selected.length, executed: results.length - skipped, skipped }
}

// Older reports did not persist termination metadata. Describe only recorded
// baseline errors; do not change their checks or retroactively rescore them.
export function reportStopReason(
  report?: ClaudeCheckReport | null
): string | null {
  if (!report) return null
  if (report.stop_reason) return report.stop_reason
  if (report.cancelled) return 'cancelled'
  if (!report.checks.some((check) => check.code === 'baseline_failed'))
    return null
  const sample = report.samples.find((sample) => sample.probe === 'basic')
  if (!sample) return 'baseline_unavailable'
  if (sample.error_code) return sample.error_code
  if (/deadline exceeded|timed? ?out/i.test(sample.error ?? ''))
    return 'probe_timeout'
  if (/no available channel for model/i.test(sample.error ?? ''))
    return 'route_unavailable'
  if ([401, 402, 403].includes(sample.http_status)) return 'access_denied'
  if (sample.http_status >= 500) return 'upstream_unavailable'
  return 'baseline_unavailable'
}
