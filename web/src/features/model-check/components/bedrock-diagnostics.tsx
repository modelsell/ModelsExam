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
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from '@/components/ui/card'
import { useBedrockLabels } from '../hooks/use-bedrock-labels'
import { useClaudeCheckLabels } from '../labels'
import { getCheckPlan, samplesForCheck } from '../lib/check-plan'
import type { ClaudeCheckReport } from '../types'

export function BedrockDiagnostics(props: {
  report: ClaudeCheckReport
  running: boolean
  activeProbe: string | null
  paper?: boolean
}) {
  const { t } = useTranslation()
  const diagnosticLabels = useBedrockLabels()
  const labels = useClaudeCheckLabels()
  const plan = getCheckPlan(props.report).filter(
    (item) => item.selected && item.stage === 'bedrock'
  )
  const observations = props.report.samples.filter(
    (sample) => sample.diagnostic
  )
  const ids = [
    ...new Set([
      ...plan.map((item) => item.id),
      ...props.report.checks
        .filter((check) => check.id.startsWith('bedrock_'))
        .map((check) => check.id),
      ...props.report.samples
        .filter((sample) => sample.probe.startsWith('bedrock_'))
        .map((sample) => sample.probe),
    ]),
  ]
  if (!props.report.options?.bedrock && !ids.length && !observations.length)
    return null
  // A v17 stored option alone is not evidence that its retired probes ran.
  if (props.report.version === 17 && !ids.length && !observations.length)
    return null
  const note = t(
    'Restrictions and error clues are separate from model identity and performance scores. Gateway responses can be rewritten.'
  )
  const muted = props.paper ? 'mc-muted' : 'text-muted-foreground text-xs'
  const evidenceStyle = props.paper
    ? undefined
    : 'bg-muted/50 mt-2 max-h-64 overflow-auto rounded-md border p-3 text-xs whitespace-pre-wrap break-all'
  const content = (
    <>
      {ids.map((id) => {
        const check = props.report.checks.find((item) => item.id === id)
        const samples = samplesForCheck(props.report, id)
        const active = props.running && props.activeProbe === id
        let conclusion = props.running
          ? t('Waiting to run')
          : t('Not collected')
        let conclusionStyle = muted
        if (active) {
          conclusion = t('Waiting for the upstream response…')
          conclusionStyle = props.paper ? 'mc-provisional' : 'text-info text-xs'
        }
        if (check) conclusion = labels.codes[check.code] || check.code
        return (
          <div
            key={id}
            className={props.paper ? 'mc-result' : 'border-b py-3 text-sm'}
          >
            <div>
              <p>{labels.names[id] || id}</p>
              <p className={conclusionStyle}>{conclusion}</p>
              <p className={muted}>
                {samples
                  .map((sample) => `HTTP ${sample.http_status || '—'}`)
                  .join(' · ')}
              </p>
              {(check || samples.length > 0) && (
                <details>
                  <summary
                    className={
                      props.paper ? undefined : 'cursor-pointer text-xs'
                    }
                  >
                    {t('View evidence')}
                  </summary>
                  <pre className={evidenceStyle}>
                    {JSON.stringify({ check, requests: samples }, null, 2)}
                  </pre>
                </details>
              )}
            </div>
          </div>
        )
      })}
      {observations.length > 0 && (
        <details>
          <summary
            className={props.paper ? undefined : 'cursor-pointer text-sm'}
          >
            {t('Observed error diagnostics')} · {observations.length}
          </summary>
          {observations.map((sample, index) => (
            <div
              key={`${sample.probe}-${index}`}
              className={
                props.paper ? 'mc-note' : 'mt-3 rounded-md border p-3 text-xs'
              }
            >
              <p>
                {labels.names[sample.probe] || sample.probe} · HTTP{' '}
                {sample.http_status || '—'} ·{' '}
                {sample.diagnostic?.exception || t('Unclassified error')}
              </p>
              <p className={muted}>
                {diagnosticLabels[sample.diagnostic?.code || 'unknown'] ||
                  diagnosticLabels.unknown}
              </p>
              <pre className={evidenceStyle}>
                {JSON.stringify(sample, null, 2)}
              </pre>
            </div>
          ))}
        </details>
      )}
    </>
  )
  if (props.paper)
    return (
      <section aria-label={t('Bedrock compatibility diagnostics')}>
        <div className='mc-section-title'>
          <h3>{t('Bedrock compatibility diagnostics')}</h3>
          <span className='mc-muted'>{t('Observation')}</span>
        </div>
        <p className='mc-note'>{note}</p>
        {content}
      </section>
    )
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Bedrock compatibility diagnostics')}</CardTitle>
        <CardDescription>{note}</CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-3'>{content}</CardContent>
    </Card>
  )
}
