import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { navigate } from '@/lib/router'
import { reportPath } from '@/lib/route'
import { useAuthStore } from '@/stores/auth-store'
import { getBaselines, getCheckHistoryReport } from '@/features/model-check/api'
import { setRetestPrefill } from '@/features/model-check/lib/link-prefill'
import type { CheckHistoryItem } from '@/features/model-check/types'
import { listCredentials, startRetest, type Credential } from '../api'
import { findCredential, hostOf } from '../lib/credentials'
import { setSchedulePrefill } from '../lib/handoff'
import { planRetest, type RetestPlan } from '../lib/retest'

// Rebuilds the check of a record from its stored report.
async function loadPlan(item: CheckHistoryItem): Promise<RetestPlan | null> {
  const detail = await getCheckHistoryReport(item.id)
  const report = detail.report as { model?: string; endpoint?: string; options?: Record<string, unknown> | null }
  const baselines = item.transport === 'openai_api' || item.transport === 'gemini_api' || item.transport === 'image_api'
    ? []
    : await getBaselines().catch(() => [])
  return planRetest(detail.run, report, baselines)
}

// "Retest": with a matching saved key, confirm and run it on the server;
// otherwise open the filled-in form with the cursor in the key field.
export function RetestActions({ item }: { item: CheckHistoryItem }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const signedIn = useAuthStore((state) => !!state.auth.user.account)
  const [busy, setBusy] = useState(false)
  const [confirm, setConfirm] = useState<{ plan: RetestPlan; credential: Credential } | null>(null)

  const prepare = async (): Promise<{ plan: RetestPlan; credential?: Credential } | null> => {
    const plan = await loadPlan(item)
    if (!plan) {
      toast.error(t('This record cannot be retested: its address or model is missing.'))
      return null
    }
    const credentials = signedIn ? await queryClient.fetchQuery({ queryKey: ['account-credentials', 'all'], queryFn: listCredentials, staleTime: 10_000 }) : []
    return { plan, credential: findCredential(credentials, plan.provider, plan.base_url) }
  }

  const retest = async () => {
    setBusy(true)
    try {
      const ready = await prepare()
      if (!ready) return
      if (ready.credential) {
        setConfirm({ plan: ready.plan, credential: ready.credential })
        return
      }
      setRetestPrefill({
        provider: ready.plan.provider,
        base_url: ready.plan.base_url,
        model: ready.plan.model,
        options: ready.plan.values,
        focusKey: true,
      })
      navigate('/#new-check')
    } catch {
      toast.error(t('Failed to load check report'))
    } finally {
      setBusy(false)
    }
  }

  const schedule = async () => {
    setBusy(true)
    try {
      const ready = await prepare()
      if (!ready) return
      setSchedulePrefill(ready.plan)
      navigate('/schedules')
    } catch {
      toast.error(t('Failed to load check report'))
    } finally {
      setBusy(false)
    }
  }

  const start = async () => {
    if (!confirm) return
    setBusy(true)
    try {
      const { run_id } = await startRetest({
        provider: confirm.plan.provider,
        credential_id: confirm.credential.id,
        model: confirm.plan.model,
        options: confirm.plan.options,
      })
      setConfirm(null)
      await queryClient.invalidateQueries({ queryKey: ['model-check-history'] })
      navigate(run_id ? reportPath(run_id) : '/records')
    } catch (error) {
      toast.error((error as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <Button size='sm' variant='ghost' disabled={busy || item.status === 'running'} onClick={() => void retest()}>
        {t('Retest')}
      </Button>
      {signedIn && (
        <Button size='sm' variant='ghost' disabled={busy} onClick={() => void schedule()}>
          {t('Schedule')}
        </Button>
      )}
      <Dialog
        open={!!confirm}
        onOpenChange={(open) => !open && setConfirm(null)}
        title={t('Retest with the saved key?')}
        description={t('The check runs on the server and keeps running if you close this page.')}
      >
        {confirm && (
          <dl className='grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm'>
            <dt className='text-muted-foreground'>{t('Host')}</dt>
            <dd className='truncate'>{hostOf(confirm.plan.base_url)}</dd>
            <dt className='text-muted-foreground'>{t('Model')}</dt>
            <dd className='truncate'>{confirm.plan.model}</dd>
            <dt className='text-muted-foreground'>{t('Suite')}</dt>
            <dd>{confirm.plan.suite}</dd>
            <dt className='text-muted-foreground'>{t('Key')}</dt>
            <dd className='font-mono text-xs'>{confirm.credential.hint}</dd>
            <dt className='text-muted-foreground'>{t('Requests')}</dt>
            <dd>{t('Up to {{count}} upstream requests', { count: confirm.plan.requests })}</dd>
          </dl>
        )}
        <div className='flex justify-end gap-2'>
          <Button variant='outline' onClick={() => setConfirm(null)}>
            {t('Cancel')}
          </Button>
          <Button disabled={busy} onClick={() => void start()}>
            {t('Start check')}
          </Button>
        </div>
      </Dialog>
    </>
  )
}
