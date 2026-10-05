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
import { BadgeGenerator, BadgeProtection } from '@/features/model-check/components/badge-generator'

export function GetBadgePage() {
  const { t } = useTranslation()
  return (
    <div className='mx-auto flex max-w-3xl flex-col gap-10 px-4 py-12 sm:px-6 sm:py-16'>
      <header className='flex flex-col gap-3'>
        <h1 className='font-serif text-3xl font-semibold tracking-tight text-balance sm:text-4xl'>
          {t('Show your ModelsExam badge')}
        </h1>
        <p className='text-muted-foreground max-w-2xl text-sm leading-6'>
          {t(
            'Enter your domain and copy the code for your site. The badge shows the verdict of your latest check; the report itself stays private.'
          )}
        </p>
      </header>
      <BadgeGenerator />
      <BadgeProtection />
    </div>
  )
}
