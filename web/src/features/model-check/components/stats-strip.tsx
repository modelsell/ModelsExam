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
import { fetchStats } from '../api'

// Live totals read from the database. Nothing is shown until the first check
// exists, so the numbers are never invented.
export function StatsStrip() {
  const { t } = useTranslation()
  const q = useQuery({ queryKey: ['mc-stats'], queryFn: fetchStats, staleTime: 60_000 })
  const s = q.data
  if (!s || s.checks === 0) return null
  const items = [
    { n: s.checks, label: t('Checks completed') },
    { n: s.sites, label: t('Sites covered') },
    { n: s.models, label: t('Models covered') },
  ]
  return (
    <dl className='mx-auto grid max-w-6xl grid-cols-3 gap-4 px-4 py-6 text-center sm:px-6'>
      {items.map((i) => (
        <div key={i.label}>
          <dd className='font-mono text-2xl font-extrabold tracking-tight sm:text-3xl'>{i.n.toLocaleString()}</dd>
          <dt className='text-muted-foreground text-xs sm:text-sm'>{i.label}</dt>
        </div>
      ))}
    </dl>
  )
}
