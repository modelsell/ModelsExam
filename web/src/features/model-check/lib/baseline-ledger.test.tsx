import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'
import { ReportSheetLedger } from '../components/report-sheet-ledger'
import type {
  ClaudeCheckReport,
  ClaudeCheckSample,
  ComparisonBaseline,
} from '../types'
import {
  baselineLedgerComparison,
  selectedLedgerBaseline,
} from './baseline-ledger'
import { reportHTML } from './report-export'

const sample = (
  probe: string,
  input: number,
  profile = 'same-question'
): ClaudeCheckSample => ({
  probe,
  request_profile: profile,
  http_status: 200,
  duration_ms: 10,
  valid_response: true,
  usage: {
    input_tokens: input,
    output_tokens: 1,
    cache_creation_input_tokens: 0,
    cache_read_input_tokens: 0,
  },
})

const baseline = (
  id: string,
  type: string,
  samples: ClaudeCheckSample[]
): ComparisonBaseline => ({
  id,
  type,
  samples,
  report_id: `source-${id}`,
  model: 'claude-opus-5',
  version: 13,
  channel_id: 1,
  channel_name: `Channel ${type}`,
  transport: 'anthropic_proxy',
  started_at: '2026-09-16T00:00:00Z',
  created_at: 1,
  checks: [],
})

const report = (
  samples: ClaudeCheckSample[],
  refs: ComparisonBaseline[]
): ClaudeCheckReport => ({
  ...baseline('run', 'test', samples),
  version: 14,
  duration_ms: 10,
  cancelled: false,
  summary: { pass: 0, fail: 0, skipped: 0, inconclusive: 0 },
  options: {
    model: 'claude-opus-5',
    cache: false,
    thinking: false,
    baseline_id: refs[0]?.id,
    baseline_type: refs[0]?.type,
    compare_baselines: true,
  },
  baselines: refs,
})

test('ledger uses only the selected saved baseline and includes cached input', () => {
  const target = sample('basic', 5)
  target.usage.cache_read_input_tokens = 700
  const reference = sample('basic', 2)
  reference.usage.cache_creation_input_tokens = 33
  const r = report(
    [target],
    [
      baseline('anthropic', 'anthropic', [sample('basic', 705)]),
      baseline('ccmax', 'ccmax', [reference]),
    ]
  )
  r.options!.baseline_id = 'ccmax'
  r.options!.baseline_type = 'ccmax'
  assert.deepEqual(baselineLedgerComparison(r, target), {
    reference: 35,
    difference: 670,
    samples: 1,
    code: 'matched',
  })
  assert.equal(selectedLedgerBaseline(r)?.id, 'ccmax')
})

test('missing selections and invalid snapshots never fall back to a nearby baseline', () => {
  const target = sample('basic', 43)
  const r = report(
    [target],
    [baseline('reference', 'anthropic', [sample('basic', 43)])]
  )
  delete r.options!.baseline_id
  assert.equal(baselineLedgerComparison(r, target).code, 'no_baseline')
  r.options!.baseline_id = 'removed'
  assert.equal(baselineLedgerComparison(r, target).code, 'snapshot_unavailable')
  r.options!.baseline_id = 'reference'
  r.options!.baseline_type = 'ccmax'
  assert.equal(baselineLedgerComparison(r, target).code, 'snapshot_unavailable')
  r.options!.baseline_type = 'anthropic'
  r.baselines![0].model = 'claude-sonnet-5'
  assert.equal(baselineLedgerComparison(r, target).code, 'snapshot_unavailable')
})

test('same minimal question uses the selected reference median across old repeat names', () => {
  const target = sample('prompt_audit_minimal_r1', 7078)
  const r = report(
    [target],
    [
      baseline(
        'reference',
        'anthropic',
        [43, 44, 45].map((n, i) => sample(`prompt_audit_floor_a_r${i + 1}`, n))
      ),
    ]
  )
  assert.deepEqual(baselineLedgerComparison(r, target), {
    reference: 44,
    difference: 7034,
    samples: 3,
    code: 'matched',
  })
  r.baselines![0].samples[1].request_profile = 'changed-max-tokens'
  assert.equal(baselineLedgerComparison(r, target).samples, 2)
})

