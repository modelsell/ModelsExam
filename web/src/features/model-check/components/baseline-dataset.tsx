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
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useBaselineLabels } from '../hooks/use-baseline-labels'
import {
  benchmarkAccuracy,
  performanceMetrics,
} from '../lib/baseline-comparison'
import type { ClaudeCheckReport } from '../types'

export function BaselineDataset(props: { report: ClaudeCheckReport }) {
  const { t } = useTranslation()
  const accuracy = benchmarkAccuracy(props.report.benchmark)
  const performance = performanceMetrics(props.report.samples)
  const names = useBaselineLabels()
  if (
    !props.report.fingerprint &&
    !props.report.benchmark &&
    !props.report.options?.performance
  )
    return null
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Baseline collection dataset')}</CardTitle>
        <CardDescription>
          {t(
            'Versioned synthetic questions with fixed parameters. Results describe this test set; they are not a general model ranking.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-4'>
        {props.report.options?.performance && (
          <div className='flex flex-wrap gap-4 text-sm tabular-nums'>
            <span>
              {t('Median TTFT')}:{' '}
              {performance.ttft === null
                ? '—'
                : `${performance.ttft.toFixed(0)} ms`}
            </span>
            <span>
              {t('End-to-end output rate')}:{' '}
              {performance.rate === null
                ? '—'
                : `${performance.rate.toFixed(1)} tok/s`}
            </span>
            <span>
              {t('Successful samples')}: {performance.count} / 3
            </span>
          </div>
        )}
        {props.report.benchmark && (
          <>
            <p className='text-sm'>
              {t('Capability accuracy')}:{' '}
              <span className='font-semibold tabular-nums'>
                {accuracy.score === null ? '—' : `${accuracy.score}/100`}
              </span>{' '}
              · {t('{{assessed}} / {{total}} questions assessed', accuracy)}
            </p>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Test question')}</TableHead>
                  <TableHead>{t('Reference answer')}</TableHead>
                  <TableHead>{t('Score')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {props.report.benchmark.items.map((item) => (
                  <TableRow key={item.id}>
                    <TableCell>{names[item.id] || item.id}</TableCell>
                    <TableCell className='font-mono text-xs'>
                      {item.expected}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {item.correct === null
                        ? '—'
                        : item.correct
                          ? '100/100'
                          : '0/100'}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </>
        )}
        {props.report.fingerprint && (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Fingerprint category')}</TableHead>
                <TableHead>{t('Valid / attempted')}</TableHead>
                <TableHead>{t('Observed distribution')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {props.report.fingerprint.cells.map((cell) => (
                <TableRow key={cell.id}>
                  <TableCell>{names[cell.id] || cell.id}</TableCell>
                  <TableCell className='tabular-nums'>
                    {cell.valid} / {cell.attempts}
                  </TableCell>
                  <TableCell className='text-xs'>
                    {Object.entries(cell.counts)
                      .sort((a, b) => b[1] - a[1])
                      .map(([answer, count]) => `${answer}: ${count}`)
                      .join(' · ') || '—'}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        <p className='text-muted-foreground text-xs'>
          {t(
            'Collection uses explicit disabled thinking, default sampling, and output caps of 64 tokens for choices and 512 tokens for capability questions. Changed parameters, truncated answers and transport errors are unscored.'
          )}
        </p>
      </CardContent>
    </Card>
  )
}
