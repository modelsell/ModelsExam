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
import type { ImageCheck, ImageCheckReport } from '../types'
import { IMAGE_CHECK_TITLES, IMAGE_STAGES } from './catalog'
import { imageBudget } from './form'
import {
  INITIAL_IMAGE_RUN,
  applyImageEvent,
  imageScore,
  interruptImageRun,
} from './run-state'
import { readImageStream } from './stream'

const base: ImageCheckReport = {
  version: 1,
  id: 'r1',
  provider: 'openai',
  model: 'gpt-image-2',
  started_at: '2026-01-01T00:00:00Z',
  duration_ms: 0,
  checks: [],
  samples: [],
  images: [],
  summary: { pass: 0, fail: 0, inconclusive: 0, skipped: 0 },
  cancelled: false,
  requests_run: 0,
  plan: [
    { id: 'img_generate_basic', stage: 'generate', kind: 'assertion', selected: true },
    { id: 'img_prov_c2pa', stage: 'provenance', kind: 'provenance', selected: true },
    { id: 'img_edit_basic', stage: 'edits', kind: 'assertion', selected: false },
  ],
}

const check = (id: string, kind: ImageCheck['kind'], status: ImageCheck['status']): ImageCheck => ({
  id, stage: 'x', kind, status, code: '',
})

test('score counts only decided assertions', () => {
  assert.equal(imageScore([]), null)
  assert.equal(
    imageScore([
      check('a', 'assertion', 'pass'),
      check('b', 'assertion', 'fail'),
      check('c', 'assertion', 'inconclusive'),
      check('d', 'provenance', 'fail'),
      check('e', 'observation', 'fail'),
    ]),
    50
  )
})

test('events build the report and provenance never moves the score', () => {
  let state = applyImageEvent(INITIAL_IMAGE_RUN, { type: 'start', report: base })
  state = applyImageEvent(state, { type: 'probe_start', probe: 'img_generate_basic' })
  assert.equal(state.activeProbe, 'img_generate_basic')
  state = applyImageEvent(state, { type: 'check', check: check('img_generate_basic', 'assertion', 'pass') })
  state = applyImageEvent(state, { type: 'check', check: check('img_prov_c2pa', 'provenance', 'fail') })
  assert.equal(state.report?.score, 100)
  assert.equal(state.report?.summary.fail, 1)
})

test('an interrupted run records unrun planned checks as skipped', () => {
  let state = applyImageEvent(INITIAL_IMAGE_RUN, { type: 'start', report: base })
  state = interruptImageRun(state, true, 1200, null)
  assert.equal(state.phase, 'cancelled')
  const codes = Object.fromEntries(state.report!.checks.map((c) => [c.id, c.code]))
  assert.equal(codes.img_generate_basic, 'cancelled')
  assert.equal(codes.img_edit_basic, 'not_requested')
})

test('budget grows with suite and options', () => {
  const basic = imageBudget({ suite: 'basic', provenance: false, baseline: false })
  const std = imageBudget({ suite: 'standard', provenance: false, baseline: false })
  const full = imageBudget({ suite: 'full', provenance: true, baseline: true })
  assert.deepEqual(basic, { requests: 2, images: 1, verifyCalls: 0 })
  assert.deepEqual(std, { requests: 11, images: 11, verifyCalls: 0 })
  assert.deepEqual(full, { requests: 20, images: 20, verifyCalls: 5 })
  // baseline alone implies the provenance stage on the server
  assert.equal(imageBudget({ suite: 'basic', provenance: false, baseline: true }).verifyCalls, 3)
})

test('every stage and check has a title', () => {
  assert.ok(IMAGE_STAGES.length >= 6)
  for (const id of ['img_generate_basic', 'img_prov_baseline', 'img_performance'])
    assert.ok(IMAGE_CHECK_TITLES[id])
})

test('stream parser handles split frames and rejects truncation', async () => {
  const frame = `data: ${JSON.stringify({ type: 'start', report: base })}\n\ndata: ${JSON.stringify({ type: 'done', report: base })}\n\n`
  const bytes = new TextEncoder().encode(frame)
  const make = (parts: Uint8Array[]) =>
    new ReadableStream<Uint8Array>({
      start(c) {
        for (const p of parts) c.enqueue(p)
        c.close()
      },
    })
  const seen: string[] = []
  await readImageStream(make([bytes.slice(0, 17), bytes.slice(17)]), (e) => seen.push(e.type))
  assert.deepEqual(seen, ['start', 'done'])
  await assert.rejects(readImageStream(make([bytes.slice(0, 40)]), () => undefined))
})
