import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'
import { ReportSheet } from '../components/report-sheet'
import { UsageTokenIntegrity } from '../components/usage-token-integrity'
import type { ClaudeCheckReport, ClaudeCheckSample } from '../types'
import { baselineLedgerComparison } from './baseline-ledger'
import { DEFAULT_CHECK_OPTIONS } from './check-defaults'
import {
  createCheckPlan,
  probeCheckID,
  requestBudget,
  samplesForCheck,
} from './check-plan'
import { reportDownloadData } from './report-download'
import { reportHTML } from './report-export'
import { sheetAssessment, totalInput } from './report-sheet'
import {
  assessUsageTokens,
  USAGE_TOKEN_CASES,
  usageTokenCounters,
} from './usage-token-integrity'

function sample(index: number, input: number | null): ClaudeCheckSample {
  return {
    probe: USAGE_TOKEN_CASES[index].probe,
    http_status: 200,
    valid_response: true,
    duration_ms: 10,
    request_profile: `usage-profile-${index}`,
    usage: {
      input_tokens: input,
      output_tokens: 2,
      cache_creation_input_tokens: null,
      cache_read_input_tokens: null,
    },
  }
}
function report(values = [54, 324, 1524, 2109]): ClaudeCheckReport {
  return {
    version: 19,
    id: 'usage-report',
    model: 'claude-opus-5',
    channel_id: 0,
    channel_name: 'Example',
    transport: 'anthropic_proxy',
    started_at: '2026-09-18T00:00:00Z',
    duration_ms: 100,
    cancelled: false,
    summary: { pass: 1, fail: 0, inconclusive: 0, skipped: 0 },
    options: { ...DEFAULT_CHECK_OPTIONS, model: 'claude-opus-5', cache: false },
    plan: createCheckPlan({ ...DEFAULT_CHECK_OPTIONS, cache: false }),
    samples: values.map((value, index) => sample(index, value)),
    checks: [{ id: 'basic', status: 'pass', code: 'valid_response' }],
  }
}
function withBaseline(
  current: ClaudeCheckReport,
  values = [94, 724, 3524, 4034]
) {
  current.options = {
    ...current.options!,
    baseline_id: 'selected',
    baseline_type: 'anthropic',
  }
  current.baselines = [
    {
      ...report(values),
      id: 'selected',
      report_id: 'baseline-source',
      type: 'anthropic',
      created_at: 1,
    },
  ]
  return current
}
async function render(children: React.ReactNode) {
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: {}, initImmediate: false })
  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>{children}</I18nextProvider>
  )
}

test('four valid usage totals score linear growth without treating word counts as tokens', () => {
  const data = assessUsageTokens(report())
  assert.equal(data.active, true)
  assert.equal(data.measured, 4)
  assert.equal(data.growth_score, 100)
  assert.equal(data.score, 100)
  assert.equal(data.baseline_score, null)
  assert.equal(data.baseline_code, 'not_selected')
  assert.equal(data.long_short_ratio, 1524 / 54)
  assert.equal(data.slope_ratio, 1)
  assert.equal(assessUsageTokens(report([20, 290, 890, 600])).score, 63)
  assert.equal(assessUsageTokens(report([50, 50, 100, 600])).score, 0)
  assert.equal(assessUsageTokens(report([0, 270, 1470, 600])).score, null)
  assert.equal(
    assessUsageTokens(report([0, 270, 1470, 600])).long_short_ratio,
    null
  )
})

