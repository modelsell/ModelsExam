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
import { useUsageTokenLabels } from '../hooks/use-usage-token-labels'
import {
  assessUsageTokens,
  usageTokenBaseline,
} from '../lib/usage-token-integrity'
import type { ClaudeCheckReport } from '../types'

export function UsageTokenIntegrity(props: {
  report: ClaudeCheckReport
  running: boolean
  activeProbe?: string | null
}) {
  const { t } = useTranslation()
  const labels = useUsageTokenLabels()
  const data = assessUsageTokens(props.report)
  if (!data.active) return null
  const baseline = usageTokenBaseline(props.report)
  const number = (value: number | null, digits = 0) =>
    value == null
      ? '—'
      : value.toLocaleString(undefined, { maximumFractionDigits: digits })
  const points = (score: number | null) =>
    score == null ? '—' : `${score}/100`
  const signed = (value: number | null, digits = 0) => {
    if (value == null) return '—'
    return `${value > 0 ? '+' : ''}${number(value, digits)}`
  }
  return (
    <section aria-label={t('Usage token integrity')}>
      <div className='mc-section-title'>
        <h3>{t('Usage token integrity')}</h3>
        <strong className={data.score === 100 ? 'mc-good' : 'mc-review'}>
          {points(data.score)}
        </strong>
      </div>
      <p className='mc-muted'>
        {t('Measured {{measured}} / {{total}} usage samples', data)} ·{' '}
        {labels.codes[data.code]}
      </p>
      <div className='mc-capability-groups'>
        <div className='mc-audit-box'>
          <h3>{t('Token growth score')}</h3>
          <strong>{points(data.growth_score)}</strong>
        </div>
        <div className='mc-audit-box'>
          <h3>{t('Baseline consistency score')}</h3>
          <strong>{points(data.baseline_score)}</strong>
          <p className='mc-muted'>{labels.codes[data.baseline_code]}</p>
          {data.baseline_code === 'baseline_incomplete' && (
            <p className='mc-muted'>
              {t(
                'Collect a new baseline run to include these four usage checks.'
              )}
            </p>
          )}
        </div>
        <div className='mc-audit-box'>
          <h3>{t('English long/short input ratio')}</h3>
          <strong>
            {data.long_short_ratio == null
              ? '—'
              : `${number(data.long_short_ratio, 2)}×`}
          </strong>
          <p className='mc-muted'>
            {t('Growth slope agreement: {{value}}', {
              value:
                data.slope_ratio == null
                  ? '—'
                  : `${number(data.slope_ratio * 100, 1)}%`,
            })}
          </p>
        </div>
      </div>
      <p className='mc-muted'>
        {baseline &&
          t('Baseline: {{type}} · {{model}} · {{channel}}', {
            type: baseline.type,
            model: baseline.model,
            channel: baseline.channel_name || '—',
          })}
      </p>
      <div className='mc-table-scroll'>
        <table className='mc-ledger'>
          <thead>
            <tr>
              {[
                t('Request'),
                t('Input'),
                t('Cache write'),
                t('Cache read'),
                t('Total input tokens'),
                t('Baseline total input'),
                t('Baseline delta'),
                t('Baseline delta (%)'),
              ].map((name) => (
                <th key={name}>{name}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {data.rows.map((row) => {
              let status = labels.codes[row.code]
              if (row.code === 'not_collected' && props.running)
                status = t('Waiting to run')
              if (props.running && props.activeProbe === row.probe)
                status = t('Waiting for the upstream response…')
              return (
                <tr key={row.probe}>
                  <td>
                    {labels.names[row.probe]}
                    {row.code !== 'observed' && (
                      <div className='mc-muted'>{status}</div>
                    )}
                  </td>
                  <td>{number(row.input)}</td>
                  <td>{number(row.cache_creation)}</td>
                  <td>{number(row.cache_read)}</td>
                  <td>{number(row.total_input)}</td>
                  <td>
                    {number(row.reference_input, 1)}
                    {row.reference_code !== 'matched' && (
                      <div className='mc-muted'>
                        {labels.codes[row.reference_code]}
                      </div>
                    )}
                  </td>
                  <td>{signed(row.diff, 1)}</td>
                  <td>
                    {row.diff_percent == null
                      ? '—'
                      : `${signed(row.diff_percent, 1)}%`}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      <p className='mc-note'>
        {t('Total input = uncached input + cache write + cache read.')}{' '}
        {t(
          'Omitted optional cache counts remain unreported and contribute zero to the reported total.'
        )}{' '}
        {t(
          'Token growth checks consistency, not actual billing or model identity. Without four matching baseline rows, the score reflects growth only.'
        )}
      </p>
      <details>
        <summary>{t('Scoring method and evidence')}</summary>
        <p className='mc-muted'>
          {t(
            'English totals must increase. Compare token growth per added word for 30→300 and 300→1500 words; slope agreement of 80% earns full growth points. Chinese input is checked separately without a character-to-token conversion.'
          )}
        </p>
        <p className='mc-muted'>
          {t(
            'Baseline totals use matching request profiles. Differences within 4 tokens or 5% earn full points; larger differences use the smaller-to-larger total ratio. The final score is the lower of growth and complete baseline scores.'
          )}
        </p>
        <pre>
          {JSON.stringify(
            {
              assessment: data,
              requests: props.report.samples.filter((sample) =>
                data.rows.some((row) => row.probe === sample.probe)
              ),
            },
            null,
            2
          )}
        </pre>
      </details>
    </section>
  )
}
