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
import { test } from 'node:test'
import type { ClaudeCheck } from '../types'
import { createCheckPlan } from './check-plan'
import { checkScore, scoreFromCounts, summarizeScore } from './report-score'

const plan = createCheckPlan({ model: 'claude', thinking: false, cache: false })
const result = (id: string, status: ClaudeCheck['status']): ClaudeCheck => ({
  id,
  status,
  code: 'observed',
})

test('scores preserve zero and exclude unknown, unselected, and boundary results', () => {
  const checks = [
    result('basic', 'pass'),
    result('system', 'fail'),
    result('token_count', 'inconclusive'),
    result('stream', 'skipped'),
    result('thinking', 'pass'),
    result('billing', 'pass'),
  ]
  const summary = summarizeScore(plan, checks)
  assert.equal(summary.value, 50)
  assert.equal(summary.assessed, 2)
  assert.equal(
    summary.eligible,
    plan.filter((item) => item.selected && item.kind !== 'boundary').length
  )
  assert.equal(summary.unscored, summary.eligible - 2)
  assert.equal(checkScore(result('basic', 'fail')), 0)
  assert.equal(checkScore(result('basic', 'inconclusive')), null)
  assert.equal(scoreFromCounts(0, 1), 0)
  assert.equal(scoreFromCounts(14, 1), 93)
})

test('empty and partial runs never imply full scoring coverage', () => {
  const empty = summarizeScore(plan, [])
  assert.equal(empty.value, null)
  assert.equal(empty.coverage, 0)
  assert.equal(scoreFromCounts(0, 0), null)
  const partial = summarizeScore(plan, [result('basic', 'pass')])
  assert.equal(partial.value, 100)
  assert.ok(partial.coverage !== null && partial.coverage < 100)
  assert.equal(partial.assessed, 1)
  const boundary = summarizeScore(
    plan.filter((item) => item.kind === 'boundary'),
    [result('billing', 'inconclusive')]
  )
  assert.equal(boundary.value, null)
  assert.equal(boundary.coverage, null)
})
