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
import type { ClaudeCheckReport } from '../types'
import { createCheckPlan, getCheckPlan, requestBudget } from './check-plan'
import { modelIdentityState } from './model-identity'

const report = (version = 6): ClaudeCheckReport => ({
  version,
  id: 'identity',
  model: 'claude-opus-5',
  channel_id: 0,
  channel_name: '',
  transport: 'anthropic_proxy',
  started_at: '',
  duration_ms: 0,
  cancelled: false,
  summary: { pass: 0, fail: 0, inconclusive: 0, skipped: 0 },
  checks: [],
  samples: [],
})

test('name consistency never presents an authenticated model identity', () => {
  const r = report()
  assert.equal(modelIdentityState(r, true), 'pending')
  assert.equal(modelIdentityState(r, false), 'unresolved')
  r.checks = [
    { id: 'model_echo', status: 'fail', code: 'model_declaration_mismatch' },
  ]
  assert.equal(modelIdentityState(r, true), 'mismatch')
  r.checks = [
    {
      id: 'model_consistency',
      status: 'pass',
      code: 'model_declaration_match',
      evidence: { identity_verified: false },
    },
  ]
  assert.equal(modelIdentityState(r), 'consistent')
  r.checks[0].status = 'inconclusive'
  assert.equal(modelIdentityState(r), 'unresolved')
  r.checks[0].status = 'fail'
  assert.equal(modelIdentityState(r), 'mismatch')
  r.version = 5
  assert.equal(modelIdentityState(r), 'legacy')
})

test('identity evidence uses existing requests and preserves historical scope', () => {
  const options = {
    model: 'claude-opus-5',
    cache: true,
    thinking: true,
    pdf: true,
    stream_comparison: true,
  }
  assert.equal(createCheckPlan(options).length, 28)
  assert.ok(
    createCheckPlan(options).some(
      (item) => item.id === 'model_consistency' && item.selected
    )
  )
  assert.equal(requestBudget(options), 17)
  const old = report(5)
  old.options = options
  assert.equal(getCheckPlan(old).length, 26)
  assert.ok(!getCheckPlan(old).some((item) => item.id === 'model_consistency'))
})
