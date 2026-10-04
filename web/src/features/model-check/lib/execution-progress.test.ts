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
import { createCheckPlan, samplesForCheck } from './check-plan'
import { executionProgress, reportStopReason } from './execution-progress'
import { historyRunState } from './history-state'
import { summarizeScore } from './report-score'

const report = (): ClaudeCheckReport => ({
  version: 4,
  id: 'test',
  model: 'claude',
  channel_id: 62,
  channel_name: 'test',
  transport: 'anthropic_proxy',
  started_at: '',
  duration_ms: 0,
  cancelled: false,
  summary: { pass: 0, fail: 0, inconclusive: 1, skipped: 1 },
  plan: createCheckPlan({ model: 'claude', thinking: false, cache: false }),
  checks: [
    { id: 'basic', status: 'inconclusive', code: 'probe_timeout' },
    { id: 'tool', status: 'skipped', code: 'run_stopped' },
  ],
  samples: [
    {
      probe: 'basic',
      http_status: 0,
      duration_ms: 35000,
      error: 'context deadline exceeded',
      usage: {
        input_tokens: null,
        output_tokens: null,
        cache_creation_input_tokens: null,
        cache_read_input_tokens: null,
      },
    },
  ],
})

test('stopped runs count only executed selected checks and stay unscored', () => {
  const r = report()
  r.stop_reason = 'probe_timeout'
  const progress = executionProgress(r.plan!, r.checks)
  assert.equal(progress.executed, 1)
  assert.equal(progress.skipped, 1)
  assert.equal(summarizeScore(r.plan!, r.checks).value, null)
  assert.equal(reportStopReason(r), 'probe_timeout')
  const state = historyRunState({
    run: {
      id: 'test',
      model: 'claude',
      channel_id: 62,
      channel_name: 'test',
      endpoint: '',
      transport: 'anthropic_proxy',
      status: 'stopped',
      started_at: 0,
      updated_at: 0,
      duration_ms: 90000,
      request_count: 2,
      pass_count: 0,
      fail_count: 0,
      active_probe: '',
    },
    report: r,
  })
  assert.equal(state.report, r)
  assert.equal(state.report?.cancelled, false)
})

test('legacy early-stop explanation does not change historical results', () => {
  const r = report()
  r.version = 3
  r.checks[0] = { id: 'basic', status: 'fail', code: 'observed' }
  r.checks[1].code = 'baseline_failed'
  const original = JSON.stringify(r)
  assert.equal(reportStopReason(r), 'probe_timeout')
  assert.equal(JSON.stringify(r), original)
  r.samples[0].error = 'No available channel for model claude under group VT-2'
  assert.equal(reportStopReason(r), 'route_unavailable')
})

test('fallback baseline evidence points to the actual stream sample', () => {
  const r = report()
  r.baseline_probe = 'stream'
  r.samples.push({
    ...r.samples[0],
    probe: 'stream',
    http_status: 200,
    error: undefined,
  })
  r.checks.push({
    id: 'model_echo',
    status: 'pass',
    code: 'model_mapping',
    evidence: { baseline_probe: 'stream' },
  })
  assert.equal(samplesForCheck(r, 'model_echo')[0].probe, 'stream')
  assert.equal(reportStopReason(r), null)
})
