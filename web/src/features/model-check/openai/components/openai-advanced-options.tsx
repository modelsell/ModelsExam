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
import { Controller, type UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { OpenAICheckForm } from '../lib/form'

type Toggle = 'responses' | 'vision' | 'logprobs'

const TOGGLES: ReadonlyArray<{
  name: Toggle
  label: string
  description: string
  forcedByFull: boolean
}> = [
  {
    name: 'responses',
    label: 'Responses API',
    description:
      'Also check POST /v1/responses: envelope, streaming, function calls with item_id, structured output.',
    forcedByFull: true,
  },
  {
    name: 'logprobs',
    label: 'Log probabilities',
    description: 'One extra request with logprobs and top_logprobs.',
    forcedByFull: true,
  },
  {
    name: 'vision',
    label: 'Image input',
    description: 'One extra request with an inline image. Not part of Full.',
    forcedByFull: false,
  },
]

export function OpenAIAdvancedOptions(props: {
  form: UseFormReturn<OpenAICheckForm>
  busy: boolean
}) {
  const { t } = useTranslation()
  const form = props.form
  const full = form.watch('suite') === 'full'
  return (
    <FieldSet>
      <FieldLegend>{t('Detection scope')}</FieldLegend>
      <FieldGroup>
        {TOGGLES.map((item) => {
          const forced = full && item.forcedByFull
          return (
            <Controller
              key={item.name}
              name={item.name}
              control={form.control}
              render={({ field }) => (
                <Field orientation='horizontal'>
                  <div className='flex-1'>
                    <FieldLabel htmlFor={`openai-${item.name}`}>
                      {t(item.label)}
                    </FieldLabel>
                    <FieldDescription>{t(item.description)}</FieldDescription>
                  </div>
                  <Switch
                    id={`openai-${item.name}`}
                    checked={forced || !!field.value}
                    onCheckedChange={field.onChange}
                    disabled={props.busy || forced}
                  />
                </Field>
              )}
            />
          )
        })}
        <Field>
          <FieldLabel>{t('Token limit parameter')}</FieldLabel>
          <Controller
            name='limit_param'
            control={form.control}
            render={({ field }) => (
              <Tabs
                value={field.value}
                onValueChange={(value) => field.onChange(value)}
              >
                <TabsList
                  className='w-full'
                  aria-label={t('Token limit parameter')}
                >
                  {(['max_completion_tokens', 'max_tokens'] as const).map(
                    (value) => (
                      <TabsTrigger
                        key={value}
                        value={value}
                        disabled={props.busy}
                        className='flex-1 font-mono text-xs'
                      >
                        {value}
                      </TabsTrigger>
                    )
                  )}
                </TabsList>
              </Tabs>
            )}
          />
          <FieldDescription>
            {t(
              'Use max_tokens for gateways that reject max_completion_tokens.'
            )}
          </FieldDescription>
        </Field>
      </FieldGroup>
    </FieldSet>
  )
}
