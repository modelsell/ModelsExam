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
import { Button } from '@/components/ui/button'
import {
  BADGE_CONFORMANT_MIN,
  BADGE_MOSTLY_MIN,
  type BadgeTier,
} from '../lib/badge'
import { ParticleField } from '@/components/particle-field'
import { ReportBadge } from './report-badge'

export function CheckHeroIntro() {
  const { t } = useTranslation()
  const legend: Array<{ tier: BadgeTier; score?: number; text: string }> = [
    {
      tier: 'conformant',
      score: BADGE_CONFORMANT_MIN,
      text: t('Every scored check passed.'),
    },
    {
      tier: 'mostly',
      score: 92,
      text: t('Most scored checks passed ({{min}} to 99). The report lists the failures.', {
        min: BADGE_MOSTLY_MIN,
      }),
    },
    {
      tier: 'review',
      score: 64,
      text: t('Scored checks failed (below {{min}}).', {
        min: BADGE_MOSTLY_MIN,
      }),
    },
    {
      tier: 'incomplete',
      text: t('The run stopped before every check ran, so it gets no verdict.'),
    },
  ]
  return (
    <header className='bg-background graticule border-b relative overflow-hidden'>
      <ParticleField />
      <div
        aria-hidden='true'
        className='pointer-events-none absolute -top-40 right-0 h-[28rem] w-[44rem] rounded-full bg-[radial-gradient(closest-side,rgb(61_123_255/0.22),transparent)]'
      />
      <div className='relative mx-auto grid max-w-6xl items-center gap-12 px-4 py-16 sm:px-6 sm:py-24 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)] lg:gap-16'>
        <div className='flex flex-col gap-6'>
          <p className='text-[#2457c5] dark:text-cobalt flex items-center gap-2 font-mono text-xs tracking-[0.18em] uppercase'>
            <span aria-hidden='true' className='bg-cobalt inline-block h-px w-8' />
            {t('Independent API conformance lab')}
          </p>
          <h1
            id='model-check-title'
            className='text-4xl leading-[1.05] font-extrabold text-balance sm:text-5xl lg:text-6xl'
          >
            {t('Independent conformance testing for AI model APIs')}
          </h1>
          <p className='max-w-xl text-lg leading-snug font-semibold text-balance text-[#2457c5] dark:text-sky-200'>
            {t('Real models, official protocol, visible check dates.')}
          </p>
          <p className='max-w-xl text-base leading-7 text-muted-foreground text-pretty'>
            {t(
              'Point it at any endpoint. It sends a fixed set of requests and reports exactly what came back, with the evidence attached.'
            )}
          </p>
          <div className='flex flex-wrap gap-3'>
            <Button
              size='lg'
              className='bg-cobalt hover:bg-cobalt/90 text-white'
              render={<a href='#new-check' />}
            >
              {t('Start a check')}
            </Button>
            <Button
              size='lg'
              variant='outline'
              render={<a href='#check-records' />}
            >
              {t('Browse check records')}
            </Button>
          </div>
          <p className='max-w-xl border-l-2 pl-4 text-sm leading-6 text-muted-foreground'>
            {t(
              'A result describes one endpoint at one moment. It is not a ranking and not an endorsement.'
            )}
          </p>
        </div>
        <section
          aria-labelledby='badge-legend-title'
          className='flex flex-col gap-5 bg-card/80 rounded-xl border p-6 shadow-lg backdrop-blur-sm'
        >
          <div className='flex items-start justify-between gap-3'>
            <div className='flex flex-col gap-1'>
              <h2 id='badge-legend-title' className='text-lg font-bold'>
                {t('Reading a badge')}
              </h2>
              <p className='text-sm text-muted-foreground'>
                {t('Every check record carries one of these.')}
              </p>
            </div>
            <span
              aria-hidden='true'
              className='mt-1.5 flex shrink-0 items-center gap-1.5 font-mono text-[10px] tracking-widest text-emerald-700 dark:text-emerald-300 uppercase'
            >
              <span className='size-1.5 rounded-full bg-emerald-500' />
              live
            </span>
          </div>
          <dl className='flex flex-col divide-y divide-border'>
            {legend.map((item) => (
              <div
                key={item.tier}
                className='grid gap-2 py-3 first:pt-0 last:pb-0 sm:grid-cols-[14rem_1fr] sm:items-center sm:gap-x-4'
              >
                <dt>
                  <ReportBadge tier={item.tier} score={item.score} />
                </dt>
                <dd className='text-sm leading-5 text-muted-foreground'>{item.text}</dd>
              </div>
            ))}
          </dl>
          <p className='text-xs leading-5 text-muted-foreground'>
            {t(
              'Observations and provenance results appear in reports but never change a badge.'
            )}
          </p>
        </section>
      </div>
    </header>
  )
}

