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
import type { ClaudeCheckReport } from '../types'
import { compareBaselines } from './baseline-comparison'
import { selectedReportBaseline } from './baseline-selection'
import { focusedComparison } from './focused-assessment'
import { presentReport } from './report-presentation'
import { reportScore } from './report-score'
import { sheetAssessment } from './report-sheet'

export function reportDownloadData(original: ClaudeCheckReport) {
  const report = presentReport(original)
  const sheet = sheetAssessment(report)
  const baseline = selectedReportBaseline(report)
  const comparison = baseline ? focusedComparison(report, baseline) : undefined
  const scoredOnly = report.version >= 17
  return {
    ...report,
    ...(sheet.usageTokens.active
      ? { usage_token_assessment: sheet.usageTokens }
      : {}),
    scoring: scoredOnly
      ? {
          version: 2,
          method: 'measured_dimensions',
          value: sheet.score,
          assessed: sheet.measured,
          eligible: 5,
          unscored: 5 - sheet.measured,
          coverage: 20 * sheet.measured,
        }
      : { version: 1, ...reportScore(report) },
    report_sheet: {
      version: scoredOnly ? 2 : 1,
      score: sheet.score,
      scored_dimensions: sheet.measured,
      dimensions: sheet.dimensions,
      identity_verified: false,
      timing: sheet.timing,
      first_event: sheet.firstEvent,
      cache_reads: sheet.warm,
      ...(!scoredOnly ? { aws_counters: sheet.aws } : {}),
    },
    baseline_comparison: scoredOnly
      ? {
          version: 2,
          selected:
            baseline && comparison
              ? {
                  baseline_id: baseline.id,
                  type: baseline.type,
                  model: baseline.model,
                  capability: comparison.benchmark,
                  performance: comparison.performance,
                }
              : null,
        }
      : {
          version: 1,
          candidates: compareBaselines(report).map(
            ({ baseline: candidate, ...values }) => ({
              baseline_id: candidate.id,
              type: candidate.type,
              model: candidate.model,
              ...values,
            })
          ),
        },
  }
}
