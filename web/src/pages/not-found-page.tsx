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
import { Link } from '@/lib/router'

export function NotFoundPage() {
  const { t } = useTranslation()
  return (
    <div className='mx-auto flex max-w-2xl flex-col gap-4 px-4 py-24 sm:px-6'>
      <h1 className='font-serif text-3xl font-semibold'>{t('Page not found')}</h1>
      <p className='text-muted-foreground text-sm leading-6'>
        {t('This page does not exist. Check the address, or start from the home page.')}
      </p>
      <p>
        <Link to='/' className='text-sm underline underline-offset-4 hover:no-underline'>
          {t('Go to the home page')}
        </Link>
      </p>
    </div>
  )
}
