import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Link } from '@/lib/router'
import { reportPath } from '@/lib/route'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'
import {
  createSchedule,
  deleteSchedule,
  listRunningJobs,
  listSchedules,
  updateSchedule,
  type Schedule,
} from '@/features/account/api'
import { RequireAccount } from '@/features/account/components/require-account'
import { credentialLabel, useCredentials } from '@/features/account/components/saved-keys'
import { hostOf, usableFor } from '@/features/account/lib/credentials'
import { takeSchedulePrefill } from '@/features/account/lib/handoff'
import { checkOptions, FORM_DEFAULTS, requestsPerDay } from '@/features/account/lib/retest'
import { findCredential } from '@/features/account/lib/credentials'
import { PROVIDER_LABEL } from './keys-page'

const SCHEDULES_KEY = ['account-schedules'] as const
const INTERVALS = [30, 60, 180, 360, 720, 1440]

function useIntervalLabel() {
  const { t } = useTranslation()
  return (minutes: number) =>
    minutes < 60 ? t('Every {{count}} minutes', { count: minutes }) : t('Every {{count}} hours', { count: minutes / 60 })
}

export function SchedulesPage() {
  const { t } = useTranslation()
  return (
    <div className='mx-auto flex max-w-6xl flex-col gap-6 px-4 py-12 sm:px-6 sm:py-16'>
      <div className='flex flex-col gap-2'>
        <h1 className='font-serif text-3xl font-semibold tracking-tight'>{t('Scheduled checks')}</h1>
        <p className='text-muted-foreground text-sm'>
          {t('Scheduled checks run on the server with a saved key, even when this page is closed. Results appear in My check records.')}
        </p>
      </div>
      <RequireAccount>
        <RunningChecks />
        <ScheduleList />
        <NewSchedule />
      </RequireAccount>
    </div>
  )
}

