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
import type { CheckPrefill } from '../../lib/link-prefill'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { CheckReportDrawer } from '../../components/check-report-drawer'
import { useOpenAICheck } from '../../openai/use-openai-check'
import { OpenAICheckReportView } from '../../openai/components/openai-check-report'
import { streamGeminiCheck } from '../api'
import type { GeminiCheckTarget } from '../types'
import { GeminiCheckFormCard } from './gemini-check-form'

// The native Gemini check reuses the OpenAI report view: both servers stream
// the same events and report shape.
export function GeminiCheck(props: {
  onSelectReport?: (id: string | undefined) => void
  onBusyChange?: (busy: boolean) => void
  onSignIn?: () => void
  prefill?: CheckPrefill
}) {
  const { t } = useTranslation()
  const run = useOpenAICheck(streamGeminiCheck)
  const [reportOpen, setReportOpen] = useState(false)
  const { onBusyChange } = props
  const start = async (target: GeminiCheckTarget) => {
    setReportOpen(true)
    onBusyChange?.(true)
    try {
      await run.start(target)
    } finally {
      onBusyChange?.(false)
    }
  }
  return (
    <>
      <GeminiCheckFormCard
        prefill={props.prefill}
        busy={run.busy}
        onStart={start}
        onCancel={run.cancel}
        onSignIn={props.onSignIn}
      />
      {run.state.phase !== 'idle' && (
        <div className='flex flex-wrap items-center justify-between gap-3 rounded-xl border p-4'>
          <div className='flex min-w-0 items-center gap-2 text-sm'>
            {run.busy && <Spinner />}
            <span>
              {run.busy ? t('Check in progress') : t('Model check report')}
            </span>
            <span className='text-muted-foreground truncate'>
              {run.state.report?.model}
            </span>
          </div>
          <Button
            size='sm'
            variant='outline'
            onClick={() => setReportOpen(true)}
          >
            {t('View report')}
          </Button>
        </div>
      )}
      <CheckReportDrawer open={reportOpen} onClose={() => setReportOpen(false)}>
        <OpenAICheckReportView state={run.state} elapsed={run.elapsed} />
        {run.state.report?.history_saved && !run.busy && (
          <Button
            variant='outline'
            onClick={() => {
              setReportOpen(false)
              props.onSelectReport?.(run.state.report?.id)
            }}
          >
            {t('View this check in history')}
          </Button>
        )}
      </CheckReportDrawer>
    </>
  )
}
