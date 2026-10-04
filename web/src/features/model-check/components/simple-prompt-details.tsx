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
import {
  Table,
  TableHeader,
  TableHead,
  TableBody,
  TableRow,
  TableCell,
} from '@/components/ui/table'
import { useTokenAuditLabels } from '../hooks/use-token-audit-labels'
import type {
  PromptAssessment,
  PromptInjectionReport,
  TokenComparison,
} from '../types'

const count = (value: number | null | undefined): string =>
  value == null ? '—' : value.toLocaleString()

export function SimplePromptDetails(props: {
  assessment: PromptAssessment
  injection?: PromptInjectionReport
  samples?: TokenComparison[]
  paper?: boolean
}) {
  const { t } = useTranslation()
  const labels = useTokenAuditLabels(6)
  const noteClass = props.paper ? 'mc-muted' : 'text-muted-foreground text-xs'
  const evidence = props.injection
  const sample = evidence?.sampling?.find((item) => item.id === 'minimal')
  const reference = evidence?.references.find(
    (item) => item.id === evidence.selected_reference_id
  )
  const pair = reference?.pairs[0]
  const difference = pair?.difference
  const delta =
    difference == null
      ? '—'
      : `${difference > 0 ? '+' : ''}${count(difference)}`
  const code = evidence?.code || 'simple_collecting'
  return (
    <div className='flex flex-col gap-2'>
      <p>
        <b>{labels.codes[code] || code}</b>
      </p>
      <p className={noteClass}>
        {t(
          'Repeat one minimal request three times without system text, tools or history.'
        )}{' '}
        <code>Return only the word PONG.</code>
      </p>
      <p className={noteClass}>
        {t(
          'Baseline median {{baseline}} · Measured median {{actual}} · Delta {{delta}}',
          {
            baseline: count(pair?.expected),
            actual: count(sample?.median),
            delta,
          }
        )}
      </p>
      {pair && (
        <p className={noteClass}>
          {t(
            'Allowed input range {{min}}–{{max}} · Tolerance ±{{tolerance}} tokens',
            {
              min: count(Math.max(0, pair.expected - pair.tolerance)),
              max: count(pair.expected + pair.tolerance),
              tolerance: count(pair.tolerance),
            }
          )}
        </p>
      )}
      <p className={noteClass}>
        {t('Valid samples {{valid}}/3 · Input range {{min}}–{{max}}', {
          valid: sample?.valid ?? props.assessment.token_measured,
          min: count(sample?.min),
          max: count(sample?.max),
        })}
      </p>
      {reference ? (
        <p className={noteClass}>
          {t('Scoring baseline')}: {reference.type} · {reference.report_id}
          {evidence?.baseline_samples != null && (
            <>
              {' '}
              ·{' '}
              {t('Baseline samples: {{count}}', {
                count: evidence.baseline_samples,
              })}
            </>
          )}
        </p>
      ) : (
        code === 'simple_no_reference' && (
          <p className={noteClass}>
            {t(
              'Run this check on a trusted native channel and manually save an Anthropic or AWS baseline on the same platform. A missing baseline earns no score.'
            )}
          </p>
        )
      )}
      {!!props.samples?.length && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Sample')}</TableHead>
              <TableHead>{t('Reported total input tokens')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {props.samples.map((item, index) => (
              <TableRow key={item.id}>
                <TableCell>{index + 1}/3</TableCell>
                <TableCell>
                  {count(item.actual)}
                  {item.actual == null && (
                    <> · {labels.codes[item.code] || item.code}</>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
      <p className={noteClass}>
        {t('Total input = uncached input + cache write + cache read.')}
      </p>
      {pair && (
        <p className={noteClass}>
          {t('The score averages all valid samples; outliers remain included.')}
        </p>
      )}
      <p className={noteClass}>
        {t(
          'The score measures input consistency with the latest compatible native baseline, not the closest match. Full marks do not prove that no prompt was added; usage can be rewritten.'
        )}
      </p>
    </div>
  )
}
