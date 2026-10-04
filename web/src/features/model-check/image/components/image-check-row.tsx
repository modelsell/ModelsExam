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
import { cn } from '@/lib/utils'
import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { useImageLabels } from '../hooks/use-image-labels'
import type { ImageCheck, ImageCheckKind, ImageSample } from '../types'

const VARIANTS: Record<ImageCheck['status'], StatusVariant> = {
  pass: 'success',
  fail: 'danger',
  inconclusive: 'warning',
  skipped: 'neutral',
}

export function ImageCheckRow(props: {
  id: string
  kind: ImageCheckKind
  selected: boolean
  check?: ImageCheck
  running: boolean
  samples: ImageSample[]
}) {
  const { t } = useTranslation()
  const labels = useImageLabels()
  const check = props.check
  let label = t('Pending')
  let variant: StatusVariant = 'neutral'
  let note = labels.detail(props.id)
  if (!props.selected && !check) {
    label = t('Not selected')
    note = t('Not selected for this run.')
  }
  if (props.running) {
    label = t('Running')
    variant = 'info'
  }
  if (check) {
    label = labels.states[check.status]
    variant = VARIANTS[check.status]
  }
  const reason = check && check.code ? labels.code(check.code) : ''
  const error = props.samples.find((sample) => sample.error)?.error
  const kindLabel = {
    assertion: t('Compatibility check'),
    observation: t('Observation'),
    provenance: t('Provenance (not scored)'),
  }[props.kind]
  return (
    <div
      className={cn(
        'px-4 py-3 transition-colors',
        props.running && 'bg-info/5'
      )}
    >
      <div className='flex items-start justify-between gap-3'>
        <div className='flex min-w-0 flex-col gap-1'>
          <span className='text-sm font-medium'>{labels.title(props.id)}</span>
          <span className='text-muted-foreground text-xs'>{kindLabel}</span>
        </div>
        <StatusBadge
          variant={variant}
          pulse={props.running}
          label={label}
          copyable={false}
        />
      </div>
      {note && (
        <p className='text-muted-foreground mt-1 text-xs leading-relaxed'>
          {note}
        </p>
      )}
      {reason && check?.status !== 'pass' && (
        <p className='mt-1 text-xs leading-relaxed font-medium'>{reason}</p>
      )}
      {check && (check.evidence || props.samples.length > 0) && (
        <details className='mt-2 text-xs'>
          <summary className='text-muted-foreground hover:text-foreground w-fit cursor-pointer'>
            {t('View evidence')}
          </summary>
          {error && <p className='mt-2 break-words'>{error}</p>}
          <pre className='bg-muted/50 mt-2 max-h-64 overflow-auto rounded-md border p-3 text-xs leading-relaxed break-all whitespace-pre-wrap'>
            {JSON.stringify(
              { evidence: check.evidence, requests: props.samples },
              null,
              2
            )}
          </pre>
        </details>
      )}
    </div>
  )
}
