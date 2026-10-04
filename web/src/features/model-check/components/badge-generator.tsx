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

import { useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { CodeBlock } from '@/components/code-block'
import {
  type BadgeTheme,
  imageSnippet,
  markdownSnippet,
  normalizeDomain,
  scriptSnippet,
  slotSnippet,
} from '@/lib/embed'
import { getBadge } from '../api'
import { BadgeStatus } from './badge-status'

// Domain in, embed code out. The badge status shows what the code will display
// right now, so a site sees "not checked yet" before it pastes anything.
export function BadgeGenerator() {
  const { t } = useTranslation()
  const [input, setInput] = useState('')
  const [domain, setDomain] = useState<string>()
  const [invalid, setInvalid] = useState(false)
  const [theme, setTheme] = useState<BadgeTheme>('auto')
  const origin = window.location.origin
  const status = useQuery({
    queryKey: ['model-check-badge', domain],
    queryFn: () => getBadge(domain!),
    enabled: !!domain,
    retry: false,
  })

  const submit = (event: FormEvent) => {
    event.preventDefault()
    const next = normalizeDomain(input)
    setInvalid(!next)
    setDomain(next ?? undefined)
  }

  return (
    <div className='flex flex-col gap-8'>
      <form onSubmit={submit} className='flex flex-col gap-2' noValidate>
        <label htmlFor='badge-domain' className='text-sm font-medium'>
          {t('Your domain')}
        </label>
        <div className='flex flex-wrap gap-3'>
          <Input
            id='badge-domain'
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder='relay.example.com'
            inputMode='url'
            autoComplete='off'
            aria-invalid={invalid}
            aria-describedby='badge-domain-help'
            className='min-w-0 flex-1 basis-56'
          />
          <Button type='submit'>{t('Get the code')}</Button>
        </div>
        <p
          id='badge-domain-help'
          className={invalid ? 'text-destructive text-sm' : 'text-muted-foreground text-xs'}
        >
          {invalid
            ? t('Enter a public domain such as relay.example.com. IP addresses are not supported.')
            : t('A URL also works; only the domain is used.')}
        </p>
      </form>

      {domain && (
        <>
          <section
            aria-labelledby='badge-status-title'
            className='bg-card flex flex-col gap-4 rounded-lg border p-5'
          >
            <h3 id='badge-status-title' className='font-serif text-lg font-semibold break-all'>
              {domain}
            </h3>
            {status.isPending && <Skeleton className='h-16' />}
            {status.isError && (
              <p className='text-destructive text-sm'>{t('Could not load the badge. Try again.')}</p>
            )}
            {status.data && <BadgeStatus data={status.data} domain={domain} />}
          </section>

          <section aria-labelledby='badge-code-title' className='flex flex-col gap-8'>
            <div className='flex flex-col gap-2'>
              <label htmlFor='badge-theme' className='text-sm font-medium'>
                {t('Badge theme')}
              </label>
              <select
                id='badge-theme'
                value={theme}
                onChange={(e) => setTheme(e.target.value as BadgeTheme)}
                className='border-input bg-background w-fit rounded-md border px-2.5 py-1.5 text-sm'
              >
                <option value='auto'>{t('Match the visitor (auto)')}</option>
                <option value='light'>{t('Light')}</option>
                <option value='dark'>{t('Dark')}</option>
              </select>
            </div>
            <h3 id='badge-code-title' className='font-serif text-xl font-semibold'>
              {t('Code for {{domain}}', { domain })}
            </h3>
            <CodeBlock
              id='code-script'
              title={t('Script (recommended)')}
              hint={t(
                'Paste it where your site allows scripts, such as a custom footer, theme or template. The badge floats in the bottom-right corner, or sits where you place the optional element below.'
              )}
              code={scriptSnippet(origin, domain, theme)}
            />
            <CodeBlock
              id='code-slot'
              title={t('Place it yourself (optional)')}
              hint={t('Add this element where the badge should appear. Without it the badge floats in the corner.')}
              code={slotSnippet()}
            />
            <CodeBlock
              id='code-image'
              title={t('Image badge')}
              hint={t(
                'Some pages render HTML but ignore scripts. This image works wherever HTML does, and it updates itself.'
              )}
              code={imageSnippet(origin, domain, theme)}
            />
            <CodeBlock
              id='code-markdown'
              title='Markdown'
              hint={t('For a README, announcement or any page that accepts Markdown.')}
              code={markdownSnippet(origin, domain, theme)}
            />
          </section>
        </>
      )}
    </div>
  )
}

export function BadgeProtection() {
  const { t } = useTranslation()
  return (
    <section aria-labelledby='badge-rules-title' className='flex flex-col gap-3'>
      <h3 id='badge-rules-title' className='font-serif text-xl font-semibold'>
        {t('How the badge is protected')}
      </h3>
      <ul className='flex list-disc flex-col gap-2 pl-5 text-sm leading-6'>
        <li>{t('It is built from the newest completed check whose endpoint is on your domain or a subdomain.')}</li>
        <li>{t('The script and the data are served only to pages on your domain, so another site cannot load your badge.')}</li>
        <li>{t('It expires 30 days after the check and then reads “Check expired” until you check again.')}</li>
        <li>{t('It always links to the public report. Nobody, including you, can edit the result.')}</li>
      </ul>
      <p className='text-muted-foreground text-xs leading-5'>
        {t('A copied screenshot cannot be prevented; the visible domain and the report link are how a visitor checks it.')}
      </p>
    </section>
  )
}
