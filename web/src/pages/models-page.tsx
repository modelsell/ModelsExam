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
import { BoardGrid } from '@/features/model-check/components/board-table'
import { Link } from '@/lib/router'

// Per-model boards: the newest scored check of each site. Results with dates,
// not endorsements.
export function ModelsPage() {
  const { t } = useTranslation()
  return (
    <div className='mx-auto flex max-w-6xl flex-col gap-8 px-4 py-12 sm:px-6 sm:py-16'>
      <header className='flex max-w-3xl flex-col gap-3'>
        <h1 className='text-3xl font-extrabold tracking-tight sm:text-4xl'>{t('Model check boards')}</h1>
        <p className='text-muted-foreground leading-7'>
          {t(
            'One board per model: the newest completed check of each site, best score first. A board lists dated results, not recommendations. Results older than 30 days are marked expired.'
          )}
        </p>
      </header>
      <BoardGrid limit={50} size={10} />
      <p className='text-sm'>
        <Link to='/#new-check' className='underline underline-offset-4 hover:no-underline'>
          {t('Check an endpoint yourself')}
        </Link>
      </p>
    </div>
  )
}
