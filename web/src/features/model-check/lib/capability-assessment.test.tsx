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
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { test } from 'node:test'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'
import { CapabilityAssessment } from '../components/capability-assessment'
import type { CapabilityBenchmark } from '../types'
import {
  CAPABILITY_IDS,
  CURRENT_CAPABILITY_IDS,
  capabilitySummary,
  capabilityPairs,
  capabilityPairSummary,
} from './capability-assessment'
import { requestBudget } from './check-plan'
import { reportHTML } from './report-export'

const benchmark = (): CapabilityBenchmark => ({
  version: 2,
  items: CAPABILITY_IDS.map((id) => ({
    id,
    category: '',
    profile: id,
    expected: 'reference',
    actual: 'answer',
    correct: true,
    score: 100,
    grader: 'exact-v1',
    code: 'scored',
  })),
})

test('v2 scores partial credit and zero while tracking all twelve planned questions', () => {
  const b = benchmark()
  b.items[0].score = 50
  b.items[1].score = 0
  b.items[2].score = null
  b.items[3].score = undefined
  b.items[4].score = NaN
  b.items[5].score = 101
  const result = capabilitySummary(b)
  assert.equal(result.score, 81)
  assert.equal(result.count, 8)
  assert.equal(result.total, 12)
  assert.equal(result.coverage, 67)
  assert.equal(result.groups[0].score, 25)
  assert.equal(result.groups[1].score, null)
  assert.equal(capabilitySummary({ version: 2, items: [] }).coverage, 0)
})

test('unknown and duplicate items cannot inflate scores or coverage', () => {
  const b = benchmark()
  b.items = [
    { ...b.items[0], score: 0 },
    b.items[0],
    { ...b.items[1], id: 'unknown' },
  ]
  assert.equal(capabilitySummary(b).score, 0)
  assert.equal(capabilitySummary(b).count, 1)
  assert.equal(capabilitySummary(b).total, 12)
  assert.equal(capabilitySummary({ ...b, version: 99 }).score, null)
})

test('historical v1 scores remain restricted to the six legacy questions', () => {
  const b = benchmark()
  b.version = 1
  b.items[0].correct = false
  b.items[1].correct = null
  assert.equal(capabilitySummary(b).count, 5)
  assert.equal(capabilitySummary(b).score, 80)
  assert.equal(capabilitySummary(b).total, 6)
})

test('baseline pairs retain partial scores and require compatible graders and profiles', () => {
  const a = benchmark(),
    b = benchmark()
  a.items[0].score = 50
  assert.equal(capabilityPairSummary(a, b).current, 96)
  assert.equal(capabilityPairs(a, b).length, 12)
  b.items[0].grader = 'different'
  b.items[1].profile = 'different'
  delete b.items[2].grader
  assert.equal(capabilityPairs(a, b).length, 9)
  b.version = 1
  assert.equal(capabilityPairs(a, b).length, 5)
  a.items[0].grader = 'other'
  assert.equal(capabilityPairs(a, b).length, 4)
})

test('current budgets include a two-request tool roundtrip and preserve v14 and v15 counts', () => {
  const base = {
    suite: 'focused' as const,
    model: 'claude',
    thinking: false,
    cache: true,
    fingerprint: true,
  }
  assert.equal(requestBudget(base), 23)
  assert.equal(requestBudget(base, 16), 39)
  assert.equal(requestBudget(base, 15), 45)
  assert.equal(requestBudget(base, 14), 38)
  const all = {
    ...base,
    bedrock: true,
    prompt_audit: true,
    vision: true,
    pdf: true,
  }
  assert.equal(requestBudget(all), 36)
  assert.equal(requestBudget(all, 17), 24)
  assert.equal(requestBudget(all, 16), 51)
  assert.equal(requestBudget(all, 15), 57)
  assert.equal(requestBudget(all, 14), 50)
})

test('report evidence renders safely with partial scores, reasons and printable details', async () => {
  const b = benchmark()
  b.items[0] = {
    ...b.items[0],
    actual: '<script>alert(1)</script>',
    score: 50,
    assertions: [{ id: 'exact_answer', passed: false }],
  }
  b.items[1] = { ...b.items[1], score: null, code: 'response_truncated' }
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: {}, initImmediate: false })
  const html = reportHTML(
    renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <CapabilityAssessment benchmark={b} running={false} />
      </I18nextProvider>
    ),
    'report',
    'en'
  )
  assert.match(html, /Scored 11 \/ 12 questions · Coverage 92%/)
  assert.match(html, /50\/100/)
  assert.match(html, /Truncated response; no capability score/)
  assert.match(html, /Tool use/)
  assert.match(html, /Question scores and grading evidence/)
  assert.match(html, /&lt;script&gt;/)
  assert.doesNotMatch(html, /<script>/)
})

