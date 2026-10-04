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
import { useImageLabels } from '../hooks/use-image-labels'
import { IMAGE_STAGES } from '../lib/catalog'
import type { ImageRunPhase, ImageRunState } from '../types'
import { ImageCheckRow } from './image-check-row'
import { ImageGallery } from './image-gallery'
import { ImageProvenanceCard } from './image-provenance-card'
import { ImageReportActions } from './image-report-actions'

export function ImageCheckReportView(props: {
  state: ImageRunState
  elapsed: number
}) {
  const { t } = useTranslation()
  const labels = useImageLabels()
  const sheetRef = useRef<HTMLDivElement>(null)
  const { state } = props
  const report = state.report
  const running = state.phase === 'connecting' || state.phase === 'running'
  const duration = running ? props.elapsed : (report?.duration_ms ?? 0)
  const requests = report?.samples.length ?? 0
  const limit = report?.limits?.max_requests ?? 0
  const phases: Record<ImageRunPhase, string> = {
    idle: t('Ready to check'),
    connecting: t('Connecting'),
    running: t('Check in progress'),
    complete: t('Report complete'),
    cancelled: t('Check stopped'),
    error: t('Check interrupted'),
  }
  const variants: Record<ImageRunPhase, StatusVariant> = {
    idle: 'neutral',
    connecting: 'info',
    running: 'info',
    complete: 'success',
    cancelled: 'warning',
    error: 'warning',
  }
  const stopReason = report?.stop_reason
  const plan = report?.plan ?? []
  const byId = new Map((report?.checks ?? []).map((c) => [c.id, c]))
  // The first generation feeds several checks, so its sample is shared.
  const samplesFor = (id: string) =>
    (report?.samples ?? []).filter(
      (s) =>
        s.probe === id ||
        (s.probe === 'img_generate_basic' &&
          ['img_response_shape', 'img_size_exact'].includes(id))
    )
  return (
    <div className='flex min-w-0 flex-col gap-4'>
      <Card>
        <CardHeader>
          <CardDescription>{t('Image model evidence report')}</CardDescription>
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
                {labels.title(state.activeProbe)}
              </p>
            )}
          </div>
          {report && report.score != null && (
            <p className='text-sm'>
              {t('Compatibility score')}:{' '}
              <span className='font-semibold tabular-nums'>{report.score}</span>
              <span className='text-muted-foreground text-xs'>
                {' '}
                · {t('Only assertions are scored. Provenance is reported separately.')}
              </span>
            </p>
          )}
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
          <ImageReportActions
            report={report}
            markdown={state.markdown}
            sheet={sheetRef}
            running={running}
          />
        </CardFooter>
      </Card>
      {report && (
        <div ref={sheetRef} className='flex min-w-0 flex-col gap-4'>
          <ImageProvenanceCard provenance={report.provenance} />
          <ImageGallery images={report.images} />
          {IMAGE_STAGES.map((stage) => {
            const items = plan.filter((item) => item.stage === stage.id)
            if (items.length === 0) return null
            return (
              <section
                key={stage.id}
                className='overflow-hidden rounded-xl border'
              >
                <h3 className='bg-muted/40 px-4 py-2 text-sm font-medium'>
                  {labels.stage(stage.id)}
                </h3>
                <div className='divide-y'>
                  {items.map((item) => (
                    <ImageCheckRow
                      key={item.id}
                      id={item.id}
                      kind={item.kind}
                      selected={item.selected}
                      check={byId.get(item.id)}
                      running={
                        running &&
                        !byId.has(item.id) &&
                        state.activeProbe === item.id
                      }
                      samples={samplesFor(item.id)}
                    />
                  ))}
                </div>
              </section>
            )
          })}
          <p className='text-muted-foreground px-1 text-xs break-all'>
            {new Date(report.started_at).toLocaleString()} · {report.id} · v
            {report.version}
          </p>
        </div>
      )}
    </div>
  )
}
