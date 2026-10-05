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
import { ArrowDown01Icon, Settings02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
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
import { CheckModelSelect } from '../../components/check-model-select'
import {
  openAICheckOptions,
  openAICheckSchema,
  openAIRequestBudget,
  type OpenAICheckForm,
} from '../lib/form'
import type { OpenAICheckTarget } from '../types'
import { KeyFieldHint } from '../../components/key-field-hint'
import type { CheckPrefill } from '../../lib/link-prefill'
import { OpenAIAdvancedOptions } from './openai-advanced-options'


export function OpenAICheckFormCard(props: {
  initialModel?: string
  prefill?: CheckPrefill
  busy: boolean
  onStart: (target: OpenAICheckTarget) => Promise<void>
  onCancel: () => void
  onSignIn?: () => void
}) {
  const { t } = useTranslation()
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const form = useForm<OpenAICheckForm>({
    resolver: zodResolver(openAICheckSchema),
    defaultValues: {
      base_url: props.prefill?.base_url ?? '',
      key: props.prefill?.key ?? '',
      model: props.prefill?.model ?? props.initialModel ?? '',
      suite: 'standard',
      responses: false,
      vision: false,
      logprobs: false,
      limit_param: 'max_completion_tokens',
    },
  })
  const errors = form.formState.errors
  const suite = form.watch('suite')
  const requests = openAIRequestBudget({
    suite,
    responses: !!form.watch('responses'),
    vision: !!form.watch('vision'),
    logprobs: !!form.watch('logprobs'),
  })

  async function submit(values: OpenAICheckForm): Promise<void> {
    await props.onStart({
      ...openAICheckOptions(values),
      base_url: values.base_url,
      key: values.key,
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
          id='openai-check-form'
          onSubmit={form.handleSubmit(submit)}
          className='flex flex-col gap-6'
        >
          <FieldGroup className='grid gap-5 sm:grid-cols-2'>
            <Field data-invalid={!!errors.base_url}>
              <FieldLabel htmlFor='openai-base-url'>Base URL</FieldLabel>
              <Input
                id='openai-base-url'
                placeholder='https://api.openai.com'
                autoComplete='off'
                spellCheck={false}
                disabled={props.busy}
                aria-invalid={!!errors.base_url}
                {...form.register('base_url')}
              />
              <FieldDescription>
                {t('OpenAI-compatible API. A /v1 suffix is accepted.')}
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
              <FieldLabel htmlFor='openai-key'>API Key</FieldLabel>
              <Input
                id='openai-key'
                type='password'
                placeholder='sk-…'
                autoComplete='new-password'
                spellCheck={false}
                disabled={props.busy}
                aria-invalid={!!errors.key}
                {...form.register('key')}
              />
              <KeyFieldHint
                value={form.watch('key')}
                prefilled={props.prefill?.key}
              />
              {errors.key && (
                <FieldError>{t('Enter a valid API key')}</FieldError>
              )}
            </Field>
            <Field className='sm:col-span-2' data-invalid={!!errors.model}>
              <FieldLabel htmlFor='openai-model'>{t('Model')}</FieldLabel>
              <Controller
                name='model'
                control={form.control}
                render={({ field }) => (
                  <CheckModelSelect
                    value={field.value}
                    onChange={field.onChange}
                    onBlur={field.onBlur}
                    inputRef={field.ref}
                    id='openai-model'
                    kind='openai'
                    baseUrl={form.watch('base_url')}
                    apiKey={form.watch('key')}
                    disabled={props.busy}
                    invalid={!!errors.model}
                  />
                )}
              />
              <FieldDescription>
                {t(
                  'OpenAI / GPT-compatible models. You can also enter a custom model name.'
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
                      'Basic: model list, response envelope, streaming and error shape.',
                    standard:
                      'Standard: adds input parameters, tool calling and structured output.',
                    full: 'Full: everything, including Responses API, logprobs and streamed tool calls.',
                  }[suite]
                )}
              </FieldDescription>
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
              <OpenAIAdvancedOptions form={form} busy={props.busy} />
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
            form='openai-check-form'
            onClick={props.onSignIn}
          >
            {props.onSignIn ? t('Sign in to check') : t('Start check')}
          </Button>
        )}
      </CardFooter>
    </Card>
  )
}
