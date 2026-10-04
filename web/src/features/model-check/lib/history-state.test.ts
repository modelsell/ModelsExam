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
import type { CheckHistoryDetail } from '../types'
import { historyRunState } from './history-state'

const detail: CheckHistoryDetail = {
  run: {
    id: 'history-test',
    model: 'claude-test',
    channel_id: 0,
    channel_name: '',
    endpoint: 'https://example.com',
    transport: 'anthropic_proxy',
    status: 'completed',
    started_at: 1,
    updated_at: 2,
    duration_ms: 500,
    request_count: 1,
    pass_count: 1,
    fail_count: 0,
    active_probe: '',
  },
  report: {
    version: 1,
    id: 'history-test',
    model: 'claude-test',
    channel_id: 0,
    channel_name: '',
    transport: 'anthropic_proxy',
    started_at: '2026-09-13T00:00:00Z',
    duration_ms: 500,
    cancelled: false,
    history_saved: true,
    summary: { pass: 1, fail: 0, inconclusive: 0, skipped: 0 },
    checks: [{ id: 'basic', status: 'pass', code: 'observed' }],
    samples: [
      {
        probe: 'basic',
        http_status: 200,
        duration_ms: 500,
        usage: {
          input_tokens: 10,
          output_tokens: 0,
          cache_creation_input_tokens: null,
          cache_read_input_tokens: null,
        },
      },
    ],
  },
}

test('stored reports reuse the completed report without changing evidence', () => {
  const state = historyRunState(detail)
  assert.equal(state.phase, 'complete')
  assert.equal(state.report, detail.report)
  assert.equal(state.report?.samples[0].usage.output_tokens, 0)
})

test('interrupted history preserves saved results and marks remaining checks skipped', () => {
  const state = historyRunState({
    ...detail,
    run: { ...detail.run, status: 'interrupted' },
  })
  assert.equal(state.phase, 'error')
  assert.equal(state.report?.checks.length, 15)
  assert.equal(state.report?.summary.pass, 1)
  assert.equal(state.report?.summary.skipped, 14)
  assert.equal(state.report?.cancelled, true)
  assert.equal(detail.report.checks.length, 1)
  const running = historyRunState({
    ...detail,
    run: { ...detail.run, status: 'running', active_probe: 'stream' },
  })
  assert.equal(running.phase, 'running')
  assert.equal(running.activeProbe, 'stream')
})
