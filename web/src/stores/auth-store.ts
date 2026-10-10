import { useSyncExternalStore } from 'react'
import { getAuth, type Account, type AuthInfo } from '@/features/account/api'

// Who is using the site. Without an account every visitor is the anonymous
// 'guest' (history then belongs to the browser's owner cookie, as before).
// Signed in, the id changes, so every query keyed on it reloads.
export type AuthUser = { id: string; role: number; account?: Account }
type State = {
  auth: {
    user: AuthUser
    loaded: boolean
    https: boolean
    requireHttps: boolean
  }
}

const GUEST: AuthUser = { id: 'guest', role: 0 }
// Until the first answer the id is empty, so queries keyed on it wait
// instead of loading the anonymous list and then the account's.
let state: State = {
  auth: { user: { id: '', role: 0 }, loaded: false, https: true, requireHttps: true },
}
const listeners = new Set<() => void>()

function emit(next: State['auth']) {
  state = { auth: next }
  listeners.forEach((listener) => listener())
}

export function userFor(account: Account | null | undefined): AuthUser {
  return account ? { id: `u${account.id}`, role: 0, account } : GUEST
}

export function setAuthInfo(info: AuthInfo) {
  emit({
    user: userFor(info.user),
    loaded: true,
    https: info.https,
    requireHttps: info.require_https,
  })
}

export function setSignedIn(account: Account | null) {
  emit({ ...state.auth, user: userFor(account), loaded: true })
}

let pending: Promise<void> | undefined
export function refreshAuth(): Promise<void> {
  pending ??= getAuth()
    .then(setAuthInfo)
    .catch(() => emit({ ...state.auth, user: state.auth.loaded ? state.auth.user : GUEST, loaded: true }))
    .finally(() => {
      pending = undefined
    })
  return pending
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  if (!state.auth.loaded) void refreshAuth()
  return () => listeners.delete(listener)
}

export function useAuthStore<T>(selector: (value: State) => T): T {
  return selector(useSyncExternalStore(subscribe, () => state, () => state))
}

export function getAuthState(): State {
  return state
}
