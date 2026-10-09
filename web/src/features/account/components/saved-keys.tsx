import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Trans, useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { Link } from '@/lib/router'
import { useAuthStore } from '@/stores/auth-store'
import {
  createCredential,
  listCredentials,
  type Credential,
  type Provider,
} from '../api'
import { daysLeft, hostOf, usableFor } from '../lib/credentials'

export const CREDENTIALS_KEY = ['account-credentials'] as const

export function useCredentials() {
  const user = useAuthStore((state) => state.auth.user)
  return useQuery({
    queryKey: [...CREDENTIALS_KEY, user.id],
    queryFn: listCredentials,
    enabled: !!user.account,
    staleTime: 30_000,
  })
}

export function credentialLabel(c: Credential): string {
  return `${c.name} · ${c.hint} · ${hostOf(c.base_url)}`
}

// Choose one of the account's saved keys for this kind of check. Picking one
// locks the Base URL to the address the key was saved with.
export function SavedKeySelect(props: {
  provider: Provider
  value: string
  disabled?: boolean
  onChange: (credential: Credential | null) => void
}) {
  const { t } = useTranslation()
  const credentials = useCredentials()
  const usable = (credentials.data ?? []).filter((c) => usableFor(c, props.provider))
  if (!usable.length && !props.value) return null
  return (
    <Field className='sm:col-span-2'>
      <FieldLabel htmlFor={`saved-key-${props.provider}`}>{t('Saved key')}</FieldLabel>
      <NativeSelect
        id={`saved-key-${props.provider}`}
        value={props.value}
        disabled={props.disabled}
        onChange={(event) =>
          props.onChange(usable.find((c) => c.id === event.target.value) ?? null)
        }
      >
        <NativeSelectOption value=''>{t('Enter a key by hand')}</NativeSelectOption>
        {usable.map((c) => (
          <NativeSelectOption key={c.id} value={c.id}>
            {credentialLabel(c)}
          </NativeSelectOption>
        ))}
      </NativeSelect>
      <FieldDescription>
        {props.value
          ? t('The saved key is sent only to the Base URL it was saved with. To use another address, enter the key again.')
          : t('Or pick a key saved in your account.')}
      </FieldDescription>
    </Field>
  )
}

const EXPIRY_DAYS = [1, 7, 30, 90] as const

