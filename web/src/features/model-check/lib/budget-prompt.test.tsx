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
import type { ClaudeCheckReport } from '../types'
import { reportHTML } from './report-export'
import { sheetAssessment } from './report-sheet'
import { INITIAL_RUN, applyCheckEvent } from './run-state'

const report = (): ClaudeCheckReport => ({
  version: 13,
  id: 'budget-report',
  model: 'claude-test',
  channel_id: 0,
  channel_name: 'Example',
  transport: 'anthropic_proxy',
  started_at: '2026-09-16T12:00:00Z',
  duration_ms: 100,
  cancelled: false,
  summary: { pass: 0, fail: 0, skipped: 0, inconclusive: 0 },
  checks: [],
  samples: [],
  plan: [],
  baselines: [],
  token_audit: {
    version: 7,
    prompt: [15, 64, 128].map((actual, index) => ({
      id: `minimal_r${index + 1}`,
      actual,
      expected: 64,
      difference: [0, 0, 64][index],
      score: [100, 100, 50][index],
      code: index === 2 ? 'budget_extra_input' : 'budget_within_allowance',
    })),
    cache: [],
    prompt_assessment: {
      scoring: 'input_budget',
      score: 83,
      token_score: 83,
      token_source: 'minimal_request_budget',
      coverage: 100,
      behavior_points: 0,
      token_points: 0,
      behavior_measured: 0,
      token_measured: 3,
    },
    injection: {
      code: 'budget_extra_input',
      allowance_tokens: 64,
      estimated_extra_tokens: 21,
      max_extra_tokens: 64,
      extra_rounds: 1,
      local_changes: 0,
      repeat_delta: null,
      system_delta: null,
      incompatible_references: 0,
      references: [],
      sampling: [
        {
          id: 'minimal',
          valid: 3,
          planned: 3,
          median: 64,
          min: 15,
          max: 128,
          stable: false,
        },
      ],
    },
  },
})

test('budget scoring preserves observed excess in progress, history and HTML without a baseline dependency', async () => {
  const r = report()
  let state = applyCheckEvent(INITIAL_RUN, { type: 'start', report: r })
  state = applyCheckEvent(state, {
    type: 'token_audit',
    token_audit: r.token_audit!,
  })
  state = applyCheckEvent(state, { type: 'done', report: r })
  const restored = JSON.parse(JSON.stringify(state.report)) as ClaudeCheckReport
  assert.equal(sheetAssessment(restored).prompt.score, 83)
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
  for (const html of [progress, reportHTML(sheet, 'Budget score', 'en')]) {
    assert.match(html, /Prompt injection score/)
    assert.match(html, /Input allowance 64 tokens · Measured median 64/)
    assert.match(html, /Average excess 21 · Maximum excess 64 tokens/)
    assert.match(html, /Valid samples 3\/3 · Above allowance 1 · Coverage 100%/)
    assert.match(html, />128<\/td>/)
    assert.match(html, /50\/100/)
    assert.match(html, /not an official token limit/)
    assert.match(html, /not an exact count of injected tokens/)
    assert.match(html, /input ≤ 64 earns 100/)
    assert.match(html, /min\(99, round\(100 × 64 \/ input\)\)/)
    assert.match(html, /rounded average of valid scores; capped at 99/)
    assert.doesNotMatch(
      html,
      /Same-endpoint|Instruction preservation|Awaiting a trusted baseline|Scoring baseline|Baseline median|MAPLE-7391/
    )
  }
})

test('one valid within-budget sample can show a score with limited coverage', async () => {
  const r = report()
  const audit = r.token_audit!
  audit.prompt[1] = {
    ...audit.prompt[1],
    actual: null,
    difference: null,
    score: null,
    code: 'missing_usage',
  }
  audit.prompt[2] = {
    ...audit.prompt[2],
    actual: null,
    difference: null,
    score: null,
    code: 'probe_unavailable',
  }
  Object.assign(audit.prompt_assessment!, {
    score: 100,
    token_score: 100,
    token_measured: 1,
    coverage: 33,
  })
  Object.assign(audit.injection!, {
    code: 'budget_no_obvious_injection',
    estimated_extra_tokens: 0,
    max_extra_tokens: 0,
    extra_rounds: 0,
  })
  Object.assign(audit.injection!.sampling![0], {
    valid: 1,
    median: 15,
    min: 15,
    max: 15,
  })
  assert.equal(sheetAssessment(r).prompt.score, 100)
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: {}, initImmediate: false })
  const html = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <TokenAudit report={r} running={false} />
    </I18nextProvider>
  )
  assert.match(html, /No obvious extra input observed/)
  assert.match(html, /100 \/ 100/)
  assert.match(html, /Valid samples 1\/3 · Above allowance 0 · Coverage 33%/)
  assert.match(html, /small additions or forged usage may go undetected/)
  assert.match(html, /Input usage not returned/)
  assert.doesNotMatch(html, /baseline/i)
})

test('observed local rewriting does not invent zero injected tokens', async () => {
  const r = report()
  const audit = r.token_audit!
  Object.assign(audit.prompt_assessment!, { score: null, token_score: null })
  Object.assign(audit.injection!, {
    code: 'local_prompt_changed',
    local_changes: 1,
    estimated_extra_tokens: null,
    max_extra_tokens: null,
  })
  assert.equal(sheetAssessment(r).prompt.score, null)
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: {}, initImmediate: false })
  const html = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <TokenAudit report={r} running={false} />
    </I18nextProvider>
  )
  assert.match(html, /Local outbound prompt content was changed/)
  assert.match(html, /Average excess — · Maximum excess — tokens/)
  assert.doesNotMatch(html, /Average excess 0|Maximum excess 0 tokens/)
})
