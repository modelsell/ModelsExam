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
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { CheckModelSelect } from '../../components/check-model-select'
import {
  imageBudget,
  imageCheckOptions,
  imageCheckSchema,
  type ImageCheckForm,
} from '../lib/form'
import type { ImageCheckTarget } from '../types'


export function ImageCheckFormCard(props: {
  initialModel?: string
  busy: boolean
  onStart: (target: ImageCheckTarget) => Promise<void>
  onCancel: () => void
  onSignIn?: () => void
}) {
  const { t } = useTranslation()
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const form = useForm<ImageCheckForm>({
    resolver: zodResolver(imageCheckSchema),
    defaultValues: {
      base_url: '',
      key: '',
      verify_key: '',
      model: props.initialModel ?? 'gpt-image-2',
      suite: 'standard',
      provenance: false,
      baseline: false,
    },
  })
  const errors = form.formState.errors
  const suite = form.watch('suite')
  const provenance = !!form.watch('provenance')
  const baseline = !!form.watch('baseline')
  const budget = imageBudget({ suite, provenance, baseline })
  const needsVerify = provenance || baseline

  async function submit(values: ImageCheckForm): Promise<void> {
    await props.onStart({
      ...imageCheckOptions(values),
      base_url: values.base_url,
      key: values.key,
      verify_key: values.verify_key,
    })
  }

  return (
    <Card className='shadow-foreground/5 gap-5 rounded-2xl pt-6 shadow-xl sm:pt-7'>
      <CardHeader className='px-5 sm:px-7'>
        <CardTitle>
          <h2>{t('Connect your image model')}</h2>
        </CardTitle>
        <CardDescription>
          {t(
            'Generates test images with random parameters and verifies them locally. Optionally checks provenance with OpenAI Verify.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='px-5 sm:px-7'>
        <form
          id='image-check-form'
          onSubmit={form.handleSubmit(submit)}
          className='flex flex-col gap-6'
        >
          <FieldGroup className='grid gap-5 sm:grid-cols-2'>
            <Field data-invalid={!!errors.base_url}>
              <FieldLabel htmlFor='image-base-url'>Base URL</FieldLabel>
              <Input
                id='image-base-url'
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
              <FieldLabel htmlFor='image-key'>API Key</FieldLabel>
              <Input
                id='image-key'
                type='password'
                placeholder='sk-…'
                autoComplete='new-password'
                spellCheck={false}
                disabled={props.busy}
                aria-invalid={!!errors.key}
                {...form.register('key')}
              />
              <FieldDescription>
                {t('Used for this session only; excluded from reports.')}
              </FieldDescription>
              {errors.key && (
                <FieldError>{t('Enter a valid API key')}</FieldError>
              )}
            </Field>
            <Field className='sm:col-span-2' data-invalid={!!errors.model}>
              <FieldLabel htmlFor='image-model'>{t('Model')}</FieldLabel>
              <Controller
                name='model'
                control={form.control}
                render={({ field }) => (
                  <CheckModelSelect
                    value={field.value}
                    onChange={field.onChange}
                    onBlur={field.onBlur}
                    inputRef={field.ref}
                    id='image-model'
                    kind='image'
                    baseUrl={form.watch('base_url')}
                    apiKey={form.watch('key')}
                    disabled={props.busy}
                    invalid={!!errors.model}
                  />
                )}
              />
              <FieldDescription>
                {t(
                  'Image models such as gpt-image-2. You can also enter a custom model name.'
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
                      'Basic: one image, response shape, exact size, error shape and a solid color.',
                    standard:
                      'Standard: adds layout, shape and count prompts, formats, sizes, transparency and n.',
                    full: 'Full: everything, including quality levels, streaming, edits and masks.',
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
              {t('Provenance check (OpenAI Verify)')}
              <HugeiconsIcon
                icon={ArrowDown01Icon}
                data-icon='inline-end'
                className={advancedOpen ? 'rotate-180' : undefined}
              />
            </CollapsibleTrigger>
            <CollapsibleContent keepMounted className='flex flex-col gap-5 pt-5'>
              <Controller
                name='provenance'
                control={form.control}
                render={({ field }) => (
                  <Field orientation='horizontal'>
                    <div className='flex-1'>
                      <FieldLabel htmlFor='image-provenance'>
                        {t('Check provenance with OpenAI Verify')}
                      </FieldLabel>
                      <FieldDescription>
                        {t(
                          'Uploads the generated test images to OpenAI to look for C2PA credentials and SynthID. Detecting nothing proves nothing: only OpenAI signals are recognised and re-encoding removes them.'
                        )}
                      </FieldDescription>
                    </div>
                    <Switch
                      id='image-provenance'
                      checked={!!field.value}
                      onCheckedChange={field.onChange}
                      disabled={props.busy}
                    />
                  </Field>
                )}
              />
              <Controller
                name='baseline'
                control={form.control}
                render={({ field }) => (
                  <Field orientation='horizontal'>
                    <div className='flex-1'>
                      <FieldLabel htmlFor='image-baseline'>
                        {t('Compare with the official API')}
                      </FieldLabel>
                      <FieldDescription>
                        {t(
                          'Sends one extra generation to api.openai.com with the official key, billed to that key, and compares the signals.'
                        )}
                      </FieldDescription>
                    </div>
                    <Switch
                      id='image-baseline'
                      checked={!!field.value}
                      onCheckedChange={field.onChange}
                      disabled={props.busy}
                    />
                  </Field>
                )}
              />
              {needsVerify && (
                <>
                  <Field data-invalid={!!errors.verify_key}>
                    <FieldLabel htmlFor='image-verify-key'>
                      {t('Official OpenAI API key')}
                    </FieldLabel>
                    <Input
                      id='image-verify-key'
                      type='password'
                      placeholder='sk-…'
                      autoComplete='new-password'
                      spellCheck={false}
                      disabled={props.busy}
                      aria-invalid={!!errors.verify_key}
                      {...form.register('verify_key')}
                    />
                    <FieldDescription>
                      {t(
                        'Sent only to api.openai.com, never to the endpoint above. The organization needs access to content provenance checks.'
                      )}
                    </FieldDescription>
                    {errors.verify_key && (
                      <FieldError>
                        {t('Enter an official OpenAI API key')}
                      </FieldError>
                    )}
                  </Field>
                  <Alert>
                    <AlertDescription>
                      {t(
                        'Test images are uploaded to OpenAI for verification. They contain only synthetic shapes and colors, never your data. Organizations with Zero Data Retention cannot use this check.'
                      )}
                    </AlertDescription>
                  </Alert>
                </>
              )}
            </CollapsibleContent>
          </Collapsible>
          <p className='text-muted-foreground text-xs leading-relaxed'>
            {t(
              'Up to about {{requests}} image requests ({{images}} images) and {{verify}} Verify calls. Up to 180 seconds per request and 15 minutes per run. Image generation is billed by the provider.',
              {
                requests: budget.requests,
                images: budget.images,
                verify: budget.verifyCalls,
              }
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
            form='image-check-form'
            onClick={props.onSignIn}
          >
            {props.onSignIn ? t('Sign in to check') : t('Start check')}
          </Button>
        )}
      </CardFooter>
    </Card>
  )
}
