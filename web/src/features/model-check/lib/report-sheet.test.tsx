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
import { test } from 'node:test'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'
import { ReportSheet } from '../components/report-sheet'
import type { ClaudeCheckReport, ClaudeCheckSample } from '../types'
import { requestBudget } from './check-plan'
import { reportHTML } from './report-export'
import {
  sheetAssessment,
  totalInput,
  awsCounterComparison,
  distribution,
  reportHeaders,
  safeReportEndpoint,
} from './report-sheet'
import { INITIAL_RUN, applyCheckEvent } from './run-state'

const report = (): ClaudeCheckReport => ({
  version: 9,
  id: 'report-fixture',
  model: 'claude-opus-5',
  channel_id: 0,
  channel_name: 'Fixture',
  transport: 'anthropic_proxy',
  started_at: '2026-09-15T12:00:00Z',
  duration_ms: 3000,
  cancelled: false,
  summary: { pass: 0, fail: 0, inconclusive: 0, skipped: 0 },
  checks: [],
  samples: [],
  plan: [],
})
const sample = (probe: string): ClaudeCheckSample => ({
  probe,
  http_status: 200,
  valid_response: true,
  duration_ms: 2000,
  usage: {
    input_tokens: 10,
    output_tokens: 128,
    cache_creation_input_tokens: 0,
    cache_read_input_tokens: 0,
  },
})

test('channel prompt evidence survives progress and HTML export while preserving its reference deltas', async () => {
  const r = report()
  r.options = {
    model: r.model,
    thinking: false,
    cache: true,
    prompt_audit: true,
  }
  const audit = {
    version: 3,
    prompt: [],
    cache: [],
    prompt_assessment: {
      score: 60,
      coverage: 100,
      behavior_points: 30,
      token_points: 30,
      behavior_measured: 3,
      token_measured: 3,
      token_source: 'saved_reference',
    },
    injection: {
      code: 'reference_fixed_excess',
      local_changes: 0,
      repeat_delta: 0,
      system_delta: 30,
      incompatible_references: 1,
      references: [
        {
          id: 'reference-id',
          report_id: 'source-report',
          type: 'aws',
          code: 'reference_fixed_excess',
          pairs: [
            {
              id: 'floor_a',
              expected: 34,
              actual: 98,
              difference: 64,
              tolerance: 2,
            },
          ],
        },
      ],
    },
  }
  let state = applyCheckEvent(INITIAL_RUN, { type: 'start', report: r })
  state = applyCheckEvent(state, { type: 'token_audit', token_audit: audit })
  const restored = JSON.parse(JSON.stringify(state.report)) as ClaudeCheckReport
  assert.equal(
    restored.token_audit?.injection?.references[0].pairs[0].difference,
    64
  )
  assert.equal(sheetAssessment(restored).prompt.score, 60)
  const instance = createInstance()
  await instance.init({ lng: 'en', resources: {}, initImmediate: false })
  const html = renderToStaticMarkup(
    <I18nextProvider i18n={instance}>
      <ReportSheet
        report={restored}
        running={false}
        phaseLabel='Report complete'
      />
    </I18nextProvider>
  )
  assert.match(html, /Stable extra input relative to references/)
  assert.match(html, /source-report/)
  assert.match(html, /34 → 98 · Δ \+64/)
  assert.match(html, /Reference token consistency 30 \/ 70/)
  assert.doesNotMatch(html, /No extra input observed/)
})

