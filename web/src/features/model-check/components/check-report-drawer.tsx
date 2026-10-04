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
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from '@/components/ui/sheet'

export function CheckReportDrawer(props: {
  open: boolean
  onClose: () => void
  children: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <Sheet
      open={props.open}
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
    >
      <SheetContent className='w-full gap-0 sm:max-w-6xl'>
        <SheetHeader className='border-b px-4 py-4 sm:px-6'>
          <SheetTitle>{t('Model check report')}</SheetTitle>
          <SheetDescription>
            {t('View results and download your report.')}
          </SheetDescription>
        </SheetHeader>
        <div className='flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4 sm:p-6'>
          {props.children}
        </div>
      </SheetContent>
    </Sheet>
  )
}
