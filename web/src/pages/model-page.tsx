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
import { ModelBoard } from '@/features/model-check/components/board-table'
import { CheckHistory } from '@/features/model-check/components/check-history'

// All public checks of one model name. The server renders the same list for
// search engines at /models/<name>.
export function ModelPage(props: { model: string }) {
  const { t } = useTranslation()
  return (
    <div className='mx-auto flex max-w-6xl flex-col gap-8 px-4 py-12 sm:px-6 sm:py-16'>
      <header className='flex max-w-3xl flex-col gap-3'>
        <h1 className='text-3xl font-extrabold tracking-tight break-all sm:text-4xl'>
          {props.model} {t('model check results')}
        </h1>
        <p className='text-muted-foreground leading-7'>
          {t(
            'Public check records for this model name, across relays and official endpoints. Each record is one check of one endpoint at one moment; it is not a ranking.'
          )}
        </p>
      </header>
      <ModelBoard model={props.model} />
      <CheckHistory initialModel={props.model} />
    </div>
  )
}
