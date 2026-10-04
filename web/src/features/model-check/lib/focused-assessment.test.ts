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
  ClaudeCheckSample,
  ComparisonBaseline,
} from '../types'
import { requestBudget, createCheckPlan } from './check-plan'
import {
  closestFocusedTypes,
  coreAccuracy,
  focusedComparison,
  focusedFingerprint,
  focusedPerformance,
  CORE_QUESTIONS,
} from './focused-assessment'
import { modelIdentityState } from './model-identity'

const report = (): ClaudeCheckReport => ({
  version: 9,
  id: 'current',
  model: 'claude-opus-5',
  channel_id: 0,
  channel_name: '',
  transport: 'anthropic_proxy',
  started_at: '',
  duration_ms: 1,
  cancelled: false,
  summary: { pass: 0, fail: 0, inconclusive: 0, skipped: 0 },
  checks: [],
  samples: [],
  options: {
    suite: 'focused',
    model: 'claude-opus-5',
    cache: true,
    thinking: false,
    fingerprint: true,
    performance_tolerance: 25,
  },
})
const baseline = (): ComparisonBaseline => ({
  ...report(),
  id: 'reference',
  report_id: 'reference',
  type: 'ccmax',
  created_at: 1,
  version: 7,
})
const fingerprint = (version = 2): BehaviorFingerprint => ({
  version,
  repetitions: 10,
  cells: (version === 1
    ? ['number', 'letter', 'color', 'animal']
    : ['letter', 'animal']
  ).map((id) => ({
    id,
    profile: id,
    valid: 10,
    attempts: 10,
    counts: { a: 10 },
  })),
})
const sample = (
  n: number,
  ttft = 1000,
  duration = 2000,
  output = 200
): ClaudeCheckSample => ({
  probe: `performance_${n}`,
  request_profile: 'same',
  http_status: 200,
  valid_response: true,
  duration_ms: duration,
  usage: {
    input_tokens: 100,
    output_tokens: output,
    cache_creation_input_tokens: 0,
    cache_read_input_tokens: 0,
  },
  stream: {
    first_text_ms: ttft,
    last_text_ms: duration,
    max_text_gap_ms: 1,
    text_events: 10,
    events: 14,
  },
})

test('focused scope and budget exclude legacy probes and retain optional audits', () => {
  const r = report()
  assert.equal(requestBudget(r.options!), 23)
  assert.equal(requestBudget({ ...r.options!, fingerprint: false }), 23)
  assert.equal(
    requestBudget({
      ...r.options!,
      prompt_audit: true,
      vision: true,
      pdf: true,
    }),
    28
  )
  const ids = createCheckPlan(r.options).map((item) => item.id)
  assert.deepEqual(ids, [
    'basic',
    'model_consistency',
    'performance_sampling',
    'usage_token_integrity',
    'capability_benchmark',
    'cache_token_audit',
  ])
})

test('v2 matches the shared v1 categories but sparse categories cannot select a type', () => {
  const a = fingerprint(),
    b = fingerprint(1)
  assert.equal(focusedFingerprint(a, b).score, 100)
  b.cells[1].valid = 8
  b.cells[1].counts = { a: 8 }
  const partial = focusedFingerprint(a, b)
  assert.equal(partial.count, 1)
  assert.equal(partial.score, 100)
  assert.equal(partial.complete, false)
  assert.equal(partial.cells[0].reason, 'samples')
  const r = report()
  r.fingerprint = a
  r.baselines = [{ ...baseline(), fingerprint: b }]
  assert.deepEqual(closestFocusedTypes(r), [])
  b.cells[3].profile = 'different'
  assert.equal(focusedFingerprint(a, b).score, null)
  assert.equal(focusedFingerprint(a, b).cells[1].reason, 'profile')
  b.cells[3].profile = 'animal'
  b.cells[3].counts = { a: 100 }
  assert.equal(focusedFingerprint(a, b).score, null)
})

