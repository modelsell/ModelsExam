import type { Credential, Provider } from '../api'

const DAY = 24 * 3600 * 1000

export function daysLeft(c: Pick<Credential, 'expires_at'>, now = Date.now()): number {
  return Math.max(0, Math.floor((c.expires_at - now) / DAY))
}

export function hostOf(url: string): string {
  try {
    return new URL(url).host
  } catch {
    return url
  }
}

// A key that may be used for a new check of this kind right now.
export function usableFor(c: Credential, provider: Provider, now = Date.now()): boolean {
  return c.provider === provider && c.expires_at > now && !c.paused_reason
}

// Mirrors the server's base URL normalizers (normalizeClaudeURL,
// openaicheck.NormalizeBaseURL, geminicheck.NormalizeBaseURL), so a stored
// endpoint can be matched against a saved key's base URL. Null when invalid.
export function normalizeBaseURL(provider: Provider, raw: string): string | null {
  let url: URL
  try {
    url = new URL(raw.trim())
  } catch {
    return null
  }
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash)
    return null
  let path = url.pathname.replace(/\/+$/, '')
  if (provider === 'claude') {
    // Older reports kept the full request URL.
    path = path.replace(/\/v1\/messages\/count_tokens$/, '').replace(/\/v1\/messages$/, '').replace(/\/v1$/, '')
  } else if (provider === 'gemini') {
    const at = path.indexOf('/models')
    if (at >= 0) path = path.slice(0, at)
    path = path.replace(/\/(v1beta|v1alpha|v1)$/, '')
  } else {
    path = path.replace(/\/(chat\/completions|responses|models)$/, '').replace(/\/v1$/, '')
  }
  return `${url.protocol}//${url.host}${path}`.replace(/\/+$/, '')
}

export function findCredential(
  credentials: Credential[],
  provider: Provider,
  baseURL: string,
  now = Date.now()
): Credential | undefined {
  const target = normalizeBaseURL(provider, baseURL)
  if (!target) return undefined
  return credentials.find(
    (c) => usableFor(c, provider, now) && normalizeBaseURL(provider, c.base_url)?.toLowerCase() === target.toLowerCase()
  )
}
