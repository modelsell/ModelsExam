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
import { navigate } from '@/lib/router'
import { reportPath } from '@/lib/route'
import { BoardGrid } from '@/features/model-check/components/board-table'
import { FaqSection } from '@/features/model-check/components/faq-section'
import { StatsStrip } from '@/features/model-check/components/stats-strip'
import { Link } from '@/lib/router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BadgeSection } from '@/features/model-check/components/badge-section'
import { BaselinesSection } from '@/features/model-check/components/baselines-section'
import { CheckHistory } from '@/features/model-check/components/check-history'
import { PromisesSection } from '@/features/model-check/components/promises-section'
import { NewCheck } from '@/features/model-check/components/new-check'
import { consumePrefill } from '@/features/model-check/lib/link-prefill'

const open = (id: string | undefined) => navigate(id ? reportPath(id) : '/records')

export function HomePage() {
  const { t } = useTranslation()
  const [prefill] = useState(consumePrefill)
  return (
    <>
      <NewCheck prefill={prefill} onSelectReport={open} />
      <StatsStrip />
      <div className='mx-auto flex max-w-6xl flex-col gap-16 px-4 py-14 sm:px-6 sm:py-20'>
        <section aria-labelledby='boards-title' id='boards' className='scroll-mt-20'>
          <div className='mb-6 flex items-end justify-between gap-4'>
            <div>
              <h2 id='boards-title' className='text-2xl font-extrabold tracking-tight sm:text-3xl'>
                {t('Latest check results by model')}
              </h2>
              <p className='text-muted-foreground mt-2 text-sm'>
                {t('Newest result per site, best score first. Dated results, not endorsements.')}
              </p>
            </div>
            <Link to='/models' className='text-sm whitespace-nowrap underline underline-offset-4 hover:no-underline'>
              {t('All boards')}
            </Link>
          </div>
          <BoardGrid limit={3} size={5} />
        </section>
        <PromisesSection />
        <BadgeSection />
        <BaselinesSection showLink />
        <FaqSection />
      </div>
      <div className='bg-muted/40 border-t px-4 py-14 sm:px-6 sm:py-20'>
        <div className='mx-auto max-w-6xl'>
          <CheckHistory />
        </div>
      </div>
    </>
  )
}