test('prompt evidence scores survive history and export without inventing token coverage', async () => {
  const r = report()
  r.token_audit = {
    version: 2,
    prompt_assessment: {
      score: 30,
      coverage: 30,
      behavior_points: 30,
      token_points: 0,
      behavior_measured: 3,
      token_measured: 0,
    },
    prompt: ['short', 'system', 'long'].map((id) => ({
      id,
      expected: null,
      actual: 100,
      difference: null,
      score: null,
      code: 'count_unavailable',
      behavior: { score: 100, code: 'prompt_answer_matched' },
    })),
    cache: [],
  }
  const restored = JSON.parse(JSON.stringify(r)) as ClaudeCheckReport
  const data = sheetAssessment(restored)
  assert.equal(data.prompt.score, 30)
  assert.equal(data.prompt.measured, 0)
  assert.equal(data.dimensions.find((d) => d.id === 'prompt')?.score, 30)
  const instance = createInstance()
  await instance.init({ lng: 'en', resources: {}, initImmediate: false })
  const html = renderToStaticMarkup(
    <I18nextProvider i18n={instance}>
      <ReportSheet
        report={restored}
        running={false}
        phaseLabel='Report complete'
      />
    </I18nextProvider>
  )
  assert.match(html, /30\/100/)
  assert.match(html, /Evidence coverage 30%/)
  assert.match(html, /Token consistency 0 \/ 70/)
  assert.match(html, /Behavior 3 \/ 3/)
  restored.token_audit!.version = 1
  assert.equal(sheetAssessment(restored).prompt.score, null)
})

test('empty and missing measurements never become zero or a clean-prompt assertion', () => {
  const data = sheetAssessment(report())
  assert.equal(data.score, null)
  assert.equal(data.prompt.score, null)
  assert.equal(data.warm.rate, null)
  assert.equal(data.timing.median, null)
  assert.equal(data.timing.deviation, null)
  assert.equal(
    totalInput({
      ...sample('basic'),
      usage: { ...sample('basic').usage, cache_read_input_tokens: null },
    }),
    null
  )
  assert.equal(totalInput(sample('prompt_audit_short_count')), null)
})
test('cache hits and write/read consistency remain independent', () => {
  const r = report()
  r.samples = [
    {
      ...sample('cache_write'),
      usage: { ...sample('basic').usage, cache_creation_input_tokens: 2249 },
    },
    ...['cache_read_1', 'cache_read_2'].map((probe, i) => ({
      ...sample(probe),
      usage: {
        ...sample('basic').usage,
        cache_read_input_tokens: 2239 - i * 10,
      },
    })),
  ]
  r.token_audit = {
    version: 1,
    prompt: [],
    cache: [
      {
        id: 'cache_read_1',
        expected: 2249,
        actual: 2239,
        difference: -10,
        score: 99,
        code: 'tokens_differ',
      },
      {
        id: 'cache_read_2',
        expected: 2249,
        actual: 2229,
        difference: -20,
        score: 99,
        code: 'tokens_differ',
      },
    ],
  }
  const before = JSON.stringify(r),
    data = sheetAssessment(r)
  assert.equal(data.warm.rate, 100)
  assert.equal(data.cache.score, 99)
  assert.equal(data.cache.equal, 0)
  assert.equal(data.prompt.score, null)
  assert.equal(totalInput(r.samples[1]), 2249)
  assert.equal(JSON.stringify(r), before)
})
test('expected Bedrock rejections do not inflate the summary', () => {
  const r = report()
  r.checks = [
    { id: 'basic', status: 'fail', code: 'invalid_response' },
    {
      id: 'bedrock_web_search',
      status: 'pass',
      code: 'bedrock_expected_rejection',
    },
  ]
  assert.equal(sheetAssessment(r).score, 0)
  r.checks.push({
    id: 'model_consistency',
    status: 'fail',
    code: 'model_mismatch',
  })
  assert.equal(sheetAssessment(r).identity, 'mismatch')
  assert.equal(sheetAssessment(r).dimensions[0].score, 0)
})
test('timing dispersion excludes short, invalid and non-comparable requests', () => {
  const r = report()
  const perf = (probe: string, time: number): ClaudeCheckSample => ({
    ...sample(probe),
    request_profile: 'same',
    stream: {
      first_text_ms: time,
      last_text_ms: 1500,
      max_text_gap_ms: 100,
      text_events: 3,
      events: 4,
    },
    first_event_ms: time - 20,
  })
  r.samples = [
    perf('performance_1', 100),
    perf('performance_2', 200),
    perf('performance_3', 300),
    perf('cache_read_1', 1800),
    perf('repeat_1', 1600),
  ]
  assert.deepEqual(sheetAssessment(r).timing, {
    count: 3,
    median: 200,
    deviation: 100,
  })
  r.samples[2].first_event_ms = 4000
  assert.equal(sheetAssessment(r).firstEvent.count, 2)
  r.samples[2].valid_response = false
  assert.equal(sheetAssessment(r).timing.count, 2)
  assert.deepEqual(distribution([0]), { count: 1, median: 0, deviation: null })
})
test('AWS reconciliation respects missing cache counters and counts explicit mismatches', () => {
  const s = {
    ...sample('basic'),
    headers: {
      'X-Amzn-Bedrock-Input-Token-Count': '11',
      'x-amzn-bedrock-output-token-count': '128',
      Authorization: 'private',
      'set-cookie': 'private',
    },
  }
  assert.deepEqual(awsCounterComparison(s), { compared: 2, differences: 1 })
  s.usage.cache_read_input_tokens = null
  assert.deepEqual(awsCounterComparison(s), { compared: 1, differences: 0 })
  assert.equal(reportHeaders(s).length, 2)
  assert.equal(
    safeReportEndpoint('https://user:password@example.com/api?key=secret#x'),
    'https://example.com/api'
  )
})
test('HTML uses the same rendered report and escapes untrusted labels', async () => {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    fallbackLng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })
  const r = report()
  r.model = '<img src=x onerror=alert(1)>'
  r.samples = [
    {
      ...sample('basic'),
      headers: { server: 'nginx', 'x-api-key': 'private-fixture' },
    },
  ]
  const markup = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <ReportSheet report={r} running={false} phaseLabel='Report complete' />
    </I18nextProvider>
  )
  const html = reportHTML(markup, r.model, 'en')
  assert.match(html, /Token ledger/)
  assert.match(html, /Measured dimension average/)
  assert.match(html, /&lt;img/)
  assert.doesNotMatch(html, /<img src=x/)
  assert.doesNotMatch(html, /private-fixture/)
  assert.match(html, /default-src 'none'/)
  assert.doesNotMatch(html, /purity|authenticity probability|<script/i)
  assert.match(html, /insufficient|unavailable|unscored/i)
})

