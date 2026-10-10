import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Link, navigate } from '@/lib/router'
import { setSignedIn, useAuthStore } from '@/stores/auth-store'
import { changePassword, logout } from '@/features/account/api'
import { HttpsNotice } from '@/features/account/components/auth-forms'
import { RequireAccount } from '@/features/account/components/require-account'
import { passwordProblems } from '@/features/account/lib/password'

export function AccountPage() {
  return (
    <div className='mx-auto flex max-w-3xl flex-col gap-6 px-4 py-12 sm:px-6 sm:py-16'>
      <RequireAccount>
        <AccountContent />
      </RequireAccount>
    </div>
  )
}

function AccountContent() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const auth = useAuthStore((state) => state.auth)
  const account = auth.user.account!
  const signOut = useMutation({
    mutationFn: logout,
    onSuccess: async () => {
      setSignedIn(null)
      await queryClient.invalidateQueries()
      navigate('/')
    },
  })
  return (
    <>
      <header className='flex flex-wrap items-end justify-between gap-4'>
        <div className='flex flex-col gap-1'>
          <h1 className='font-serif text-3xl font-semibold tracking-tight'>{t('My account')}</h1>
          <p className='text-muted-foreground text-sm'>
            {account.username} · {t('Created {{date}}', { date: new Date(account.created_at).toLocaleDateString() })}
          </p>
        </div>
        <div className='flex flex-wrap gap-2 text-sm'>
          <Link to='/keys' className='underline underline-offset-4'>{t('My keys')}</Link>
          <Link to='/schedules' className='underline underline-offset-4'>{t('Scheduled checks')}</Link>
          <Link to='/records' className='underline underline-offset-4'>{t('My check records')}</Link>
        </div>
      </header>
      <PasswordCard />
      <Card>
        <CardHeader>
          <CardTitle>{t('Sign out')}</CardTitle>
          <CardDescription>{t('Ends the session in this browser.')}</CardDescription>
        </CardHeader>
        <CardContent>
          <Button variant='outline' disabled={signOut.isPending} onClick={() => signOut.mutate()}>
            {t('Sign out')}
          </Button>
        </CardContent>
      </Card>
    </>
  )
}

function PasswordCard() {
  const { t } = useTranslation()
  const [oldPassword, setOld] = useState('')
  const [newPassword, setNew] = useState('')
  const [error, setError] = useState('')
  const change = useMutation({
    mutationFn: () => changePassword(oldPassword, newPassword),
    onSuccess: () => {
      setOld('')
      setNew('')
      toast.success(t('Password changed. Other sessions were signed out.'))
    },
    onError: (err) => setError(err.message),
  })
  const submit = (event: FormEvent) => {
    event.preventDefault()
    setError('')
    const problems = passwordProblems(newPassword)
    if (problems.length) return setError(t(problems[0]))
    change.mutate()
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Change password')}</CardTitle>
        <CardDescription>{t('Changing the password signs out every other session.')}</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={submit} className='flex flex-col gap-4'>
          <HttpsNotice />
          <FieldGroup className='grid gap-4 sm:grid-cols-2'>
            <Field>
              <FieldLabel htmlFor='old-password'>{t('Current password')}</FieldLabel>
              <Input id='old-password' type='password' autoComplete='current-password' value={oldPassword} onChange={(e) => setOld(e.target.value)} />
            </Field>
            <Field>
              <FieldLabel htmlFor='new-password'>{t('New password')}</FieldLabel>
              <Input id='new-password' type='password' autoComplete='new-password' value={newPassword} onChange={(e) => setNew(e.target.value)} />
            </Field>
          </FieldGroup>
          {error && <FieldError>{error}</FieldError>}
          <Alert>
            <AlertDescription>{t('There is no password recovery. Keep the new password somewhere safe.')}</AlertDescription>
          </Alert>
          <Button type='submit' className='w-fit' disabled={change.isPending || !oldPassword || !newPassword}>
            {t('Change password')}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
