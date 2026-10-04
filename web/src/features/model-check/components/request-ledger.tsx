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
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useClaudeCheckLabels } from '../labels'
import type { ClaudeCheckSample } from '../types'

export function RequestLedger(props: {
  samples: ClaudeCheckSample[]
  activeProbe: string | null
}) {
  const { t } = useTranslation()
  const labels = useClaudeCheckLabels()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Request ledger')}</CardTitle>
        <CardDescription>
          {t(
            'Live request timings and reported token usage. A dash means unavailable.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='px-0'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className='ps-4'>{t('Request')}</TableHead>
              <TableHead>HTTP</TableHead>
              <TableHead>{t('Duration')}</TableHead>
              <TableHead>{t('First event')}</TableHead>
              <TableHead>TTFT</TableHead>
              <TableHead>{t('Input')}</TableHead>
              <TableHead>{t('Output')}</TableHead>
              <TableHead>{t('Cache write')}</TableHead>
              <TableHead className='pe-4'>{t('Cache read')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {props.samples.map((sample, index) => (
              <TableRow
                key={`${sample.probe}-${index}`}
                className='text-xs tabular-nums'
              >
                <TableCell className='ps-4 font-medium'>
                  {labels.names[sample.probe] || sample.probe}
                </TableCell>
                <TableCell>{sample.http_status || '—'}</TableCell>
                <TableCell>
                  {(sample.duration_ms / 1000).toFixed(2)} s
                </TableCell>
                <TableCell>
                  {sample.first_event_ms != null
                    ? `${sample.first_event_ms} ms`
                    : '—'}
                </TableCell>
                <TableCell>
                  {sample.stream?.first_text_ms != null
                    ? `${sample.stream.first_text_ms} ms`
                    : '—'}
                </TableCell>
                <TableCell>{sample.usage.input_tokens ?? '—'}</TableCell>
                <TableCell>{sample.usage.output_tokens ?? '—'}</TableCell>
                <TableCell>
                  {sample.usage.cache_creation_input_tokens ?? '—'}
                </TableCell>
                <TableCell className='pe-4'>
                  {sample.usage.cache_read_input_tokens ?? '—'}
                </TableCell>
              </TableRow>
            ))}
            {props.activeProbe &&
              !props.samples.some(
                (sample) => sample.probe === props.activeProbe
              ) && (
                <TableRow>
                  <TableCell colSpan={9} className='text-info px-4 text-xs'>
                    {labels.names[props.activeProbe]} ·{' '}
                    {t('Waiting for the upstream response…')}
                  </TableCell>
                </TableRow>
              )}
            {!props.samples.length && !props.activeProbe && (
              <TableRow>
                <TableCell
                  colSpan={9}
                  className='text-muted-foreground h-24 text-center text-sm'
                >
                  {t('Requests will appear here as the check runs.')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