test('usage counters add explicit caches but preserve omitted counters as unreported', () => {
  const current = sample(0, 10)
  assert.equal(usageTokenCounters(current).total_input, 10)
  assert.equal(usageTokenCounters(current).cache_read, null)
  current.usage.cache_creation_input_tokens = 20
  current.usage.cache_read_input_tokens = 30
  assert.equal(usageTokenCounters(current).total_input, 60)
  assert.equal(totalInput(current), 60)
  for (const invalid of [-1, 0.5, Number.MAX_SAFE_INTEGER + 1, Infinity, NaN]) {
    current.usage.cache_read_input_tokens = invalid
    assert.equal(usageTokenCounters(current).code, 'invalid_usage')
  }
  current.usage = {
    ...current.usage,
    input_tokens: Number.MAX_SAFE_INTEGER,
    cache_creation_input_tokens: 1,
    cache_read_input_tokens: 0,
  }
  assert.equal(usageTokenCounters(current).code, 'usage_overflow')
  current.usage.input_tokens = null
  assert.equal(usageTokenCounters(current).code, 'missing_input')
  assert.equal(usageTokenCounters(sample(0, 0)).code, 'nonpositive_input')
  assert.equal(usageTokenCounters(sample(3, 0)).code, 'nonpositive_input')
  assert.equal(
    usageTokenCounters({ ...sample(0, 10), http_status: 400 }).code,
    'invalid_response'
  )
  assert.equal(
    usageTokenCounters({ ...sample(0, 10), valid_response: false }).code,
    'invalid_response'
  )
})

test('incomplete and duplicate usage samples reduce coverage without manufacturing zero scores', () => {
  const current = report()
  current.samples.pop()
  let data = assessUsageTokens(current)
  assert.equal(data.measured, 3)
  assert.equal(data.score, null)
  assert.equal(data.long_short_ratio, null)
  current.samples.push(sample(3, 2109), sample(0, 54))
  data = assessUsageTokens(current)
  assert.equal(data.measured, 3)
  assert.equal(data.rows[0].code, 'ambiguous_sample')
  assert.equal(data.score, null)
})

test('fixed matching baseline scores reported differences and changes protocol score', () => {
  const current = withBaseline(report())
  const original = JSON.stringify(current)
  const data = assessUsageTokens(current)
  assert.deepEqual(
    data.rows.map((row) => row.baseline_score),
    [57, 45, 43, 52]
  )
  assert.equal(data.growth_score, 100)
  assert.equal(data.baseline_score, 49)
  assert.equal(data.score, 49)
  assert.equal(data.rows[0].diff, -40)
  assert.ok(Math.abs(data.rows[0].diff_percent! - (-40 / 94) * 100) < 1e-10)
  assert.equal(
    sheetAssessment(current).dimensions.find((item) => item.id === 'protocol')
      ?.score,
    75
  )
  assert.equal(reportDownloadData(current).usage_token_assessment?.score, 49)
  assert.equal(JSON.stringify(current), original)
  const ledger = baselineLedgerComparison(current, current.samples[0])
  assert.equal(ledger.reference, 94)
  assert.equal(ledger.difference, -40)
})

test('baseline comparison uses exact probe and profile medians and the four-token or five-percent allowance', () => {
  const current = withBaseline(
    report([54, 324, 1524, 2109]),
    [50, 320, 1500, 2100]
  )
  current.baselines![0].samples.push(sample(0, 58), sample(0, 10000))
  current.baselines![0].samples.at(-1)!.request_profile = 'different'
  current.options!.baseline_type = ' Anthropic '
  const data = assessUsageTokens(current)
  assert.equal(data.rows[0].reference_input, 54)
  assert.equal(data.rows[0].reference_samples, 2)
  assert.equal(data.baseline_score, 100)
  assert.equal(data.score, 100)
  const zeroReference = withBaseline(report(), [0, 324, 1524, 2109])
  assert.equal(assessUsageTokens(zeroReference).rows[0].baseline_score, null)
  assert.equal(assessUsageTokens(zeroReference).rows[0].diff_percent, null)
})

test('whitespace-only usage profiles stay incomparable in assessment and token ledger', () => {
  const current = withBaseline(report())
  for (const item of [...current.samples, ...current.baselines![0].samples])
    item.request_profile = ' \t\n'
  const data = assessUsageTokens(current)
  assert.equal(data.growth_score, 100)
  assert.equal(data.score, 100)
  assert.equal(data.baseline_score, null)
  assert.equal(data.baseline_code, 'baseline_incomplete')
  for (const row of data.rows) {
    assert.equal(row.reference_input, null)
    assert.equal(row.baseline_score, null)
    assert.equal(row.reference_code, 'parameters_differ')
  }
  const ledger = baselineLedgerComparison(current, current.samples[0])
  assert.equal(ledger.reference, null)
  assert.equal(ledger.difference, null)
  assert.equal(ledger.code, 'parameters_differ')
})

