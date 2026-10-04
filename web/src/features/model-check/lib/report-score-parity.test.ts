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
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import type { ClaudeCheckReport } from '../types'
import { presentReport } from './report-presentation'
import { sheetAssessment } from './report-sheet'

// This is also consumed by pkg/claudecheck/report_score_test.go so the history
// endpoint and the visible report cannot silently drift to different formulas.
const fixtures = JSON.parse(
  readFileSync(
    new URL(
      '../../../../../pkg/claudecheck/testdata/report_scores.json',
      import.meta.url
    ),
    'utf8'
  )
) as Array<{
  name: string
  report: Partial<ClaudeCheckReport>
  score: number | null
}>

assert.ok(fixtures.length > 0)
for (const fixture of fixtures) {
  test(`report score parity: ${fixture.name}`, () => {
    const report: ClaudeCheckReport = {
      version: 14,
      id: 'score-fixture',
      model: 'claude-opus-5',
      channel_id: 0,
      channel_name: 'Score fixture',
      transport: 'anthropic_proxy',
      started_at: '2026-09-16T12:00:00Z',
      duration_ms: 0,
      cancelled: false,
      summary: { pass: 0, fail: 0, skipped: 0, inconclusive: 0 },
      samples: [],
      checks: [],
      ...fixture.report,
    }
    assert.equal(sheetAssessment(presentReport(report)).score, fixture.score)
  })
}
