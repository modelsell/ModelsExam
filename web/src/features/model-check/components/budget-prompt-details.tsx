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

export function BudgetPromptDetails(props: {
  assessment: PromptAssessment
  injection?: PromptInjectionReport
  samples?: TokenComparison[]
  paper?: boolean
}) {
  const { t } = useTranslation()
  const labels = useTokenAuditLabels(7)
  const noteClass = props.paper ? 'mc-muted' : 'text-muted-foreground text-xs'
  const evidence = props.injection
  const sample = evidence?.sampling?.find((item) => item.id === 'minimal')
  const code = evidence?.code || 'budget_collecting'
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
          'Input allowance {{allowance}} tokens · Measured median {{median}}',
          {
            allowance: count(evidence?.allowance_tokens),
            median: count(sample?.median),
          }
        )}
      </p>
      <p className={noteClass}>
        {t('Average excess {{average}} · Maximum excess {{maximum}} tokens', {
          average: count(evidence?.estimated_extra_tokens),
          maximum: count(evidence?.max_extra_tokens),
        })}
      </p>
      <p className={noteClass}>
        {t(
          'Valid samples {{valid}}/3 · Above allowance {{extra}} · Coverage {{coverage}}%',
          {
            valid: sample?.valid ?? props.assessment.token_measured,
            extra: evidence?.extra_rounds ?? 0,
            coverage: props.assessment.coverage,
          }
        )}
      </p>
      {!!props.samples?.length && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Sample')}</TableHead>
              <TableHead>{t('Total input tokens')}</TableHead>
              <TableHead>{t('Excess above allowance')}</TableHead>
              <TableHead>{t('Score')}</TableHead>
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
                <TableCell>{count(item.difference)}</TableCell>
                <TableCell>
                  {item.score == null ? '—' : `${item.score}/100`}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
      <p className={noteClass}>
        {t('Total input = uncached input + cache write + cache read.')}
      </p>
      <p className={noteClass}>
        {t(
          'Per sample: input ≤ {{allowance}} earns 100; otherwise min(99, round(100 × {{allowance}} / input)). Final score: rounded average of valid scores; capped at 99 if any sample exceeds the allowance.',
          { allowance: count(evidence?.allowance_tokens) }
        )}
      </p>
      <p className={noteClass}>
        {t(
          'The allowance is an empirical margin, not an official token limit. Excess is a clue to added context, not an exact count of injected tokens.'
        )}
      </p>
      <p className={noteClass}>
        {t(
          '100 means no obvious extra input was observed; small additions or forged usage may go undetected.'
        )}
      </p>
    </div>
  )
}
