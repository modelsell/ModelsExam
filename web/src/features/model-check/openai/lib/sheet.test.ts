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
import type { OpenAICheck, OpenAICheckReport, OpenAISample } from '../types'
import { median, samplesFor, sheetData } from './sheet'

const check = (
  id: string,
  stage: string,
  status: OpenAICheck['status'],
  kind: OpenAICheck['kind'] = 'assertion'
): OpenAICheck => ({ id, stage, kind, status, code: '' })
const sample = (
  probe: string,
  extra: Partial<OpenAISample> = {}
): OpenAISample => ({
  probe,
  kind: 'chat',
  method: 'POST',
  path: '/v1/chat/completions',
  http_status: 200,
  duration_ms: 100,
  usage: { input_tokens: 1, output_tokens: 2, total_tokens: 3 },
  ...extra,
})
const report = (
  checks: OpenAICheck[],
  samples: OpenAISample[] = [],
  plan?: OpenAICheckReport['plan']
): OpenAICheckReport => ({
  version: 1,
  id: 'r',
  provider: 'openai',
  model: 'gpt-4o',
  started_at: '2026-01-01T00:00:00Z',
  duration_ms: 1,
  checks,
  samples,
  plan,
  summary: { pass: 0, fail: 0, inconclusive: 0, skipped: 0 },
  score: null,
  cancelled: false,
  requests_run: samples.length,
})

test('median handles empty, odd and even inputs', () => {
  assert.equal(median([]), null)
  assert.equal(median([5, 1, 3]), 3)
  assert.equal(median([4, 1, 3, 2]), 2.5)
})

test('dimensions follow the selected plan and score decided assertions', () => {
  const plan = [
    {
      id: 'chat_basic',
      stage: 'chat_basic',
      kind: 'assertion' as const,
      selected: true,
    },
    {
      id: 'chat_stream',
      stage: 'chat_stream',
      kind: 'assertion' as const,
      selected: true,
    },
    {
      id: 'usage_fields',
      stage: 'protocol',
      kind: 'assertion' as const,
      selected: true,
    },
    {
      id: 'responses_basic',
      stage: 'responses',
      kind: 'assertion' as const,
      selected: false,
    },
  ]
  const data = sheetData(
    report(
      [
        check('chat_basic', 'chat_basic', 'pass'),
        check('chat_stream', 'chat_stream', 'fail'),
        check('usage_fields', 'protocol', 'inconclusive'),
      ],
      [],
      plan
    ),
    false
  )
  assert.deepEqual(
    data.dimensions.map((d) => [d.id, d.score]),
    [
      ['chat', 100],
      ['stream', 0],
      ['protocol', null],
    ]
  )
  assert.equal(data.verdict, 'review')
  assert.deepEqual(
    data.findings.map((c) => c.id),
    ['chat_stream', 'usage_fields']
  )
})

test('verdict covers pending, clean, partial and unknown', () => {
  const pass = check('a', 'chat_basic', 'pass')
  assert.equal(sheetData(report([pass]), true).verdict, 'pending')
  assert.equal(sheetData(report([pass]), false).verdict, 'clean')
  assert.equal(
    sheetData(report([pass, check('b', 'chat_basic', 'inconclusive')]), false)
      .verdict,
    'partial'
  )
  assert.equal(
    sheetData(report([check('b', 'chat_basic', 'inconclusive')]), false)
      .verdict,
    'unknown'
  )
  // Observations never decide the verdict.
  assert.equal(
    sheetData(report([check('p', 'reliability', 'fail', 'observation')]), false)
      .verdict,
    'unknown'
  )
})

test('timing and models use answered requests only', () => {
  const data = sheetData(
    report(
      [],
      [
        sample('a', {
          duration_ms: 100,
          response_model: 'gpt-4o',
          system_fingerprint: 'fp1',
        }),
        sample('b', {
          duration_ms: 300,
          first_event_ms: 50,
          response_model: 'gpt-4o-2024',
        }),
        sample('c', {
          duration_ms: 9000,
          http_status: 0,
          error_code: 'timeout',
        }),
        sample('d', {
          duration_ms: 9000,
          http_status: 429,
          error_code: 'rate_limited',
        }),
      ]
    ),
    false
  )
  assert.equal(data.latency.median, 200)
  assert.equal(data.latency.max, 300)
  assert.equal(data.latency.count, 2)
  assert.equal(data.firstEvent.median, 50)
  assert.deepEqual(data.returnedModels, ['gpt-4o', 'gpt-4o-2024'])
  assert.deepEqual(data.fingerprints, ['fp1'])
  assert.equal(data.totalTokens, 12)
  assert.equal(sheetData(report([], []), false).totalTokens, null)
})

test('samplesFor matches second-request probes', () => {
  const r = report(
    [],
    [
      sample('chat_tool_roundtrip'),
      sample('chat_tool_roundtrip_result'),
      sample('chat_basic'),
    ]
  )
  assert.equal(samplesFor(r, 'chat_tool_roundtrip').length, 2)
})
