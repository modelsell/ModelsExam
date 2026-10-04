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
  CardFooter,
} from '@/components/ui/card'
import {
  Table,
  TableHeader,
  TableHead,
  TableBody,
  TableRow,
  TableCell,
} from '@/components/ui/table'
import { useTokenAuditLabels } from '../hooks/use-token-audit-labels'
import { useClaudeCheckLabels } from '../labels'
import type { ClaudeCheckReport, TokenComparison } from '../types'
import { CacheAssessmentDetails } from './cache-assessment-details'
import { PromptIntegrityDetails } from './prompt-integrity-details'
import { ScoreBadge } from './score-badge'

const count = (value: number | null | undefined): string =>
  value == null ? '—' : value.toLocaleString()

function TokenAuditTable(props: {
  items: TokenComparison[]
  cache: boolean
  running: boolean
  version: number
}) {
  const { t } = useTranslation()
  const labels = useClaudeCheckLabels()
  const audit = useTokenAuditLabels(props.version)
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('Test question')}</TableHead>
          <TableHead>
            {props.cache
              ? t('First cache write tokens')
              : t('Estimated request tokens')}
          </TableHead>
          <TableHead>
            {props.cache
              ? t('Reported cache read tokens')
              : t('Reported total input tokens')}
          </TableHead>
          <TableHead>{t('Token delta')}</TableHead>
          <TableHead>
            {!props.cache && props.version >= 3
              ? t('Same-endpoint count score')
              : t('Consistency score')}
          </TableHead>
          {!props.cache && props.version >= 2 && (
            <TableHead>{t('Instruction preservation')}</TableHead>
          )}
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.items.map((item) => {
          let code = item.code
          if (code === 'pending' && !props.running) code = 'not_collected'
          const delta = item.difference
          return (
            <TableRow key={item.id}>
              <TableCell className='max-w-md whitespace-normal'>
                <div className='flex flex-col gap-1'>
                  <span>{audit.names[item.id] || item.id}</span>
                  {audit.questions[item.id] && (
                    <p className='text-muted-foreground text-xs'>
                      {audit.questions[item.id]}
                    </p>
                  )}
                  <p className='text-muted-foreground text-xs'>
                    {audit.codes[code] || labels.codes[code] || code}
                  </p>
                  {item.tolerance != null && (
                    <p className='text-muted-foreground text-xs'>
                      {t('Token tolerance: ±{{count}}', {
                        count: item.tolerance,
                      })}
                    </p>
                  )}
                  {item.request_changed && (
                    <p className='text-muted-foreground text-xs'>
                      {t(
                        'Channel settings changed the request. The count uses the effective request.'
                      )}
                    </p>
                  )}
                  {props.cache && (
                    <p className='text-muted-foreground text-xs'>
                      {t('Additional cache write: {{count}} tokens', {
                        count: count(item.repeated_write),
                      })}
                    </p>
                  )}
                </div>
              </TableCell>
              <TableCell className='tabular-nums'>
                {count(item.expected)}
              </TableCell>
              <TableCell className='tabular-nums'>
                {count(item.actual)}
              </TableCell>
              <TableCell className='tabular-nums'>
                {delta != null && delta > 0 ? '+' : ''}
                {count(delta)}
              </TableCell>
              <TableCell>
                <ScoreBadge value={item.score} />
              </TableCell>
              {!props.cache && props.version >= 2 && (
                <TableCell>
                  <ScoreBadge value={item.behavior?.score ?? null} />
                  <p className='text-muted-foreground text-xs'>
                    {item.behavior
                      ? audit.codes[item.behavior.code]
                      : t('Not scored')}
                  </p>
                </TableCell>
              )}
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}

export function TokenAudit(props: {
  report: ClaudeCheckReport
  running: boolean
}) {
  const { t } = useTranslation()
  const audit = props.report.token_audit
  if (!audit) return null
  let promptTitle = t('Prompt injection token check')
  if (audit.version >= 2) promptTitle = t('Prompt integrity evidence')
  if (audit.prompt_assessment?.scoring === 'measured_checks')
    promptTitle = t('Prompt consistency score')
  const scoring = audit.prompt_assessment?.scoring
  const minimalPrompt =
    scoring === 'input_consistency' || scoring === 'input_budget'
  if (scoring === 'input_consistency')
    promptTitle = t('Input consistency score')
  if (scoring === 'input_budget') promptTitle = t('Prompt injection score')
  let description = t(
    'Score = smaller count / larger count × 100. Only exact equality receives 100; missing evidence is unscored.'
  )
  if (audit.version >= 2)
    description = t(
      'Cache scores require exact equality for 100. Prompt token scores allow a small estimate tolerance; raw differences remain visible.'
    )
  if (scoring === 'input_consistency')
    description = t(
      'Compare repeated total input with a trusted baseline and compare cache reads with writes.'
    )
  if (scoring === 'input_budget')
    description = t(
      'Inspect minimal input overhead and compare cache reads with writes.'
    )
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Prompt and cache token audit')}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-6'>
        {!!audit.prompt.length && (
          <section className='flex flex-col gap-2'>
            <h3 className='text-sm font-medium'>{promptTitle}</h3>
            {!minimalPrompt && (
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Total input = uncached input + cache write + cache read. Count tokens against the same effective request.'
                )}
              </p>
            )}
            {audit.prompt_assessment && (
              <>
                <ScoreBadge value={audit.prompt_assessment.score} />
                <PromptIntegrityDetails
                  assessment={audit.prompt_assessment}
                  injection={audit.injection}
                  samples={audit.prompt}
                />
              </>
            )}
            {!minimalPrompt && (
              <TokenAuditTable
                version={audit.version}
                items={audit.prompt}
                cache={false}
                running={props.running}
              />
            )}
          </section>
        )}
        {!!audit.cache.length && (
          <section className='flex flex-col gap-2'>
            <h3 className='text-sm font-medium'>
              {t('Cache write/read token consistency')}
            </h3>
            <p className='text-muted-foreground text-xs'>
              {t(
                audit.version >= 4
                  ? 'Provide 384 synthetic records per round. Use two fresh prefixes and read each prefix three times.'
                  : 'Provide 384 synthetic records and ask for the first word in Record 73. Send the identical prefix and question three times.'
              )}
            </p>
            {audit.cache_assessment && (
              <>
                <ScoreBadge value={audit.cache_assessment.score} />
                <CacheAssessmentDetails assessment={audit.cache_assessment} />
              </>
            )}
            <TokenAuditTable
              version={audit.version}
              items={audit.cache}
              cache
              running={props.running}
            />
          </section>
        )}
      </CardContent>
      <CardFooter className='flex flex-col items-start gap-2'>
        {!minimalPrompt && (
          <p className='text-muted-foreground text-xs'>
            {t(
              'Count estimates can differ slightly. Extra input is a clue, not proof of injection; equal counts cannot exclude changes applied to both endpoints.'
            )}
          </p>
        )}
        <p className='text-muted-foreground text-xs'>
          {t(
            'Cache differences can reflect misses, rewriting or routing. This checks reported counters, not latency or independently verified billing.'
          )}
        </p>
      </CardFooter>
    </Card>
  )
}
