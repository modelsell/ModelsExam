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
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Link } from '@/lib/router'
import { modelPath, reportPath } from '@/lib/route'
import { fetchBoard, fetchBoards, type BoardData, type BoardEntry } from '../api'

const TIER_STYLE: Record<BoardEntry['tier'], string> = {
  conformant: 'border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
  mostly: 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300',
  review: 'border-rose-500/40 bg-rose-500/10 text-rose-700 dark:text-rose-300',
}

function useTierLabel() {
  const { t } = useTranslation()
  return (tier: BoardEntry['tier']) =>
    tier === 'conformant' ? t('Conformant') : tier === 'mostly' ? t('Mostly conformant') : t('Review suggested')
}

function useSourceName() {
  const { t } = useTranslation()
  return (s: BoardEntry['source']) => (s === 'official' ? t('Official') : s === 'aws' ? t('AWS Bedrock') : t('Other'))
}

export const dayOf = (ms: number) => new Date(ms).toISOString().slice(0, 10)

// One model's board: the newest scored check of each site, best score first.
// It lists results; it is not a recommendation.
export function BoardTable({ board, className }: { board: BoardData; className?: string }) {
  const { t } = useTranslation()
  const tierLabel = useTierLabel()
  const sourceName = useSourceName()
  const path = modelPath(board.model)
  return (
    <section className={cn('bg-card flex flex-col gap-3 rounded-lg border p-4', className)}>
      <div className='flex items-baseline justify-between gap-3'>
        <h3 className='font-semibold break-all'>
          {path ? (
            <Link to={path} className='hover:underline'>
              {board.model}
            </Link>
          ) : (
            board.model
          )}
        </h3>
        <span className='text-muted-foreground text-xs whitespace-nowrap'>
          {t('{{count}} checks', { count: board.checks })}
        </span>
      </div>
      {board.entries.length === 0 ? (
        <p className='text-muted-foreground text-sm'>{t('No results yet.')}</p>
      ) : (
        <ol className='flex flex-col divide-y text-sm'>
          {board.entries.map((e) => (
            <li key={e.report_id} className='flex items-center gap-3 py-2'>
              <span className='text-muted-foreground w-5 font-mono text-xs'>{e.rank}</span>
              <div className='min-w-0 flex-1'>
                <Link to={`/sites/${e.host}`} className='block truncate font-medium hover:underline'>
                  {e.site || e.host}
                </Link>
                <span className='text-muted-foreground block truncate text-xs'>
                  {sourceName(e.source)} · {dayOf(e.checked_at)}
                  {!e.fresh && ` · ${t('Expired')}`}
                </span>
              </div>
              <Link
                to={reportPath(e.report_id)}
                title={tierLabel(e.tier)}
                className={cn(
                  'rounded border px-2 py-0.5 font-mono text-xs whitespace-nowrap',
                  TIER_STYLE[e.tier],
                  !e.fresh && 'opacity-60'
                )}
              >
                {e.score}
              </Link>
            </li>
          ))}
        </ol>
      )}
    </section>
  )
}

export function BoardGrid(props: { limit: number; size: number; showAll?: boolean }) {
  const { t } = useTranslation()
  const q = useQuery({ queryKey: ['mc-boards', props.limit, props.size], queryFn: () => fetchBoards(props.limit, props.size) })
  if (q.isError) return null
  if (q.data && q.data.length === 0) return <p className='text-muted-foreground text-sm'>{t('No results yet.')}</p>
  return (
    <div className='grid gap-4 md:grid-cols-2 lg:grid-cols-3'>
      {(q.data ?? []).map((b) => (
        <BoardTable key={b.model} board={b} />
      ))}
    </div>
  )
}

export function ModelBoard({ model }: { model: string }) {
  const q = useQuery({ queryKey: ['mc-board', model], queryFn: () => fetchBoard(model, 10) })
  if (!q.data || q.data.entries.length === 0) return null
  return <BoardTable board={q.data} />
}
