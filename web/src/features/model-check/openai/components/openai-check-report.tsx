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
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { useOpenAILabels } from '../hooks/use-openai-labels'
import { checkIdForProbe } from '../lib/catalog'
import type { OpenAIRunPhase, OpenAIRunState } from '../types'
import { OpenAIReportActions } from './openai-report-actions'
import { OpenAIReportSheet } from './openai-report-sheet'
import { OpenAIReportStages } from './openai-report-stages'

export function OpenAICheckReportView(props: {
  state: OpenAIRunState
  elapsed: number
}) {
  const { t } = useTranslation()
  const labels = useOpenAILabels()
  const sheetRef = useRef<HTMLDivElement>(null)
  const { state } = props
  const report = state.report
  const running = state.phase === 'connecting' || state.phase === 'running'
  const duration = running ? props.elapsed : (report?.duration_ms ?? 0)
  const requests = report?.samples.length ?? 0
  const limit = report?.limits?.max_requests ?? 0
  const phases: Record<OpenAIRunPhase, string> = {
    idle: t('Ready to check'),
    connecting: t('Connecting'),
    running: t('Check in progress'),
    complete: t('Report complete'),
    cancelled: t('Check stopped'),
    error: t('Check interrupted'),
  }
  const variants: Record<OpenAIRunPhase, StatusVariant> = {
    idle: 'neutral',
    connecting: 'info',
    running: 'info',
    complete: 'success',
    cancelled: 'warning',
    error: 'warning',
  }
  const stopReason = report?.stop_reason
  return (
    <div className='flex min-w-0 flex-col gap-4'>
      <Card>
        <CardHeader>
          <CardDescription>{t('Model evidence report')}</CardDescription>
          <CardTitle>{report?.model || t('Model check report')}</CardTitle>
          <CardAction>
            <StatusBadge
              variant={variants[state.phase]}
              label={phases[state.phase]}
              pulse={running}
              copyable={false}
            />
          </CardAction>
        </CardHeader>
        <CardContent className='flex flex-col gap-4'>
          <div className='flex flex-col gap-2'>
            <div className='text-muted-foreground flex justify-between gap-3 text-xs tabular-nums'>
              <span>
                {limit
                  ? t('{{count}} / {{limit}} requests', {
                      count: requests,
                      limit,
                    })
                  : t('{{count}} requests', { count: requests })}
              </span>
              <span>{(duration / 1000).toFixed(1)} s</span>
            </div>
            <Progress
              value={
                state.phase === 'complete' && !stopReason
                  ? 100
                  : Math.min(99, (100 * requests) / Math.max(1, limit))
              }
              aria-label={t('Check progress')}
            />
            {running && state.activeProbe && (
              <p role='status' className='text-info text-xs'>
                {labels.title(checkIdForProbe(state.activeProbe))} ·{' '}
                {state.activeProbe}
              </p>
            )}
          </div>
          {running && (
            <p className='text-muted-foreground text-xs'>
              {t('Live results are provisional until collection finishes.')}
            </p>
          )}
          {report?.history_saved === false && (
            <Alert>
              <AlertDescription>
                {t(
                  'History could not be saved. Export this report before leaving.'
                )}
              </AlertDescription>
            </Alert>
          )}
          {stopReason && !running && (
            <Alert>
              <AlertDescription>
                {t('Stopped before all checks ran')} · {labels.code(stopReason)}
              </AlertDescription>
            </Alert>
          )}
          {state.error && (
            <Alert variant='destructive'>
              <AlertDescription className='whitespace-pre-wrap'>
                {state.error}
              </AlertDescription>
            </Alert>
          )}
        </CardContent>
        <CardFooter className='flex flex-wrap justify-between gap-3'>
          <p className='text-muted-foreground max-w-xl text-xs'>
            {t(
              'Latency is measured by this server against the endpoint. Network and load affect the result.'
            )}
          </p>
          <OpenAIReportActions
            report={report}
            markdown={state.markdown}
            sheet={sheetRef}
            running={running}
          />
        </CardFooter>
      </Card>
      {report && (
        <>
          <div ref={sheetRef}>
            <OpenAIReportSheet
              report={report}
              running={running}
              phaseLabel={phases[state.phase]}
              durationMS={duration}
            />
          </div>
          <details className='rounded-xl border p-4'>
            <summary className='cursor-pointer text-sm font-medium'>
              {t('All request evidence')} · {requests}
            </summary>
            <div className='mt-4'>
              <OpenAIReportStages
                report={report}
                activeProbe={state.activeProbe}
                live={running}
              />
            </div>
          </details>
          <p className='text-muted-foreground px-1 text-xs break-all'>
            {new Date(report.started_at).toLocaleString()} · {report.id} · v
            {report.version}
          </p>
        </>
      )}
    </div>
  )
}