test('repeated audits preserve samples and use final evidence scores in progress and export', async () => {
  const r = report()
  r.options = {
    model: r.model,
    thinking: false,
    cache: true,
    prompt_audit: true,
  }
  r.version = 10
  r.token_audit = {
    version: 4,
    prompt: [],
    cache: [
      {
        id: 'cache_read_1',
        expected: 5000,
        actual: 5000,
        difference: 0,
        score: 100,
        code: 'tokens_equal',
      },
    ],
    cache_assessment: {
      score: null,
      measured: 1,
      planned: 6,
      rounds: 2,
      coverage: 17,
    },
    prompt_assessment: {
      score: null,
      behavior_points: 30,
      token_points: 0,
      behavior_measured: 9,
      behavior_planned: 9,
      token_measured: 0,
      coverage: 30,
      token_source: 'reference_unavailable',
    },
    injection: {
      code: 'prompt_samples_unstable',
      local_changes: 0,
      repeat_delta: null,
      system_delta: 30,
      incompatible_references: 0,
      references: [],
      sampling: [
        {
          id: 'floor_a',
          planned: 3,
          valid: 3,
          median: 30,
          min: 30,
          max: 90,
          stable: false,
        },
      ],
    },
  }
  assert.equal(sheetAssessment(r, true).cache.score, null)
  assert.equal(sheetAssessment(r, true).prompt.score, null)
  r.token_audit.cache_assessment!.score = 83
  r.token_audit.cache_assessment!.measured = 6
  r.token_audit.cache_assessment!.coverage = 100
  r.token_audit.prompt_assessment!.score = 30
  const state = applyCheckEvent(INITIAL_RUN, { type: 'done', report: r })
  const restored = JSON.parse(JSON.stringify(state.report)) as ClaudeCheckReport
  assert.equal(sheetAssessment(restored).cache.score, 83)
  assert.equal(sheetAssessment(restored).prompt.score, 30)
  const instance = createInstance()
  await instance.init({ lng: 'en', resources: {}, initImmediate: false })
  const html = renderToStaticMarkup(
    <I18nextProvider i18n={instance}>
      <ReportSheet report={restored} running={false} phaseLabel='Complete' />
    </I18nextProvider>
  )
  assert.match(html, /Median 30.*Range 30–90/)
  assert.match(html, /Behavior 9.*9/)
  assert.match(html, /Measured reads 6.*6/)
  assert.match(html, /83\/100/)
  assert.match(
    reportHTML(html, 'Repeated audit', 'en'),
    /no injection conclusion/
  )
  assert.equal(
    requestBudget(
      {
        suite: 'focused',
        thinking: false,
        model: 'claude',
        cache: true,
        prompt_audit: true,
      },
      10
    ),
    36
  )
  assert.equal(
    requestBudget(
      {
        suite: 'focused',
        thinking: false,
        model: 'claude',
        cache: true,
        prompt_audit: true,
      },
      9
    ),
    19
  )
})

