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
import type { OpenAICheck, OpenAICheckReport } from '../types'
import { OPENAI_CHECK_TITLES, OPENAI_STAGES, checkIdForProbe } from './catalog'
import { openAICheckOptions, openAIRequestBudget } from './form'
import {
  INITIAL_OPENAI_RUN,
  applyOpenAIEvent,
  interruptOpenAIRun,
  openAIScore,
} from './run-state'
import { readOpenAIStream } from './stream'

const base = {
  version: 1,
  id: 'r1',
  provider: 'openai' as const,
  model: 'gpt-4o',
  started_at: '2026-01-01T00:00:00Z',
  duration_ms: 0,
  checks: [] as OpenAICheck[],
  samples: [],
  summary: { pass: 0, fail: 0, inconclusive: 0, skipped: 0 },
  cancelled: false,
  requests_run: 0,
}
const plan = [
  {
    id: 'models_list',
    stage: 'discovery',
    kind: 'assertion' as const,
    selected: true,
  },
  {
    id: 'chat_basic',
    stage: 'chat_basic',
    kind: 'assertion' as const,
    selected: true,
  },
  {
    id: 'chat_vision',
    stage: 'chat_vision',
    kind: 'assertion' as const,
    selected: false,
  },
]
const check = (
  id: string,
  status: OpenAICheck['status'],
  kind: OpenAICheck['kind'] = 'assertion'
): OpenAICheck => ({ id, stage: 's', kind, status, code: '' })

test('request budget matches the Go planner', () => {
  const budget = (suite: 'basic' | 'standard' | 'full', extra = {}) =>
    openAIRequestBudget({
      suite,
      responses: false,
      vision: false,
      logprobs: false,
      ...extra,
    })
  assert.equal(budget('basic'), 4)
  assert.equal(budget('standard'), 15)
  assert.equal(budget('full'), 30)
  assert.equal(budget('full', { vision: true }), 31)
  assert.equal(budget('basic', { responses: true }), 12)
})

test('full suite forces responses and logprobs in the request', () => {
  const options = openAICheckOptions({
    base_url: 'https://x.test',
    key: 'sk-test',
    model: 'gpt-4o',
    suite: 'full',
    responses: false,
    vision: false,
    logprobs: false,
    limit_param: 'max_tokens',
  })
  assert.equal(options.responses, true)
  assert.equal(options.logprobs, true)
  assert.equal(options.vision, false)
  assert.equal(options.limit_param, 'max_tokens')
})

test('score counts decided assertions only and rounds half up', () => {
  assert.equal(openAIScore([]), null)
  assert.equal(
    openAIScore([check('a', 'inconclusive'), check('b', 'skipped')]),
    null
  )
  assert.equal(
    openAIScore([
      check('a', 'pass'),
      check('b', 'fail'),
      check('c', 'pass', 'observation'),
    ]),
    50
  )
  assert.equal(
    openAIScore([check('a', 'pass'), check('b', 'pass'), check('c', 'fail')]),
    67
  )
  assert.equal(
    openAIScore(
      Array.from({ length: 8 }, (_, i) =>
        check(`c${i}`, i === 0 ? 'fail' : 'pass')
      )
    ),
    88
  )
})

test('events build up the report and done keeps markdown', () => {
  let state = applyOpenAIEvent(INITIAL_OPENAI_RUN, {
    type: 'start',
    report: { ...base, plan } as OpenAICheckReport,
  })
  assert.equal(state.phase, 'running')
  state = applyOpenAIEvent(state, { type: 'probe_start', probe: 'chat_basic' })
  assert.equal(state.activeProbe, 'chat_basic')
  state = applyOpenAIEvent(state, {
    type: 'check',
    check: check('models_list', 'pass'),
  })
  state = applyOpenAIEvent(state, {
    type: 'check',
    check: check('models_list', 'fail'),
  })
  assert.equal(state.report?.checks.length, 1)
  assert.equal(state.report?.summary.fail, 1)
  assert.equal(state.report?.score, 0)
  state = applyOpenAIEvent(state, {
    type: 'done',
    report: { ...base, checks: [check('models_list', 'pass')], score: 100 },
    markdown: '# report',
  })
  assert.equal(state.phase, 'complete')
  assert.equal(state.markdown, '# report')
  assert.equal(state.activeProbe, null)
})

