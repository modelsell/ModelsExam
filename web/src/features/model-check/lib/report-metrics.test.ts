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
import type { ClaudeCheckReport, ClaudeCheckSample } from '../types'
import {
  createCheckPlan,
  getCheckPlan,
  requestBudget,
  probeCheckID,
} from './check-plan'
import { reportMetrics } from './report-metrics'
import { interruptRun } from './run-state'

const sample = (
  probe: string,
  first: number | null,
  valid = true
): ClaudeCheckSample => ({
  probe,
  http_status: valid ? 200 : 429,
  duration_ms: 500,
  valid_response: valid,
  first_event_ms: 10,
  stream: {
    first_text_ms: first,
    last_text_ms: first,
    max_text_gap_ms: null,
    text_events: 1,
    events: 6,
  },
  usage: {
    input_tokens: 10,
    output_tokens: 0,
    cache_creation_input_tokens: 0,
    cache_read_input_tokens: 0,
  },
})

test('latency uses successful repeated text samples and retains failure denominator', () => {
  const m = reportMetrics([
    sample('repeat_1', 100),
    sample('repeat_2', null, false),
    sample('repeat_3', 300),
    sample('stream', 9999),
    sample('error_validation', null, false),
  ])
  assert.equal(m.ttftMedian, 200)
  assert.equal(m.repeatAttempts, 3)
  assert.equal(m.repeatSuccess, 2)
  assert.equal(m.output, 0)
  assert.equal(m.ttftSamples, 2)
})

test('cache writes never count as hits, missing reads stay unavailable', () => {
  const writes = sample('cache_read_1', null)
  writes.usage.cache_creation_input_tokens = 4000
  const missing = sample('cache_read_2', null)
  missing.usage.cache_read_input_tokens = null
  const m = reportMetrics([writes, missing])
  assert.equal(m.warmHits, 0)
  assert.equal(m.warmAttempts, 2)
  assert.equal(m.warmMeasured, 1)
  assert.equal(m.cacheRead, null)
})

test('historical and interrupted reports retain their original versioned scope', () => {
  const report: ClaudeCheckReport = {
    version: 1,
    id: 'old',
    model: 'claude',
    channel_id: 0,
    channel_name: '',
    transport: 'anthropic_proxy',
    started_at: '',
    duration_ms: 0,
    cancelled: false,
    checks: [],
    samples: [],
    summary: { pass: 0, fail: 0, inconclusive: 0, skipped: 0 },
  }
  assert.equal(getCheckPlan(report)?.length, 15)
  assert.equal(
    getCheckPlan(report).some((item) => item.id === 'vision'),
    false
  )
  report.version = 2
  report.plan = createCheckPlan(
    {
      model: 'claude',
      cache: true,
      thinking: true,
      repeat: true,
      vision: true,
    },
    2
  )
  assert.equal(report.plan?.length, 23)
  const stopped = interruptRun(
    { report, phase: 'running', activeProbe: 'repeat_1', error: null },
    true,
    200,
    null
  )
  assert.equal(stopped.report?.checks?.length, 23)
  assert.equal(report.checks?.length, 0)
  assert.equal(
    requestBudget(
      report.options ?? {
        model: 'claude',
        cache: true,
        thinking: true,
        repeat: true,
        vision: true,
      }
    ),
    18
  )
})

test('v5 integrates semantic probes without expanding historical plans', () => {
  const options = {
    model: 'claude',
    cache: true,
    thinking: true,
    vision: true,
    repeat: true,
    pdf: true,
    stream_comparison: true,
  }
  const current = createCheckPlan(options)
  assert.equal(current.length, 28)
  assert.equal(requestBudget(options), 21)
  assert.ok(current.some((item) => item.id === 'zero_output'))
  assert.equal(probeCheckID('comparison_stream'), 'stream_comparison')
  const historical = createCheckPlan(options, 2)
  assert.equal(historical.length, 23)
  assert.ok(historical.some((item) => item.id === 'error_validation'))
  assert.ok(!historical.some((item) => item.id.startsWith('veridrop_')))
})
