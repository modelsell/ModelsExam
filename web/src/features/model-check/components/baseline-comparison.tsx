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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
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
import { focusedComparisons } from '../lib/focused-assessment'
import type { ClaudeCheckReport } from '../types'

export function BaselineComparison({
  report,
  running,
}: {
  report: ClaudeCheckReport
  running: boolean
}) {
  const { t } = useTranslation()
  const names = useBaselineLabels()
  const [expanded, setExpanded] = useState(false)
  const rows = focusedComparisons(report)
  const points = (n: number | null) => (n === null ? '—' : `${n}/100`)
  const number = (n: number | null, digits = 0) =>
    n === null ? '—' : n.toFixed(digits)
  const performanceLabel = (score: number | null): string => {
    if (score === null) return t('Insufficient samples')
    return score === 100 ? t('Meets standard') : t('Below standard')
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Baseline differences')}</CardTitle>
        <CardDescription>
          {t(
            'Only identical questions and parameters are compared. References are frozen when the check starts.'
          )}
          {running && ` ${t('Collecting comparison evidence')}`}
        </CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-4'>
        {!rows.length ? (
          <p className='text-muted-foreground text-sm'>
            {t(
              'No baseline was available when this run started. Review a completed report and set it as a baseline for future checks.'
            )}
          </p>
        ) : (
          <>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Baseline type / model')}</TableHead>
                  <TableHead>{t('Distribution similarity')}</TableHead>
                  <TableHead>{t('Capability: current / baseline')}</TableHead>
                  <TableHead>{t('Performance standard')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(expanded ? rows : rows.slice(0, 5)).map((row) => (
                  <TableRow key={row.baseline.id}>
                    <TableCell className='max-w-60 min-w-40 align-top whitespace-normal'>
                      <p className='font-medium'>{row.baseline.type}</p>
                      <p className='text-xs break-all'>{row.baseline.model}</p>
                      <a
                        className='text-muted-foreground text-xs underline underline-offset-4'
                        href={`/?history_id=${encodeURIComponent(row.baseline.report_id)}`}
                      >
                        {t('Source report')} ·{' '}
                        {new Date(row.baseline.started_at).toLocaleDateString()}
                      </a>
                    </TableCell>
                    <TableCell className='min-w-44 align-top whitespace-normal'>
                      <p className='font-semibold tabular-nums'>
                        {points(row.fingerprint.score)}
                      </p>
                      <p className='text-muted-foreground text-xs'>
                        {t('{{count}} / 2 categories comparable', {
                          count: row.fingerprint.count,
                        })}
                      </p>
                      {row.fingerprint.cells.map((cell) => (
                        <p key={cell.id} className='mt-1 text-xs'>
                          {names[cell.id] || cell.id}:{' '}
                          {cell.score === null
                            ? t('Not scored')
                            : points(Math.round(cell.score))}
                          {cell.reason === 'samples' && (
                            <span className='text-muted-foreground'>
                              {' '}
                              ·{' '}
                              {t(
                                'Valid samples {{current}} / {{reference}}; need 10 each',
                                cell
                              )}
                            </span>
                          )}
                          {cell.reason === 'missing' && (
                            <span className='text-muted-foreground'>
                              {' '}
                              · {t('Not collected')}
                            </span>
                          )}
                          {cell.reason === 'profile' && (
                            <span className='text-muted-foreground'>
                              {' '}
                              · {t('Question or parameters differ')}
                            </span>
                          )}
                        </p>
                      ))}
                      {!row.fingerprint.complete &&
                        row.fingerprint.count > 0 && (
                          <p className='text-muted-foreground mt-1 text-xs'>
                            {t('Partial similarity; excluded from ranking')}
                          </p>
                        )}
                    </TableCell>
                    <TableCell className='min-w-44 align-top whitespace-normal'>
                      <p className='font-semibold tabular-nums'>
                        {points(row.benchmark.current)} /{' '}
                        {points(row.benchmark.reference)}
                      </p>
                      <p className='text-muted-foreground text-xs'>
                        {t('{{count}} questions comparable', {
                          count: row.benchmark.count,
                        })}
                      </p>
                      {row.benchmark.differences.map((q) => (
                        <p className='mt-1 text-xs' key={q.id}>
                          {names[q.id] || q.id}: {q.current} / {q.reference}
                        </p>
                      ))}
                      {row.benchmark.count > 0 &&
                        !row.benchmark.differences.length && (
                          <p className='text-muted-foreground mt-1 text-xs'>
                            {t('No differences in comparable answers')}
                          </p>
                        )}
                    </TableCell>
                    <TableCell className='min-w-48 align-top whitespace-normal'>
                      <p className='font-semibold tabular-nums'>
                        {points(row.performance.score)} ·{' '}
                        {performanceLabel(row.performance.score)}
                      </p>
                      <p className='text-muted-foreground text-xs'>
                        {t('{{count}} / 3 paired performance samples', {
                          count: row.performance.count,
                        })}
                      </p>
                      {row.performance.score !== null && (
                        <div className='mt-1 text-xs tabular-nums'>
                          <p>
                            {t('Median TTFT')}:{' '}
                            {number(row.performance.current.ttft)} ms ·{' '}
                            {t('Limit')} ≤ {number(row.performance.ttftLimit)}{' '}
                            ms
                          </p>
                          <p>
                            {t('Output rate')}:{' '}
                            {number(row.performance.current.rate, 1)} tok/s ·{' '}
                            {t('Limit')} ≥{' '}
                            {number(row.performance.rateLimit, 1)} tok/s
                          </p>
                        </div>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {rows.length > 5 && (
              <Button
                variant='ghost'
                size='sm'
                onClick={() => setExpanded(!expanded)}
              >
                {expanded ? t('Show fewer baselines') : t('Show all baselines')}
              </Button>
            )}
          </>
        )}
        <p className='text-muted-foreground text-xs'>
          {t(
            'Similarity describes two small answer distributions, not an identity probability. Baseline types are operator labels; a high score cannot prove the underlying model.'
          )}
        </p>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Performance scores 50 points per met threshold: median TTFT and end-to-end output rate. Tolerance: {{value}}%. At least 3 paired samples are required.',
            { value: report.options?.performance_tolerance ?? 25 }
          )}
        </p>
        {report.version < 9 && (
          <p className='text-muted-foreground text-xs'>
            {t(
              'Historical view uses the common questions and a 25% performance tolerance. Original results and reference snapshots are unchanged.'
            )}
          </p>
        )}
      </CardContent>
    </Card>
  )
}
