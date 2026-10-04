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
import { useTranslation } from 'react-i18next'
import { useClaudeCheckLabels } from '../labels'
import {
  baselineLedgerComparison,
  selectedLedgerBaseline,
} from '../lib/baseline-ledger'
import { isCountProbe } from '../lib/check-plan'
import {
  awsCounterComparison,
  reportHeaders,
  totalInput,
} from '../lib/report-sheet'
import {
  isUsageTokenProbe,
  usageTokenCounters,
} from '../lib/usage-token-integrity'
import type { ClaudeCheckReport } from '../types'

export function ReportSheetLedger(props: { report: ClaudeCheckReport }) {
  const { t } = useTranslation()
  const labels = useClaudeCheckLabels()
  const number = (value: number | null | undefined) =>
    value == null ? '—' : value.toLocaleString()
  const samples = props.report.samples.filter((s) => !isCountProbe(s.probe))
  const baseline = selectedLedgerBaseline(props.report)
  const comparisons = samples.map((sample) =>
    baselineLedgerComparison(props.report, sample)
  )
  const reasons = {
    no_baseline: t('No baseline selected for this run.'),
    snapshot_unavailable: t('Selected baseline snapshot unavailable'),
    no_sample: t('No matching baseline sample'),
    parameters_differ: t('Baseline request parameters differ'),
    reference_usage_unavailable: t('Baseline usage unavailable'),
    current_usage_unavailable: t('Current request usage unavailable'),
  }
  return (
    <section>
      <div className='mc-section-title'>
        <h3>{t('Token ledger')}</h3>
        <span className='mc-muted'>{t('Reported usage')}</span>
      </div>
      <p className='mc-muted'>
        {t('Total input = uncached input + cache write + cache read.')}
      </p>
      <p className='mc-muted'>
        {baseline
          ? t('Baseline: {{type}} · {{model}} · {{channel}}', {
              type: baseline.type,
              model: baseline.model,
              channel: baseline.channel_name || '—',
            })
          : reasons[
              props.report.options?.baseline_id
                ? 'snapshot_unavailable'
                : 'no_baseline'
            ]}
        {' · '}
        {t('Compared {{matched}} / {{total}} rows', {
          matched: comparisons.filter((item) => item.code === 'matched').length,
          total: samples.length,
        })}
      </p>
      <div className='mc-table-scroll'>
        <table className='mc-ledger'>
          <thead>
            <tr>
              {[
                t('Request'),
                'HTTP',
                t('Input'),
                t('Output'),
                t('Cache write'),
                t('Cache read'),
                t('Total input tokens'),
                t('Baseline total input'),
                t('Baseline delta'),
                ...(props.report.version < 17
                  ? [t('AWS header reconciliation')]
                  : []),
              ].map((name) => (
                <th key={name}>{name}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {samples.map((s, index) => {
              const comparison = comparisons[index]
              const referenceNote =
                comparison.code === 'matched'
                  ? t('Matched {{count}} baseline samples', {
                      count: comparison.samples,
                    })
                  : reasons[comparison.code]
              const aws = awsCounterComparison(s)
              const usage = isUsageTokenProbe(s.probe)
                ? usageTokenCounters(s)
                : undefined
              return (
                <tr key={`${s.probe}-${index}`}>
                  <td>{labels.names[s.probe] || s.probe}</td>
                  <td>{s.http_status || '—'}</td>
                  <td>{number(s.usage.input_tokens)}</td>
                  <td>{number(s.usage.output_tokens)}</td>
                  <td>
                    {number(
                      usage
                        ? usage.cache_creation
                        : s.usage.cache_creation_input_tokens
                    )}
                  </td>
                  <td>
                    {number(
                      usage ? usage.cache_read : s.usage.cache_read_input_tokens
                    )}
                  </td>
                  <td>{number(totalInput(s))}</td>
                  <td title={referenceNote}>
                    {number(comparison.reference)}
                    {comparison.reference == null && baseline && (
                      <div className='mc-muted'>{referenceNote}</div>
                    )}
                  </td>
                  <td className={comparison.difference ? 'mc-review' : ''}>
                    {comparison.difference != null && comparison.difference > 0
                      ? '+'
                      : ''}
                    {number(comparison.difference)}
                  </td>
                  {props.report.version < 17 && (
                    <td>
                      {aws.compared
                        ? `${aws.compared - aws.differences}/${aws.compared}`
                        : '—'}
                    </td>
                  )}
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      <p className='mc-muted'>
        {t(
          'Reference tokens use the selected baseline median for the same question and request parameters. Missing matches remain unscored.'
        )}
      </p>
      {!!props.report.token_audit?.prompt.length && (
        <p className='mc-muted'>
          {t(
            'This baseline comparison does not change the prompt injection score.'
          )}
        </p>
      )}
      <p className='mc-muted'>
        {t(
          'Count-only requests are references, not inference usage. Costs require an independent billing record.'
        )}
      </p>
      <details>
        <summary>
          {t('Per-request response headers')} · {props.report.samples.length}
        </summary>
        {props.report.samples.map((s, index) => (
          <div key={`${s.probe}-${index}`}>
            <p className='mc-muted'>
              {index + 1}. {labels.names[s.probe] || s.probe} · HTTP{' '}
              {s.http_status || '—'}
            </p>
            <pre>
              {JSON.stringify(Object.fromEntries(reportHeaders(s)), null, 2)}
            </pre>
          </div>
        ))}
      </details>
    </section>
  )
}
