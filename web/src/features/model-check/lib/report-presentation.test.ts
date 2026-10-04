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
import {
  createCheckPlan,
  requestBudget,
  samplesForCheck,
  STAGES,
} from './check-plan'
import { presentReport, presentEvent } from './report-presentation'
import { reportScore } from './report-score'

const legacyReport = (): ClaudeCheckReport => ({
  version: 4,
  id: 'history',
  model: 'claude',
  channel_id: 0,
  channel_name: '',
  transport: 'anthropic_proxy',
  started_at: '',
  duration_ms: 10,
  cancelled: false,
  options: { model: 'claude', thinking: false, cache: false, veridrop: true },
  summary: { pass: 1, fail: 1, inconclusive: 1, skipped: 0 },
  reference: {
    source_url: 'https://github.com/canarybyte/veridrop',
    commit: 'abc',
    license: 'AGPL-3.0',
    snapshots: [],
  },
  plan: [
    {
      id: 'veridrop_pdf',
      stage: 'veridrop',
      kind: 'assertion',
      selected: true,
    },
    {
      id: 'veridrop_tool',
      stage: 'veridrop',
      kind: 'assertion',
      selected: true,
    },
    {
      id: 'veridrop_baseline',
      stage: 'veridrop',
      kind: 'observation',
      selected: true,
    },
  ],
  checks: [
    {
      id: 'veridrop_pdf',
      status: 'fail',
      code: 'veridrop_invalid_response',
      evidence: { http_status: 200 },
    },
    {
      id: 'veridrop_tool',
      status: 'pass',
      code: 'veridrop_observed',
      evidence: { schema_matched: true },
    },
    {
      id: 'veridrop_baseline',
      status: 'inconclusive',
      code: 'veridrop_reference_only',
      evidence: {
        source_url: 'https://github.com/canarybyte/veridrop',
        commit: 'abc',
        matched_model: 'claude',
      },
    },
  ],
  samples: [
    {
      probe: 'veridrop_integrity_stream',
      http_status: 200,
      duration_ms: 1,
      usage: {
        input_tokens: 1,
        output_tokens: 1,
        cache_creation_input_tokens: 0,
        cache_read_input_tokens: 0,
      },
    },
  ],
})

test('legacy presentation and export preserve results without suite branding', () => {
  const raw = legacyReport(),
    before = JSON.stringify(raw)
  const shown = presentReport(raw)
  assert.equal(JSON.stringify(raw), before)
  assert.equal(/veridrop/i.test(JSON.stringify(shown)), false)
  assert.equal(shown.reference, undefined)
  assert.equal(shown.options?.pdf, true)
  assert.equal(shown.options?.stream_comparison, true)
  assert.deepEqual(shown.summary, raw.summary)
  assert.deepEqual(reportScore(shown), reportScore(raw))
  assert.equal(reportScore(shown).value, 50)
  assert.deepEqual(
    shown.checks.map((c) => c.status),
    raw.checks.map((c) => c.status)
  )
  assert.deepEqual(
    shown.plan?.map((p) => p.stage),
    ['capabilities', 'protocol', 'reliability']
  )
  assert.equal(presentReport(shown), shown)
  assert.equal(samplesForCheck(shown, 'stream_comparison').length, 1)
  assert.equal(samplesForCheck(shown, 'stream_stop_reason').length, 1)
  assert.equal(
    presentEvent({ type: 'probe_start', probe: 'veridrop_integrity_stream' })
      .type,
    'probe_start'
  )
  assert.equal(
    JSON.stringify(
      presentEvent({ type: 'check', check: raw.checks[0] })
    ).includes('veridrop'),
    false
  )
})

test('standard and extended scopes share five stages and exact request budgets', () => {
  const standard = {
    model: 'claude',
    thinking: true,
    cache: true,
    pdf: true,
    stream_comparison: true,
  }
  const plan = createCheckPlan(standard)
  assert.equal(plan.length, 28)
  assert.equal(requestBudget(standard), 17)
  assert.equal(requestBudget({ ...standard, repeat: true, vision: true }), 21)
  assert.equal(
    requestBudget({ model: 'claude', thinking: false, cache: false }),
    8
  )
  assert.equal(new Set(plan.map((p) => p.stage)).size, 5)
  assert.ok(
    plan.every((p) => STAGES.includes(p.stage as (typeof STAGES)[number]))
  )
  for (const id of ['tool', 'pdf', 'stream_comparison', 'stream_stop_reason'])
    assert.ok(plan.find((p) => p.id === id)?.selected)
  assert.equal(createCheckPlan(standard, 4).length, 27)
  assert.ok(
    !createCheckPlan(standard, 4).some((p) => p.id === 'stream_stop_reason')
  )
})
