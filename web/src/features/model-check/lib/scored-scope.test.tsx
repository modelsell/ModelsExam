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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'
import { BedrockDiagnostics } from '../components/bedrock-diagnostics'
import { CheckForm } from '../components/check-form'
import { CheckReport } from '../components/check-report'
import { ReportSheet } from '../components/report-sheet'
import { BASELINE_QUERY_KEY } from '../hooks/use-baselines'
import type { ClaudeCheckReport } from '../types'
import { DEFAULT_CHECK_OPTIONS } from './check-defaults'
import { createCheckPlan, requestBudget } from './check-plan'
import { modelCheckOptions, modelCheckSchema } from './form'
import { reportDownloadData } from './report-download'
import { reportHTML } from './report-export'
import { sheetAssessment } from './report-sheet'
import { INITIAL_RUN } from './run-state'

const report = (): ClaudeCheckReport => ({
  version: 17,
  id: 'scored-report',
  model: 'claude-opus-5',
  channel_id: 0,
  channel_name: 'Example',
  transport: 'bedrock_runtime',
  started_at: '2026-09-16T00:00:00Z',
  duration_ms: 100,
  cancelled: false,
  summary: { pass: 1, fail: 1, skipped: 0, inconclusive: 1 },
  options: { ...DEFAULT_CHECK_OPTIONS, model: 'claude-opus-5' },
  samples: [],
  checks: [
    { id: 'basic', status: 'fail', code: 'invalid_response' },
    { id: 'model_consistency', status: 'pass', code: 'consistent' },
  ],
})
async function render(children: React.ReactNode) {
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: {}, initImmediate: false })
  const client = new QueryClient()
  client.setQueryData(BASELINE_QUERY_KEY, [])
  return renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>{children}</I18nextProvider>
    </QueryClientProvider>
  )
}

test('current form restores optional Bedrock diagnostics while removing retired preference sampling', async () => {
  const options = modelCheckOptions(
    modelCheckSchema.parse({
      mode: 'channel',
      model: 'claude',
      base_url: '',
      key: '',
      cache: true,
      thinking: false,
      repeat: false,
      vision: false,
      pdf: false,
      stream_comparison: false,
      fingerprint: true,
      bedrock: true,
    })
  )
  assert.notEqual(options.fingerprint, true)
  assert.equal(options.bedrock, true)
  assert.equal(requestBudget(options), 31)
  const form = await render(
    <CheckForm busy={false} onStart={async () => {}} onCancel={() => {}} />
  )
  assert.doesNotMatch(form, /check-fingerprint/)
  assert.match(form, /Optional scored checks/)
  assert.match(form, /Optional diagnostics/)
  assert.match(form, /Up to 8 requests:.*invalid thinking signature rejection/)
  const toggle = form
    .match(/<input[^>]*>/g)
    ?.find((tag) => tag.includes('id="check-bedrock"'))
  assert.ok(toggle)
  assert.doesNotMatch(toggle, /\schecked(?:=|\s|\/>)/)
  assert.match(form, /check-prompt_audit/)
  assert.match(form, /check-pdf/)
  assert.match(form, /check-vision/)
  assert.match(form, /Up to 23 upstream requests/)
  const preview = await render(<CheckReport state={INITIAL_RUN} elapsed={0} />)
  assert.match(preview, /0 \/ 23 requests/)
})

test('v17 plans and budgets ignore retired flags while preserving historical scope', () => {
  const options = { ...DEFAULT_CHECK_OPTIONS, fingerprint: true, bedrock: true }
  assert.equal(requestBudget(options, 17), 19)
  assert.equal(
    requestBudget(
      { ...options, prompt_audit: true, pdf: true, vision: true },
      17
    ),
    24
  )
  assert.equal(requestBudget(options, 16), 46)
  assert.equal(
    requestBudget(
      { ...options, prompt_audit: true, pdf: true, vision: true },
      16
    ),
    51
  )
  const current = createCheckPlan(options, 17).map((item) => item.id)
  assert.ok(!current.includes('behavior_fingerprint'))
  assert.ok(!current.some((id) => id.startsWith('bedrock_')))
  assert.ok(!current.includes('cache'))
  assert.ok(current.includes('cache_token_audit'))
  assert.ok(
    createCheckPlan(options, 16).some(
      (item) => item.id === 'behavior_fingerprint'
    )
  )
})

