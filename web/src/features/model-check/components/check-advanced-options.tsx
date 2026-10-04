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
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldSet,
  FieldLegend,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { useBaselines } from '../hooks/use-baselines'
import type { ModelCheckForm } from '../lib/form'
import type { BaselineListItem } from '../types'
import { BaselinePicker } from './baseline-picker'

export function CheckAdvancedOptions(props: {
  form: UseFormReturn<ModelCheckForm>
  busy: boolean
}) {
  const { t } = useTranslation()
  const form = props.form
  const baselines = useBaselines()
  const currentModel = form.watch('model')
  const baselineID = form.watch('baseline_id') || ''
  const baselineType = form.watch('baseline_type') || ''
  return (
    <div className='flex flex-col gap-6'>
      <BaselinePicker
        model={currentModel}
        items={baselines.data ?? []}
        selectedId={baselineID}
        selectedType={baselineType}
        loading={baselines.isFetching}
        loadError={baselines.isError}
        invalid={!!form.formState.errors.baseline_id}
        busy={props.busy}
        onChange={(item?: BaselineListItem) => {
          form.setValue('baseline_id', item?.id || '')
          form.setValue('baseline_type', item?.type || '')
          form.clearErrors('baseline_id')
        }}
      />
      <FieldSet>
        <FieldLegend>{t('Detection scope')}</FieldLegend>
        <FieldDescription>
          {t(
            'Always included: model-name consistency, 4 token-usage checks, 6 capability questions across 4 domains (7 requests), and 3 fixed performance samples.'
          )}
        </FieldDescription>
        <FieldGroup>
          {(
            [
              [
                'cache',
                'Cache consistency',
                '8 requests: two fresh prefixes, each written once and read three times. Score all six reads.',
              ],
            ] as const
          ).map(([name, label, description]) => (
            <Controller
              key={name}
              name={name}
              control={form.control}
              render={({ field }) => (
                <Field orientation='horizontal'>
                  <div className='flex-1'>
                    <FieldLabel htmlFor={`check-${name}`}>
                      {t(label)}
                    </FieldLabel>
                    <FieldDescription>{t(description)}</FieldDescription>
                  </div>
                  <Switch
                    id={`check-${name}`}
                    checked={!!field.value}
                    onCheckedChange={field.onChange}
                    disabled={props.busy}
                  />
                </Field>
              )}
            />
          ))}
          <Field data-invalid={!!form.formState.errors.performance_tolerance}>
            <FieldLabel htmlFor='performance-tolerance'>
              {t('Performance tolerance (%)')}
            </FieldLabel>
            <Input
              id='performance-tolerance'
              type='number'
              min={0}
              max={100}
              step={1}
              disabled={props.busy}
              aria-invalid={!!form.formState.errors.performance_tolerance}
              {...form.register('performance_tolerance', {
                valueAsNumber: true,
              })}
            />
            <FieldDescription>
              {t(
                'Against the selected baseline: TTFT may increase by this percentage; throughput must remain at least 1 / (1 + tolerance) of baseline.'
              )}
            </FieldDescription>
            {form.formState.errors.performance_tolerance && (
              <FieldError>{t('Enter a percentage from 0 to 100')}</FieldError>
            )}
          </Field>
        </FieldGroup>
        <details className='rounded-lg border p-3'>
          <summary className='cursor-pointer text-sm font-medium'>
            {t('Optional scored checks')}
          </summary>
          <FieldGroup className='mt-4'>
            {(
              [
                [
                  'prompt_audit',
                  'Prompt injection score',
                  'Three identical minimal requests. Score reported total input against a 64-token allowance, without a saved baseline or a token-count endpoint.',
                ],
                ['pdf', 'PDF document reading', 'One extra document question.'],
                ['vision', 'Vision understanding', 'One extra image question.'],
              ] as const
            ).map(([name, label, description]) => (
              <Controller
                key={name}
                name={name}
                control={form.control}
                render={({ field }) => (
                  <Field orientation='horizontal'>
                    <div className='flex-1'>
                      <FieldLabel htmlFor={`check-${name}`}>
                        {t(label)}
                      </FieldLabel>
                      <FieldDescription>{t(description)}</FieldDescription>
                    </div>
                    <Switch
                      id={`check-${name}`}
                      checked={!!field.value}
                      onCheckedChange={field.onChange}
                      disabled={props.busy}
                    />
                  </Field>
                )}
              />
            ))}
          </FieldGroup>
        </details>
        <details className='rounded-lg border p-3'>
          <summary className='cursor-pointer text-sm font-medium'>
            {t('Optional diagnostics')}
          </summary>
          <FieldGroup className='mt-4'>
            <Controller
              name='bedrock'
              control={form.control}
              render={({ field }) => (
                <Field orientation='horizontal'>
                  <div className='flex-1'>
                    <FieldLabel htmlFor='check-bedrock'>
                      {t('Bedrock compatibility diagnostics')}
                    </FieldLabel>
                    <FieldDescription>
                      {t(
                        'Up to 8 requests: invalid role, unknown beta, web_search, web_fetch, code_execution, advisor, model-specific sampling limits and invalid thinking signature rejection.'
                      )}
                    </FieldDescription>
                  </div>
                  <Switch
                    id='check-bedrock'
                    checked={!!field.value}
                    onCheckedChange={field.onChange}
                    disabled={props.busy}
                  />
                </Field>
              )}
            />
          </FieldGroup>
        </details>
      </FieldSet>
    </div>
  )
}
