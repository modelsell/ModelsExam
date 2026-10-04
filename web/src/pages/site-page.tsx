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
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Skeleton } from '@/components/ui/skeleton'
import { Link } from '@/lib/router'
import { getBadge } from '@/features/model-check/api'
import { BadgeStatus } from '@/features/model-check/components/badge-status'

// Where a site's badge leads: its latest result and the report behind it.
export function SitePage({ domain }: { domain: string }) {
  const { t } = useTranslation()
  const status = useQuery({
    queryKey: ['model-check-badge', domain],
    queryFn: () => getBadge(domain),
    retry: false,
  })
  return (
    <div className='mx-auto flex max-w-3xl flex-col gap-6 px-4 py-12 sm:px-6 sm:py-16'>
      <h1 className='font-serif text-3xl font-semibold tracking-tight break-all sm:text-4xl'>{domain}</h1>
      <p className='text-muted-foreground text-sm leading-6'>
        {t('The latest ModelsExam check result for this site. A badge describes one check at one moment.')}
      </p>
      {status.isPending && <Skeleton className='h-20' />}
      {status.isError && (
        <p className='text-destructive text-sm'>{t('Could not load the badge. Try again.')}</p>
      )}
      {status.data && <BadgeStatus data={status.data} domain={domain} />}
      <p className='text-sm'>
        <Link to='/get-badge' className='underline underline-offset-4 hover:no-underline'>
          {t('Show your own badge')}
        </Link>
      </p>
    </div>
  )
}
