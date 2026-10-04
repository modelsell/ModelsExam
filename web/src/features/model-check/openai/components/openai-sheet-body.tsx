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
import { useOpenAILabels } from '../hooks/use-openai-labels'
import { OPENAI_STAGES } from '../lib/catalog'
import type { OpenAISheetData } from '../lib/sheet'
import type { OpenAICheck, OpenAICheckReport } from '../types'

const num = (value: number | null | undefined) =>
  value == null ? '—' : value.toLocaleString()

export function OpenAISheetBody(props: {
  report: OpenAICheckReport
  data: OpenAISheetData
  running: boolean
  durationMS: number
}) {
  const { t } = useTranslation()
  const labels = useOpenAILabels()
  const { report, data } = props
  const done = new Map(report.checks.map((check) => [check.id, check]))
  const plan =
    report.plan && report.plan.length > 0
      ? report.plan.filter((item) => item.selected)
      : report.checks
  const pending = props.running ? t('Waiting to run') : t('Not collected')
  const result = (check?: OpenAICheck) => {
    if (!check) return { text: '—', good: false }
    if (check.kind === 'observation')
      return { text: t('Observation'), good: true }
    if (check.status === 'pass') return { text: '100/100', good: true }
    if (check.status === 'fail') return { text: '0/100', good: false }
    return { text: labels.states[check.status], good: false }
  }
  return (
    <>
      {data.findings.length > 0 && (
        <section>
          <div className='mc-section-title'>
            <h3>{t('Findings that need attention')}</h3>
            <span className='mc-muted'>{data.findings.length}</span>
          </div>
          {data.findings.map((check) => (
            <div className='mc-result' key={check.id}>
              <div>
                <p>{labels.title(check.id)}</p>
                <p className='mc-muted'>{labels.code(check.code)}</p>
              </div>
              <b className='mc-review'>{labels.states[check.status]}</b>
            </div>
          ))}
        </section>
      )}
      <section>
        <div className='mc-section-title'>
          <h3>{t('Observed model route')}</h3>
          <span className='mc-muted'>{t('Clues only')}</span>
        </div>
        <div className='mc-flow'>
          <div>
            <small>{t('Requested model')}</small>
            <code>{report.model}</code>
          </div>
          <div>
            <small>{t('Returned model')}</small>
            <code>{data.returnedModels.join(' / ') || '—'}</code>
          </div>
          <div>
            <small>system_fingerprint</small>
            <code>{data.fingerprints.join(' / ') || '—'}</code>
          </div>
        </div>
        <p className='mc-note'>
          {t(
            'Model names and fingerprints are reported by the endpoint and can be rewritten.'
          )}
        </p>
      </section>
      {report.samples.length > 0 && (
        <section>
          <div className='mc-section-title'>
            <h3>{t('Request ledger')}</h3>
            <span className='mc-muted'>{t('Reported usage')}</span>
          </div>
          <div className='mc-table-scroll'>
            <table className='mc-ledger'>
              <thead>
                <tr>
                  {[
                    t('Probe'),
                    'HTTP',
                    t('Latency'),
                    t('First event'),
                    t('Input'),
                    t('Output'),
                    t('Model'),
                    t('Notes'),
                  ].map((name) => (
                    <th key={name}>{name}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {report.samples.map((s, index) => (
                  <tr key={`${s.probe}-${index}`}>
                    <td>{s.probe}</td>
                    <td>{s.http_status || '—'}</td>
                    <td>{s.duration_ms} ms</td>
                    <td>{s.first_event_ms ? `${s.first_event_ms} ms` : '—'}</td>
                    <td>{num(s.usage?.input_tokens)}</td>
                    <td>{num(s.usage?.output_tokens)}</td>
                    <td>{s.response_model || '—'}</td>
                    <td>
                      {[
                        s.error_code,
                        s.finish_reason,
                        ...(s.validation_errors ?? []),
                        ...(s.usage_issues ?? []),
                      ]
                        .filter(Boolean)
                        .join('; ') || '—'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
      <div className='mc-results'>
        {OPENAI_STAGES.map((stage) => {
          const rows = plan.filter((item) => item.stage === stage.id)
          if (rows.length === 0) return null
          return (
            <section key={stage.id}>
              <h3>{labels.stage(stage.id)}</h3>
              {rows.map((item) => {
                const check = done.get(item.id)
                const shown = result(check)
                return (
                  <div className='mc-result' key={item.id}>
                    <div>
                      <p>{labels.title(item.id)}</p>
                      <p className='mc-muted'>
                        {check
                          ? labels.code(check.code) || labels.detail(item.id)
                          : pending}
                      </p>
                    </div>
                    <b className={shown.good ? 'mc-good' : 'mc-review'}>
                      {shown.text}
                    </b>
                  </div>
                )
              })}
            </section>
          )
        })}
      </div>
      <footer className='mc-foot'>
        {report.id} · {t('Duration')} {(props.durationMS / 1000).toFixed(1)} s ·{' '}
        {report.samples.length} {t('Requests')}
        <br />
        {t(
          'Results describe API compatibility only; this report does not verify model identity or billing.'
        )}
      </footer>
    </>
  )
}
