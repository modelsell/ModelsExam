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
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'

// A snippet to copy. The code is shown as plain text so it can be read before
// it is pasted into a site.
export function CodeBlock(props: { id: string; title: string; hint: string; code: string }) {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  return (
    <div className='flex flex-col gap-2'>
      <div className='flex flex-wrap items-baseline justify-between gap-2'>
        <h3 id={props.id} className='text-base font-bold'>
          {props.title}
        </h3>
        <Button
          size='sm'
          variant='outline'
          onClick={() => {
            void copyToClipboard(props.code)
            toast.success(t('Copied'))
          }}
        >
          {t('Copy')}
        </Button>
      </div>
      <p className='text-muted-foreground text-sm leading-6'>{props.hint}</p>
      <pre
        aria-labelledby={props.id}
        className='bg-navy-deep overflow-x-auto rounded-md border border-white/10 p-4 font-mono text-xs leading-5 text-sky-100'
      >
        <code>{props.code}</code>
      </pre>
    </div>
  )
}
