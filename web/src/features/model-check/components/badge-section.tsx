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
import { BadgeGenerator } from './badge-generator'
import { ReportBadge } from './report-badge'

// The badge on the home page: what it is, what it looks like on a site, and
// the generator that makes the code.
export function BadgeSection() {
  const { t } = useTranslation()
  const facts = [
    t('Shows the verdict of your latest check. The report itself stays private.'),
    t('Served only to pages on your own domain, so it cannot be borrowed.'),
    t('Expires after 30 days. A badge you do not renew says so.'),
  ]
  return (
    <section aria-labelledby='badge-title' id='get-badge' className='scroll-mt-20'>
      <div className='grid gap-10 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)] lg:gap-16'>
        <div className='flex flex-col gap-6'>
          <h2
            id='badge-title'
            className='font-serif text-2xl font-semibold tracking-tight text-balance sm:text-3xl'
          >
            {t('Put a ModelsExam badge on your site')}
          </h2>
          <p className='text-muted-foreground max-w-xl text-sm leading-6'>
            {t(
              'Visitors can see that your API was examined, what it scored and when. The mark is not something you type in: it is read from the latest check of your endpoint.'
            )}
          </p>
          <figure className='bg-card flex flex-col gap-4 rounded-lg border p-5'>
            <div className='flex flex-wrap items-center gap-3'>
              <ReportBadge size='md' tier='conformant' score={100} />
              <ReportBadge size='md' tier='mostly' score={92} />
              <ReportBadge size='md' tier='review' score={64} />
            </div>
            <figcaption className='text-muted-foreground text-xs leading-5'>
              {t('How it looks on your site, whatever the result: conformant, mostly conformant, or review suggested. It shows the real score, good or bad.')}
            </figcaption>
          </figure>
          <ul className='flex list-disc flex-col gap-2 pl-5 text-sm leading-6'>
            {facts.map((fact) => (
              <li key={fact}>{fact}</li>
            ))}
          </ul>
          <p className='text-sm'>
            <Link to='/get-badge' className='underline underline-offset-4 hover:no-underline'>
              {t('How the badge is protected')}
            </Link>
          </p>
        </div>
        <div className='bg-muted/40 flex flex-col gap-4 rounded-lg border p-5 sm:p-6'>
          <h3 className='font-serif text-lg font-semibold'>{t('Get your code')}</h3>
          <BadgeGenerator />
        </div>
      </div>
    </section>
  )
}
