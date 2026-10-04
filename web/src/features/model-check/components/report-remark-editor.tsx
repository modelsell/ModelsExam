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
import { useId, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { updateCheckReportRemark } from '../api'
import { reportRemark, updateHistoryRemark } from '../lib/report-remark'
import type { CheckHistoryPage, ClaudeCheckReport } from '../types'

export function ReportRemarkEditor({
  report,
  running,
}: {
  report: ClaudeCheckReport
  running: boolean
}) {
  const { t } = useTranslation()
  const inputId = useId()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')
  const remark = reportRemark(report)
  const mutation = useMutation({
    mutationFn: (value: string) => updateCheckReportRemark(report.id, value),
    onMutate: async () => {
      await queryClient.cancelQueries({
        queryKey: ['model-check-history', userId, 'detail', report.id],
        exact: true,
      })
      await queryClient.cancelQueries({
        queryKey: ['model-check-history', userId, 'list'],
      })
    },
    onSuccess: (saved) => {
      queryClient.setQueryData(
        ['model-check-history', userId, 'detail', report.id],
        saved
      )
      queryClient.setQueriesData<CheckHistoryPage>(
        { queryKey: ['model-check-history', userId, 'list'] },
        (page) => updateHistoryRemark(page, saved)
      )
      void queryClient.invalidateQueries({
        queryKey: ['model-check-history', userId, 'list'],
      })
      setEditing(false)
      toast.success(t('Saved successfully'))
    },
    onError: (error) => {
      toast.error(error.message || t('Failed to save report remark'))
    },
  })
  if (!editing)
    return (
      <div className='flex flex-wrap items-center gap-2 text-sm'>
        <span className='min-w-0 flex-1 break-words'>{remark || '—'}</span>
        <Button
          size='sm'
          variant='ghost'
          disabled={running || report.history_saved === false}
          onClick={() => {
            setDraft(remark)
            setEditing(true)
          }}
        >
          {t('Edit')}
        </Button>
      </div>
    )
  return (
    <form
      className='flex flex-col gap-3'
      onSubmit={(event) => {
        event.preventDefault()
        if (!mutation.isPending && !running) mutation.mutate(draft)
      }}
    >
      <FieldGroup>
        <Field data-disabled={mutation.isPending}>
          <FieldLabel htmlFor={inputId} className='sr-only'>
            {t('Name')}
          </FieldLabel>
          <Input
            id={inputId}
            autoFocus
            value={draft}
            maxLength={200}
            disabled={mutation.isPending}
            placeholder={t('Name')}
            onChange={(event) => setDraft(event.target.value)}
          />
        </Field>
      </FieldGroup>
      <div className='flex items-center gap-2'>
        <Button
          type='submit'
          size='sm'
          disabled={mutation.isPending || running}
        >
          {mutation.isPending ? t('Saving…') : t('Save')}
        </Button>
        <Button
          type='button'
          size='sm'
          variant='outline'
          disabled={mutation.isPending}
          onClick={() => setEditing(false)}
        >
          {t('Cancel')}
        </Button>
      </div>
    </form>
  )
}
