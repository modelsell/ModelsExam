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
import { Suspense, lazy, useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'
import { Link } from '@/lib/router'
import { getChannel } from '@/features/channels/api'
import { ImageCheck } from '../image/components/image-check'
import { OpenAICheck } from '../openai/components/openai-check'
import type { CheckTarget } from '../types'
import { useModelCheck } from '../use-model-check'
import { hasCredentials, type CheckPrefill } from '../lib/link-prefill'
import { CheckForm } from './check-form'
import { CheckHeroIntro } from './check-landing'

const CheckReport = lazy(() => import('./check-report').then((m) => ({ default: m.CheckReport })))
const CheckReportDrawer = lazy(() => import('./check-report-drawer').then((m) => ({ default: m.CheckReportDrawer })))

export function NewCheck(props: {
  channelId?: number
  initialModel?: string
  prefill?: CheckPrefill
  onSelectReport: (id: string | undefined) => void
  onSignIn?: () => void
}) {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const channelId =
    (user?.role ?? 0) >= ROLE.ADMIN ? props.channelId : undefined
  const run = useModelCheck()
  const prefill = props.prefill
  const fromLink = !!prefill && hasCredentials(prefill)
  const [provider, setProvider] = useState<'claude' | 'openai' | 'image'>(
    prefill?.provider ?? 'claude'
  )
  // A link that fills in the form lands on the form, not the hero.
  useEffect(() => {
    if (fromLink) document.getElementById('new-check')?.scrollIntoView()
  }, [fromLink])
  const claudePrefill =
    (prefill?.provider ?? 'claude') === 'claude' ? prefill : undefined
  const [openAIBusy, setOpenAIBusy] = useState(false)
  const [imageBusy, setImageBusy] = useState(false)
  const [reportOpen, setReportOpen] = useState(false)
  const channel = useQuery({
    queryKey: ['model-check-channel', user?.id, channelId],
    enabled: !!channelId,
    queryFn: async () => {
      const response = await getChannel(channelId!)
      if (
        !response.success ||
        !response.data ||
        ![14, 33].includes(response.data.type)
      )
        throw new Error('Channel unavailable')
      return response.data
    },
    staleTime: 60_000,
    retry: false,
  })
  const start = async (target: CheckTarget) => {
    setReportOpen(true)
    await run.start(target)
  }
  const platforms = [
    {
      id: 'claude' as const,
      title: t('Claude'),
      protocol: t('Anthropic Messages'),
      text: t('Capabilities, protocol behaviour, token usage and caching.'),
    },
    {
      id: 'openai' as const,
      title: t('OpenAI-compatible'),
      protocol: t('Chat Completions and Responses'),
      text: t('Streaming, tools, structured output and error shapes.'),
    },
    {
      id: 'image' as const,
      title: t('Image generation'),
      protocol: t('OpenAI Images'),
      text: t('Pixel-verified output, edits and optional OpenAI Verify provenance.'),
    },
  ]
  const locked = run.busy || openAIBusy || imageBusy
  return (
    <section aria-labelledby='model-check-title'>
      <CheckHeroIntro />
      <div className='bg-muted/40 border-y px-4 py-12 sm:px-6 sm:py-16'>
        <div
          id='new-check'
          className='mx-auto flex w-full max-w-6xl scroll-mt-20 flex-col gap-6'
        >
          <div className='flex flex-col gap-2'>
            <h2 className='font-serif text-2xl font-semibold tracking-tight sm:text-3xl'>
              {t('Start a check')}
            </h2>
            <p className='text-muted-foreground text-sm leading-6'>
              {t('Choose the protocol your endpoint speaks.')}
            </p>
            <p className='text-muted-foreground text-xs leading-5'>
              {t('Run an API relay? Link your users here with the Base URL, key and model already filled in.')}{' '}
              <Link
                to='/integrate'
                className='text-foreground underline underline-offset-4 hover:no-underline'
              >
                {t('Relay integration')}
              </Link>
            </p>
          </div>
          <div
            role='radiogroup'
            aria-label={t('Model provider')}
            className='grid gap-3 sm:grid-cols-3'
          >
            {platforms.map((item) => {
              const selected = provider === item.id
              return (
                <button
                  key={item.id}
                  type='button'
                  role='radio'
                  aria-checked={selected}
                  disabled={locked}
                  onClick={() => setProvider(item.id)}
                  className={cn(
                    'bg-card flex flex-col gap-1.5 rounded-lg border p-4 text-left transition-colors',
                    'focus-visible:ring-ring focus-visible:ring-2 focus-visible:outline-none',
                    'disabled:cursor-not-allowed disabled:opacity-60',
                    selected
                      ? 'border-foreground ring-foreground ring-1'
                      : 'hover:border-foreground/40'
                  )}
                >
                  <span className='text-sm font-semibold'>{item.title}</span>
                  <span className='text-muted-foreground font-mono text-xs'>
                    {item.protocol}
                  </span>
                  <span className='text-muted-foreground text-xs leading-5'>
                    {item.text}
                  </span>
                </button>
              )
            })}
          </div>
          {fromLink && (
            <Alert>
              <AlertDescription>
                {t('Filled in from the link. Review the details, then press Start check.')}
              </AlertDescription>
            </Alert>
          )}
          <div className={provider === 'claude' ? 'contents' : 'hidden'}>
            {channel.isError && (
              <Alert variant='destructive'>
                <AlertDescription>
                  {t(
                    'The channel could not be loaded. You can still check a custom endpoint.'
                  )}
                </AlertDescription>
              </Alert>
            )}
            {channelId && channel.isPending ? (
              <Skeleton className='h-72 rounded-xl' />
            ) : (
              <CheckForm
                channel={channel.data}
                initialModel={props.initialModel}
                prefill={claudePrefill}
                busy={run.busy}
                onStart={start}
                onCancel={run.cancel}
                onSignIn={props.onSignIn}
              />
            )}
            {run.state.phase !== 'idle' && (
              <div className='bg-card flex flex-wrap items-center justify-between gap-3 rounded-lg border p-4'>
                <div className='flex min-w-0 items-center gap-2 text-sm'>
                  {run.busy && <Spinner />}
                  <span>
                    {run.busy
                      ? t('Check in progress')
                      : t('Model check report')}
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
          </div>
          <div
            className={provider === 'openai' ? 'flex flex-col gap-4' : 'hidden'}
          >
            <OpenAICheck
              prefill={prefill?.provider === 'openai' ? prefill : undefined}
              onSelectReport={props.onSelectReport}
              onBusyChange={setOpenAIBusy}
              onSignIn={props.onSignIn}
            />
          </div>
          <div
            className={provider === 'image' ? 'flex flex-col gap-4' : 'hidden'}
          >
            <ImageCheck
              prefill={prefill?.provider === 'image' ? prefill : undefined}
              onSelectReport={props.onSelectReport}
              onBusyChange={setImageBusy}
              onSignIn={props.onSignIn}
            />
          </div>
        </div>
      </div>
      {reportOpen && (
      <Suspense fallback={null}>
      <CheckReportDrawer open={reportOpen} onClose={() => setReportOpen(false)}>
        <CheckReport state={run.state} elapsed={run.elapsed} />
        {run.state.report?.history_saved && (
          <Button
            variant='outline'
            onClick={() => {
              setReportOpen(false)
              props.onSelectReport(run.state.report?.id)
            }}
          >
            {t('View this check in history')}
          </Button>
        )}
      </CheckReportDrawer>
      </Suspense>
      )}
    </section>
  )
}
