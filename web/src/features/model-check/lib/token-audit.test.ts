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
import type { ClaudeCheckReport, TokenAuditReport } from '../types'
import {
  createCheckPlan,
  isCountProbe,
  probeCheckID,
  requestBudget,
  samplesForCheck,
} from './check-plan'
import { readCheckStream } from './event-stream'
import { reportMetrics } from './report-metrics'
import { applyCheckEvent, INITIAL_RUN } from './run-state'

const report: ClaudeCheckReport = {
  version: 8,
  id: 'audit-test',
  model: 'claude-test',
  channel_id: 0,
  channel_name: '',
  transport: 'anthropic_proxy',
  started_at: '',
  duration_ms: 0,
  cancelled: false,
  summary: { pass: 0, fail: 0, skipped: 0, inconclusive: 0 },
  checks: [],
  samples: [],
}
const audit: TokenAuditReport = {
  version: 1,
  prompt: [
    {
      id: 'short',
      expected: 100,
      actual: 125,
      difference: 25,
      score: 80,
      code: 'tokens_differ',
    },
  ],
  cache: [],
}

test('token audit progress, exported history and signed deltas stay intact', async () => {
  let state = INITIAL_RUN
  const events = [
    { type: 'start', report },
    { type: 'token_audit', token_audit: audit },
    { type: 'done', report: { ...report, token_audit: audit } },
  ]
  const bytes = new TextEncoder().encode(
    events.map((e) => `data: ${JSON.stringify(e)}\n\n`).join('')
  )
  await readCheckStream(
    new ReadableStream({
      start(controller) {
        controller.enqueue(bytes)
        controller.close()
      },
    }),
    (event) => {
      state = applyCheckEvent(state, event)
    }
  )
  assert.equal(state.report?.token_audit?.prompt[0].score, 80)
  assert.equal(state.report?.token_audit?.prompt[0].difference, 25)
  assert.equal(report.token_audit, undefined)
})

test('count requests stay out of inference totals and map to the prompt check', () => {
  const sample = {
    probe: 'prompt_audit_short',
    http_status: 200,
    valid_response: true,
    duration_ms: 10,
    usage: {
      input_tokens: 100,
      output_tokens: 2,
      cache_creation_input_tokens: 0,
      cache_read_input_tokens: 0,
    },
  }
  const r = {
    ...report,
    samples: [
      sample,
      {
        ...sample,
        probe: 'prompt_audit_short_count',
        usage: {
          input_tokens: null,
          output_tokens: null,
          cache_creation_input_tokens: null,
          cache_read_input_tokens: null,
        },
      },
    ],
  }
  assert.equal(reportMetrics(r.samples).input, 100)
  assert.equal(isCountProbe(r.samples[1].probe), true)
  assert.equal(probeCheckID(r.samples[1].probe), 'prompt_integrity')
  assert.equal(samplesForCheck(r, 'prompt_integrity').length, 2)
  assert.equal(samplesForCheck(r, 'usage_fields').length, 1)
})

test('legacy prompt requests preserve their budgets and report plans', () => {
  const options = {
    model: 'claude-test',
    cache: true,
    thinking: true,
    pdf: true,
    stream_comparison: true,
    prompt_audit: true,
  }
  assert.equal(requestBudget(options), 23)
  assert.equal(requestBudget(options, 11), 23)
  assert.equal(
    requestBudget({
      ...options,
      repeat: true,
      vision: true,
      fingerprint: true,
      benchmark: true,
      performance: true,
    }),
    37
  )
  assert.equal(
    createCheckPlan(options, 7).some((i) => i.id === 'cache_token_audit'),
    false
  )
  assert.equal(
    createCheckPlan(options, 8).find((i) => i.id === 'cache_token_audit')?.kind,
    'boundary'
  )
})
