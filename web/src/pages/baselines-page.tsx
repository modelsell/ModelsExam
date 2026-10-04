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
import { BaselinesSection } from '@/features/model-check/components/baselines-section'

export function BaselinesPage() {
  const { t } = useTranslation()
  const steps = [
    t('Run the exam directly on the official endpoint with the maintainers’ own key.'),
    t('Publish the report as it came out, without editing it.'),
    t('Run the same exam on a relay and read the two reports side by side.'),
  ]
  return (
    <div className='mx-auto flex max-w-4xl flex-col gap-14 px-4 py-12 sm:px-6 sm:py-16'>
      <BaselinesSection headingLevel='h1' />
      <section aria-labelledby='baseline-how' className='flex flex-col gap-4'>
        <h2 id='baseline-how' className='font-serif text-xl font-semibold'>
          {t('How baselines are made')}
        </h2>
        <ol className='flex list-decimal flex-col gap-2 pl-5 text-sm leading-6'>
          {steps.map((step) => (
            <li key={step}>{step}</li>
          ))}
        </ol>
        <p className='text-muted-foreground max-w-2xl text-sm leading-6'>
          {t(
            'A baseline is one run at one time. Providers change models quietly, so each row links to a dated report and is replaced when it goes stale. Anyone can repeat a baseline with the open-source tool.'
          )}
        </p>
        <p className='text-sm'>
          <Link to='/#new-check' className='underline underline-offset-4 hover:no-underline'>
            {t('Run the exam on your relay')}
          </Link>
        </p>
      </section>
    </div>
  )
}