export function RunningChecks() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const running = useQuery({
    queryKey: ['account-running', user.id],
    queryFn: listRunningJobs,
    enabled: !!user.account,
    refetchInterval: 5000,
  })
  if (!running.data?.length) return null
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Running now')}</CardTitle>
      </CardHeader>
      <CardContent className='flex flex-col gap-2'>
        {running.data.map((job) => (
          <div key={job.id} className='flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3 text-sm'>
            <span className='flex min-w-0 items-center gap-2'>
              <Spinner />
              <span className='truncate font-medium'>{job.model}</span>
              <span className='text-muted-foreground truncate text-xs'>{hostOf(job.endpoint)}</span>
              {job.scheduled && <span className='rounded border px-1.5 py-0.5 text-[10px]'>{t('Scheduled')}</span>}
            </span>
            <Link to={reportPath(job.id)} className='underline underline-offset-4'>
              {t('View report')}
            </Link>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}

function ScheduleList() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const intervalLabel = useIntervalLabel()
  const schedules = useQuery({
    queryKey: [...SCHEDULES_KEY, user.id],
    queryFn: listSchedules,
    refetchInterval: 30_000,
  })
  const refresh = () => queryClient.invalidateQueries({ queryKey: SCHEDULES_KEY })
  const update = useMutation({
    mutationFn: (input: { id: string; enabled: boolean; interval_minutes: number }) =>
      updateSchedule(input.id, { enabled: input.enabled, interval_minutes: input.interval_minutes }),
    onSuccess: refresh,
    onError: (error) => toast.error(error.message),
  })
  const remove = useMutation({
    mutationFn: deleteSchedule,
    onSuccess: refresh,
    onError: (error) => toast.error(error.message),
  })
  if (schedules.isPending) return <Skeleton className='h-32 rounded-xl' />
  if (!schedules.data?.length) {
    return <p className='text-muted-foreground text-sm'>{t('No scheduled checks yet. Create one below, or use “Schedule” on a record.')}</p>
  }
  return (
    <Card>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Model')}</TableHead>
              <TableHead>{t('Key')}</TableHead>
              <TableHead>{t('Interval')}</TableHead>
              <TableHead>{t('Next run')}</TableHead>
              <TableHead>{t('Last result')}</TableHead>
              <TableHead>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {schedules.data.map((s: Schedule) => {
              const bad = s.last_status === 'failed' || s.last_status === 'skipped' || s.last_status === 'interrupted'
              return (
                <TableRow key={s.id} className={cn(bad && 'bg-destructive/5')}>
                  <TableCell className='font-medium'>
                    <span className='text-muted-foreground mr-2 rounded border px-1.5 py-0.5 text-[10px] font-normal'>
                      {PROVIDER_LABEL[s.provider]}
                    </span>
                    {s.model}
                    <span className='text-muted-foreground block text-xs'>{hostOf(s.base_url)}</span>
                  </TableCell>
                  <TableCell className='font-mono text-xs'>{s.credential_hint || '—'}</TableCell>
                  <TableCell>
                    <NativeSelect
                      aria-label={t('Interval')}
                      value={String(s.interval_minutes)}
                      onChange={(e) => update.mutate({ id: s.id, enabled: s.enabled, interval_minutes: Number(e.target.value) })}
                    >
                      {INTERVALS.map((m) => (
                        <NativeSelectOption key={m} value={String(m)}>
                          {intervalLabel(m)}
                        </NativeSelectOption>
                      ))}
                    </NativeSelect>
                  </TableCell>
                  <TableCell className='text-xs tabular-nums'>
                    {s.enabled ? new Date(s.next_run_at).toLocaleString() : t('Paused')}
                  </TableCell>
                  <TableCell className='text-xs'>
                    {s.last_run_at ? (
                      <span className={cn(bad && 'text-destructive font-semibold')}>
                        {s.last_status || '—'}
                        {s.last_score != null && ` · ${s.last_score}`}
                        {s.last_run_id && (
                          <>
                            {' · '}
                            <Link to={reportPath(s.last_run_id)} className='underline underline-offset-4'>
                              {t('View report')}
                            </Link>
                          </>
                        )}
                        {s.last_error && <span className='block font-normal'>{t(s.last_error)}</span>}
                      </span>
                    ) : (
                      '—'
                    )}
                  </TableCell>
                  <TableCell>
                    <div className='flex items-center gap-3'>
                      <label className='flex items-center gap-2 text-xs'>
                        <Switch
                          checked={s.enabled}
                          onCheckedChange={(enabled) => update.mutate({ id: s.id, enabled: !!enabled, interval_minutes: s.interval_minutes })}
                        />
                        {t('On')}
                      </label>
                      <Button size='sm' variant='ghost' onClick={() => remove.mutate(s.id)}>
                        {t('Delete')}
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

function NewSchedule() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const intervalLabel = useIntervalLabel()
  const credentials = useCredentials()
  const [prefill] = useState(takeSchedulePrefill)
  const usable = (credentials.data ?? []).filter((c) => usableFor(c, c.provider))
  const matched = prefill && credentials.data ? findCredential(credentials.data, prefill.provider, prefill.base_url) : undefined
  const [chosen, setChosen] = useState('')
  const credentialId = chosen || matched?.id || ''
  const credential = usable.find((c) => c.id === credentialId)
  const [model, setModel] = useState(prefill?.model ?? '')
  const [interval, setInterval] = useState(1440)
  const [suite, setSuite] = useState('')
  const plan = useMemo(() => {
    if (!credential) return undefined
    const samePrefill = prefill && prefill.provider === credential.provider
    const values: Record<string, unknown> = samePrefill ? { ...prefill.values } : { ...FORM_DEFAULTS[credential.provider] }
    if (suite && credential.provider !== 'claude') values.suite = suite
    return checkOptions(credential.provider, model || 'model', values)
  }, [credential, prefill, suite, model])
  const create = useMutation({
    mutationFn: () =>
      createSchedule({ credential_id: credentialId, model: model.trim(), options: plan!.options, interval_minutes: interval }),
    onSuccess: async () => {
      toast.success(t('Schedule created. The first run starts in about a minute.'))
      await queryClient.invalidateQueries({ queryKey: SCHEDULES_KEY })
    },
    onError: (error) => toast.error(error.message),
  })
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('New scheduled check')}</CardTitle>
        <CardDescription>{t('Up to 10 schedules per account; each key runs at most 50 checks a day.')}</CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-4'>
        {prefill && (
          <Alert>
            <AlertDescription>
              {matched
                ? t('Filled in from the record. Review the details, then create the schedule.')
                : t('No saved key matches this record’s Base URL. Save its key first (My keys), then come back.')}
            </AlertDescription>
          </Alert>
        )}
        {!usable.length && !credentials.isPending ? (
          <p className='text-muted-foreground text-sm'>
            {t('A schedule needs a saved key.')}{' '}
            <Link to='/keys' className='text-foreground underline underline-offset-4'>
              {t('My keys')}
            </Link>
          </p>
        ) : (
          <FieldGroup className='grid gap-4 sm:grid-cols-2'>
            <Field>
              <FieldLabel htmlFor='schedule-key'>{t('Saved key')}</FieldLabel>
              <NativeSelect id='schedule-key' value={credentialId} onChange={(e) => setChosen(e.target.value)}>
                <NativeSelectOption value=''>{t('Choose a key')}</NativeSelectOption>
                {usable.map((c) => (
                  <NativeSelectOption key={c.id} value={c.id}>
                    {`${PROVIDER_LABEL[c.provider]} · ${credentialLabel(c)}`}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </Field>
            <Field>
              <FieldLabel htmlFor='schedule-model'>{t('Model')}</FieldLabel>
              <Input id='schedule-model' value={model} maxLength={200} onChange={(e) => setModel(e.target.value)} />
            </Field>
            {credential && credential.provider !== 'claude' && (
              <Field>
                <FieldLabel htmlFor='schedule-suite'>{t('Suite')}</FieldLabel>
                <NativeSelect id='schedule-suite' value={suite || String(plan?.suite ?? 'standard')} onChange={(e) => setSuite(e.target.value)}>
                  {['basic', 'standard', 'full'].map((s) => (
                    <NativeSelectOption key={s} value={s}>
                      {t(s.charAt(0).toUpperCase() + s.slice(1))}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </Field>
            )}
            <Field>
              <FieldLabel htmlFor='schedule-interval'>{t('Interval')}</FieldLabel>
              <NativeSelect id='schedule-interval' value={String(interval)} onChange={(e) => setInterval(Number(e.target.value))}>
                {INTERVALS.map((m) => (
                  <NativeSelectOption key={m} value={String(m)}>
                    {intervalLabel(m)}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
              <FieldDescription>{t('Each run starts within ±10% of the interval.')}</FieldDescription>
            </Field>
          </FieldGroup>
        )}
        {plan && (
          <Alert variant='destructive'>
            <AlertDescription>
              {t('Scheduled checks keep using this key and cost money: about {{count}} requests per day.', {
                count: requestsPerDay(plan.requests, interval),
              })}
            </AlertDescription>
          </Alert>
        )}
        <Button className='w-fit' disabled={!plan || !model.trim() || create.isPending} onClick={() => create.mutate()}>
          {t('Create schedule')}
        </Button>
      </CardContent>
    </Card>
  )
}
