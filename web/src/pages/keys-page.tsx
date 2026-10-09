import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog } from '@/components/ui/dialog'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { cn } from '@/lib/utils'
import {
  deleteCredential,
  renewCredential,
  resumeCredential,
  type Credential,
  type Provider,
} from '@/features/account/api'
import { HttpsNotice } from '@/features/account/components/auth-forms'
import { RequireAccount } from '@/features/account/components/require-account'
import { CREDENTIALS_KEY, SaveKeyPanel, useCredentials } from '@/features/account/components/saved-keys'
import { daysLeft, hostOf } from '@/features/account/lib/credentials'

export const PROVIDER_LABEL: Record<Provider, string> = {
  claude: 'Claude',
  openai: 'OpenAI',
  gemini: 'Gemini',
  image: 'Image',
}

export function KeysPage() {
  const { t } = useTranslation()
  return (
    <div className='mx-auto flex max-w-6xl flex-col gap-6 px-4 py-12 sm:px-6 sm:py-16'>
      <h1 className='font-serif text-3xl font-semibold tracking-tight'>{t('My keys')}</h1>
      <RequireAccount>
        <KeysContent />
      </RequireAccount>
    </div>
  )
}

function KeysContent() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const credentials = useCredentials()
  const [deleting, setDeleting] = useState<Credential | null>(null)
  const [deleted, setDeleted] = useState(false)
  const [renewDays, setRenewDays] = useState<Record<string, number>>({})
  const refresh = () => queryClient.invalidateQueries({ queryKey: CREDENTIALS_KEY })
  const remove = useMutation({
    mutationFn: (id: string) => deleteCredential(id),
    onSuccess: async () => {
      setDeleting(null)
      setDeleted(true)
      await refresh()
      await queryClient.invalidateQueries({ queryKey: ['account-schedules'] })
    },
    onError: (error) => toast.error(error.message),
  })
  const renew = useMutation({
    mutationFn: ({ id, days }: { id: string; days: number }) => renewCredential(id, days),
    onSuccess: async () => {
      toast.success(t('Expiry updated.'))
      await refresh()
    },
    onError: (error) => toast.error(error.message),
  })
  const resume = useMutation({
    mutationFn: (id: string) => resumeCredential(id),
    onSuccess: refresh,
    onError: (error) => toast.error(error.message),
  })
  const now = Date.now()
  return (
    <>
      <Alert variant='destructive'>
        <AlertDescription>
          {t('These keys are stored in plaintext on the server. Delete any key you no longer use for checks right away, and revoke it at your provider.')}
        </AlertDescription>
      </Alert>
      {deleted && (
        <Alert>
          <AlertDescription>
            {t('Deleted from this site. Revoke the key at your provider as well, so it can no longer be used.')}
          </AlertDescription>
        </Alert>
      )}
      <Card>
        <CardContent className='flex flex-col gap-4'>
          {credentials.isPending && <Skeleton className='h-24' />}
          {credentials.data && credentials.data.length === 0 && (
            <p className='text-muted-foreground py-6 text-center text-sm'>
              {t('No saved keys. Turn on “Save to my account” under a check form’s key field, or add one below.')}
            </p>
          )}
          {!!credentials.data?.length && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Name')}</TableHead>
                  <TableHead>{t('Key')}</TableHead>
                  <TableHead>{t('Base URL')}</TableHead>
                  <TableHead>{t('Expires')}</TableHead>
                  <TableHead>{t('Last used')}</TableHead>
                  <TableHead>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {credentials.data.map((c) => {
                  const left = daysLeft(c, now)
                  return (
                    <TableRow key={c.id}>
                      <TableCell className='font-medium'>
                        <span className='text-muted-foreground mr-2 rounded border px-1.5 py-0.5 text-[10px] font-normal'>
                          {PROVIDER_LABEL[c.provider]}
                        </span>
                        {c.name}
                        {c.paused_reason && (
                          <span className='text-destructive ml-2 text-xs'>{t('Paused after repeated 401/403 answers')}</span>
                        )}
                      </TableCell>
                      <TableCell className='font-mono text-xs'>{c.hint}</TableCell>
                      <TableCell className='max-w-56 truncate text-xs' title={c.base_url}>
                        {hostOf(c.base_url)}
                      </TableCell>
                      <TableCell className={cn('text-xs tabular-nums', left < 3 && 'text-destructive font-semibold')}>
                        {t('{{count}} days left', { count: left })}
                      </TableCell>
                      <TableCell className='text-xs tabular-nums'>
                        {c.last_used_at ? new Date(c.last_used_at).toLocaleString() : t('Never')}
                        <span className='text-muted-foreground block'>{t('{{count}} checks', { count: c.use_count })}</span>
                      </TableCell>
                      <TableCell>
                        <div className='flex flex-wrap items-center gap-2'>
                          <NativeSelect
                            aria-label={t('Renew for')}
                            value={String(renewDays[c.id] ?? 7)}
                            onChange={(e) => setRenewDays((prev) => ({ ...prev, [c.id]: Number(e.target.value) }))}
                          >
                            {[1, 7, 30, 90].map((d) => (
                              <NativeSelectOption key={d} value={String(d)}>
                                {t('{{count}} days', { count: d })}
                              </NativeSelectOption>
                            ))}
                          </NativeSelect>
                          <Button size='sm' variant='outline' disabled={renew.isPending} onClick={() => renew.mutate({ id: c.id, days: renewDays[c.id] ?? 7 })}>
                            {t('Renew')}
                          </Button>
                          {c.paused_reason && (
                            <Button size='sm' variant='outline' disabled={resume.isPending} onClick={() => resume.mutate(c.id)}>
                              {t('Resume')}
                            </Button>
                          )}
                          <Button size='sm' variant='destructive' onClick={() => setDeleting(c)}>
                            {t('Delete')}
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
      <AddKeyCard />
      <Dialog
        open={!!deleting}
        onOpenChange={(open) => !open && setDeleting(null)}
        title={t('Delete this key?')}
        description={t('The key and every schedule that uses it are deleted. Revoke it at your provider as well.')}
      >
        <p className='font-mono text-xs'>{deleting && `${deleting.name} · ${deleting.hint}`}</p>
        <div className='flex justify-end gap-2'>
          <Button variant='outline' onClick={() => setDeleting(null)}>
            {t('Cancel')}
          </Button>
          <Button variant='destructive' disabled={remove.isPending} onClick={() => deleting && remove.mutate(deleting.id)}>
            {t('Delete')}
          </Button>
        </div>
      </Dialog>
    </>
  )
}

function AddKeyCard() {
  const { t } = useTranslation()
  const [provider, setProvider] = useState<Provider>('claude')
  const [baseUrl, setBaseUrl] = useState('')
  const [secret, setSecret] = useState('')
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Add a key')}</CardTitle>
        <CardDescription>{t('A saved key can only be sent to the Base URL it is saved with.')}</CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-4'>
        <HttpsNotice />
        <FieldGroup className='grid gap-4 sm:grid-cols-3'>
          <Field>
            <FieldLabel htmlFor='add-key-provider'>{t('Check type')}</FieldLabel>
            <NativeSelect id='add-key-provider' value={provider} onChange={(e) => setProvider(e.target.value as Provider)}>
              {(Object.keys(PROVIDER_LABEL) as Provider[]).map((p) => (
                <NativeSelectOption key={p} value={p}>
                  {PROVIDER_LABEL[p]}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </Field>
          <Field>
            <FieldLabel htmlFor='add-key-base'>Base URL</FieldLabel>
            <Input id='add-key-base' placeholder='https://api.example.com' value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} />
          </Field>
          <Field>
            <FieldLabel htmlFor='add-key-secret'>API Key</FieldLabel>
            <Input id='add-key-secret' type='password' autoComplete='new-password' value={secret} onChange={(e) => setSecret(e.target.value)} />
          </Field>
          <SaveKeyPanel
            provider={provider}
            baseUrl={baseUrl}
            secret={secret}
            onSaved={() => {
              setSecret('')
              setBaseUrl('')
            }}
          />
        </FieldGroup>
      </CardContent>
    </Card>
  )
}
