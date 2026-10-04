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
import { CheckHistory } from '../components/check-history'
import { CheckReport } from '../components/check-report'
import { ReportSheet } from '../components/report-sheet'
import type { CheckHistoryDetail, CheckHistoryPage } from '../types'
import { reportHTML } from './report-export'
import {
  reportRemark,
  updateHistoryRemark,
  withSavedRemark,
} from './report-remark'

function detail(remark?: string | null): CheckHistoryDetail {
  return {
    run: {
      id: 'saved-report',
      model: 'claude-opus-5',
      channel_id: 7,
      channel_name: 'Original channel name',
      remark,
      score: 92,
      endpoint: 'https://example.com',
      transport: 'anthropic_proxy',
      status: 'completed',
      started_at: Date.UTC(2026, 8, 16),
      updated_at: Date.UTC(2026, 8, 16),
      duration_ms: 3000,
      request_count: 20,
      pass_count: 19,
      fail_count: 1,
      active_probe: '',
    },
    report: {
      version: 14,
      id: 'saved-report',
      model: 'claude-opus-5',
      channel_id: 7,
      channel_name: 'Original channel name',
      remark,
      transport: 'anthropic_proxy',
      started_at: '2026-09-16T00:00:00Z',
      duration_ms: 3000,
      cancelled: false,
      checks: [],
      samples: [],
      plan: [],
      summary: { pass: 0, fail: 0, skipped: 0, inconclusive: 0 },
    },
  }
}

async function render(children: React.ReactNode, client = new QueryClient()) {
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: {}, initImmediate: false })
  return renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>{children}</I18nextProvider>
    </QueryClientProvider>
  )
}

test('clearing a remark survives a saved-report round trip without restoring the old channel name', () => {
  const original = detail()
  assert.equal(reportRemark(original.report), 'Original channel name')
  const saved = JSON.parse(JSON.stringify(detail(''))) as CheckHistoryDetail
  const report = withSavedRemark(original.report, saved)!
  assert.equal(reportRemark(report), '')
  assert.equal(report.channel_name, 'Original channel name')
  assert.equal(report.checks, original.report.checks)
  assert.equal(
    withSavedRemark({ ...original.report, id: 'next-run' }, saved)?.remark,
    undefined
  )
  const page: CheckHistoryPage = {
    items: [original.run, { ...original.run, id: 'other' }],
    page: 1,
    page_size: 20,
    total: 2,
  }
  // An older detail response may not carry a hydrated score. Editing metadata
  // must not erase the score already displayed in the list.
  saved.run.score = null
  const updated = updateHistoryRemark(page, saved)!
  assert.equal(updated.items[0].remark, '')
  assert.equal(updated.items[0].score, 92)
  assert.equal(updated.items[1], page.items[1])
})

test('edited remarks appear in exported HTML as escaped text and can be cleared', async () => {
  const markup = await render(
    <ReportSheet
      report={detail('<img src=x onerror=alert(1)>').report}
      running={false}
      phaseLabel='Completed'
    />
  )
  const exported = reportHTML(markup, 'Report', 'en')
  assert.doesNotMatch(exported, /Remark:/)
  assert.match(exported, /&lt;img src=x onerror=alert\(1\)&gt;/)
  assert.doesNotMatch(exported, /<img src=x/)
  assert.doesNotMatch(exported, /Original channel name/)
  const cleared = await render(
    <ReportSheet
      report={detail('').report}
      running={false}
      phaseLabel='Completed'
    />
  )
  assert.doesNotMatch(cleared, /Original channel name/)
})

test('history displays saved report scores including zero without fabricating unscored results', async () => {
  const client = new QueryClient()
  const item = detail('Saved note').run
  client.setQueryData(
    [
      'model-check-history',
      undefined,
      'list',
      { model: '', status: '', page: 1 },
    ],
    {
      items: [
        item,
        { ...item, id: 'zero', score: 0 },
        { ...item, id: 'unscored', score: null },
      ],
      total: 3,
      page: 1,
      page_size: 20,
    }
  )
  const markup = await render(
    <CheckHistory />,
    client
  )
  assert.match(markup, /Report score/)
  assert.match(markup, /Saved note/)
  assert.match(markup, />92\/100</)
  assert.match(markup, />0\/100</)
  assert.equal((markup.match(/\/100</g) || []).length, 2)
})

test('live and historical report views use the same saved remark without replacing diagnostic evidence', async () => {
  const original = detail()
  const client = new QueryClient()
  client.setQueryData(
    ['model-check-history', undefined, 'detail', original.report.id],
    detail('Edited from history')
  )
  const markup = await render(
    <CheckReport
      state={{
        phase: 'complete',
        report: original.report,
        error: null,
        activeProbe: null,
      }}
      elapsed={3000}
    />,
    client
  )
  assert.match(markup, /Edited from history/)
  assert.match(markup, />Edit</)
  assert.doesNotMatch(markup, /Remark:|Edit remark/)
  assert.doesNotMatch(markup, /Original channel name/)
})
