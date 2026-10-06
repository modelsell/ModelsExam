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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CodeBlock } from '@/components/code-block'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  NativeSelect,
  NativeSelectOption,
} from '@/components/ui/native-select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Link } from '@/lib/router'
import {
  buildPrefillLink,
  type PrefillProvider,
} from '@/features/model-check/lib/link-prefill'

const origin = () =>
  typeof window === 'undefined' ? 'https://modelsexam.com' : window.location.origin

function snippet(site: string): string {
  return `// Build the link on your console page, where the user's key is known.
function modelsExamLink({ type = 'claude', baseUrl, key, model }) {
  const params = new URLSearchParams({ base_url: baseUrl, key, model })
  if (type !== 'claude') params.set('type', type)
  return '${site}/#' + params.toString()
}

// <a href={modelsExamLink({ baseUrl: 'https://api.your-relay.com', key, model: 'claude-sonnet-4-5' })}
//    target="_blank" rel="noopener noreferrer">Check this key on ModelsExam</a>`
}

export function IntegratePage() {
  const { t } = useTranslation()
  const site = origin()
  const [provider, setProvider] = useState<PrefillProvider>('claude')
  const [baseUrl, setBaseUrl] = useState('https://api.your-relay.com')
  const [key, setKey] = useState('')
  const [model, setModel] = useState('claude-sonnet-4-5')
  const link = buildPrefillLink(site, {
    provider,
    base_url: baseUrl,
    key: key || 'sk-your-key',
    model,
  })
  const params: Array<[string, string, string]> = [
    ['type', 'claude / openai / gemini / image', t('Which check opens. Optional, Claude by default.')],
    ['base_url', 'https://api.your-relay.com', t('Your API address. A /v1 suffix is accepted.')],
    ['key', 'sk-…', t('The user’s API key. Shown masked on the page.')],
    ['model', 'claude-sonnet-4-5', t('Model to check. The user can still pick another.')],
  ]
  return (
    <div className='mx-auto flex max-w-3xl flex-col gap-12 px-4 py-12 sm:px-6 sm:py-16'>
      <header className='flex flex-col gap-3'>
        <h1 className='font-serif text-3xl font-semibold tracking-tight text-balance sm:text-4xl'>
          {t('Relay integration')}
        </h1>
        <p className='text-muted-foreground max-w-2xl text-sm leading-6'>
          {t(
            'Run an API relay? Add a “Check on ModelsExam” link to your console. It opens the check form with the Base URL, key and model already filled in. Your user reviews the details and starts the check themselves.'
          )}
        </p>
      </header>

      <section aria-labelledby='integrate-steps' className='flex flex-col gap-4'>
        <h2 id='integrate-steps' className='text-xl font-bold tracking-tight'>
          {t('How it works')}
        </h2>
        <ol className='text-muted-foreground flex list-decimal flex-col gap-2 pl-5 text-sm leading-6'>
          <li>{t('Your console builds a link with the user’s Base URL, key and model.')}</li>
          <li>{t('The user opens it and lands on the ModelsExam check form, already filled in. The key is shown masked.')}</li>
          <li>{t('The user reviews the form and presses Start check. Nothing runs until they do.')}</li>
          <li>{t('The report is listed only in the browser that ran the check. Share its link to show it to others.')}</li>
        </ol>
      </section>

      <section aria-labelledby='integrate-format' className='flex flex-col gap-4'>
        <h2 id='integrate-format' className='text-xl font-bold tracking-tight'>
          {t('Link format')}
        </h2>
        <pre className='bg-navy-deep overflow-x-auto rounded-md border border-white/10 p-4 font-mono text-xs leading-5 text-sky-100'>
          <code>{`${site}/#type=claude&base_url=<Base URL>&key=<API key>&model=<model>`}</code>
        </pre>
        <p className='text-muted-foreground text-sm leading-6'>
          {t(
            'Put the values after # so browsers never send them to the ModelsExam server. Query parameters (?base_url=…) also work. Encode every value, for example with URLSearchParams or encodeURIComponent.'
          )}
        </p>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Parameter')}</TableHead>
              <TableHead>{t('Example')}</TableHead>
              <TableHead>{t('Meaning')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {params.map(([name, example, meaning]) => (
              <TableRow key={name}>
                <TableCell className='font-mono text-xs'>{name}</TableCell>
                <TableCell className='font-mono text-xs break-all whitespace-normal'>{example}</TableCell>
                <TableCell className='text-sm whitespace-normal'>{meaning}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </section>

      <section aria-labelledby='integrate-builder' className='flex flex-col gap-5'>
        <h2 id='integrate-builder' className='text-xl font-bold tracking-tight'>
          {t('Try it')}
        </h2>
        <FieldGroup className='grid gap-4 sm:grid-cols-2'>
          <Field>
            <FieldLabel htmlFor='integrate-type'>{t('Check type')}</FieldLabel>
            <NativeSelect
              id='integrate-type'
              value={provider}
              onChange={(e) => setProvider(e.target.value as PrefillProvider)}
            >
              <NativeSelectOption value='claude'>{t('Claude')}</NativeSelectOption>
              <NativeSelectOption value='openai'>{t('OpenAI-compatible')}</NativeSelectOption>
              <NativeSelectOption value='gemini'>{t('Gemini')}</NativeSelectOption>
              <NativeSelectOption value='image'>{t('Image generation')}</NativeSelectOption>
            </NativeSelect>
          </Field>
          <Field>
            <FieldLabel htmlFor='integrate-model'>{t('Model')}</FieldLabel>
            <Input
              id='integrate-model'
              value={model}
              spellCheck={false}
              onChange={(e) => setModel(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor='integrate-base'>Base URL</FieldLabel>
            <Input
              id='integrate-base'
              value={baseUrl}
              spellCheck={false}
              onChange={(e) => setBaseUrl(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor='integrate-key'>API Key</FieldLabel>
            <Input
              id='integrate-key'
              type='password'
              placeholder='sk-your-key'
              autoComplete='off'
              spellCheck={false}
              value={key}
              onChange={(e) => setKey(e.target.value)}
            />
            <FieldDescription>
              {t('Stays in this browser tab; nothing is sent while you type.')}
            </FieldDescription>
          </Field>
        </FieldGroup>
        <CodeBlock
          id='integrate-link'
          title={t('Your link')}
          hint={t('Open it to see what your users will see.')}
          code={link}
        />
        <a
          href={link}
          target='_blank'
          rel='noopener noreferrer'
          className='text-sm underline underline-offset-4 hover:no-underline'
        >
          {t('Open this link in a new tab')}
        </a>
        <CodeBlock
          id='integrate-code'
          title={t('Code example')}
          hint={t('Build the link in your own console, where the user is signed in.')}
          code={snippet(site)}
        />
      </section>

      <section aria-labelledby='integrate-safety' className='flex flex-col gap-4'>
        <h2 id='integrate-safety' className='text-xl font-bold tracking-tight'>
          {t('Keys and privacy')}
        </h2>
        <ul className='text-muted-foreground flex list-disc flex-col gap-2 pl-5 text-sm leading-6'>
          <li>{t('Values after # are read in the browser and never sent to the server with the page request.')}</li>
          <li>{t('As soon as the page reads the link, the Base URL and key are removed from the address bar, so they do not stay in history or get shared onwards.')}</li>
          <li>{t('The key field is a password field; a masked form such as sk-abc••••wxyz confirms which key was filled in.')}</li>
          <li>{t('The key is used only for that check and is never written into reports. Check records are listed only in the browser that ran them.')}</li>
          <li>{t('Only put a key in a link for the user who owns it. Suggest a dedicated key with a small quota for checks.')}</li>
        </ul>
      </section>

      <section aria-labelledby='integrate-badge' className='flex flex-col gap-3'>
        <h2 id='integrate-badge' className='text-xl font-bold tracking-tight'>
          {t('Show the result on your site')}
        </h2>
        <p className='text-muted-foreground text-sm leading-6'>
          {t('Once your endpoint has a completed check, you can show its latest result with a badge.')}{' '}
          <Link to='/get-badge' className='text-foreground underline underline-offset-4 hover:no-underline'>
            {t('Get your badge')}
          </Link>
        </p>
      </section>
    </div>
  )
}
