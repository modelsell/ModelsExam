import { AuthForm } from '@/features/account/components/auth-forms'

export function LoginPage() {
  return (
    <div className='px-4 py-12 sm:px-6 sm:py-16'>
      <AuthForm mode='login' />
    </div>
  )
}

export function RegisterPage() {
  return (
    <div className='px-4 py-12 sm:px-6 sm:py-16'>
      <AuthForm mode='register' />
    </div>
  )
}
