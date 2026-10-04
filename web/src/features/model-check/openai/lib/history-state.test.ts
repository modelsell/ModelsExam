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
import type { CheckHistoryDetail, OpenAIHistoryDetail } from '../../types'
import { isOpenAIHistory, openAIHistoryState } from './history-state'

const detail: OpenAIHistoryDetail = {
  run: {
    id: '0b0c8a0e-1d9a-4f43-a0a5-4a8e0a3a9f11',
    model: 'gpt-test',
    channel_id: 0,
    channel_name: '',
    endpoint: 'https://api.example.com/v1',
    transport: 'openai_api',
    status: 'completed',
    started_at: 1,
    updated_at: 2,
    duration_ms: 900,
    request_count: 1,
    pass_count: 1,
    fail_count: 0,
    active_probe: '',
    score: 100,
  },
  report: {
    version: 1,
    id: '0b0c8a0e-1d9a-4f43-a0a5-4a8e0a3a9f11',
    provider: 'openai',
    model: 'gpt-test',
    started_at: '2026-10-04T00:00:00Z',
    duration_ms: 900,
    cancelled: false,
    requests_run: 1,
    summary: { pass: 1, fail: 0, inconclusive: 0, skipped: 0 },
    checks: [
      {
        id: 'chat_basic',
        stage: 'chat',
        kind: 'assertion',
        status: 'pass',
        code: 'ok',
      },
    ],
    samples: [],
    plan: [
      { id: 'chat_basic', stage: 'chat', kind: 'assertion', selected: true },
      { id: 'chat_stream', stage: 'chat', kind: 'assertion', selected: true },
    ],
  },
  markdown: '# report',
}

test('openai runs are told apart by transport', () => {
  assert.equal(isOpenAIHistory(detail), true)
  const claude = {
    ...detail,
    run: { ...detail.run, transport: 'anthropic_proxy' },
  } as unknown as CheckHistoryDetail
  assert.equal(isOpenAIHistory(claude), false)
})

test('completed stored runs keep their evidence and markdown', () => {
  const state = openAIHistoryState(detail)
  assert.equal(state.phase, 'complete')
  assert.equal(state.markdown, '# report')
  assert.equal(state.report?.checks.length, 1)
  assert.equal(state.report?.score, 100)
})

test('running rows show the active probe and no export markdown', () => {
  const state = openAIHistoryState({
    ...detail,
    run: { ...detail.run, status: 'running', active_probe: 'chat_stream' },
  })
  assert.equal(state.phase, 'running')
  assert.equal(state.activeProbe, 'chat_stream')
  assert.equal(state.markdown, null)
})

test('interrupted rows mark unfinished planned checks as skipped', () => {
  const state = openAIHistoryState({
    ...detail,
    run: { ...detail.run, status: 'interrupted' },
  })
  assert.equal(state.phase, 'error')
  const skipped = state.report?.checks.find((c) => c.id === 'chat_stream')
  assert.equal(skipped?.status, 'skipped')
  assert.equal(skipped?.code, 'interrupted')
  assert.equal(state.report?.score, 100)
})

test('cancelled rows keep the cancelled phase', () => {
  const state = openAIHistoryState({
    ...detail,
    run: { ...detail.run, status: 'cancelled' },
    report: { ...detail.report, cancelled: true },
  })
  assert.equal(state.phase, 'cancelled')
})
