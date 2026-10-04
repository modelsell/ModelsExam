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
import {
  OFFICIAL_BASELINES,
  type BaselineProvider,
  type OfficialBaseline,
} from '@/config/baselines'
import { useReportBadge } from '../hooks/use-report-badge'
import { ReportBadge } from './report-badge'

function useProviderLabels(): Record<BaselineProvider, { name: string; suite: string }> {
  const { t } = useTranslation()
  return {
    anthropic: { name: 'Anthropic API', suite: t('Claude conformance suite') },
    bedrock: { name: 'AWS Bedrock', suite: t('Claude conformance suite') },
    openai: { name: 'OpenAI API', suite: t('OpenAI conformance suite') },
    'openai-image': { name: 'OpenAI Images', suite: t('Image suite with provenance') },
  }
}

function BaselineRow({ baseline }: { baseline: OfficialBaseline }) {
  const { t } = useTranslation()
  const labels = useProviderLabels()[baseline.provider]
  const { badge, loading, missing } = useReportBadge(baseline.reportId)
  return (
    <li className='grid gap-3 py-4 sm:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_auto] sm:items-center sm:gap-6'>
      <div className='min-w-0'>
        <p className='font-serif text-lg font-semibold'>{labels.name}</p>
        <p className='text-muted-foreground truncate text-xs'>{baseline.endpoint}</p>
      </div>
      <p className='text-muted-foreground text-sm'>{labels.suite}</p>
      <div className='flex items-center gap-3 sm:justify-end'>
        {badge ? (
          <>
            <ReportBadge tier={badge.tier} score={badge.score} />
            <Link
              to={reportPath(baseline.reportId!)}
              className='text-sm underline underline-offset-4 hover:no-underline'
            >
              {t('Read the report')}
            </Link>
          </>
        ) : (
          <span className='text-muted-foreground border-dashed rounded-[5px] border px-2 py-0.5 text-xs'>
            {loading
              ? t('Loading')
              : missing
                ? t('Report unavailable')
                : t('Reference run pending')}
          </span>
        )}
      </div>
    </li>
  )
}

// The answer key: one row per official endpoint. Rows turn into badges as the
// maintainers publish reference runs (config/baselines.ts).
export function BaselinesSection(props: { headingLevel?: 'h1' | 'h2'; showLink?: boolean }) {
  const { t } = useTranslation()
  const Heading = props.headingLevel ?? 'h2'
  return (
    <section aria-labelledby='baselines-title' id='baselines' className='scroll-mt-20'>
      <div className='mb-6 flex flex-col gap-2'>
        <Heading
          id='baselines-title'
          className='font-serif text-2xl font-semibold tracking-tight sm:text-3xl'
        >
          {t('Official baselines')}
        </Heading>
        <p className='text-muted-foreground max-w-2xl text-sm leading-6'>
          {t(
            'A baseline is the same exam taken directly on the provider’s own endpoint. Read a relay’s report next to it and you can see where the relay differs from the source.'
          )}
        </p>
      </div>
      <ul className='divide-y border-y'>
        {OFFICIAL_BASELINES.map((baseline) => (
          <BaselineRow key={baseline.id} baseline={baseline} />
        ))}
      </ul>
      {props.showLink && (
        <p className='mt-4 text-sm'>
          <Link to='/baselines' className='underline underline-offset-4 hover:no-underline'>
            {t('How baselines are made')}
          </Link>
        </p>
      )}
    </section>
  )
}
