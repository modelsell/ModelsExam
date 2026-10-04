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
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldLabel,
  FieldError,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { saveBaseline } from '../api'
import { BASELINE_QUERY_KEY, useBaselines } from '../hooks/use-baselines'
import { isCountProbe } from '../lib/check-plan'
import { baselineTypeSchema } from '../lib/form'
import { modelIdentityState } from '../lib/model-identity'
import type { ClaudeCheckReport } from '../types'

export function SaveBaseline(props: {
  report: ClaudeCheckReport
  running: boolean
}) {
  const { t } = useTranslation()
  const form = useForm({
    resolver: zodResolver(baselineTypeSchema),
    defaultValues: {
      type: props.report.transport === 'bedrock_runtime' ? 'aws' : 'anthropic',
    },
  })
  const type = form.watch('type')
  const baselines = useBaselines()
  const client = useQueryClient()
  const saved = baselines.data?.some(
    (item) => item.report_id === props.report.id
  )
  const mutation = useMutation({
    mutationFn: () => saveBaseline(props.report.id, type.trim()),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: BASELINE_QUERY_KEY })
      toast.success(t('Comparison baseline saved'))
    },
    onError: (error: Error) => toast.error(t(error.message)),
  })
  const available =
    !props.running &&
    !props.report.cancelled &&
    !props.report.stop_reason &&
    props.report.version >= 6 &&
    props.report.history_saved !== false &&
    modelIdentityState(props.report) !== 'mismatch' &&
    props.report.samples.some(
      (sample) => sample.valid_response && !isCountProbe(sample.probe)
    )
  return (
    <Field data-invalid={!!form.formState.errors.type}>
      <FieldLabel htmlFor='baseline-type'>
        {t('Baseline type (your label)')}
      </FieldLabel>
      <div className='flex flex-wrap gap-2'>
        <Input
          id='baseline-type'
          list='baseline-types'
          aria-invalid={!!form.formState.errors.type}
          value={type}
          maxLength={40}
          disabled={!available || saved || mutation.isPending}
          {...form.register('type')}
          className='w-48'
        />
        <datalist id='baseline-types'>
          {['anthropic', 'aws', 'ccmax', 'kiro'].map((value) => (
            <option key={value} value={value} />
          ))}
        </datalist>
        <Button
          variant='outline'
          size='sm'
          disabled={!available || saved || mutation.isPending || !type.trim()}
          onClick={() => void form.handleSubmit(() => mutation.mutate())()}
        >
          {saved
            ? t('Saved as baseline')
            : mutation.isPending
              ? t('Saving…')
              : t('Set as baseline')}
        </Button>
      </div>
      {form.formState.errors.type && (
        <FieldError>
          {t(
            'Enter a baseline type using letters, numbers, spaces, underscores or hyphens'
          )}
        </FieldError>
      )}
      <FieldDescription>
        {t(
          'Review the report, assign a type, then save an immutable reference. Type labels do not verify the provider.'
        )}
      </FieldDescription>
    </Field>
  )
}
