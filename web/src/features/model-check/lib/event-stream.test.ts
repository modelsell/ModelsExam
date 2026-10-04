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
import type { CheckEvent, ClaudeCheckReport } from '../types'
import { readCheckStream } from './event-stream'
import { applyCheckEvent, INITIAL_RUN, interruptRun } from './run-state'

const report: ClaudeCheckReport = {
  version: 1,
  id: 'test',
  model: 'claude-test',
  channel_id: 0,
  channel_name: '',
  transport: 'anthropic_proxy',
  started_at: '2026-09-13T00:00:00Z',
  duration_ms: 0,
  cancelled: false,
  summary: { pass: 0, fail: 0, inconclusive: 0, skipped: 0 },
  checks: [],
  samples: [],
}

function stream(text: string): ReadableStream<Uint8Array> {
  const bytes = new TextEncoder().encode(text)
  return new ReadableStream({
    start(controller) {
      // Every byte is its own chunk, including multi-byte characters and CRLF.
      for (const byte of bytes) controller.enqueue(new Uint8Array([byte]))
      controller.close()
    },
  })
}

test('progress accepts fragmented UTF-8, multiline frames, comments, and a final report', async () => {
  const events: CheckEvent[] = []
  await readCheckStream(
    stream(
      `: heartbeat\r\n\r\ndata: {"type":"probe_start",\r\ndata: "probe":"中文"}\r\n\r\ndata: ${JSON.stringify({ type: 'done', report })}\n\n`
    ),
    (event) => events.push(event)
  )
  assert.deepEqual(events[0], { type: 'probe_start', probe: '中文' })
  assert.equal(events[1].type, 'done')
})

test('an interrupted stream preserves completed checks and explicit zero usage in an exportable partial report', async () => {
  let state = applyCheckEvent(INITIAL_RUN, { type: 'start', report })
  const sample: CheckEvent = {
    type: 'sample',
    sample: {
      probe: 'basic',
      http_status: 200,
      duration_ms: 20,
      usage: {
        input_tokens: 10,
        output_tokens: 0,
        cache_creation_input_tokens: null,
        cache_read_input_tokens: null,
      },
    },
  }
  const wire = [
    sample,
    { type: 'check', check: { id: 'basic', status: 'pass', code: 'observed' } },
  ]
    .map((event) => `data: ${JSON.stringify(event)}\n\n`)
    .join('')
  await assert.rejects(
    readCheckStream(stream(wire), (event) => {
      state = applyCheckEvent(state, event)
    }),
    /before the report completed/
  )
  const interrupted = interruptRun(state, false, 123, 'connection lost')
  assert.equal(interrupted.phase, 'error')
  assert.equal(interrupted.report?.checks.length, 15)
  assert.equal(interrupted.report?.summary.pass, 1)
  assert.equal(interrupted.report?.summary.skipped, 14)
  assert.equal(interrupted.report?.samples[0].usage.output_tokens, 0)
  assert.equal(
    interrupted.report?.samples[0].usage.cache_read_input_tokens,
    null
  )
  assert.equal(interrupted.report?.duration_ms, 123)
  const cancelled = interruptRun(state, true, 125, null)
  assert.equal(cancelled.report?.checks[1].code, 'cancelled')
})

test('a new run starts with an empty report and the final event replaces partial state', () => {
  let state = applyCheckEvent(INITIAL_RUN, { type: 'start', report })
  state = applyCheckEvent(state, {
    type: 'check',
    check: { id: 'basic', status: 'pass', code: 'observed' },
  })
  state = applyCheckEvent(state, {
    type: 'start',
    report: { ...report, id: 'second' },
  })
  assert.equal(state.report?.checks.length, 0)
  state = applyCheckEvent(state, {
    type: 'done',
    report: { ...report, id: 'final', duration_ms: 600 },
  })
  assert.equal(state.phase, 'complete')
  assert.equal(state.report?.id, 'final')
})
