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
import { FaqSection } from '@/features/model-check/components/faq-section'
import { useEffect, useState } from 'react'
import { BadgeSection } from '@/features/model-check/components/badge-section'
import { BaselinesSection } from '@/features/model-check/components/baselines-section'
import { CheckHistory } from '@/features/model-check/components/check-history'
import { PromisesSection } from '@/features/model-check/components/promises-section'
import { NewCheck } from '@/features/model-check/components/new-check'
import { consumePrefill, RETEST_EVENT, takeRetestPrefill } from '@/features/model-check/lib/link-prefill'

const open = (id: string | undefined) => navigate(id ? reportPath(id) : '/records')

export function HomePage() {
  const [state, setState] = useState(() => ({ prefill: takeRetestPrefill() ?? consumePrefill(), generation: 0 }))
  // A retest started from the records list on this same page refills the form.
  useEffect(() => {
    const onRetest = () => {
      const prefill = takeRetestPrefill()
      if (prefill) setState((prev) => ({ prefill, generation: prev.generation + 1 }))
    }
    window.addEventListener(RETEST_EVENT, onRetest)
    return () => window.removeEventListener(RETEST_EVENT, onRetest)
  }, [])
  return (
    <>
      <NewCheck key={state.generation} prefill={state.prefill} onSelectReport={open} />
      <div className='mx-auto flex max-w-6xl flex-col gap-16 px-4 py-14 sm:px-6 sm:py-20'>
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
