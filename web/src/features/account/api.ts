import { t } from 'i18next'
import { api } from '@/lib/api'

// Accounts are optional: without one the site works as before, with the
// browser's own owner cookie. Every call here sends that cookie and the
// session cookie; the server never returns a saved key's secret.

export type Account = { id: number; username: string; created_at: number }
export type AuthInfo = {
  user: Account | null
  /** This request reached the server over HTTPS. */
  https: boolean
  require_https: boolean
  /** Anonymous records of this browser that can move into the account. */
  claimable: number
}

export type Provider = 'claude' | 'openai' | 'gemini' | 'image'

export type Credential = {
  id: string
  name: string
  provider: Provider
  base_url: string
  hint: string
  expires_at: number
  acknowledged_at: number
  created_at: number
  last_used_at: number
  use_count: number
  paused_reason: string
}

export type Schedule = {
  id: string
  credential_id: string
  provider: Provider
  model: string
  options: Record<string, unknown>
  interval_minutes: number
  enabled: boolean
  next_run_at: number
  last_run_at: number
  last_run_id: string
  last_status: string
  last_score: number | null
  last_error: string
  created_at: number
  credential_hint: string
  credential_name: string
  base_url: string
}

export type RunningJob = {
  id: string
  model: string
  transport: string
  endpoint: string
  started_at: number
  active_probe: string
  scheduled: boolean
}

type Envelope<T> = {
  success: boolean
  data: T
  message?: string
  code?: string
  minutes?: number
}

export class ApiError extends Error {
  code?: string
  status?: number
  constructor(message: string, code?: string, status?: number) {
    super(message)
    this.code = code
    this.status = status
  }
}

// Server messages are English sentences used as i18n keys.
async function call<T>(run: () => Promise<{ data: Envelope<T> }>): Promise<T> {
  try {
    const { data } = await run()
    if (!data.success) throw new ApiError(t(data.message || 'Request failed'), data.code)
    return data.data
  } catch (error) {
    if (error instanceof ApiError) throw error
    const response = (error as { response?: { status?: number; data?: Partial<Envelope<T>> } })?.response
    const body = response?.data
    if (body?.message)
      throw new ApiError(t(body.message, { minutes: body.minutes }), body.code, response?.status)
    throw new ApiError(t('Request failed'), undefined, response?.status)
  }
}

export const getAuth = () => call<AuthInfo>(() => api.get('/api/auth/me'))
export const register = (username: string, password: string) =>
  call<{ user: Account; claimable: number }>(() => api.post('/api/auth/register', { username, password }))
export const login = (username: string, password: string) =>
  call<{ user: Account; claimable: number }>(() => api.post('/api/auth/login', { username, password }))
export const logout = () => call<void>(() => api.post('/api/auth/logout', {}))
export const changePassword = (old_password: string, new_password: string) =>
  call<void>(() => api.post('/api/auth/password', { old_password, new_password }))
export const claimRecords = () => call<{ moved: number }>(() => api.post('/api/auth/claim', {}))

export const listCredentials = () => call<Credential[]>(() => api.get('/api/credentials'))
export type NewCredential = {
  name?: string
  provider: Provider
  base_url: string
  secret: string
  expires_days: number
  ack_test_key: boolean
  ack_quota: boolean
  ack_plaintext: boolean
}
export const createCredential = (input: NewCredential) =>
  call<Credential>(() => api.post('/api/credentials', input))
export const renewCredential = (id: string, days: number) =>
  call<Credential>(() => api.post(`/api/credentials/${encodeURIComponent(id)}/renew`, { days }))
export const resumeCredential = (id: string) =>
  call<Credential>(() => api.post(`/api/credentials/${encodeURIComponent(id)}/resume`, {}))
export const deleteCredential = (id: string) =>
  call<void>(() => api.delete(`/api/credentials/${encodeURIComponent(id)}`))

export const listSchedules = () => call<Schedule[]>(() => api.get('/api/schedules'))
export const createSchedule = (input: {
  credential_id: string
  model: string
  options: Record<string, unknown>
  interval_minutes: number
}) => call<Schedule>(() => api.post('/api/schedules', input))
export const updateSchedule = (id: string, input: { enabled: boolean; interval_minutes: number }) =>
  call<Schedule>(() => api.patch(`/api/schedules/${encodeURIComponent(id)}`, input))
export const deleteSchedule = (id: string) =>
  call<void>(() => api.delete(`/api/schedules/${encodeURIComponent(id)}`))

export const startRetest = (input: {
  provider: Provider
  credential_id: string
  model: string
  options: Record<string, unknown>
}) => call<{ run_id: string }>(() => api.post('/api/jobs/retest', input))
export const listRunningJobs = () => call<RunningJob[]>(() => api.get('/api/jobs/running'))