export function CheckCoverage() {
  const { t } = useTranslation()
  const rows = [
    {
      protocol: t('Anthropic Messages'),
      reaches: t('Claude models, directly or through a gateway'),
      endpoints: '/v1/messages',
      provenance: '—',
    },
    {
      protocol: t('OpenAI Chat Completions and Responses'),
      reaches: t('OpenAI, Azure OpenAI and OpenAI-compatible relays'),
      endpoints: '/v1/models · /v1/chat/completions · /v1/responses',
      provenance: '—',
    },
    {
      protocol: t('OpenAI Images'),
      reaches: t('gpt-image models and compatible image relays'),
      endpoints: '/v1/images/generations · /v1/images/edits',
      provenance: t('OpenAI Verify (C2PA, SynthID)'),
    },
  ]
  return (
    <section
      aria-labelledby='check-coverage-title'
      className='mx-auto flex w-full max-w-6xl flex-col gap-6'
    >
      <div className='flex max-w-2xl flex-col gap-2'>
        <h2
          id='check-coverage-title'
          className='font-serif text-2xl font-semibold tracking-tight sm:text-3xl'
        >
          {t('Coverage')}
        </h2>
        <p className='text-muted-foreground text-sm leading-6'>
          {t(
            'A provider can be checked when it exposes one of these protocols, whatever its name or brand.'
          )}
        </p>
      </div>
      <div className='overflow-x-auto rounded-lg border'>
        <table className='w-full min-w-[40rem] text-left text-sm'>
          <thead className='bg-muted/50 text-muted-foreground text-xs'>
            <tr>
              <th scope='col' className='px-4 py-2.5 font-medium'>
                {t('Protocol')}
              </th>
              <th scope='col' className='px-4 py-2.5 font-medium'>
                {t('Reaches')}
              </th>
              <th scope='col' className='px-4 py-2.5 font-medium'>
                {t('Endpoints exercised')}
              </th>
              <th scope='col' className='px-4 py-2.5 font-medium'>
                {t('Provenance')}
              </th>
            </tr>
          </thead>
          <tbody className='divide-y'>
            {rows.map((row) => (
              <tr key={row.protocol}>
                <th scope='row' className='px-4 py-3 font-medium'>
                  {row.protocol}
                </th>
                <td className='text-muted-foreground px-4 py-3'>
                  {row.reaches}
                </td>
                <td className='text-muted-foreground px-4 py-3 font-mono text-xs'>
                  {row.endpoints}
                </td>
                <td className='text-muted-foreground px-4 py-3'>
                  {row.provenance}
                </td>
              </tr>
            ))}
            <tr className='bg-muted/30'>
              <th scope='row' className='text-muted-foreground px-4 py-3 font-medium'>
                {t('Other native protocols')}
              </th>
              <td colSpan={3} className='text-muted-foreground px-4 py-3'>
                {t(
                  'Not covered yet. Providers that offer an OpenAI-compatible endpoint can already be checked through it.'
                )}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  )
}

export function CheckMethod() {
  const { t } = useTranslation()
  const principles = [
    {
      title: t('Assertions and observations'),
      text: t(
        'Only objective protocol checks are scored. Latency, caching and similar measurements are observations and never change a score.'
      ),
    },
    {
      title: t('One attempt per request'),
      text: t(
        'Each request is sent once. A failure is reported as it happened, not retried until it passes.'
      ),
    },
    {
      title: t('Evidence with every result'),
      text: t(
        'Reports list the requests that were sent, so a result can be checked and reproduced.'
      ),
    },
    {
      title: t('Your keys stay yours'),
      text: t(
        'Keys are used for the run and removed from every report. An official provenance key is only ever sent to the official host.'
      ),
    },
    {
      title: t('What a badge does not say'),
      text: t(
        'A badge covers one endpoint at one moment. It does not rank providers, certify a vendor or prove which model answered.'
      ),
    },
  ]
  return (
    <section
      aria-labelledby='check-method-title'
      className='mx-auto grid w-full max-w-6xl gap-8 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)] lg:gap-16'
    >
      <div className='flex flex-col gap-2'>
        <h2
          id='check-method-title'
          className='font-serif text-2xl font-semibold tracking-tight sm:text-3xl'
        >
          {t('Method and independence')}
        </h2>
        <p className='text-muted-foreground max-w-sm text-sm leading-6'>
          {t('The rules every check follows, whoever runs it.')}
        </p>
      </div>
      <dl className='flex flex-col divide-y border-y'>
        {principles.map((item) => (
          <div
            key={item.title}
            className='grid gap-1 py-4 sm:grid-cols-[13rem_1fr] sm:gap-6'
          >
            <dt className='text-sm font-semibold'>{item.title}</dt>
            <dd className='text-muted-foreground text-sm leading-6'>
              {item.text}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  )
}
