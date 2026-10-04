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
import { useTranslation } from 'react-i18next'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { baselineCandidates } from '../lib/baseline-selection'
import type { BaselineListItem } from '../types'

export function BaselinePicker(props: {
  model: string
  items: BaselineListItem[]
  selectedId: string
  selectedType: string
  loading: boolean
  loadError: boolean
  invalid?: boolean
  busy: boolean
  onChange: (item?: BaselineListItem) => void
}) {
  const { t } = useTranslation()
  const compatible = baselineCandidates(props.items, props.model)
  const types = [...new Set(compatible.map((item) => item.type))].sort()
  const choices = compatible.filter((item) => item.type === props.selectedType)
  const chosen = choices.find((item) => item.id === props.selectedId)
  const label = (item: BaselineListItem) =>
    `${item.type} · ${item.model} · ${item.channel_name || '—'} · ${new Date(item.created_at).toLocaleString()}`
  return (
    <FieldSet>
      <FieldLegend>{t('Comparison baseline')}</FieldLegend>
      <FieldDescription>
        {t(
          'Choose a saved baseline before starting. Leave it empty to collect a new baseline without comparison.'
        )}
      </FieldDescription>
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor='check-baseline-type'>
            {t('Baseline type')}
          </FieldLabel>
          <NativeSelect
            id='check-baseline-type'
            className='w-full'
            value={props.selectedType}
            disabled={props.busy || props.loading}
            onChange={(event) => {
              if (props.busy) return
              props.onChange(
                compatible.find((item) => item.type === event.target.value)
              )
            }}
          >
            <NativeSelectOption value=''>
              {t('No baseline comparison')}
            </NativeSelectOption>
            {types.map((type) => (
              <NativeSelectOption key={type} value={type}>
                {type}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </Field>
        {!!props.selectedType && (
          <Field data-invalid={props.invalid}>
            <FieldLabel htmlFor='check-baseline-id'>
              {t('Saved baseline')}
            </FieldLabel>
            <NativeSelect
              id='check-baseline-id'
              className='w-full'
              value={props.selectedId}
              disabled={props.busy || props.loading}
              aria-invalid={props.invalid}
              onChange={(event) => {
                if (props.busy) return
                props.onChange(
                  choices.find((item) => item.id === event.target.value)
                )
              }}
            >
              {!chosen && (
                <NativeSelectOption value=''>
                  {t('Select a saved baseline')}
                </NativeSelectOption>
              )}
              {choices.map((item, index) => (
                <NativeSelectOption key={item.id} value={item.id}>
                  {label(item)}
                  {index === 0 ? ` · ${t('Latest')}` : ''}
                </NativeSelectOption>
              ))}
            </NativeSelect>
            {chosen && <FieldDescription>{label(chosen)}</FieldDescription>}
            {props.invalid && (
              <FieldError>
                {t('Select a saved baseline for the chosen type')}
              </FieldError>
            )}
          </Field>
        )}
      </FieldGroup>
      {props.loading && (
        <FieldDescription>{t('Loading saved baselines…')}</FieldDescription>
      )}
      {props.loadError && (
        <FieldDescription>
          {t('Could not load comparison baselines')}
        </FieldDescription>
      )}
      {!props.loading && !props.loadError && !compatible.length && (
        <FieldDescription>
          {t(
            'No saved baseline matches this model. You can run a check without comparison.'
          )}
        </FieldDescription>
      )}
    </FieldSet>
  )
}
