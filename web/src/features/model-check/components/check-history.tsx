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
import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Clock01Icon, FileDownloadIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { useDebounce } from '@/hooks/use-debounce'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
import { Link } from '@/lib/router'
import { reportPath } from '@/lib/route'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  downloadCheckReport,
  getCheckHistory,
  getCheckHistoryReport,
} from '../api'
import { reportRemark } from '../lib/report-remark'
import { siteLabel } from '../lib/site-label'
import { sourceOfEndpoint } from '../lib/claude-source'
import { SourceBadge } from './source-badge'
import { badgeTier } from '../lib/badge'
import { ReportBadge } from './report-badge'
import { downloadImageReportJSON } from '../image/api'
import { isImageHistory } from '../image/lib/history-state'
import { downloadOpenAIReportJSON } from '../openai/api'
import { isOpenAIHistory } from '../openai/lib/history-state'
import type { CheckHistoryItem, CheckHistoryStatus } from '../types'
import { RetestActions } from '@/features/account/components/retest-actions'
import { scoreDelta } from '@/features/account/lib/retest'
import { cn } from '@/lib/utils'

// Score change against the previous run of the same configuration.
function ScoreDelta({ item }: { item: CheckHistoryItem }) {
  const delta = scoreDelta(item.score, item.previous_score)
  if (delta == null || delta === 0) return null
  return (
    <span
      className={cn('ml-1 text-xs font-semibold tabular-nums', delta < 0 ? 'text-destructive' : 'text-emerald-600 dark:text-emerald-400')}
      title={`${item.previous_score} → ${item.score}`}
    >
      {delta > 0 ? `+${delta}` : delta}
    </span>
  )
}

// The tested site: its name, and the address that was tested. Both come from
// whoever ran the check, so the address is shown as text and never linked out;
// the name links to this site's own page here.
function SiteCell({ item }: { item: CheckHistoryItem }) {
  const site = siteLabel(item)
  let hostname = ''
  try {
    hostname = new URL(site.url).hostname.toLowerCase()
  } catch {
    hostname = ''
  }
  const sitePage = /^[a-z0-9.-]+$/.test(hostname) ? `/sites/${hostname}` : null
  return (
    <div className='flex min-w-0 flex-col'>
      {sitePage ? (
        <Link
          to={sitePage}
          className='truncate font-medium underline-offset-4 hover:underline'
          title={site.name}
        >
          {site.name}
        </Link>
      ) : (
        <span className='truncate font-medium' title={site.name}>
          {site.name}
        </span>
      )}
      <span className='text-muted-foreground truncate text-xs' title={site.url}>
        {site.url || '—'}
      </span>
    </div>
  )
}

