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

// What a badge stands for, in four lines. Each promise names what the exam
// actually looks at; none of them is a guarantee about the future.
export function PromisesSection() {
  const { t } = useTranslation()
  const promises = [
    {
      title: t('Real models only. No fakes, no dilution.'),
      body: t(
        'Each check looks at how an endpoint actually behaves: identity answers, behavior fingerprints and capability probes. A cheaper model passed off as a premium one tends to show up here.'
      ),
    },
    {
      title: t('Official protocol. No inflated labels.'),
      body: t(
        'The same requests follow the official contract: fields, streaming, tool calls and error formats. Reported token usage is compared with what was sent, so padded counts are flagged.'
      ),
    },
    {
      title: t('Stable service, with a check date you can see.'),
      body: t(
        'A badge is only as fresh as its last check. It expires after 30 days, and every report lists latency and failed requests.'
      ),
    },
    {
      title: t('Open source. Public results.'),
      body: t('The code is AGPL-3.0 and every record is public.'),
    },
  ]
  return (
    <section aria-labelledby='promises-title' id='promises' className='scroll-mt-20'>
      <p className='text-[#2457c5] dark:text-cobalt mb-3 font-mono text-xs tracking-[0.18em] uppercase'>
        {t('Specification')}
      </p>
      <h2
        id='promises-title'
        className='mb-8 text-2xl font-extrabold tracking-tight sm:text-3xl'
      >
        {t('What a ModelsExam badge stands for')}
      </h2>
      <div className='grid gap-x-10 gap-y-8 sm:grid-cols-2'>
        {promises.map((promise, i) => (
          <div key={promise.title} className='border-foreground/30 flex flex-col gap-2 border-t-2 pt-4'>
            <span className='text-[#2457c5] dark:text-cobalt font-mono text-xs'>{String(i + 1).padStart(2, '0')}</span>
            <h3 className='text-lg leading-snug font-bold text-balance'>
              {promise.title}
            </h3>
            <p className='text-muted-foreground text-sm leading-6'>{promise.body}</p>
          </div>
        ))}
      </div>
      <p className='text-muted-foreground mt-6 max-w-2xl text-xs leading-5'>
        {t('Results describe one check at one moment. They are evidence, not a guarantee.')}{' '}
        <Link to='/get-badge' className='text-foreground underline underline-offset-4 hover:no-underline'>
          {t('Show the badge on your site')}
        </Link>
      </p>
    </section>
  )
}