test('missing questions and changed request parameters keep references unavailable', () => {
  const target = sample('prompt_audit_minimal_r1', 43)
  const r = report(
    [target],
    [baseline('v7', 'anthropic', [sample('basic', 43)])]
  )
  assert.equal(baselineLedgerComparison(r, target).code, 'no_sample')
  const cache = sample('cache_read_1', 43, 'unique-run-nonce')
  r.baselines![0].samples = [sample('cache_read_1', 43, 'old-run-nonce')]
  assert.equal(baselineLedgerComparison(r, cache).code, 'parameters_differ')
  assert.equal(baselineLedgerComparison(r, cache).reference, null)
})

test('renamed historical probes remain comparable only with matching request parameters', () => {
  const target = sample('pdf', 400)
  const r = report(
    [target],
    [baseline('old', 'anthropic', [sample('veridrop_pdf', 390)])]
  )
  assert.deepEqual(baselineLedgerComparison(r, target), {
    reference: 390,
    difference: 10,
    samples: 1,
    code: 'matched',
  })
})

test('cache write and each read stage remain separate even for identical request bodies', () => {
  const target = sample('cache_read_1', 50)
  const r = report(
    [target],
    [
      baseline('reference', 'anthropic', [
        sample('cache_write', 100),
        sample('cache_read_1', 30),
        sample('cache_read_2', 70),
      ]),
    ]
  )
  assert.deepEqual(baselineLedgerComparison(r, target), {
    reference: 30,
    difference: 20,
    samples: 1,
    code: 'matched',
  })
})

test('invalid, missing, overflowed and CountTokens usage cannot become reference tokens', () => {
  const target = sample('basic', 43)
  for (const input of [null, -1, Number.MAX_SAFE_INTEGER + 1, 1.5]) {
    const reference = sample('basic', 43)
    reference.usage.input_tokens = input
    const r = report(
      [target],
      [baseline('reference', 'anthropic', [reference])]
    )
    assert.equal(
      baselineLedgerComparison(r, target).code,
      'reference_usage_unavailable'
    )
  }
  const reference = sample('basic', 43)
  reference.usage.cache_read_input_tokens = null
  const r = report([target], [baseline('reference', 'anthropic', [reference])])
  assert.equal(baselineLedgerComparison(r, target).reference, null)
  r.baselines![0].samples = [sample('count_tokens', 43)]
  assert.equal(baselineLedgerComparison(r, target).code, 'no_sample')
  r.baselines![0].samples = [sample('basic', 43)]
  target.valid_response = false
  assert.deepEqual(baselineLedgerComparison(r, target), {
    reference: 43,
    difference: null,
    samples: 1,
    code: 'current_usage_unavailable',
  })
})

test('history and exported ledger display saved input rather than the injection allowance', async () => {
  const target = sample('prompt_audit_minimal_r1', 7078)
  const r = report(
    [target],
    [
      baseline('reference', 'anthropic', [
        sample('prompt_audit_minimal_r1', 43),
      ]),
    ]
  )
  r.token_audit = {
    version: 7,
    cache: [],
    prompt: [
      {
        id: 'minimal_r1',
        actual: 7078,
        expected: 64,
        difference: 7014,
        score: 1,
        code: 'budget_extra_input',
      },
    ],
  }
  const restored = JSON.parse(JSON.stringify(r)) as ClaudeCheckReport
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: {}, initImmediate: false })
  const html = reportHTML(
    renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <ReportSheetLedger report={restored} />
      </I18nextProvider>
    ),
    'Report',
    'en'
  )
  assert.match(html, /Baseline: anthropic/)
  assert.match(html, /Compared 1 \/ 1 rows/)
  assert.match(html, /title="Matched 1 baseline samples">43<\/td>/)
  assert.match(html, /\+7,035/)
  assert.doesNotMatch(html, />64<\/td>|>7,014<\/td>/)
  assert.equal(restored.token_audit!.prompt[0].score, 1)
})
