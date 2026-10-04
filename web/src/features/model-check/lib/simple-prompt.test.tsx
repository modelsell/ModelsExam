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
import { TokenAudit } from '../components/token-audit'
import type { ClaudeCheckOptions, ClaudeCheckReport } from '../types'
import { requestBudget } from './check-plan'
import { reportHTML } from './report-export'
import { sheetAssessment } from './report-sheet'
import { INITIAL_RUN, applyCheckEvent } from './run-state'

const report = (): ClaudeCheckReport => ({
  version: 12,
  id: 'simple-report',
  model: 'claude-test',
  channel_id: 0,
  channel_name: 'Example',
  transport: 'anthropic_proxy',
  started_at: '2026-09-15T12:00:00Z',
  duration_ms: 100,
  cancelled: false,
  summary: { pass: 0, fail: 0, skipped: 0, inconclusive: 0 },
  plan: [],
  checks: [],
  samples: [],
  token_audit: {
    version: 6,
    prompt: [26, 26, 27].map((actual, index) => ({
      id: `minimal_r${index + 1}`,
      actual,
      expected: null,
      difference: null,
      score: null,
      code: 'input_observed',
    })),
    cache: [],
    prompt_assessment: {
      scoring: 'input_consistency',
      score: 70,
      token_score: 70,
      token_source: 'saved_reference',
      coverage: 100,
      behavior_points: 0,
      token_points: 0,
      behavior_measured: 0,
      token_measured: 3,
    },
    injection: {
      code: 'simple_extra_input',
      platform: 'anthropic',
      selected_reference_id: 'native-baseline',
      baseline_samples: 3,
      local_changes: 0,
      repeat_delta: null,
      system_delta: null,
      incompatible_references: 2,
      sampling: [
        {
          id: 'minimal',
          valid: 3,
          planned: 3,
          median: 26,
          min: 26,
          max: 27,
          stable: true,
        },
      ],
      references: [
        {
          id: 'native-baseline',
          report_id: 'trusted-run',
          type: 'anthropic',
          code: 'simple_extra_input',
          pairs: [
            {
              id: 'minimal',
              expected: 15,
              actual: 26,
              difference: 11,
              tolerance: 2,
            },
          ],
        },
      ],
    },
  },
})

test('minimal prompt results share the same evidence in progress and exported history', async () => {
  const r = report()
  const state = applyCheckEvent(INITIAL_RUN, { type: 'start', report: r })
  const final = applyCheckEvent(state, { type: 'done', report: r })
  const restored = JSON.parse(JSON.stringify(final.report)) as ClaudeCheckReport
  assert.equal(sheetAssessment(restored).prompt.score, 70)
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: {}, initImmediate: false })
  const progress = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <TokenAudit report={r} running />
    </I18nextProvider>
  )
  const sheet = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <ReportSheet report={restored} running={false} phaseLabel='Complete' />
    </I18nextProvider>
  )
  for (const html of [progress, reportHTML(sheet, 'Input consistency', 'en')]) {
    assert.match(html, /Input consistency score/)
    assert.match(html, /Baseline median 15 · Measured median 26 · Delta \+11/)
    assert.match(html, /Allowed input range 13–17/)
    assert.match(html, /Valid samples 3\/3/)
    assert.match(html, /trusted-run/)
    assert.match(html, /Return only the word PONG\./)
    assert.match(html, /Full marks do not prove/)
    assert.doesNotMatch(
      html,
      /Same-endpoint|Instruction preservation|Known system input|MAPLE-7391/
    )
  }
})

test('no baseline stays unscored despite three completed input samples', async () => {
  const r = report()
  const audit = r.token_audit!
  audit.prompt_assessment!.score = null
  audit.prompt_assessment!.token_score = null
  audit.prompt_assessment!.token_source = 'unavailable'
  audit.injection!.code = 'simple_no_reference'
  audit.injection!.references = []
  delete audit.injection!.selected_reference_id
  assert.equal(sheetAssessment(r).prompt.score, null)
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: {}, initImmediate: false })
  const html = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <TokenAudit report={r} running={false} />
    </I18nextProvider>
  )
  assert.match(html, /Awaiting a trusted baseline/)
  assert.match(html, /A missing baseline earns no score/)
  assert.match(html, /Valid samples 3\/3/)
  assert.doesNotMatch(html, /100 \/ 100/)
})

test('current prompt budget is three requests while older scopes retain their budgets', () => {
  const options: ClaudeCheckOptions = {
    suite: 'focused',
    model: 'claude-test',
    thinking: false,
    cache: false,
    prompt_audit: true,
  }
  assert.equal(requestBudget(options), 18)
  assert.equal(requestBudget(options, 18), 14)
  assert.equal(requestBudget(options, 11), 28)
  assert.equal(requestBudget(options, 10), 28)
  assert.equal(requestBudget(options, 9), 16)
})

test('mixed request evidence remains visible without a misleading missing-baseline instruction', async () => {
  const r = report()
  r.token_audit!.prompt_assessment!.score = null
  r.token_audit!.prompt_assessment!.token_score = null
  r.token_audit!.injection!.code = 'simple_request_mismatch'
  r.token_audit!.injection!.references = []
  delete r.token_audit!.injection!.selected_reference_id
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: {}, initImmediate: false })
  const html = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <TokenAudit report={r} running={false} />
    </I18nextProvider>
  )
  assert.match(html, /Request model or parameters differ across samples/)
  assert.match(html, />27<\/td>/)
  assert.doesNotMatch(html, /A missing baseline earns no score|100 \/ 100/)
})
