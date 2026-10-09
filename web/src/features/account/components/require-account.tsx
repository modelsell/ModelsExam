import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { buttonVariants } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Link } from '@/lib/router'
import { useAuthStore } from '@/stores/auth-store'

// Account pages show a sign-in prompt to visitors without an account.
export function RequireAccount({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const auth = useAuthStore((state) => state.auth)
  if (!auth.loaded) return <Skeleton className='mx-auto h-60 max-w-3xl rounded-xl' />
  if (!auth.user.account) {
    return (
      <div className='mx-auto flex max-w-md flex-col items-center gap-4 py-10 text-center'>
        <p className='text-muted-foreground text-sm'>{t('Sign in to use this page.')}</p>
        <Link to='/login' className={buttonVariants()}>
          {t('Sign in')}
        </Link>
      </div>
    )
  }
  return <>{children}</>
}
