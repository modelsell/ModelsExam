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
import { Controller, useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import type { Credential } from '@/features/account/api'
import {
  SaveKeyPanel,
  SavedKeySelect,
  SavedKeyMask,
  useCredentialHint,
} from '@/features/account/components/saved-keys'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { CheckModelSelect } from '../../components/check-model-select'
import { KeyFieldHint } from '../../components/key-field-hint'
import type { CheckPrefill } from '../../lib/link-prefill'
import {
  geminiCheckOptions,
  geminiCheckSchema,
  geminiRequestBudget,
  type GeminiCheckForm,
} from '../lib/form'
import type { GeminiCheckTarget } from '../types'

export function GeminiCheckFormCard(props: {
  prefill?: CheckPrefill
  busy: boolean
  onStart: (target: GeminiCheckTarget) => Promise<void>
  onCancel: () => void
  onSignIn?: () => void
}) {
  const { t } = useTranslation()
  const form = useForm<GeminiCheckForm>({
    resolver: zodResolver(geminiCheckSchema),
    defaultValues: {
      base_url: props.prefill?.base_url ?? '',
      key: props.prefill?.key ?? '',
      model: props.prefill?.model ?? '',
      suite: 'standard',
      vision: false,
      credential_id: '',
      // A retest restores the options of the earlier check.
      ...(props.prefill?.options ?? {}),
    },
  })
  const errors = form.formState.errors
  // Kept in state as well: credential_id has no input, so watch() would not re-render.
  const [credentialId, setCredentialId] = useState('')
  const credentialHint = useCredentialHint(credentialId)
  const watchedBase = form.watch('base_url')
  // A saved key fixes the Base URL: it is only ever sent to that address.
  const pickCredential = (credential: Credential | null) => {
    form.setValue('credential_id', credential?.id ?? '')
    setCredentialId(credential?.id ?? '')
    if (credential) {
      form.setValue('base_url', credential.base_url)
      form.setValue('key', '')
    }
    form.clearErrors(['base_url', 'key'])
  }
  const suite = form.watch('suite')
  const requests = geminiRequestBudget({ suite, vision: !!form.watch('vision') })

  async function submit(values: GeminiCheckForm): Promise<void> {
    await props.onStart({
      ...geminiCheckOptions(values),
      base_url: values.base_url,
      ...(values.credential_id
        ? { credential_id: values.credential_id }
        : { key: values.key }),
    })
  }

  return (
    <Card className='shadow-foreground/5 gap-5 rounded-2xl pt-6 shadow-xl sm:pt-7'>
      <CardHeader className='px-5 sm:px-7'>
        <CardTitle>
          <h2>{t('Connect your model')}</h2>
        </CardTitle>
        <CardDescription>
          {t('Your endpoint. Your key. Your check.')}
        </CardDescription>
      </CardHeader>
      <CardContent className='px-5 sm:px-7'>
        <form
          id='gemini-check-form'
          onSubmit={form.handleSubmit(submit)}
          className='flex flex-col gap-6'
        >
          <FieldGroup className='grid gap-5 sm:grid-cols-2'>
            <SavedKeySelect
              provider='gemini'
              value={credentialId}
              disabled={props.busy}
              onChange={pickCredential}
            />
            <Field data-invalid={!!errors.base_url}>
              <FieldLabel htmlFor='gemini-base-url'>Base URL</FieldLabel>
              <Input
                id='gemini-base-url'
                readOnly={!!credentialId}
                placeholder='https://generativelanguage.googleapis.com'
                autoComplete='off'
                spellCheck={false}
                disabled={props.busy}
                aria-invalid={!!errors.base_url}
                {...form.register('base_url')}
              />
              <FieldDescription>
                {t(
                  'Native Gemini API. A /v1beta suffix is accepted; the key is sent in the x-goog-api-key header.'
                )}
              </FieldDescription>
              {errors.base_url && (
                <FieldError>
                  {t(
                    'Enter a valid Base URL without credentials or query parameters'
                  )}
                </FieldError>
              )}
            </Field>
            <Field data-invalid={!!errors.key}>
              <FieldLabel htmlFor='gemini-key'>API Key</FieldLabel>
              {credentialId ? (
                <SavedKeyMask id='gemini-key' hint={credentialHint} />
              ) : (
                <Input
                  id='gemini-key'
                  type='password'
                  placeholder='AIza…'
                  autoComplete='new-password'
                  spellCheck={false}
                  disabled={props.busy}
                  aria-invalid={!!errors.key}
                  {...form.register('key')}
                />
              )}
              {!credentialId && (
                <KeyFieldHint
                  value={form.watch('key')}
                  prefilled={props.prefill?.key}
                />
              )}
              {errors.key && (
                <FieldError>{t('Enter a valid API key')}</FieldError>
              )}
            </Field>
            {!credentialId && (
              <SaveKeyPanel
                provider='gemini'
                baseUrl={watchedBase}
                secret={form.watch('key')}
                disabled={props.busy}
                onSaved={pickCredential}
              />
            )}
            <Field className='sm:col-span-2' data-invalid={!!errors.model}>
              <FieldLabel htmlFor='gemini-model'>{t('Model')}</FieldLabel>
              <Controller
                name='model'
                control={form.control}
                render={({ field }) => (
                  <CheckModelSelect
                    value={field.value}
                    onChange={field.onChange}
                    onBlur={field.onBlur}
                    inputRef={field.ref}
                    id='gemini-model'
                    kind='gemini'
                    baseUrl={form.watch('base_url')}
                    apiKey={form.watch('key')}
                    disabled={props.busy}
                    invalid={!!errors.model}
                  />
                )}
              />
              <FieldDescription>
                {t(
                  'Gemini model id, for example gemini-2.5-flash. A models/ prefix is accepted.'
                )}
              </FieldDescription>
              {errors.model && (
                <FieldError>{t('Enter a model name')}</FieldError>
              )}
            </Field>
            <Field className='sm:col-span-2'>
              <FieldLabel>{t('Check suite')}</FieldLabel>
              <Controller
                name='suite'
                control={form.control}
                render={({ field }) => (
                  <Tabs
                    value={field.value}
                    onValueChange={(value) => field.onChange(value)}
                  >
                    <TabsList className='w-full' aria-label={t('Check suite')}>
                      {(['basic', 'standard', 'full'] as const).map((value) => (
                        <TabsTrigger
                          key={value}
                          value={value}
                          disabled={props.busy}
                          className='flex-1'
                        >
                          {t(
                            {
                              basic: 'Basic',
                              standard: 'Standard',
                              full: 'Full',
                            }[value]
                          )}
                        </TabsTrigger>
                      ))}
                    </TabsList>
                  </Tabs>
                )}
              />
              <FieldDescription>
                {t(
                  {
                    basic:
                      'Basic: model resource, generateContent envelope, streaming and error shape.',
                    standard:
                      'Standard: adds generation config, function calling, structured output and countTokens.',
                    full: 'Full: everything, including multiple candidates, thoughts, function calling NONE and streamed or parallel function calls.',
                  }[suite]
                )}
              </FieldDescription>
            </Field>
            <Controller
              name='vision'
              control={form.control}
              render={({ field }) => (
                <Field orientation='horizontal' className='sm:col-span-2'>
                  <div className='flex-1'>
                    <FieldLabel htmlFor='gemini-vision'>
                      {t('Image input')}
                    </FieldLabel>
                    <FieldDescription>
                      {t('One extra request with an inline image. Not part of Full.')}
                    </FieldDescription>
                  </div>
                  <Switch
                    id='gemini-vision'
                    checked={!!field.value}
                    onCheckedChange={field.onChange}
                    disabled={props.busy}
                  />
                </Field>
              )}
            />
          </FieldGroup>
          <p className='text-muted-foreground text-xs leading-relaxed'>
            {t(
              'Up to {{count}} upstream requests. Up to 90 seconds per request and 10 minutes per run. Provider charges may apply.',
              { count: requests }
            )}
          </p>
        </form>
      </CardContent>
      <CardFooter className='px-5 py-5 sm:px-7'>
        {props.busy ? (
          <Button
            key='stop'
            type='button'
            className='w-full'
            size='lg'
            variant='outline'
            onClick={props.onCancel}
          >
            {t('Stop check')}
          </Button>
        ) : (
          <Button
            key='start'
            className='w-full'
            size='lg'
            type={props.onSignIn ? 'button' : 'submit'}
            form='gemini-check-form'
            onClick={props.onSignIn}
          >
            {props.onSignIn ? t('Sign in to check') : t('Start check')}
          </Button>
        )}
      </CardFooter>
    </Card>
  )
}