test('main report removes unscored summaries without hiding zeros or changing historical data', async () => {
  const r = report()
  r.options = { ...r.options!, fingerprint: true, bedrock: true }
  r.fingerprint = {
    version: 2,
    repetitions: 10,
    cells: [
      {
        id: 'letter',
        profile: 'a',
        valid: 10,
        attempts: 10,
        counts: { a: 10 },
      },
    ],
  }
  r.plan = createCheckPlan(r.options, 16)
  r.checks.push(
    { id: 'cache', status: 'pass', code: 'cache_observed' },
    {
      id: 'bedrock_web_search',
      status: 'pass',
      code: 'bedrock_expected_rejection',
    }
  )
  const before = JSON.stringify(r)
  const html = reportHTML(
    await render(
      <ReportSheet report={r} running={false} phaseLabel='Complete' />
    ),
    'Report',
    'en'
  )
  assert.match(html, /Scored checks/)
  assert.match(html, /0\/100/)
  assert.doesNotMatch(
    html,
    /Answer preference sampling|AWS header reconciliation|Cold request cache counters|Behavior fingerprint suite/
  )
  assert.equal(JSON.stringify(r), before)
})

test('core request error classification remains available in detailed evidence', async () => {
  const r = report()
  r.samples.push({
    probe: 'basic',
    http_status: 400,
    duration_ms: 100,
    valid_response: false,
    error: 'ValidationException: unsupported parameter',
    diagnostic: {
      code: 'unsupported_parameter',
      exception: 'ValidationException',
      source: 'aws',
      retryable: false,
    },
    usage: {
      input_tokens: null,
      output_tokens: null,
      cache_creation_input_tokens: null,
      cache_read_input_tokens: null,
    },
  })
  const html = await render(
    <CheckReport
      state={{ ...INITIAL_RUN, phase: 'complete', report: r }}
      elapsed={100}
    />
  )
  assert.match(html, /ValidationException/)
  assert.match(html, /unsupported_parameter/)
  assert.match(html, /Bedrock compatibility diagnostics/)
})

test('audit cards require selected tests or historical row evidence, not an empty assessment', async () => {
  const r = report()
  r.token_audit = {
    version: 7,
    prompt: [],
    cache: [],
    prompt_assessment: {
      scoring: 'input_budget',
      score: null,
      coverage: 0,
      behavior_points: 0,
      token_points: 0,
      behavior_measured: 0,
      token_measured: 0,
    },
  }
  let html = await render(
    <ReportSheet report={r} running={false} phaseLabel='Complete' />
  )
  assert.doesNotMatch(html, /<h3>Prompt injection score<\/h3>/)
  assert.match(html, /<h3>Cache write\/read token consistency<\/h3>/)
  r.options = { ...r.options!, cache: false, prompt_audit: true }
  html = await render(
    <ReportSheet report={r} running={false} phaseLabel='Complete' />
  )
  assert.match(html, /<h3>Prompt injection score<\/h3>/)
  assert.doesNotMatch(html, /<h3>Cache write\/read token consistency<\/h3>/)
})

test('v17 JSON export uses only the selected baseline and scored comparison dimensions', () => {
  const r = report()
  r.benchmark = {
    version: 3,
    items: [
      {
        id: 'instruction_format',
        category: 'instruction',
        profile: 'profile',
        expected: 'answer',
        correct: false,
        score: 50,
        grader: 'line-constraints-v1',
      },
    ],
  }
  r.options = {
    ...r.options!,
    baseline_id: 'selected',
    baseline_type: 'anthropic',
  }
  r.baselines = [
    {
      ...report(),
      id: 'selected',
      report_id: 'source',
      type: 'anthropic',
      created_at: 1,
      benchmark: {
        ...r.benchmark,
        items: [{ ...r.benchmark.items[0], correct: true, score: 100 }],
      },
      fingerprint: { version: 2, repetitions: 10, cells: [] },
    },
  ]
  const before = JSON.stringify(r)
  const data = reportDownloadData(r)
  assert.equal(data.scoring.value, sheetAssessment(r).score)
  assert.ok(!Object.hasOwn(data.report_sheet, 'aws_counters'))
  assert.ok('selected' in data.baseline_comparison)
  if ('selected' in data.baseline_comparison) {
    assert.equal(data.baseline_comparison.selected?.baseline_id, 'selected')
    assert.equal(data.baseline_comparison.selected?.capability.current, 50)
    assert.equal(data.baseline_comparison.selected?.capability.reference, 100)
  }
  assert.doesNotMatch(
    JSON.stringify(data.baseline_comparison),
    /fingerprint|protocol|candidates/
  )
  assert.equal(JSON.stringify(r), before)
  assert.deepEqual(data.baselines, r.baselines)
  delete r.options!.baseline_id
  assert.equal(reportDownloadData(r).baseline_comparison.selected, null)
  r.version = 16
  assert.ok(Object.hasOwn(reportDownloadData(r).report_sheet, 'aws_counters'))
  assert.ok(
    Object.hasOwn(reportDownloadData(r).baseline_comparison, 'candidates')
  )
})

