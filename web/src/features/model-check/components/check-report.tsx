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
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
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
import { downloadCheckReport, getCheckHistoryReport } from '../api'
import { useClaudeCheckLabels } from '../labels'
import { DEFAULT_CHECK_OPTIONS } from '../lib/check-defaults'
import {
  getCheckPlan,
  probeCheckID,
  requestBudget,
  samplesForCheck,
  SCORED_CHECK_IDS,
} from '../lib/check-plan'
import { reportStopReason } from '../lib/execution-progress'
import {
  captureReportHTML,
  downloadReportHTML,
  printReportHTML,
} from '../lib/report-export'
import { withSavedRemark } from '../lib/report-remark'
import { assessUsageTokens } from '../lib/usage-token-integrity'
import type { ClaudeCheckOptions, RunPhase, RunState } from '../types'
import { BedrockDiagnostics } from './bedrock-diagnostics'
import { CheckRow } from './check-row'
import { ReportRemarkEditor } from './report-remark-editor'
import { ReportSheet } from './report-sheet'
import { RequestLedger } from './request-ledger'
import { SaveBaseline } from './save-baseline'
import { TokenAudit } from './token-audit'

export function CheckReport({
  state,
  elapsed,
  preview,
}: {
  state: RunState
  elapsed: number
  preview?: ClaudeCheckOptions
}) {
  const { t, i18n } = useTranslation()
  const sheetRef = useRef<HTMLDivElement>(null)
  const labels = useClaudeCheckLabels()
  const userId = useAuthStore((store) => store.auth.user?.id)
  // Subscribe to metadata edits shared by the live report and history view.
  // This observer does not issue additional requests for diagnostic snapshots.
  const saved = useQuery({
    queryKey: ['model-check-history', userId, 'detail', state.report?.id],
    queryFn: ({ signal }) => getCheckHistoryReport(state.report!.id, signal),
    enabled: false,
  })
  const report = withSavedRemark(state.report, saved.data)
  const usageTokens = report ? assessUsageTokens(report) : undefined
  const running = state.phase === 'running' || state.phase === 'connecting'
  const plan = getCheckPlan(report, preview)
  const stopReason = reportStopReason(report)
  const duration = running ? elapsed : (report?.duration_ms ?? 0)
  const requests = report?.samples.length ?? 0
  let limit = requestBudget(preview ?? DEFAULT_CHECK_OPTIONS)
  if (report?.options) limit = requestBudget(report.options, report.version)
  if (report?.limits) limit = report.limits.max_requests
  const phases: Record<RunPhase, string> = {
    idle: t('Ready to check'),
    connecting: t('Connecting'),
    running: t('Check in progress'),
    complete: t('Report complete'),
    cancelled: t('Check stopped'),
    error: t('Check interrupted'),
  }
  const variants: Record<RunPhase, StatusVariant> = {
    idle: 'neutral',
    connecting: 'info',
    running: 'info',
    complete: 'success',
    cancelled: 'warning',
    error: 'warning',
  }
  const exportDocument = (print: boolean) => {
    if (!report || !sheetRef.current) return
    try {
      const html = captureReportHTML(
        sheetRef.current,
        `${t('Model check report')} · ${report.model}`,
        i18n.resolvedLanguage || 'en'
      )
      if (print) printReportHTML(html)
      else downloadReportHTML(html, report.id)
    } catch {
      toast.error(t('Could not export the report document'))
    }
  }
  return (
    <div className='flex min-w-0 flex-col gap-4'>
      <Card>
        <CardHeader>
          <CardDescription>{t('Model evidence report')}</CardDescription>
          <CardTitle>{report?.model || t('Model check report')}</CardTitle>
          <CardAction>
            <StatusBadge
              variant={
                stopReason && !running ? 'warning' : variants[state.phase]
              }
              label={
                stopReason && !running
                  ? t('Check stopped')
                  : phases[state.phase]
              }
              pulse={running}
              copyable={false}
            />
          </CardAction>
        </CardHeader>
        <CardContent className='flex flex-col gap-4'>
          {report ? (
            <ReportRemarkEditor
              key={report.id}
              report={report}
              running={running}
            />
          ) : (
            <p className='text-muted-foreground text-xs'>
              {t(
                'Model names → performance → token usage → capability questions → cache. Results are saved as they arrive.'
              )}
            </p>
          )}
          <div className='flex flex-col gap-2'>
            <div className='text-muted-foreground flex justify-between gap-3 text-xs tabular-nums'>
              <span>
                {t('{{count}} / {{limit}} requests', {
                  count: requests,
                  limit,
                })}
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
                {labels.names[probeCheckID(state.activeProbe) || ''] ||
                  state.activeProbe}{' '}
                · {state.activeProbe}
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
                {t('Stopped before all checks ran')} ·{' '}
                {labels.codes[stopReason] || stopReason}
              </AlertDescription>
            </Alert>
          )}
          {state.error && (
            <Alert>
              <AlertDescription>{state.error}</AlertDescription>
            </Alert>
          )}
          {report && !running && (
            <details className='rounded-lg border p-3'>
              <summary className='cursor-pointer text-sm font-medium'>
                {t('Save this check as a baseline')}
              </summary>
              <div className='mt-3'>
                <SaveBaseline report={report} running={running} />
              </div>
            </details>
          )}
        </CardContent>
        <CardFooter className='flex flex-wrap justify-between gap-3'>
          <p className='text-muted-foreground max-w-xl text-xs'>
            {t(
              'Rates use reported output tokens divided by total request time. Network, load and reported counters affect the result.'
            )}
          </p>
          <div className='flex flex-wrap gap-2'>
            <Button
              variant='outline'
              size='sm'
              disabled={!report || running}
              onClick={() => exportDocument(false)}
            >
              {t('Download HTML report')}
            </Button>
            <Button
              variant='outline'
              size='sm'
              disabled={!report || running}
              onClick={() => exportDocument(true)}
            >
              {t('Print / Save PDF')}
            </Button>
            <Button
              variant='outline'
              size='sm'
              disabled={!report || running}
              onClick={() => {
                if (report) downloadCheckReport(report)
              }}
            >
              {t('Export JSON')}
            </Button>
          </div>
        </CardFooter>
      </Card>
      {report && (
        <>
          <div ref={sheetRef}>
            <ReportSheet
              report={report}
              running={running}
              phaseLabel={phases[state.phase]}
              durationMS={duration}
              activeProbe={running ? state.activeProbe : null}
            />
          </div>
          <details className='rounded-xl border p-4'>
            <summary className='cursor-pointer text-sm font-medium'>
              {t('Detailed comparison evidence')}
            </summary>
            <div className='mt-4 flex flex-col gap-4'>
              <BedrockDiagnostics
                report={report}
                running={running}
                activeProbe={state.activeProbe}
              />
              {report.token_audit && (
                <details className='rounded-xl border p-4'>
                  <summary className='cursor-pointer text-sm font-medium'>
                    {t('Prompt and cache token evidence')}
                  </summary>
                  <div className='mt-4'>
                    <TokenAudit report={report} running={running} />
                  </div>
                </details>
              )}
              <details className='rounded-xl border p-4'>
                <summary className='cursor-pointer text-sm font-medium'>
                  {t('All request evidence')} · {requests}
                </summary>
                <div className='mt-4 flex flex-col gap-4'>
                  <div className='divide-y'>
                    {plan
                      .filter(
                        (item) => item.selected && SCORED_CHECK_IDS.has(item.id)
                      )
                      .map((item) => (
                        <CheckRow
                          key={item.id}
                          id={item.id}
                          kind={item.kind}
                          score={
                            item.id === 'usage_token_integrity'
                              ? usageTokens?.score
                              : undefined
                          }
                          descriptionCode={
                            item.id === 'usage_token_integrity'
                              ? usageTokens?.code
                              : undefined
                          }
                          selected={item.selected}
                          check={report.checks.find(
                            (check) => check.id === item.id
                          )}
                          running={
                            running &&
                            probeCheckID(state.activeProbe) === item.id
                          }
                          samples={samplesForCheck(report, item.id)}
                        />
                      ))}
                  </div>
                  <RequestLedger
                    samples={report.samples}
                    activeProbe={running ? state.activeProbe : null}
                  />
                </div>
              </details>
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
