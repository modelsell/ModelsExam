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
import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { useImageLabels } from '../hooks/use-image-labels'
import type { ProvenanceSummary } from '../types'

const VARIANTS: Record<string, StatusVariant> = {
  trusted: 'success',
  synthid: 'success',
  untrusted: 'warning',
  none: 'neutral',
  unavailable: 'warning',
  off: 'neutral',
}

// Provenance is shown beside the score, never inside it: OpenAI Verify only
// recognises OpenAI signals, so "nothing detected" is not evidence of anything.
export function ImageProvenanceCard(props: { provenance?: ProvenanceSummary }) {
  const { t } = useTranslation()
  const labels = useImageLabels()
  const p = props.provenance
  if (!p?.enabled) return null
  const v = p.verdict
  return (
    <div className='flex flex-col gap-3 rounded-xl border p-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <span className='text-sm font-medium'>
          {t('Provenance (OpenAI Verify)')}
        </span>
        <StatusBadge
          variant={VARIANTS[p.level] ?? 'neutral'}
          label={labels.level(p.level)}
          copyable={false}
        />
      </div>
      <p className='text-muted-foreground text-xs leading-relaxed'>
        {labels.levelNote(p.level)}
        {p.unavailable_code ? ` · ${labels.code(p.unavailable_code)}` : ''}
      </p>
      {p.control_ok === false && (
        <p className='text-xs font-medium'>
          {t('The negative control failed, so this verifier run is unreliable.')}
        </p>
      )}
      {v && (
        <dl className='grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs'>
          <dt className='text-muted-foreground'>C2PA</dt>
          <dd>{v.c2pa_state}</dd>
          {v.issuer && (
            <>
              <dt className='text-muted-foreground'>{t('Issuer')}</dt>
              <dd className='break-all'>{v.issuer}</dd>
            </>
          )}
          {v.model && (
            <>
              <dt className='text-muted-foreground'>{t('Model')}</dt>
              <dd className='break-all'>
                {v.model}
                {v.model_match === true && ` · ${t('matches')}`}
                {v.model_match === false && ` · ${t('does not match')}`}
              </dd>
            </>
          )}
          <dt className='text-muted-foreground'>SynthID</dt>
          <dd>{v.synthid ? t('Detected') : t('Not detected')}</dd>
        </dl>
      )}
      {p.baseline && (
        <p className='text-xs'>
          {t('Official baseline')}: {labels.level(p.baseline.level)}
        </p>
      )}
      {p.formats && Object.keys(p.formats).length > 0 && (
        <p className='text-xs'>
          {t('Across formats')}:{' '}
          {Object.entries(p.formats)
            .map(([format, level]) => `${format} ${labels.level(level)}`)
            .join(' · ')}
        </p>
      )}
    </div>
  )
}