test('v18 Bedrock opt-in adds seven requests without restoring fingerprints or changing v17 history', () => {
  assert.equal(requestBudget(DEFAULT_CHECK_OPTIONS, 18), 19)
  const options = { ...DEFAULT_CHECK_OPTIONS, bedrock: true, fingerprint: true }
  assert.equal(requestBudget(options, 18), 26)
  assert.equal(
    requestBudget(
      { ...options, prompt_audit: true, vision: true, pdf: true },
      18
    ),
    31
  )
  assert.equal(requestBudget(options, 17), 19)
  assert.equal(requestBudget(options, 16), 46)
  const plan = createCheckPlan(options, 18)
  assert.equal(plan.filter((item) => item.stage === 'bedrock').length, 7)
  assert.ok(
    !plan.some(
      (item) => item.id === 'behavior_fingerprint' || item.id === 'cache'
    )
  )
})

test('Bedrock transport alone never creates a diagnostics card, while a saved selected plan does', async () => {
  const r = { ...report(), version: 18 }
  assert.equal(
    await render(
      <BedrockDiagnostics report={r} running={false} activeProbe={null} />
    ),
    ''
  )
  const html = await render(
    <ReportSheet report={r} running={false} phaseLabel='Complete' />
  )
  assert.doesNotMatch(html, /Bedrock compatibility diagnostics/)
  r.plan = [
    {
      id: 'bedrock_beta',
      stage: 'bedrock',
      kind: 'observation',
      selected: true,
    },
  ]
  assert.match(
    await render(
      <BedrockDiagnostics
        report={r}
        running={true}
        activeProbe='bedrock_beta'
      />
    ),
    /Waiting for the upstream response/
  )
})

test('Bedrock observations preserve real HTTP evidence across HTML and JSON exports without capability points', async () => {
  const r = { ...report(), version: 18 }
  const originalScore = sheetAssessment(r).score
  r.options = { ...r.options!, bedrock: true }
  r.plan = createCheckPlan(r.options)
  r.checks.push({
    id: 'bedrock_web_search',
    status: 'pass',
    code: 'bedrock_expected_rejection',
  })
  r.samples.push({
    probe: 'bedrock_web_search',
    http_status: 400,
    valid_response: false,
    duration_ms: 2,
    error: 'ValidationException: unsupported web_search',
    diagnostic: {
      code: 'server_tools',
      source: 'aws',
      exception: 'ValidationException',
      retryable: false,
    },
    usage: {
      input_tokens: null,
      output_tokens: null,
      cache_creation_input_tokens: null,
      cache_read_input_tokens: null,
    },
  })
  const diagnosticHTML = await render(
    <BedrockDiagnostics report={r} running={false} activeProbe={null} paper />
  )
  assert.match(diagnosticHTML, /Bedrock compatibility diagnostics/)
  assert.match(diagnosticHTML, /HTTP 400/)
  assert.match(diagnosticHTML, /unsupported web_search/)
  assert.match(diagnosticHTML, /View evidence/)
  assert.doesNotMatch(diagnosticHTML, /\d+\/100|score-badge/)
  const html = reportHTML(
    await render(
      <ReportSheet report={r} running={false} phaseLabel='Complete' />
    ),
    'report',
    'en'
  )
  assert.match(html, /unsupported web_search/)
  assert.equal(sheetAssessment(r).score, originalScore)
  const data = reportDownloadData(r)
  assert.deepEqual(data.samples, r.samples)
  assert.deepEqual(data.plan, r.plan)
  assert.equal(data.scoring.value, originalScore)
  assert.ok(!Object.hasOwn(data.report_sheet, 'aws_counters'))
})
