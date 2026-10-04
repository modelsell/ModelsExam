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
  BehaviorFingerprint,
  ClaudeCheckReport,
  ComparisonBaseline,
} from '../types'
import {
  benchmarkAccuracy,
  compareBaselines,
  fingerprintSimilarity,
  nearestBaselineTypes,
} from './baseline-comparison'
import { createCheckPlan, requestBudget } from './check-plan'
import { reportScore } from './report-score'
import { applyCheckEvent, INITIAL_RUN } from './run-state'

test('v3 baseline accuracy uses partial points and ignores retired questions', () => {
  const items = [
    { id: 'instruction_format', score: 50 },
    { id: 'json_types', score: 100 },
    { id: 'digit_count', score: 100 },
  ].map((item) => ({
    ...item,
    category: '',
    profile: item.id,
    expected: 'reference',
    correct: item.score === 100,
  }))
  assert.deepEqual(benchmarkAccuracy({ version: 3, items }), {
    assessed: 2,
    total: 6,
    score: 75,
  })
})

const fingerprint = (answer = 'A'): BehaviorFingerprint => ({
  version: 1,
  repetitions: 10,
  cells: ['number', 'letter', 'color', 'animal'].map((id) => ({
    id,
    profile: id,
    valid: 10,
    attempts: 10,
    counts: { [answer]: 10 },
  })),
})
const report = (): ClaudeCheckReport => ({
  version: 7,
  id: 'target',
  model: 'claude-opus-5',
  channel_id: 0,
  channel_name: '',
  transport: 'anthropic_proxy',
  started_at: '',
  duration_ms: 1,
  cancelled: false,
  summary: { pass: 1, fail: 0, skipped: 0, inconclusive: 0 },
  checks: [{ id: 'system', status: 'pass', code: 'observed' }],
  samples: [],
  fingerprint: fingerprint(),
})
const baseline = (
  id: string,
  type: string,
  answer = 'A'
): ComparisonBaseline => ({
  ...report(),
  id,
  report_id: id,
  created_at: 1,
  type,
  fingerprint: fingerprint(answer),
})

test('distribution scores have clear endpoints and reject missing or incompatible samples', () => {
  assert.equal(fingerprintSimilarity(fingerprint(), fingerprint()), 100)
  assert.equal(fingerprintSimilarity(fingerprint(), fingerprint('B')), 0)
  assert.equal(fingerprintSimilarity(undefined, fingerprint()), null)
  const changed = fingerprint()
  changed.cells[0].profile = 'changed'
  assert.equal(fingerprintSimilarity(changed, fingerprint()), null)
  const sparse = fingerprint()
  sparse.cells[0].valid = 9
  sparse.cells[0].counts = { A: 9 }
  assert.equal(fingerprintSimilarity(sparse, fingerprint()), null)
})

test('automatic multi-type ranking keeps ties and ignores declared names as scoring evidence', () => {
  const r = report()
  r.baselines = [
    baseline('aws', 'aws'),
    baseline('kiro', 'kiro', 'B'),
    baseline('official', 'anthropic'),
  ]
  r.baselines[0].model = 'claude-sonnet-5'
  assert.deepEqual(nearestBaselineTypes(r), ['aws', 'anthropic'])
  assert.equal(compareBaselines(r)[2].fingerprint, 0)
  delete r.fingerprint
  assert.deepEqual(nearestBaselineTypes(r), [])
  assert.equal(compareBaselines(r)[0].protocol.score, 100)
})

test('history snapshots and compatibility scores survive new collection events', () => {
  const r = report()
  r.baselines = [baseline('ref', 'aws')]
  const original = reportScore(r)
  let state = applyCheckEvent(INITIAL_RUN, { type: 'start', report: r })
  state = applyCheckEvent(state, {
    type: 'fingerprint',
    fingerprint: fingerprint('B'),
  })
  state = applyCheckEvent(state, {
    type: 'benchmark',
    benchmark: {
      version: 1,
      items: [
        {
          id: 'math',
          category: 'math',
          expected: '1',
          profile: 'hash',
          correct: false,
        },
      ],
    },
  })
  assert.equal(state.report?.baselines?.[0].fingerprint?.cells[0].counts.A, 10)
  assert.deepEqual(reportScore(state.report!), original)
  assert.equal(benchmarkAccuracy(state.report?.benchmark).score, 0)
  assert.equal(
    benchmarkAccuracy({
      version: 1,
      items: [
        {
          id: 'math',
          category: 'math',
          expected: '1',
          profile: 'hash',
          correct: null,
        },
      ],
    }).score,
    null
  )
  assert.equal(
    requestBudget({
      model: 'claude-opus-5',
      thinking: true,
      cache: true,
      vision: true,
      repeat: true,
      pdf: true,
      stream_comparison: true,
      benchmark: true,
      fingerprint: true,
    }),
    28
  )
  assert.equal(
    createCheckPlan(
      {
        model: 'x',
        cache: false,
        thinking: false,
        benchmark: true,
        fingerprint: true,
      },
      16
    ).filter((item) => item.kind === 'boundary').length,
    5
  )
})