test('practical prompt score distinguishes behavior-only coverage from input verification', async () => {
  const r = report()
  r.options = {
    model: r.model,
    thinking: false,
    cache: true,
    prompt_audit: true,
  }
  r.version = 11
  r.token_audit = {
    version: 5,
    prompt: [],
    cache: [],
    prompt_assessment: {
      scoring: 'measured_checks',
      score: 100,
      coverage: 30,
      behavior_score: 100,
      behavior_points: 30,
      token_points: 0,
      behavior_measured: 9,
      behavior_planned: 9,
      token_measured: 0,
      token_source: 'unavailable',
    },
    injection: {
      code: 'prompt_behavior_only',
      local_changes: 0,
      repeat_delta: null,
      system_delta: 30,
      incompatible_references: 1,
      selected_reference_id: 'legacy',
      sampling: [
        {
          id: 'floor_a',
          valid: 3,
          planned: 3,
          median: 15,
          min: 15,
          max: 26,
          stable: true,
          outliers: 1,
        },
      ],
      references: [
        {
          id: 'legacy',
          report_id: 'v7',
          type: 'anthropic',
          code: 'reference_incomplete',
          pairs: [],
        },
      ],
    },
  }
  assert.equal(sheetAssessment(r).prompt.score, 100)
  const instance = createInstance()
  await instance.init({ lng: 'en', resources: {}, initImmediate: false })
  const html = renderToStaticMarkup(
    <I18nextProvider i18n={instance}>
      <ReportSheet report={r} running={false} phaseLabel='Complete' />
    </I18nextProvider>
  )
  assert.match(html, /Prompt consistency score/)
  assert.match(html, /Coverage 30%/)
  assert.match(html, /Only instruction behavior was measured/)
  assert.match(html, /Full marks do not prove/)
  assert.match(html, /Outlier samples: 1/)
  assert.match(html, /v7/)
  assert.match(html, /Reference used for scoring/)
  assert.doesNotMatch(
    html,
    /Behavior alone earns at most 30|References must use this test version/
  )
  const restored = JSON.parse(JSON.stringify(r)) as ClaudeCheckReport
  assert.equal(
    restored.token_audit?.prompt_assessment?.scoring,
    'measured_checks'
  )
  assert.match(
    reportHTML(html, 'Prompt audit', 'en'),
    /Saved references are optional/
  )
})
