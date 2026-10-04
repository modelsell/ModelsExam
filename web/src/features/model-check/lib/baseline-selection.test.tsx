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
import { BaselinePicker } from '../components/baseline-picker'
import { ReportSheet } from '../components/report-sheet'
import type { BaselineListItem, ClaudeCheckReport } from '../types'
import {
  baselineCandidates,
  baselineSelectionOptions,
  comparisonModelsMatch,
  selectedReportBaseline,
} from './baseline-selection'
import { modelCheckOptions, modelCheckSchema } from './form'

const items: BaselineListItem[] = [
  {
    id: 'older',
    type: 'anthropic',
    model: 'claude-opus-5',
    channel_name: 'Original channel',
    report_id: 'original-report',
    created_at: Date.UTC(2026, 8, 10),
    fingerprint: true,
  },
  {
    id: 'latest',
    type: 'anthropic',
    model: 'global.anthropic.claude-opus-5-v1:0',
    channel_name: 'Newer channel',
    report_id: 'newer-report',
    created_at: Date.UTC(2026, 8, 16),
    fingerprint: true,
  },
  {
    id: 'wrapped',
    type: 'ccmax',
    model: 'claude-opus-5',
    channel_name: 'Wrapped channel',
    report_id: 'wrapped-report',
    created_at: Date.UTC(2026, 8, 17),
    fingerprint: true,
  },
  {
    id: 'other-model',
    type: 'custom-type',
    model: 'claude-sonnet-5',
    channel_name: 'Other model',
    report_id: 'sonnet-report',
    created_at: Date.UTC(2026, 8, 18),
    fingerprint: true,
  },
]
const report = (): ClaudeCheckReport => ({
  version: 14,
  id: 'new-report',
  model: 'claude-opus-5',
  channel_id: 0,
  channel_name: 'Current channel',
  transport: 'anthropic_proxy',
  started_at: '2026-09-16T12:00:00Z',
  duration_ms: 100,
  cancelled: false,
  summary: { pass: 0, fail: 0, skipped: 0, inconclusive: 0 },
  samples: [],
  checks: [],
  plan: [],
})

test('baseline candidates match declared model aliases and select newest only within a chosen type', () => {
  assert.ok(
    comparisonModelsMatch(
      'claude-opus-5',
      'global.anthropic.claude-opus-5-v1:0'
    )
  )
  assert.ok(
    comparisonModelsMatch(
      'claude-opus-5',
      'arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-opus-5-v1:0'
    )
  )
  assert.ok(
    comparisonModelsMatch(
      'claude-sonnet-4-5',
      'anthropic.claude-sonnet-4-5-20250929-v1:0'
    )
  )
  assert.equal(comparisonModelsMatch('claude-opus-5', 'claude-sonnet-5'), false)
  assert.equal(
    comparisonModelsMatch('claude-opus-5', 'claude-opus-5-20260901'),
    false
  )
  assert.equal(
    comparisonModelsMatch(
      'anthropic.claude-opus-5-v1:0',
      'anthropic.claude-opus-5-v2:0'
    ),
    false
  )
  assert.equal(
    comparisonModelsMatch('claude-sonnet-4-5', 'claude-sonnet-4-5-20260230'),
    false
  )
  assert.ok(comparisonModelsMatch(' Custom Alias ', 'custom alias'))
  const candidates = baselineCandidates(items, 'claude-opus-5', 'anthropic')
  assert.deepEqual(
    candidates.map((item) => item.id),
    ['latest', 'older']
  )
  assert.deepEqual(baselineSelectionOptions(candidates[0]), {
    compare_baselines: true,
    baseline_id: 'latest',
    baseline_type: 'anthropic',
  })
  assert.equal(
    baselineCandidates(items, 'claude-sonnet-5').some(
      (item) => item.id === 'latest'
    ),
    false
  )
  assert.deepEqual(baselineSelectionOptions(), {
    compare_baselines: false,
    baseline_id: '',
    baseline_type: '',
  })
})

test('form options carry the same selected snapshot for channel and custom targets and never implicitly compare all', () => {
  const base = {
    mode: 'channel',
    model: 'claude-opus-5',
    key: '',
    base_url: '',
    cache: true,
    thinking: false,
    repeat: false,
    vision: false,
    pdf: false,
    stream_comparison: false,
    baseline_id: 'older',
    baseline_type: 'anthropic',
  }
  const values = modelCheckSchema.parse(base)
  const options = modelCheckOptions(values, items[0])
  const targets = [
    { ...options, channel_id: 12 },
    { ...options, base_url: 'https://example.test', key: 'test-only' },
  ]
  for (const target of targets) {
    const restored = JSON.parse(JSON.stringify(target))
    assert.equal(restored.baseline_id, 'older')
    assert.equal(restored.baseline_type, 'anthropic')
    assert.equal(restored.compare_baselines, true)
  }
  assert.equal(modelCheckOptions(values).compare_baselines, false)
  assert.equal(
    modelCheckSchema.safeParse({ ...base, baseline_id: '' }).success,
    false
  )
  assert.equal(
    modelCheckSchema.safeParse({ ...base, baseline_type: '' }).success,
    false
  )
  assert.equal(
    modelCheckSchema.safeParse({ ...base, baseline_id: '', baseline_type: '' })
      .success,
    true
  )
})

test('picker uses saved types and millisecond timestamps while locking both controls during a run', async () => {
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: {}, initImmediate: false })
  const html = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <BaselinePicker
        model='claude-opus-5'
        items={items}
        selectedType='anthropic'
        selectedId='older'
        loading={false}
        loadError={false}
        busy
        onChange={() => undefined}
      />
    </I18nextProvider>
  )
  assert.match(html, /anthropic/)
  assert.match(html, /ccmax/)
  assert.doesNotMatch(html, /custom-type/)
  assert.match(html, /Original channel/)
  assert.match(html, /2026/)
  assert.match(
    html,
    new RegExp(
      new Date(items[0].created_at)
        .toLocaleString()
        .replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    )
  )
  assert.equal((html.match(/<select[^>]*disabled=""/g) || []).length, 2)
})

test('reports retain the explicitly selected snapshot without standalone baseline panels or fallback selection', async () => {
  const r = report()
  r.options = {
    model: r.model,
    thinking: false,
    cache: false,
    ...baselineSelectionOptions(items[0]),
  }
  r.baselines = [items[0], items[1]].map((item) => ({
    ...report(),
    ...item,
    fingerprint: undefined,
  }))
  assert.equal(selectedReportBaseline(r)?.id, 'older')
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: {}, initImmediate: false })
  const html = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <ReportSheet report={r} running={false} phaseLabel='Complete' />
    </I18nextProvider>
  )
  assert.match(html, /Baseline: anthropic · claude-opus-5 · Original channel/)
  assert.match(html, /2026/)
  assert.doesNotMatch(html, /<h3>Baseline differences|Newer channel/)
  const original = JSON.stringify(r.baselines)
  r.options.baseline_type = 'ccmax'
  assert.equal(selectedReportBaseline(r), undefined)
  r.options.baseline_type = 'anthropic'
  r.model = 'claude-sonnet-5'
  assert.equal(selectedReportBaseline(r), undefined)
  delete r.options.baseline_id
  r.baselines = [r.baselines[0]]
  assert.equal(selectedReportBaseline(r), undefined)
  assert.equal(
    JSON.stringify(JSON.parse(original)[0]),
    JSON.stringify(r.baselines[0])
  )
})