export function CheckHistory(props: { initialModel?: string }) {
  const { t } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const signedIn = useAuthStore((state) => !!state.auth.user.account)
  const [filters, setFilters] = useState({ model: props.initialModel ?? '', status: '', page: 1 })
  const debounced = useDebounce(filters, 300)
  const labels: Record<CheckHistoryStatus, string> = {
    running: t('Check in progress'),
    completed: t('Completed'),
    stopped: t('Check stopped'),
    failed: t('Review suggested'),
    cancelled: t('Check stopped'),
    interrupted: t('Check interrupted'),
  }
  const list = useQuery({
    queryKey: ['model-check-history', userId, 'list', debounced],
    // No AbortSignal: React Query aborts it on StrictMode's mount/unmount
    // replay and axios then rejects with CanceledError, leaving the list in an
    // error state before any request is sent.
    queryFn: () => getCheckHistory(debounced),
    // Records belong to accounts: without one there is no list to load.
    enabled: signedIn,
    refetchInterval: (query) =>
      query.state.data?.items.some((item) => item.status === 'running')
        ? 3000
        : false,
  })
  const total = list.data?.total ?? 0
  const page = list.data?.page ?? debounced.page
  const size = list.data?.page_size ?? 20
  const download = useMutation({
    mutationFn: (id: string) => getCheckHistoryReport(id),
    onSuccess: (data) =>
      isImageHistory(data)
        ? downloadImageReportJSON(data.report)
        : isOpenAIHistory(data)
          ? downloadOpenAIReportJSON(data.report)
          : downloadCheckReport(data.report),
    onError: () => toast.error(t('Failed to load check report')),
  })

  return (
    <section
      id='check-records'
      aria-labelledby='check-history-title'
      className='flex scroll-mt-28 flex-col gap-6'
    >
      <header className='flex flex-wrap items-end justify-between gap-4'>
        <div className='flex flex-col gap-2'>
          <h2
            id='check-history-title'
            className='font-serif text-2xl font-semibold tracking-tight sm:text-3xl'
          >
            {t('My check records')}
          </h2>
          <p className='text-muted-foreground text-sm leading-6'>
            {t('Checks of your account, from any device. Nobody else can see this list.')}
          </p>
        </div>
        <p className='text-muted-foreground flex items-center gap-2 text-xs'>
          <HugeiconsIcon
            icon={FileDownloadIcon}
            className='size-4'
            aria-hidden='true'
          />
          HTML · JSON · PDF
        </p>
      </header>
      <Card>
        <CardHeader className='sr-only'>
          <CardTitle>{t('My check records')}</CardTitle>
          <CardDescription>
            {t('Your latest checks while signed in. Other people cannot see this list; a report opens only for people you share its link with.')}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex flex-col gap-4'>
          <div className='flex flex-wrap items-center gap-2'>
            <Input
              className='w-full sm:max-w-72'
              aria-label={t('Filter by model')}
              placeholder={t('Filter by model')}
              value={filters.model}
              maxLength={200}
              disabled={!signedIn}
              onChange={(event) =>
                setFilters((previous) => ({
                  ...previous,
                  model: event.target.value,
                  page: 1,
                }))
              }
            />
            <NativeSelect
              aria-label={t('Status')}
              value={filters.status}
              disabled={!signedIn}
              onChange={(event) =>
                setFilters((previous) => ({
                  ...previous,
                  status: event.target.value,
                  page: 1,
                }))
              }
            >
              <NativeSelectOption value=''>
                {t('All statuses')}
              </NativeSelectOption>
              {(Object.keys(labels) as CheckHistoryStatus[]).map((status) => (
                <NativeSelectOption key={status} value={status}>
                  {labels[status]}
                </NativeSelectOption>
              ))}
            </NativeSelect>
            <Button
              variant='outline'
              disabled={!signedIn || list.isFetching}
              onClick={() => {
                void list.refetch()
              }}
            >
              {t('Refresh')}
            </Button>
          </div>
          {list.isError && (
            <Alert variant='destructive'>
              <AlertDescription>
                {t('Failed to load check history')}
              </AlertDescription>
            </Alert>
          )}
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Time')}</TableHead>
                <TableHead>{t('Website')}</TableHead>
                <TableHead>{t('Model')}</TableHead>
                <TableHead>{t('Remark')}</TableHead>
                <TableHead>{t('Badge')}</TableHead>
                <TableHead>{t('Duration')}</TableHead>
                <TableHead>{t('Requests')}</TableHead>
                <TableHead>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {signedIn && list.isPending && (
                <TableRow>
                  <TableCell colSpan={8}>
                    <Skeleton className='h-24' />
                  </TableCell>
                </TableRow>
              )}
              {list.data?.items.map((item) => (
                <TableRow
                  key={item.id}
                  className={cn(
                    // A failed check or a score below the previous run stands out.
                    (item.status === 'failed' || (scoreDelta(item.score, item.previous_score) ?? 0) < 0) &&
                      'bg-destructive/5'
                  )}
                >
                  <TableCell className='text-xs tabular-nums'>
                    {new Date(item.started_at).toLocaleString()}
                    {item.scheduled && (
                      <span className='text-muted-foreground mt-1 block w-fit rounded border px-1.5 py-0.5 text-[10px]'>
                        {t('Scheduled')}
                      </span>
                    )}
                  </TableCell>
                  <TableCell className='max-w-72'>
                    <SiteCell item={item} />
                  </TableCell>
                  <TableCell
                    className='max-w-64 truncate font-medium'
                    title={item.model}
                  >
                    <span className='text-muted-foreground mr-2 rounded border px-1.5 py-0.5 text-[10px] font-normal'>
                      {item.transport === 'openai_api'
                        ? 'OpenAI'
                        : item.transport === 'gemini_api'
                          ? 'Gemini'
                          : item.transport === 'image_api'
                          ? t('Image')
                          : 'Claude'}
                    </span>
                    {item.transport !== 'openai_api' &&
                      item.transport !== 'gemini_api' &&
                      item.transport !== 'image_api' && (
                      <SourceBadge
                        source={sourceOfEndpoint(item.endpoint, item.transport)}
                        className='mr-2 align-middle'
                      />
                    )}
                    {item.model}
                  </TableCell>
                  <TableCell
                    className='max-w-64 truncate text-xs'
                    title={reportRemark(item)}
                  >
                    {reportRemark(item) || '—'}
                  </TableCell>
                  <TableCell>
                    <ReportBadge
                      tier={badgeTier(item.status, item.score)}
                      score={item.score}
                    />
                    <ScoreDelta item={item} />
                  </TableCell>
                  <TableCell className='text-xs tabular-nums'>
                    {(item.duration_ms / 1000).toFixed(1)} s
                  </TableCell>
                  <TableCell className='text-xs tabular-nums'>
                    {item.request_count}
                  </TableCell>
                  <TableCell>
                    <div className='flex items-center gap-2'>
                      <Link
                        to={reportPath(item.id)}
                        className={buttonVariants({ size: 'sm', variant: 'outline' })}
                        aria-label={t('View report for {{model}} at {{time}}', {
                          model: item.model,
                          time: new Date(item.started_at).toLocaleString(),
                        })}
                      >
                        {t('View report')}
                      </Link>
                      <Button
                        size='sm'
                        variant='ghost'
                        disabled={
                          item.status === 'running' || download.isPending
                        }
                        onClick={() => download.mutate(item.id)}
                      >
                        {t('Export JSON')}
                      </Button>
                      <RetestActions item={item} />
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {!signedIn && (
                <TableRow>
                  <TableCell colSpan={8}>
                    <Empty className='py-10'>
                      <EmptyHeader>
                        <EmptyMedia variant='icon'>
                          <HugeiconsIcon icon={Clock01Icon} />
                        </EmptyMedia>
                        <EmptyTitle>
                          {t('Your model checks, all in one place')}
                        </EmptyTitle>
                        <EmptyDescription>
                          {t('Sign in to see your check records. Checks you run while signed in are saved to your account; checks run without an account are not added to any list.')}
                        </EmptyDescription>
                        <div className='flex justify-center gap-2'>
                          <Link to='/login' className={buttonVariants({ size: 'sm' })}>
                            {t('Sign in')}
                          </Link>
                          <Link to='/register' className={buttonVariants({ size: 'sm', variant: 'outline' })}>
                            {t('Create account')}
                          </Link>
                        </div>
                      </EmptyHeader>
                    </Empty>
                  </TableCell>
                </TableRow>
              )}
              {signedIn &&
                !list.isPending &&
                !list.isError &&
                !list.data?.items.length && (
                  <TableRow>
                    <TableCell colSpan={8}>
                      <Empty className='py-10'>
                        <EmptyHeader>
                          <EmptyMedia variant='icon'>
                            <HugeiconsIcon icon={Clock01Icon} />
                          </EmptyMedia>
                          <EmptyTitle>{t('No check history found')}</EmptyTitle>
                          <EmptyDescription>
                            {t(
                              'Run a check above to create your first report, or adjust your filters.'
                            )}
                          </EmptyDescription>
                        </EmptyHeader>
                      </Empty>
                    </TableCell>
                  </TableRow>
                )}
            </TableBody>
          </Table>
        </CardContent>
        <CardFooter className='flex flex-wrap justify-between gap-3'>
          <p className='text-muted-foreground text-xs'>
            {t('{{count}} saved checks', { count: total })}
          </p>
          <div className='flex items-center gap-3'>
            <Button
              size='sm'
              variant='outline'
              disabled={!signedIn || filters.page <= 1 || list.isFetching}
              onClick={() =>
                setFilters((previous) => ({
                  ...previous,
                  page: previous.page - 1,
                }))
              }
            >
              {t('Previous')}
            </Button>
            <span className='text-muted-foreground text-xs tabular-nums'>
              {page} / {Math.max(1, Math.ceil(total / size))}
            </span>
            <Button
              size='sm'
              variant='outline'
              disabled={
                !signedIn || filters.page * size >= total || list.isFetching
              }
              onClick={() =>
                setFilters((previous) => ({
                  ...previous,
                  page: previous.page + 1,
                }))
              }
            >
              {t('Next')}
            </Button>
          </div>
        </CardFooter>
      </Card>
    </section>
  )
}