test('progress before start is rejected', () => {
  assert.throws(() =>
    applyOpenAIEvent(INITIAL_OPENAI_RUN, { type: 'probe_start', probe: 'x' })
  )
})

test('interrupt skips unfinished checks and keeps finished ones', () => {
  let state = applyOpenAIEvent(INITIAL_OPENAI_RUN, {
    type: 'start',
    report: { ...base, plan } as OpenAICheckReport,
  })
  state = applyOpenAIEvent(state, {
    type: 'check',
    check: check('models_list', 'pass'),
  })
  const stopped = interruptOpenAIRun(state, true, 1234, null)
  assert.equal(stopped.phase, 'cancelled')
  const codes = Object.fromEntries(
    stopped.report!.checks.map((c) => [c.id, c.code])
  )
  assert.equal(codes.chat_basic, 'cancelled')
  assert.equal(codes.chat_vision, 'not_requested')
  assert.equal(stopped.report!.cancelled, true)
  assert.equal(stopped.report!.duration_ms, 1234)
  assert.equal(stopped.report!.score, 100)
  const failed = interruptOpenAIRun(state, false, 1, 'boom')
  assert.equal(failed.phase, 'error')
  assert.equal(failed.error, 'boom')
  assert.equal(
    failed.report!.checks.find((c) => c.id === 'chat_basic')!.code,
    'interrupted'
  )
  assert.equal(
    interruptOpenAIRun(INITIAL_OPENAI_RUN, false, 1, 'x').report,
    null
  )
})

test('catalog covers every stage and probe suffix', () => {
  assert.equal(
    checkIdForProbe('chat_tool_roundtrip_result'),
    'chat_tool_roundtrip'
  )
  assert.equal(checkIdForProbe('chat_basic'), 'chat_basic')
  assert.equal(
    new Set(OPENAI_STAGES.map((s) => s.id)).size,
    OPENAI_STAGES.length
  )
  assert.ok(OPENAI_CHECK_TITLES.responses_tool_stream)
})

function streamOf(chunks: Uint8Array[]): ReadableStream<Uint8Array> {
  return new ReadableStream({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(chunk)
      controller.close()
    },
  })
}

test('stream reader handles split UTF-8, comments and the done event', async () => {
  const frame = (event: object) => `data: ${JSON.stringify(event)}\n\n`
  const text =
    ': keep-alive\n\n' +
    frame({ type: 'start', report: { ...base, model: '模型' } }) +
    frame({ type: 'done', report: base, markdown: 'ok' })
  const bytes = new TextEncoder().encode(text)
  // One byte into the three-byte character.
  const split =
    new TextEncoder().encode(text.slice(0, text.indexOf('模'))).length + 1
  const seen: string[] = []
  await readOpenAIStream(
    streamOf([bytes.slice(0, split), bytes.slice(split)]),
    (event) => seen.push(event.type)
  )
  assert.deepEqual(seen, ['start', 'done'])
})

test('stream reader fails when the stream ends early or the type is unknown', async () => {
  const start = `data: ${JSON.stringify({ type: 'start', report: base })}\n\n`
  await assert.rejects(
    readOpenAIStream(
      streamOf([new TextEncoder().encode(start)]),
      () => undefined
    ),
    /ended before/
  )
  await assert.rejects(
    readOpenAIStream(
      streamOf([new TextEncoder().encode('data: {"type":"nope"}\n\n')]),
      () => undefined
    ),
    /Invalid progress event/
  )
})
