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
import { getCheckPlan } from './check-plan'
import { presentReport } from './report-presentation'

export function scoreFromCounts(
  passed: number,
  deducted: number
): number | null {
  const assessed = passed + deducted
  if (!assessed) return null
  return Math.round((passed / assessed) * 100)
}

export function checkScore(check?: ClaudeCheck): number | null {
  if (check?.status === 'pass') return 100
  if (check?.status === 'fail') return 0
  return null
}

export function summarizeScore(plan: CheckPlanItem[], checks: ClaudeCheck[]) {
  const eligible = plan.filter(
    (item) =>
      item.selected && item.kind !== 'boundary' && item.stage !== 'bedrock'
  )
  const results = eligible.map((item) =>
    checks.find((check) => check.id === item.id)
  )
  const passed = results.filter((check) => check?.status === 'pass').length
  const deducted = results.filter((check) => check?.status === 'fail').length
  const assessed = passed + deducted
  return {
    value: scoreFromCounts(passed, deducted),
    assessed,
    eligible: eligible.length,
    unscored: eligible.length - assessed,
    coverage: eligible.length
      ? Math.round((assessed / eligible.length) * 100)
      : null,
  }
}

export function reportScore(report: ClaudeCheckReport) {
  report = presentReport(report)
  return summarizeScore(getCheckPlan(report), report.checks)
}
