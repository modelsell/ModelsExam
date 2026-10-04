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
import { ReportRadar, ReportRing } from '../../components/report-sheet-charts'
import type { OpenAISheetData, SheetVerdict } from '../lib/sheet'
import type { OpenAICheckReport } from '../types'

const ms = (value: number | null, digits = 0) =>
  value === null ? '—' : value.toFixed(digits)

export function OpenAISheetHero(props: {
  report: OpenAICheckReport
  data: OpenAISheetData
}) {
  const { t } = useTranslation()
  const data = props.data
  const verdicts: Record<SheetVerdict, string> = {
    pending: t('Collecting evidence'),
    review: t('Some compatibility checks failed'),
    partial: t('Passed, with undecided checks'),
    clean: t('All decided checks passed'),
    unknown: t('No check could be decided'),
  }
  const points = (value: number | null) =>
    value === null ? '—' : `${Math.round(value)}/100`
  const scored = data.dimensions.filter((d) => d.score !== null).length
  return (
    <>
      <div className='mc-hero'>
        <div className='mc-score'>
          <ReportRing
            score={data.score}
            label={`${t('Compatibility score')}: ${points(data.score)}`}
          />
          <strong>{t('Compatibility score')}</strong>
          <p className='mc-muted'>
            {t('{{decided}} / {{total}} assertions decided', {
              decided: data.decided,
              total: data.assertions,
            })}
          </p>
        </div>
        {data.dimensions.length >= 3 ? (
          <ReportRadar
            dimensions={data.dimensions.map((d) => ({
              ...d,
              label: t(d.label),
            }))}
            title={t('Measured dimensions')}
            partialLabel={t(
              'Only scored vertices are connected. Dashed edges span unscored dimensions.'
            )}
          />
        ) : (
          <div />
        )}
        <div className='mc-findings'>
          <span
            className={`mc-chip ${data.verdict === 'review' ? 'mc-review' : ''}`}
          >
            {verdicts[data.verdict]}
          </span>
          <p className='mc-muted'>
            {t(
              'The check measures API compatibility. It cannot prove which model is behind the endpoint.'
            )}
          </p>
          <p>
            {t('Dimensions scored')}: {scored}/{data.dimensions.length}
          </p>
          <p className='mc-muted'>
            {t(
              'Share of passed assertions among decided ones; observations and undecided checks are excluded.'
            )}
          </p>
        </div>
      </div>
      <div className='mc-dimension-list'>
        {data.dimensions.map((d) => (
          <span className='mc-chip' key={d.id}>
            {t(d.label)} {points(d.score)}
          </span>
        ))}
      </div>
      <div className='mc-metrics'>
        <div className='mc-metric'>
          <span className='mc-muted'>{t('Median latency')}</span>
          <strong>
            {ms(
              data.latency.median === null ? null : data.latency.median / 1000,
              2
            )}{' '}
            s
          </strong>
          <span className='mc-muted'>
            {t('Maximum')}: {ms(data.latency.max)} ms · n={data.latency.count}
          </span>
        </div>
        <div className='mc-metric'>
          <span className='mc-muted'>{t('First streamed event')}</span>
          <strong>{ms(data.firstEvent.median)} ms</strong>
          <span className='mc-muted'>n={data.firstEvent.count}</span>
        </div>
        <div className='mc-metric'>
          <span className='mc-muted'>{t('Reported tokens')}</span>
          <strong>{data.totalTokens?.toLocaleString() ?? '—'}</strong>
          <span className='mc-muted'>
            {t('{{count}} requests', { count: props.report.samples.length })}
          </span>
        </div>
      </div>
    </>
  )
}