test('performance thresholds use three fixed pairs and never mix unrelated probes', () => {
  const r = report(),
    b = baseline()
  r.samples = [1, 2, 3].map((n) => sample(n, 1250, 2500))
  b.samples = [1, 2, 3].map((n) => sample(n))
  r.samples.push({ ...sample(4, 90000, 100000), probe: 'cache_read_1' })
  b.samples.push({ ...sample(4, 1, 2), probe: 'cache_read_1' })
  let p = focusedComparison(r, b).performance
  assert.equal(p.score, 100)
  assert.equal(p.ttftLimit, 1250)
  assert.equal(p.rateLimit, 80)
  assert.equal(focusedPerformance(r.samples).count, 3)
  r.samples[0].stream!.first_text_ms = 1400
  r.samples[1].stream!.first_text_ms = 1400
  assert.equal(focusedComparison(r, b).performance.score, 50)
  r.samples[2].request_profile = 'changed'
  p = focusedComparison(r, b).performance
  assert.equal(p.count, 2)
  assert.equal(p.score, null)
  r.samples[2].request_profile = 'same'
  r.options!.performance_tolerance = 0
  assert.equal(focusedComparison(r, b).performance.score, 0)
  delete r.samples[2].stream
  assert.equal(focusedComparison(r, b).performance.score, null)
})

test('local observed capability patterns compare only six identical question profiles', () => {
  const r = report(),
    b = baseline()
  const answersA = [true, true, true, true, false, true],
    answersB = [true, false, false, true, true, true]
  r.benchmark = {
    version: 1,
    items: CORE_QUESTIONS.map((id, i) => ({
      id,
      category: 'core',
      profile: id,
      expected: '',
      correct: answersA[i],
    })),
  }
  b.benchmark = {
    version: 1,
    items: CORE_QUESTIONS.map((id, i) => ({
      id,
      category: 'core',
      profile: id,
      expected: '',
      correct: answersB[i],
    })),
  }
  const result = focusedComparison(r, b).benchmark
  assert.equal(coreAccuracy(r).score, 83)
  assert.equal(result.current, 83)
  assert.equal(result.reference, 67)
  assert.deepEqual(
    result.differences.map((q) => q.id),
    ['boolean_count', 'python_alias', 'zh_constraint']
  )
  b.benchmark.items[0].profile = 'changed'
  b.benchmark.items[1].correct = null
  assert.equal(focusedComparison(r, b).benchmark.count, 4)
  assert.equal(focusedComparison(r, baseline()).benchmark.current, null)
})

test('local observed performance metrics differ from mixed-probe latency ratios', () => {
  const r = report(),
    b = baseline()
  r.samples = [
    sample(1, 12715, 16176, 512),
    sample(2, 2498, 5896, 512),
    sample(3, 2628, 6026, 512),
  ]
  b.samples = [
    sample(1, 2697, 5592, 124),
    sample(2, 2628, 6186, 124),
    sample(3, 2181, 5595, 124),
  ]
  const p = focusedComparison(r, b).performance
  assert.equal(p.count, 3)
  assert.equal(p.current.ttft, 2628)
  assert.equal(p.reference.ttft, 2628)
  assert.ok(Math.abs(p.current.rate! - 84.965) < 0.01)
  assert.equal(p.score, 100)
})

test('model mismatch remains explicit despite perfect behavioral similarity', () => {
  const r = report()
  r.fingerprint = fingerprint()
  r.baselines = [{ ...baseline(), fingerprint: fingerprint() }]
  r.checks = [
    { id: 'model_consistency', status: 'fail', code: 'model_mismatch' },
  ]
  r.samples = [{ ...sample(1), response_model: 'claude-sonnet-4-6' }]
  assert.equal(modelIdentityState(r), 'mismatch')
  assert.deepEqual(closestFocusedTypes(r), ['ccmax'])
  assert.equal(coreAccuracy(r).score, null)
})
