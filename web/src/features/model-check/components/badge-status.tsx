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
import { Link } from '@/lib/router'
import { reportPath } from '@/lib/route'
import type { BadgeData } from '../api'
import type { BadgeTier } from '../lib/badge'
import { ReportBadge } from './report-badge'

const day = (ms?: number) => (ms ? new Date(ms).toLocaleDateString() : '')

// What a domain's badge currently says: the verdict, when it was checked and
// when it expires, with a link to the report behind it.
export function BadgeStatus({ data, domain }: { data: BadgeData; domain: string }) {
  const { t } = useTranslation()
  if (data.status === 'none') {
    return (
      <div className='flex flex-col gap-2'>
        <p className='text-sm leading-6'>
          {t('No completed check found for {{domain}} yet.', { domain })}
        </p>
        <p className='text-muted-foreground text-sm leading-6'>
          {t(
            'Run a check on this site’s API endpoint. The endpoint’s host must be this domain or one of its subdomains.'
          )}
        </p>
        <p className='text-sm'>
          <Link to='/#new-check' className='underline underline-offset-4 hover:no-underline'>
            {t('Start a check')}
          </Link>
        </p>
      </div>
    )
  }
  const tier: BadgeTier = data.status === 'stale' ? 'incomplete' : (data.tier ?? 'unscored')
  return (
    <div className='flex flex-col gap-3'>
      <div className='flex flex-wrap items-center gap-3'>
        <ReportBadge size='md' tier={tier} score={data.status === 'ok' ? data.score : null} />
        {data.status === 'stale' && (
          <span className='text-sm'>{t('Check expired. Run a new check to refresh it.')}</span>
        )}
      </div>
      <p className='text-muted-foreground text-xs leading-5'>
        {t('Checked {{checked}}', { checked: day(data.checked_at) })}
        {data.status === 'ok' && ` · ${t('valid until {{expires}}', { expires: day(data.expires_at) })}`}
        {data.model ? ` · ${data.model}` : ''}
      </p>
      {data.report_id && (
        <p className='text-sm'>
          <Link
            to={reportPath(data.report_id)}
            className='underline underline-offset-4 hover:no-underline'
          >
            {t('Read the report')}
          </Link>
        </p>
      )}
    </div>
  )
}