// The request body is the unchanged digit_count fixture used in both suites.
// Its profile ignores only provider/model envelope fields, never prompt options.
test('legacy baseline and v2 share the real fixed-question profile and exact-v1 grader', () => {
  const profile = createHash('sha256')
    .update(
      JSON.stringify({
        max_tokens: 512,
        messages: [
          {
            content:
              'Write every integer from 0 to 9999 in ordinary decimal notation without leading zeros. How many times does digit 1 occur in total? Return only the count.',
            role: 'user',
          },
        ],
        system:
          'Reply using exactly one allowed answer. Do not explain or add formatting.',
        thinking: { type: 'disabled' },
      })
    )
    .digest('hex')
  const item = {
    id: 'digit_count',
    category: 'reasoning',
    profile,
    expected: '4000',
    correct: true,
  }
  const old: CapabilityBenchmark = { version: 1, items: [item] }
  const current: CapabilityBenchmark = {
    version: 2,
    items: [{ ...item, score: 100, grader: 'exact-v1' }],
  }
  assert.equal(capabilityPairs(current, old).length, 1)
  assert.equal(capabilityPairs(old, current).length, 1)
  current.items[0].grader = 'exact-v2'
  assert.equal(capabilityPairs(current, old).length, 0)
})

test('v3 scores only six new questions in four domains and preserves earlier suites', () => {
  const current = { ...benchmark(), version: 3 }
  const summary = capabilitySummary(current)
  assert.equal(summary.count, 6)
  assert.equal(summary.total, 6)
  assert.equal(summary.score, 100)
  assert.equal(summary.coverage, 100)
  assert.deepEqual(
    summary.rows.map((row) => row.id),
    CURRENT_CAPABILITY_IDS
  )
  assert.deepEqual(
    summary.groups.map((group) => [group.id, group.total]),
    [
      ['instruction', 1],
      ['structured', 2],
      ['context', 1],
      ['tools', 2],
    ]
  )
  const first = current.items.find((item) => item.id === 'instruction_format')!
  first.score = 50
  current.items.find((item) => item.id === 'json_extraction')!.score = null
  assert.equal(capabilitySummary(current).score, 90)
  assert.equal(capabilitySummary(current).coverage, 83)
  assert.equal(capabilitySummary(benchmark()).total, 12)
  assert.equal(capabilitySummary({ ...benchmark(), version: 1 }).total, 6)
})

test('v3 baselines compare only new shared questions and never match a v1 ability suite', () => {
  const current = { ...benchmark(), version: 3 }
  const previous = benchmark()
  assert.equal(capabilityPairs(current, previous).length, 6)
  assert.equal(capabilityPairs(previous, current).length, 6)
  assert.equal(capabilityPairs(current, { ...previous, version: 1 }).length, 0)
  assert.equal(capabilityPairs({ ...previous, version: 1 }, current).length, 0)
  previous.items.find((item) => item.id === 'tool_selection')!.grader =
    'changed'
  previous.items.find((item) => item.id === 'json_types')!.profile = 'changed'
  assert.equal(capabilityPairs(current, previous).length, 4)
})

test('v3 report omits retired question names and empty domains', async () => {
  const instance = createInstance()
  await instance.init({ lng: 'en', resources: {}, initImmediate: false })
  const html = renderToStaticMarkup(
    <I18nextProvider i18n={instance}>
      <CapabilityAssessment
        benchmark={{ ...benchmark(), version: 3 }}
        running={false}
      />
    </I18nextProvider>
  )
  assert.match(html, /Scored 6 \/ 6 questions/)
  assert.match(html, /Multi-constraint formatting/)
  assert.doesNotMatch(
    html,
    /Digit counting|Boolean constraints|Python aliasing|JavaScript microtasks|Chinese filtering constraints|Long-context retrieval|>Reasoning<|>Code</
  )
})

test('new nonfocused collection uses seven capability requests while saved plans keep twelve', () => {
  const options = {
    model: 'claude',
    cache: false,
    thinking: false,
    benchmark: true,
  }
  assert.equal(requestBudget(options), 15)
  assert.equal(requestBudget(options, 15), 20)
  assert.equal(requestBudget(options, 8), 20)
})
