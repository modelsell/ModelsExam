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
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@/lib/router'
import { REPO_URL } from '@/config/site'
import { setLanguage } from '@/i18n/config'
import { INTERFACE_LANGUAGE_OPTIONS } from '@/i18n/languages'
import type { RouteName } from '@/lib/route'
import { cn } from '@/lib/utils'
import { Moon, Sun } from 'lucide-react'
import { useState } from 'react'
import { readTheme, saveTheme, type Theme } from '@/lib/theme'
import { useAuthStore } from '@/stores/auth-store'
import { CredentialBanners } from '@/features/account/components/saved-keys'

// Mark: the shield with a tick, the same icon the badge carries.
function BrandMark() {
  return (
    <svg viewBox='0 0 16 16' className='size-8 shrink-0' aria-hidden='true'>
      <path
        d='M8 1.2 2.6 3.1v4.2c0 3.2 2.2 5.7 5.4 7.5 3.2-1.8 5.4-4.3 5.4-7.5V3.1L8 1.2Z'
        fill='none'
        strokeWidth='1.4'
        strokeLinejoin='round'
        className='stroke-foreground'
      />
      <path
        d='m5.5 8.1 1.8 1.8 3.3-3.6'
        fill='none'
        strokeWidth='1.7'
        strokeLinecap='round'
        strokeLinejoin='round'
        className='stroke-[#2457c5] dark:stroke-cobalt'
      />
    </svg>
  )
}

const navLink =
  'whitespace-nowrap text-muted-foreground hover:text-foreground focus-visible:ring-cobalt rounded-sm focus-visible:ring-2 focus-visible:outline-none aria-[current=page]:text-foreground aria-[current=page]:font-semibold'

function ThemeToggle() {
  const { t } = useTranslation()
  const [theme, setTheme] = useState<Theme>(readTheme)
  const next: Theme = theme === 'dark' ? 'light' : 'dark'
  return (
    <button
      type='button'
      aria-label={next === 'light' ? t('Switch to light theme') : t('Switch to dark theme')}
      title={next === 'light' ? t('Switch to light theme') : t('Switch to dark theme')}
      onClick={() => {
        saveTheme(next)
        setTheme(next)
      }}
      className='border-input text-muted-foreground hover:text-foreground focus-visible:ring-ring inline-flex size-8 shrink-0 items-center justify-center rounded-md border focus-visible:ring-2 focus-visible:outline-none'
    >
      {theme === 'dark' ? <Sun className='size-4' /> : <Moon className='size-4' />}
    </button>
  )
}

// Sign-in link, or the account's name linking to its pages.
function AccountLink(props: { current: RouteName }) {
  const { t } = useTranslation()
  const auth = useAuthStore((state) => state.auth)
  if (!auth.loaded) return null
  const account = auth.user.account
  const current = ['login', 'register', 'account', 'keys', 'schedules'].includes(props.current)
  return (
    <Link
      to={account ? '/account' : '/login'}
      className={cn(navLink, 'max-w-28 truncate')}
      aria-current={current ? 'page' : undefined}
      title={account?.username}
    >
      {account ? account.username : t('Sign in')}
    </Link>
  )
}

export function SiteLayout(props: { current: RouteName; children: ReactNode }) {
  const { t, i18n } = useTranslation()
  const signedIn = useAuthStore((state) => !!state.auth.user.account)
  const item = (name: RouteName, to: string, label: string, showFrom?: 'md' | 'lg') => (
    <Link
      to={to}
      className={cn(navLink, showFrom === 'md' && 'hidden md:inline', showFrom === 'lg' && 'hidden lg:inline')}
      aria-current={props.current === name ? 'page' : undefined}
    >
      {label}
    </Link>
  )
  return (
    <div className='bg-background text-foreground min-h-svh'>
      <a
        href='#main'
        className='bg-background focus-visible:ring-ring sr-only rounded-md px-3 py-2 text-sm focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus-visible:ring-2'
      >
        {t('Skip to content')}
      </a>
      <header className='bg-background/90 text-foreground sticky top-0 z-30 border-b backdrop-blur'>
        <div className='mx-auto flex max-w-6xl items-center justify-between gap-4 px-4 py-3 sm:px-6'>
          <Link to='/' className='flex shrink-0 items-center gap-2 sm:gap-3' aria-label='ModelsExam'>
            <BrandMark />
            <span className='flex flex-col leading-tight'>
              <span className='text-base tracking-tight whitespace-nowrap sm:text-lg'>
                <span className='font-light'>Models</span>
                <span className='font-extrabold'>Exam</span>
              </span>
              <span className='hidden font-mono text-[10px] tracking-wider text-muted-foreground uppercase text-xs sm:block'>
                {t('Independent API conformance testing')}
              </span>
            </span>
          </Link>
          <nav className='flex min-w-0 items-center gap-3 text-sm sm:gap-6' aria-label={t('Main')}>
            {item('home', '/#new-check', t('Start check'), 'lg')}
            {item('baselines', '/baselines', t('Official baselines'), 'lg')}
            {item('getbadge', '/#get-badge', t('Get your badge'), 'lg')}
            {item('records', '/records', t('My check records'), 'lg')}
            {item('method', '/method', t('Method and independence'), 'lg')}
            <AccountLink current={props.current} />
            <ThemeToggle />
            <select
              aria-label={t('Language')}
              className='border-input bg-background w-16 rounded-md border px-1.5 py-1 text-xs sm:w-auto sm:px-2'
              value={i18n.resolvedLanguage}
              onChange={(e) => void setLanguage(e.target.value)}
            >
              {INTERFACE_LANGUAGE_OPTIONS.map((o) => (
                <option key={o.code} value={o.code}>
                  {o.label}
                </option>
              ))}
            </select>
          </nav>
        </div>
      </header>
      <main id='main'>
        {signedIn && <CredentialBanners />}
        {props.children}
      </main>
      <footer className='bg-background border-t px-4 py-12 sm:px-6'>
        <div className='mx-auto grid max-w-6xl gap-8 sm:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]'>
          <div className='flex flex-col gap-3'>
            <p className='text-lg tracking-tight'>
              <span className='font-light'>Models</span>
              <span className='font-extrabold'>Exam</span>
            </p>
            <p className='max-w-xl text-xs leading-5 text-muted-foreground'>
              {t(
                'Check records are private to the browser that ran them. Results describe the endpoint and model tested at the time of the run; they are not rankings, certifications or endorsements.'
              )}
            </p>
            <p className='max-w-xl text-xs leading-5 text-muted-foreground'>
              {t(
                'ModelsExam is an independent open-source project (AGPL-3.0). It is not affiliated with Anthropic, OpenAI or Amazon Web Services.'
              )}
            </p>
          </div>
          <nav aria-label={t('Footer')} className='flex flex-col gap-2 text-sm sm:items-end'>
            <Link to='/baselines' className={navLink}>{t('Official baselines')}</Link>
            <Link to='/#get-badge' className={navLink}>{t('Get your badge')}</Link>
            <Link to='/records' className={navLink}>{t('My check records')}</Link>
            <Link to='/method' className={navLink}>{t('Method and independence')}</Link>
            <Link to='/integrate' className={navLink}>{t('Relay integration')}</Link>
            {REPO_URL && (
              <a href={REPO_URL} className={navLink} rel='noopener noreferrer'>
                {t('Source code')}
              </a>
            )}
          </nav>
        </div>
      </footer>
    </div>
  )
}
