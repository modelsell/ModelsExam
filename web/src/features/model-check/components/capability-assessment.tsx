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
import { useBaselineLabels } from '../hooks/use-baseline-labels'
import { useCapabilityLabels } from '../hooks/use-capability-labels'
import { capabilitySummary } from '../lib/capability-assessment'
import type { CapabilityBenchmark } from '../types'

export function CapabilityAssessment(props: {
  benchmark?: CapabilityBenchmark
  running: boolean
}) {
  const { t } = useTranslation()
  const names = useBaselineLabels()
  const labels = useCapabilityLabels()
  const summary = capabilitySummary(props.benchmark)
  if (!props.benchmark) return null
  const points = (score: number | null) =>
    score === null ? '—' : `${Math.round(score)}/100`
  return (
    <section aria-label={t('Capability assessment')}>
      <div className='mc-section-title'>
        <h3>
          {t('Capability assessment')} · v{props.benchmark.version}
        </h3>
        <strong>{points(summary.score)}</strong>
      </div>
      <p className='mc-muted'>
        {t(
          'Scored {{count}} / {{total}} questions · Coverage {{coverage}}%',
          summary
        )}
      </p>
      <p className='mc-note'>
        {t(
          'Objective checks score this small test set. Uncollected or invalid responses reduce coverage; they do not count as wrong answers. This is not an official benchmark or proof of model identity.'
        )}
      </p>
      <div className='mc-capability-groups'>
        {summary.groups.map((group) => (
          <div className='mc-metric' key={group.id}>
            <span className='mc-muted'>
              {labels.groups[group.id] || group.id}
            </span>
            <strong>{points(group.score)}</strong>
            <span className='mc-muted'>
              {group.count}/{group.total} · {group.coverage}%
            </span>
          </div>
        ))}
      </div>
      <details>
        <summary>{t('Question scores and grading evidence')}</summary>
        {summary.rows.map(({ id, item, score }) => (
          <details className='mc-capability-question' key={id}>
            <summary>
              <span>{names[id] || id}</span> <b>{points(score)}</b>
            </summary>
            <p className='mc-muted'>
              {t('Grading method')}:{' '}
              {labels.graders[item?.grader || 'exact-v1'] ||
                item?.grader ||
                '—'}
            </p>
            {score === null && (
              <p className='mc-muted'>
                {labels.codes[item?.code || ''] ||
                  (props.running ? t('Waiting to run') : t('Not collected'))}
              </p>
            )}
            <div className='mc-audit-grid'>
              <div>
                <p className='mc-muted'>{t('Reference answer')}</p>
                <pre>{item?.expected || '—'}</pre>
              </div>
              <div>
                <p className='mc-muted'>{t('Actual answer')}</p>
                <pre>{item?.actual || '—'}</pre>
              </div>
            </div>
            {!!item?.assertions?.length && (
              <div className='mc-dimension-list'>
                {item.assertions.map((assertion) => (
                  <span className='mc-chip' key={assertion.id}>
                    {assertion.id.startsWith('field_')
                      ? t('Field {{field}}', { field: assertion.id.slice(6) })
                      : labels.assertions[assertion.id] || assertion.id}
                    : {assertion.passed ? '100' : '0'}/100
                  </span>
                ))}
              </div>
            )}
          </details>
        ))}
      </details>
    </section>
  )
}
