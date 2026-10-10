import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Link, navigate } from '@/lib/router'
import { setSignedIn, useAuthStore } from '@/stores/auth-store'
import { login, register } from '../api'
import { passwordProblems, USERNAME_PATTERN } from '../lib/password'

export function HttpsNotice() {
  const { t } = useTranslation()
  const auth = useAuthStore((state) => state.auth)
  if (!auth.loaded || !auth.requireHttps || auth.https) return null
  return (
    <Alert variant='destructive'>
      <AlertDescription>
        {t('Signing in and saving keys need HTTPS, so passwords and keys never travel in plain text. Open this site over https:// to use an account. Checks without an account work as usual.')}
      </AlertDescription>
    </Alert>
  )
}

export function AuthForm({ mode }: { mode: 'login' | 'register' }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const auth = useAuthStore((state) => state.auth)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const problems = mode === 'register' && password ? passwordProblems(username, password) : []
  const blocked = auth.loaded && auth.requireHttps && !auth.https

  async function submit(event: FormEvent) {
    event.preventDefault()
    setError('')
    if (mode === 'register') {
      if (!USERNAME_PATTERN.test(username)) return setError(t('Username must be 3 to 32 letters, digits or underscores'))
      if (problems.length) return setError(t(problems[0]))
      if (password !== confirm) return setError(t('The two passwords do not match'))
    }
    setBusy(true)
    try {
      const result = mode === 'login' ? await login(username, password) : await register(username, password)
      setSignedIn(result.user)
      await queryClient.invalidateQueries()
      navigate('/')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const title = mode === 'login' ? t('Sign in') : t('Create account')
  return (
    <Card className='mx-auto w-full max-w-md'>
      <CardHeader>
        <CardTitle>
          <h1 className='font-serif text-2xl'>{title}</h1>
        </CardTitle>
        <CardDescription>
          {t('An account is optional. It lets you save test keys, retest with one click and run scheduled checks on the server.')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={submit} className='flex flex-col gap-5'>
          <HttpsNotice />
          {mode === 'register' && (
            <Alert>
              <AlertDescription>
                {t('There is no email and no password recovery. Remember your password: if it is lost, only the site operator can reset it, and that deletes every key saved in the account.')}
              </AlertDescription>
            </Alert>
          )}
          <FieldGroup className='flex flex-col gap-4'>
            <Field>
              <FieldLabel htmlFor='auth-username'>{t('Username')}</FieldLabel>
              <Input
                id='auth-username'
                autoComplete='username'
                value={username}
                maxLength={32}
                onChange={(event) => setUsername(event.target.value.trim())}
                required
              />
              {mode === 'register' && (
                <FieldDescription>{t('3 to 32 letters, digits or underscores.')}</FieldDescription>
              )}
            </Field>
            <Field>
              <FieldLabel htmlFor='auth-password'>{t('Password')}</FieldLabel>
              <Input
                id='auth-password'
                type='password'
                autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                required
              />
              {mode === 'register' && (
                <FieldDescription>
                  {t('At least 10 characters with 3 of: uppercase, lowercase, digits, symbols. Not your username, not a common password.')}
                </FieldDescription>
              )}
            </Field>
            {mode === 'register' && (
              <Field>
                <FieldLabel htmlFor='auth-confirm'>{t('Repeat password')}</FieldLabel>
                <Input
                  id='auth-confirm'
                  type='password'
                  autoComplete='new-password'
                  value={confirm}
                  onChange={(event) => setConfirm(event.target.value)}
                  required
                />
              </Field>
            )}
          </FieldGroup>
          {error && <FieldError>{error}</FieldError>}
          <Button type='submit' size='lg' disabled={busy || blocked || !username || !password}>
            {title}
          </Button>
          <p className='text-muted-foreground text-center text-xs'>
            {mode === 'login' ? (
              <>
                {t('No account yet?')}{' '}
                <Link to='/register' className='text-foreground underline underline-offset-4'>
                  {t('Create account')}
                </Link>
              </>
            ) : (
              <>
                {t('Already have an account?')}{' '}
                <Link to='/login' className='text-foreground underline underline-offset-4'>
                  {t('Sign in')}
                </Link>
              </>
            )}
          </p>
        </form>
      </CardContent>
    </Card>
  )
}
