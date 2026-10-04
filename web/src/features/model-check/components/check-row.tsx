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
import { useClaudeCheckLabels } from '../labels'
import { checkScore } from '../lib/report-score'
import type { ClaudeCheck, ClaudeCheckSample } from '../types'
import { ScoreBadge } from './score-badge'

const variants: Record<string, StatusVariant> = {
  pass: 'success',
  fail: 'warning',
  inconclusive: 'warning',
  skipped: 'neutral',
}

export function CheckRow(props: {
  id: string
  check?: ClaudeCheck
  running: boolean
  score?: number | null
  descriptionCode?: string
  kind?: string
  selected?: boolean
  samples: ClaudeCheckSample[]
}) {
  const { t } = useTranslation()
  const labels = useClaudeCheckLabels()
  const check = props.check
  let label = t('Pending')
  let variant: StatusVariant = 'neutral'
  let description = t('Waiting to run')
  if (props.selected === false) {
    label = t('Not selected')
    description = t('Not selected for this run.')
  }
  if (props.running) {
    label = t('Running')
    variant = 'info'
    description = t('Waiting for the upstream response…')
  }
  if (check) {
    label = labels.states[check.status]
    variant = variants[check.status]
    description = labels.codes[check.code] || check.code
  }
  if (props.descriptionCode)
    description = labels.codes[props.descriptionCode] || props.descriptionCode
  const error = props.samples.find((sample) => sample.error)?.error
  const displayScore =
    props.score !== undefined ? props.score : checkScore(check)

  const kinds: Record<string, string> = {
    assertion: t('Compatibility check'),
    observation: t('Observation'),
    boundary: t('Verification boundary'),
  }

  return (
    <div
      className={cn(
        'px-4 py-3 transition-colors',
        props.running && 'bg-info/5'
      )}
    >
      <div className='flex items-start justify-between gap-3'>
        <div className='flex flex-col gap-1'>
          <span className='text-sm font-medium'>
            {labels.names[props.id] || props.id}
          </span>
          <span className='text-muted-foreground text-xs'>
            {kinds[props.kind || 'assertion']}
          </span>
        </div>
        {props.score !== undefined || (check && props.kind !== 'boundary') ? (
          <ScoreBadge value={displayScore} />
        ) : (
          <StatusBadge
            variant={variant}
            pulse={props.running && !check}
            label={label}
            copyable={false}
          />
        )}
      </div>
      <p className='text-muted-foreground mt-1 text-xs leading-relaxed'>
        {description}
      </p>
      {check && (check.evidence || props.samples.length > 0) && (
        <details className='mt-2 text-xs'>
          <summary className='text-muted-foreground hover:text-foreground w-fit cursor-pointer'>
            {t('View evidence')}
            {error && (
              <span className='ms-2'>
                HTTP{' '}
                {props.samples.find((sample) => sample.error)?.http_status ||
                  '—'}
              </span>
            )}
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
