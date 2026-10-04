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
import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { Link } from '@/lib/router'
import { applySeo, SITE_NAME } from '@/lib/seo'
import { useAuthStore } from '@/stores/auth-store'
import { getCheckHistoryReport } from '@/features/model-check/api'
import { ReportDetail } from '@/features/model-check/components/report-detail'
import { siteLabel } from '@/features/model-check/lib/site-label'
import { badgeTier } from '@/features/model-check/lib/badge'

// One stored report on its own page, at /reports/<id>. The address is the
// share link, and for completed runs with a site name it is also indexed:
// the tested site's name and description become the page title and summary.
export function ReportPage({ id }: { id: string }) {
  const { t, i18n } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const detail = useQuery({
    queryKey: ['model-check-history', userId, 'detail', id],
    queryFn: () => getCheckHistoryReport(id),
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.run.status === 'running' ? 3000 : false,
  })
  const run = detail.data?.run
  const site = run ? siteLabel(run) : undefined

  useEffect(() => {
    const indexable = run?.status === 'completed' && !!run.channel_name?.trim()
    const tier = run ? badgeTier(run.status, run.score) : undefined
    const result =
      run && tier && tier !== 'incomplete' && tier !== 'running' && run.score != null
        ? ` ${run.model}: ${run.score}/100.`
        : run
          ? ` ${run.model}.`
          : ''
    applySeo(
      { name: 'report', id },
      {
        title: site ? `${site.name} ${t('check report')} | ${SITE_NAME}` : `${t('Check report')} | ${SITE_NAME}`,
        description: site
          ? `${site.description ? `${site.description} ` : ''}${t('ModelsExam check result for')}${result}`.trim()
          : t('A single ModelsExam check report.'),
        noindex: !indexable,
      },
      i18n.resolvedLanguage ?? 'en'
    )
  }, [run, site, id, t, i18n.resolvedLanguage])

  return (
    <div className='mx-auto flex max-w-5xl flex-col gap-8 px-4 py-10 sm:px-6 sm:py-14'>
      <p className='text-sm'>
        <Link to='/records' className='text-muted-foreground hover:text-foreground underline underline-offset-4'>
          {t('Back to check records')}
        </Link>
      </p>
      {detail.isPending && <Skeleton className='h-80 rounded-xl' />}
      {detail.isError && (
        <Alert variant='destructive'>
          <AlertDescription>{t('Failed to load check report')}</AlertDescription>
        </Alert>
      )}
      {run && site && (
        <header className='flex flex-col gap-3'>
          <h1 className='font-serif text-3xl font-semibold tracking-tight text-balance sm:text-4xl'>
            {site.name}
          </h1>
          {/^https?:\/\//.test(site.url) && (
            // Untrusted address supplied by whoever ran the check: never followed by crawlers.
            <a
              href={site.url}
              target='_blank'
              rel='nofollow ugc noopener noreferrer'
              className='text-muted-foreground w-fit max-w-full truncate text-sm underline underline-offset-4 hover:no-underline'
            >
              {site.url}
            </a>
          )}
          {site.description && (
            <p className='max-w-2xl text-sm leading-6'>{site.description}</p>
          )}
          <p className='text-muted-foreground text-xs'>
            {site.description
              ? `${t('Description read from the site’s home page when the check ran.')} `
              : ''}
            {run.model} · {new Date(run.started_at).toLocaleString()}
          </p>
        </header>
      )}
      {detail.data && <ReportDetail detail={detail.data} />}
    </div>
  )
}
