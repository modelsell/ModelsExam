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
import { useEffect, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { ArrowDown01Icon, Settings02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { StatusBadge } from '@/components/status-badge'
import type { Credential } from '@/features/account/api'
import { SaveKeyPanel, SavedKeySelect, useCredentialHint, SavedKeyMask } from '@/features/account/components/saved-keys'
import type { Channel } from '@/features/channels/types'
import { useBaselines } from '../hooks/use-baselines'
import { baselineCandidates } from '../lib/baseline-selection'
import { requestBudget } from '../lib/check-plan'
import {
  AWS_BASE_URL_EXAMPLE,
  OFFICIAL_BASE_URL,
  SOURCE_STYLE,
  sourceOfEndpoint,
  type ClaudeSource,
} from '../lib/claude-source'
import {
  modelCheckSchema,
  modelCheckOptions,
  type ModelCheckForm,
} from '../lib/form'
import type { CheckPrefill } from '../lib/link-prefill'
import type { CheckTarget } from '../types'
import { CheckAdvancedOptions } from './check-advanced-options'
import { CheckModelSelect } from './check-model-select'
import { KeyFieldHint } from './key-field-hint'

export function CheckForm(props: {
  channel?: Channel
  initialModel?: string
  prefill?: CheckPrefill
  busy: boolean
  onStart: (target: CheckTarget) => Promise<void>
  onCancel: () => void
  onSignIn?: () => void
}) {
  const { t } = useTranslation()
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const prefill = props.prefill
  const [source, setSource] = useState<ClaudeSource>(() =>
    prefill?.base_url ? sourceOfEndpoint(prefill.base_url) : 'relay'
  )
  const baselines = useBaselines()
  const channel = props.channel
  const models =
    channel?.models
      .split(',')
      .map((model) => model.trim())
      .filter(Boolean) ?? []
  const form = useForm<ModelCheckForm>({
    resolver: zodResolver(modelCheckSchema),
    defaultValues: {
      mode: channel && !prefill?.base_url ? 'channel' : 'endpoint',
      base_url:
        prefill?.base_url ??
        (channel?.type === 14 ? channel.base_url || '' : ''),
      key: prefill?.key ?? '',
      model:
        prefill?.model ||
        props.initialModel ||
        channel?.test_model ||
        models.find((model) => model.includes('claude')) ||
        '',
      cache: true,
      thinking: false,
      repeat: false,
      vision: false,
      pdf: false,
      stream_comparison: false,
      benchmark: true,
      performance: true,
      prompt_audit: false,
      bedrock: !!prefill?.base_url && sourceOfEndpoint(prefill.base_url) === 'aws',
      performance_tolerance: 25,
      baseline_id: '',
      baseline_type: '',
      credential_id: '',
      // A retest restores the options of the earlier check.
      ...(prefill?.options ?? {}),
    },
  })
  const mode = form.watch('mode')
  const currentModel = form.watch('model')
  const watchedBase = form.watch('base_url')
  const watchedKey = form.watch('key')
  // Kept in state as well: credential_id has no input, so watch() would not re-render.
  const [credentialId, setCredentialId] = useState('')
  const savedKeys = useCredentialHint(credentialId)
  // A saved key fixes the Base URL: it is only ever sent to that address.
  const pickCredential = (credential: Credential | null) => {
    form.setValue('credential_id', credential?.id ?? '')
    setCredentialId(credential?.id ?? '')
    if (credential) {
      form.setValue('base_url', credential.base_url)
      form.setValue('key', '')
      setSource(sourceOfEndpoint(credential.base_url))
    }
    form.clearErrors(['base_url', 'key'])
  }
  const baselineID = form.watch('baseline_id') || ''
  const baselineType = form.watch('baseline_type') || ''
  const selectedBaseline = baselineCandidates(
    baselines.data ?? [],
    currentModel,
    baselineType
  ).find((item) => item.id === baselineID)
  const setValue = form.setValue
  const pickSource = (next: ClaudeSource) => {
    const current = sourceOfEndpoint(form.getValues('base_url'))
    setSource(next)
    setValue('credential_id', '')
    setCredentialId('')
    setValue('bedrock', next === 'aws')
    if (next === 'official') setValue('base_url', OFFICIAL_BASE_URL)
    else if (next === 'aws') setValue('base_url', AWS_BASE_URL_EXAMPLE)
    else if (current !== 'relay') setValue('base_url', '')
    form.clearErrors(['base_url', 'key', 'model'])
  }
  const sourceOptions: Array<{ id: ClaudeSource; title: string; text: string }> = [
    {
      id: 'official',
      title: t('Official key'),
      text: t('Anthropic’s own API, api.anthropic.com.'),
    },
    {
      id: 'aws',
      title: t('AWS Bedrock'),
      text: t('Claude on Bedrock’s Anthropic-compatible endpoint.'),
    },
    {
      id: 'relay',
      title: t('Other type'),
      text: t('Any other Anthropic-compatible address.'),
    },
  ]
  const clearErrors = form.clearErrors
  useEffect(() => {
    if (props.busy || !baselines.isSuccess || !baselineID || selectedBaseline)
      return
    setValue('baseline_id', '')
    setValue('baseline_type', '')
    clearErrors('baseline_id')
  }, [
    props.busy,
    baselines.isSuccess,
    baselineID,
    selectedBaseline,
    setValue,
    clearErrors,
  ])
  const cache = form.watch('cache')
  const vision = form.watch('vision')
  const pdf = form.watch('pdf')
  const promptAudit = !!form.watch('prompt_audit')
  const bedrock = !!form.watch('bedrock')
  const requests = requestBudget({
    suite: 'focused',
    model: '',
    cache,
    thinking: false,
    vision,
    pdf,
    prompt_audit: promptAudit,
    bedrock,
  })

  async function submit(values: ModelCheckForm): Promise<void> {
    const reference = baselineCandidates(
      baselines.data ?? [],
      values.model,
      values.baseline_type
    ).find((item) => item.id === values.baseline_id)
    if (values.baseline_id && !reference) {
      form.setError('baseline_id', {
        type: 'validate',
        message: 'Select a saved baseline for the chosen type',
      })
      return
    }
    const options = modelCheckOptions(values, reference)
    if (values.mode === 'channel' && channel) {
      await props.onStart({ ...options, channel_id: channel.id })
    } else if (values.credential_id) {
      await props.onStart({
        ...options,
        base_url: values.base_url,
        credential_id: values.credential_id,
      })
    } else {
      await props.onStart({
        ...options,
        base_url: values.base_url,
        key: values.key,
      })
    }
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
          id='model-check-form'
          onSubmit={form.handleSubmit(submit, (errors) => {
            if (errors.baseline_id || errors.performance_tolerance)
              setAdvancedOpen(true)
          })}
          className='flex flex-col gap-6'
        >
          {channel && (
            <Controller
              name='mode'
              control={form.control}
              render={({ field }) => (
                <Tabs
                  value={field.value}
                  onValueChange={(value) => field.onChange(value)}
                >
                  <TabsList
                    className='w-full'
                    aria-label={t('Connection mode')}
                  >
                    <TabsTrigger
                      value='channel'
                      disabled={props.busy}
                      className='flex-1'
                    >
                      {t('Configured channel')}
                    </TabsTrigger>
                    <TabsTrigger
                      value='endpoint'
                      disabled={props.busy}
                      className='flex-1'
                    >
                      {t('Custom endpoint')}
                    </TabsTrigger>
                  </TabsList>
                </Tabs>
              )}
            />
          )}
          {mode === 'channel' && channel && (
            <div className='bg-muted/50 flex flex-col gap-2 rounded-lg border p-3'>
              <p className='font-medium break-words'>
                {channel.name}{' '}
                <span className='text-muted-foreground'>#{channel.id}</span>
              </p>
              <StatusBadge
                variant='info'
                copyable={false}
                label={
                  channel.type === 33
                    ? 'AWS Bedrock Runtime'
                    : 'Anthropic Messages API'
                }
              />
              <p className='text-muted-foreground text-xs leading-relaxed'>
                {t(
                  'Uses the channel credential and model mapping on the server.'
                )}
              </p>
            </div>
          )}
          <FieldGroup className='grid gap-5 sm:grid-cols-2'>
            {mode === 'endpoint' && (
              <>
                <div
                  role='radiogroup'
                  aria-label={t('Claude source')}
                  className='grid gap-2 sm:col-span-2 sm:grid-cols-3'
                >
                  {sourceOptions.map((option) => {
                    const selected = source === option.id
                    return (
                      <button
                        key={option.id}
                        type='button'
                        role='radio'
                        aria-checked={selected}
                        disabled={props.busy}
                        onClick={() => pickSource(option.id)}
                        className={cn(
                          'focus-visible:ring-ring flex flex-col gap-1 rounded-lg border p-3 text-left transition-colors focus-visible:ring-2 focus-visible:outline-none disabled:opacity-50',
                          selected
                            ? 'border-foreground ring-foreground ring-1'
                            : 'hover:bg-accent'
                        )}
                      >
                        <span className='flex items-center gap-2 text-sm font-semibold'>
                          <span
                            aria-hidden='true'
                            className={cn(
                              'size-2 rounded-full border',
                              SOURCE_STYLE[option.id]
                            )}
                          />
                          {option.title}
                        </span>
                        <span className='text-muted-foreground text-xs leading-5'>
                          {option.text}
                        </span>
                      </button>
                    )
                  })}
                </div>
                <SavedKeySelect
                  provider='claude'
                  value={credentialId}
                  disabled={props.busy}
                  onChange={pickCredential}
                />
                <Field data-invalid={!!form.formState.errors.base_url}>
                  <FieldLabel htmlFor='check-base-url'>Base URL</FieldLabel>
                  <Input
                    id='check-base-url'
                    placeholder='https://api.example.com'
                    autoComplete='off'
                    spellCheck={false}
                    disabled={props.busy}
                    readOnly={!!credentialId}
                    aria-invalid={!!form.formState.errors.base_url}
                    {...form.register('base_url')}
                  />
                  <FieldDescription>
                    {source === 'official'
                      ? t('Filled in with Anthropic’s own API address. Edit it if yours differs.')
                      : source === 'aws'
                        ? t('Bedrock’s Anthropic-compatible endpoint. Change the region in the address to match your key.')
                        : t('Anthropic Messages API. A /v1 suffix is accepted.')}
                  </FieldDescription>
                  {form.formState.errors.base_url && (
                    <FieldError>
                      {t(
                        'Enter a valid Base URL without credentials or query parameters'
                      )}
                    </FieldError>
                  )}
                </Field>
                <Field data-invalid={!!form.formState.errors.key}>
                  <FieldLabel htmlFor='check-key'>
                    {source === 'aws' ? t('AWS Bedrock API key') : source === 'official' ? t('Anthropic API key') : 'API Key'}
                  </FieldLabel>
                  {credentialId ? (
                    <SavedKeyMask id='check-key' hint={savedKeys} />
                  ) : (
                    <Input
                      id='check-key'
                      type='password'
                      placeholder={source === 'official' ? 'sk-ant-…' : 'sk-…'}
                      autoComplete='new-password'
                      spellCheck={false}
                      disabled={props.busy}
                      aria-invalid={!!form.formState.errors.key}
                      {...form.register('key')}
                    />
                  )}
                  {!credentialId && <KeyFieldHint value={watchedKey} prefilled={prefill?.key} />}
                  {form.formState.errors.key && (
                    <FieldError>{t('Enter a valid API key')}</FieldError>
                  )}
                </Field>
                {!credentialId && (
                  <SaveKeyPanel
                    provider='claude'
                    baseUrl={watchedBase}
                    secret={watchedKey}
                    disabled={props.busy}
                    onSaved={pickCredential}
                  />
                )}
              </>
            )}
            <Field
              className='sm:col-span-2'
              data-invalid={!!form.formState.errors.model}
            >
              <FieldLabel htmlFor='check-model'>{t('Model')}</FieldLabel>
              <Controller
                name='model'
                control={form.control}
                render={({ field }) => (
                  <CheckModelSelect
                    value={field.value}
                    onChange={field.onChange}
                    onBlur={field.onBlur}
                    inputRef={field.ref}
                    kind='claude'
                    models={mode === 'channel' ? models : undefined}
                    baseUrl={watchedBase}
                    apiKey={watchedKey}
                    disabled={props.busy}
                    invalid={!!form.formState.errors.model}
                  />
                )}
              />
              <FieldDescription>
                {t('Claude / Anthropic-compatible models. Pick one, or type a custom name.')}
              </FieldDescription>
              {form.formState.errors.model && (
                <FieldError>{t('Enter a model name')}</FieldError>
              )}
            </Field>
          </FieldGroup>
          <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
            <CollapsibleTrigger
              render={<Button type='button' variant='ghost' size='sm' />}
            >
              <HugeiconsIcon icon={Settings02Icon} data-icon='inline-start' />
              {t('Advanced settings')}
              <HugeiconsIcon
                icon={ArrowDown01Icon}
                data-icon='inline-end'
                className={advancedOpen ? 'rotate-180' : undefined}
              />
            </CollapsibleTrigger>
            <CollapsibleContent keepMounted className='pt-5'>
              <CheckAdvancedOptions form={form} busy={props.busy} />
            </CollapsibleContent>
          </Collapsible>
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
            onClick={(event) => {
              event.preventDefault()
              props.onCancel()
            }}
          >
            {t('Stop check')}
          </Button>
        ) : (
          <Button
            key='start'
            className='w-full'
            size='lg'
            type={props.onSignIn ? 'button' : 'submit'}
            form='model-check-form'
            onClick={props.onSignIn}
          >
            {props.onSignIn ? t('Sign in to check') : t('Start check')}
          </Button>
        )}
      </CardFooter>
    </Card>
  )
}