test('missing and incompatible baselines never replace the selected snapshot or fill missing scores', () => {
  for (const mismatch of [
    'profile',
    'model',
    'type',
    'missing-type',
    'id',
    'duplicate',
  ]) {
    const current = withBaseline(report())
    if (mismatch === 'profile')
      current.baselines![0].samples[0].request_profile = 'different'
    if (mismatch === 'model') current.baselines![0].model = 'claude-sonnet-5'
    if (mismatch === 'type') current.baselines![0].type = 'ccmax'
    if (mismatch === 'missing-type') delete current.options!.baseline_type
    if (mismatch === 'id') current.options!.baseline_id = 'missing'
    if (mismatch === 'duplicate')
      current.baselines!.push({ ...current.baselines![0], type: 'ccmax' })
    const data = assessUsageTokens(current)
    assert.equal(data.baseline_score, null, mismatch)
    assert.equal(data.score, 100, mismatch)
  }
  const current = withBaseline(report())
  current.baselines![0].samples = []
  assert.equal(assessUsageTokens(current).baseline_code, 'baseline_incomplete')
})

test('new focused budgets and probe mappings include four usage requests without altering history or legacy', () => {
  const all = {
    ...DEFAULT_CHECK_OPTIONS,
    bedrock: true,
    prompt_audit: true,
    vision: true,
    pdf: true,
  }
  assert.equal(requestBudget(DEFAULT_CHECK_OPTIONS), 23)
  assert.equal(requestBudget(all), 36)
  assert.equal(requestBudget(DEFAULT_CHECK_OPTIONS, 18), 19)
  assert.equal(requestBudget(all, 18), 31)
  const legacy = {
    model: 'claude',
    cache: false,
    thinking: false,
    benchmark: true,
  }
  assert.equal(requestBudget(legacy, 19), requestBudget(legacy, 18))
  const plan = createCheckPlan(DEFAULT_CHECK_OPTIONS)
  assert.deepEqual(
    plan.slice(2, 5).map((item) => item.id),
    ['performance_sampling', 'usage_token_integrity', 'capability_benchmark']
  )
  assert.ok(
    !createCheckPlan(DEFAULT_CHECK_OPTIONS, 18).some(
      (item) => item.id === 'usage_token_integrity'
    )
  )
  for (const item of USAGE_TOKEN_CASES)
    assert.equal(probeCheckID(item.probe), 'usage_token_integrity')
  assert.equal(samplesForCheck(report(), 'usage_token_integrity').length, 4)
})

test('usage evidence shares live, historical and standalone report markup with no invented cache counters', async () => {
  const current = withBaseline(report())
  const html = await render(
    <ReportSheet report={current} running={false} phaseLabel='Complete' />
  )
  assert.match(html, /Usage token integrity/)
  assert.match(html, /49\/100/)
  assert.match(html, /English · 1,?500 words/)
  assert.match(html, /Chinese · 1,?000 characters/)
  assert.match(html, /28.22×/)
  assert.match(html, /-42.6%/)
  assert.match(html, /Omitted optional cache counts remain unreported/)
  assert.match(html, /<td>54<\/td><td>—<\/td><td>—<\/td><td>54<\/td>/)
  assert.ok(
    html.indexOf('<h3>Usage token integrity') < html.indexOf('<h3>Token ledger')
  )
  assert.match(reportHTML(html, 'Usage report', 'en'), /Usage token integrity/)
  const partial = report([])
  const live = await render(
    <UsageTokenIntegrity
      report={partial}
      running
      activeProbe='usage_tokens_en_30'
    />
  )
  assert.match(live, /Measured 0 \/ 4 usage samples/)
  assert.match(live, /Waiting for the upstream response/)
  assert.doesNotMatch(live, /0\/100|100\/100/)
  current.version = 18
  assert.equal(
    await render(<UsageTokenIntegrity report={current} running={false} />),
    ''
  )
  assert.ok(
    !Object.hasOwn(reportDownloadData(current), 'usage_token_assessment')
  )
})
