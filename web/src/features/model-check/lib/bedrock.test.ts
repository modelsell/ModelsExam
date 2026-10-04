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
import type {
  ClaudeCheckOptions,
  ClaudeCheck,
  ClaudeCheckReport,
} from '../types'
import { createCheckPlan, getCheckPlan, requestBudget } from './check-plan'
import { summarizeScore } from './report-score'

const base: ClaudeCheckOptions = {
  suite: 'focused',
  model: 'claude-opus-5',
  cache: true,
  thinking: false,
  fingerprint: true,
}
test('current Bedrock diagnostics include an optional signature rejection probe', () => {
  const options = { ...base, bedrock: true }
  const plan = createCheckPlan(options)
  assert.deepEqual(
    plan.find((item) => item.id === 'bedrock_signature'),
    {
      id: 'bedrock_signature',
      stage: 'bedrock',
      kind: 'observation',
      selected: true,
    }
  )
  assert.equal(plan.filter((item) => item.stage === 'bedrock').length, 8)
  assert.equal(requestBudget(options) - requestBudget(base), 8)
  assert.equal(requestBudget(options), 31)
  assert.ok(
    !createCheckPlan(base).some((item) => item.id === 'bedrock_signature')
  )
  const check: ClaudeCheck = {
    id: 'bedrock_signature',
    status: 'pass',
    code: 'bedrock_expected_rejection',
  }
  assert.equal(summarizeScore(plan, [check]).assessed, 0)
})

test('v19 reports do not gain a signature check or a larger request budget', () => {
  const options = { ...base, bedrock: true }
  const report = { version: 19, options } as ClaudeCheckReport
  assert.equal(requestBudget(options, 19), 30)
  assert.ok(
    !getCheckPlan(report).some((item) => item.id === 'bedrock_signature')
  )
  report.version = 20
  report.plan = createCheckPlan(options, 19)
  assert.deepEqual(getCheckPlan(report), report.plan)
})

test('historical Bedrock diagnostics retain their optional budgets and plans', () => {
  assert.equal(requestBudget(base, 16), 39)
  assert.equal(requestBudget({ ...base, bedrock: true }, 16), 46)
  assert.equal(
    createCheckPlan(base, 16).filter((item) => item.stage === 'bedrock').length,
    0
  )
  const plan = createCheckPlan({ ...base, bedrock: true }, 16)
  assert.equal(plan.filter((item) => item.stage === 'bedrock').length, 7)
  assert.deepEqual(
    plan.filter((item) => item.stage === 'bedrock').map((item) => item.id),
    [
      'bedrock_role',
      'bedrock_beta',
      'bedrock_web_search',
      'bedrock_web_fetch',
      'bedrock_code_execution',
      'bedrock_advisor',
      'bedrock_sampling',
    ]
  )
  assert.equal(
    requestBudget(
      {
        ...base,
        bedrock: true,
        prompt_audit: true,
        vision: true,
        pdf: true,
      },
      16
    ),
    51
  )
})
test('saved history keeps its original server-tool scope', () => {
  const report = {
    options: { ...base, bedrock: true },
    plan: [
      {
        id: 'bedrock_server_tools',
        stage: 'bedrock',
        kind: 'observation',
        selected: true,
      },
    ],
  } as ClaudeCheckReport
  assert.deepEqual(getCheckPlan(report), report.plan)
})
test('expected Bedrock rejection points do not inflate model scores', () => {
  const plan = createCheckPlan({ ...base, bedrock: true }, 16)
  const checks: ClaudeCheck[] = [
    { id: 'basic', status: 'fail', code: 'invalid_response' },
    ...plan
      .filter((item) => item.stage === 'bedrock')
      .map(
        (item): ClaudeCheck => ({
          id: item.id,
          status: 'pass',
          code: 'bedrock_expected_rejection',
        })
      ),
  ]
  assert.equal(summarizeScore(plan, checks).value, 0)
  assert.equal(summarizeScore(plan, checks).assessed, 1)
})