// "Save to my account": off by default. Turning it on shows what saving means
// and needs all three confirmations before Save is enabled.
export function SaveKeyPanel(props: {
  provider: Provider
  baseUrl: string
  secret: string
  disabled?: boolean
  onSaved: (credential: Credential) => void
}) {
  const { t } = useTranslation()
  const auth = useAuthStore((state) => state.auth)
  const queryClient = useQueryClient()
  const [on, setOn] = useState(false)
  const [acks, setAcks] = useState({ test: false, quota: false, plaintext: false })
  const [days, setDays] = useState(7)
  const save = useMutation({
    mutationFn: () =>
      createCredential({
        provider: props.provider,
        base_url: props.baseUrl,
        secret: props.secret,
        expires_days: days,
        ack_test_key: acks.test,
        ack_quota: acks.quota,
        ack_plaintext: acks.plaintext,
      }),
    onSuccess: (credential) => {
      void queryClient.invalidateQueries({ queryKey: CREDENTIALS_KEY })
      setOn(false)
      setAcks({ test: false, quota: false, plaintext: false })
      toast.success(t('Key saved. Only its mask is shown from now on.'))
      props.onSaved(credential)
    },
    onError: (error) => toast.error(error.message),
  })
  if (!auth.loaded) return null
  if (!auth.user.account) {
    return (
      <p className='text-muted-foreground text-xs sm:col-span-2'>
        <Link to='/login' className='text-foreground underline underline-offset-4'>
          {t('Sign in')}
        </Link>{' '}
        {t('to save a test key for one-click retests and scheduled checks.')}
      </p>
    )
  }
  const id = `save-key-${props.provider}`
  const ready = acks.test && acks.quota && acks.plaintext
  const blockedByHttp = auth.requireHttps && !auth.https
  return (
    <div className='flex flex-col gap-3 sm:col-span-2'>
      <label htmlFor={id} className='flex items-center gap-3 text-sm'>
        <Switch
          id={id}
          checked={on}
          disabled={props.disabled}
          onCheckedChange={(checked) => setOn(!!checked)}
        />
        {t('Save to my account')}
      </label>
      {on && (
        <div className='border-destructive/40 bg-destructive/5 flex flex-col gap-3 rounded-lg border p-4 text-sm'>
          <p className='font-semibold'>{t('Only save a test key made just for checks')}</p>
          <ul className='flex list-disc flex-col gap-1.5 pl-5 text-xs leading-5'>
            <li>
              <Trans
                i18nKey='This site stores the key <b>in plaintext on the server</b> for one-click retests and scheduled checks. Site operators, database backups and anyone who breaks into the server may be able to read it. After saving, even you will only see its mask.'
                components={{ b: <strong /> }}
              />
            </li>
            <li>
              <Trans
                i18nKey='At your provider, <b>create a separate test key</b> with a <b>low spending limit</b>. <b>Do not use a key that production uses</b>, and do not share one key between checks and production.'
                components={{ b: <strong /> }}
              />
            </li>
            <li>
              <Trans
                i18nKey='Choose the shortest <b>expiry</b> you can. When it expires, this site deletes the key automatically.'
                components={{ b: <strong /> }}
              />
            </li>
            <li>
              <Trans
                i18nKey='<b>When you no longer need checks, delete the key here right away and revoke it at your provider.</b>'
                components={{ b: <strong /> }}
              />
            </li>
          </ul>
          {(
            [
              ['test', t('This is a test key made only for checks. It is not used in production.')],
              ['quota', t('I have set a spending limit for this key at the provider.')],
              ['plaintext', t('I understand this key is stored in plaintext on this site’s server.')],
            ] as const
          ).map(([name, label]) => (
            <label key={name} className='flex items-start gap-2 text-xs leading-5'>
              <input
                type='checkbox'
                className='mt-0.5'
                checked={acks[name]}
                onChange={(event) => setAcks((prev) => ({ ...prev, [name]: event.target.checked }))}
              />
              {label}
            </label>
          ))}
          <label className='flex flex-wrap items-center gap-2 text-xs'>
            {t('Expires after')}
            <NativeSelect
              value={String(days)}
              onChange={(event) => setDays(Number(event.target.value))}
              aria-label={t('Expires after')}
            >
              {EXPIRY_DAYS.map((d) => (
                <NativeSelectOption key={d} value={String(d)}>
                  {t('{{count}} days', { count: d })}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </label>
          {blockedByHttp && (
            <Alert variant='destructive'>
              <AlertDescription>{t('Saving keys needs HTTPS. Open this site over https://')}</AlertDescription>
            </Alert>
          )}
          <Button
            type='button'
            size='sm'
            className='w-fit'
            disabled={!ready || blockedByHttp || props.secret.trim().length < 4 || !props.baseUrl || save.isPending}
            onClick={() => save.mutate()}
          >
            {t('Save key')}
          </Button>
        </div>
      )}
    </div>
  )
}

// Reminders shown on every page while a saved key needs attention.
export function CredentialBanners() {
  const { t } = useTranslation()
  const credentials = useCredentials()
  const now = Date.now()
  const expiring = (credentials.data ?? []).filter((c) => daysLeft(c, now) < 3)
  const idle = (credentials.data ?? []).filter(
    (c) => now - (c.last_used_at || c.created_at) > 14 * 24 * 3600 * 1000
  )
  if (!expiring.length && !idle.length) return null
  return (
    <div className='mx-auto flex max-w-6xl flex-col gap-2 px-4 pt-4 sm:px-6'>
      {expiring.length > 0 && (
        <Alert>
          <AlertDescription>
            {t('{{count}} saved keys expire within 3 days and will then be deleted.', { count: expiring.length })}{' '}
            <Link to='/keys' className='underline underline-offset-4'>
              {t('My keys')}
            </Link>
          </AlertDescription>
        </Alert>
      )}
      {idle.length > 0 && (
        <Alert>
          <AlertDescription>
            {t('{{count}} saved keys have not been used for 14 days. If you no longer run checks, delete them.', { count: idle.length })}{' '}
            <Link to='/keys' className='underline underline-offset-4'>
              {t('My keys')}
            </Link>
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}

// The mask of the chosen saved key, shown in place of the key field.
export function useCredentialHint(id: string): string | undefined {
  const credentials = useCredentials()
  return id ? credentials.data?.find((c) => c.id === id)?.hint : undefined
}

// Shown in place of the key input while a saved key is chosen.
export function SavedKeyMask({ id, hint }: { id: string; hint?: string }) {
  const { t } = useTranslation()
  return (
    <div
      id={id}
      role='status'
      aria-label={t('Saved key')}
      className='border-input bg-muted/40 flex h-8 items-center rounded-lg border px-2.5 font-mono text-sm'
    >
      {hint ?? '••••'}
    </div>
  )
}
