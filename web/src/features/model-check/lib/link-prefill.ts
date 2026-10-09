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
// A check can be started from a link: /#base_url=…&key=…&model=…&type=openai
// fills in the form and the visitor presses Start themselves. The fragment is
// preferred because browsers never send it to the server; query parameters
// work too. Either way the values are removed from the address bar as soon as
// they are read, so the key does not stay in history or get shared onwards.

export type PrefillProvider = 'claude' | 'openai' | 'gemini' | 'image'

export type CheckPrefill = {
  provider?: PrefillProvider
  base_url?: string
  key?: string
  model?: string
  /** Form values of a retest (never from a link). */
  options?: Record<string, unknown>
  /** Move focus to the key field: a retest without a saved key. */
  focusKey?: boolean
}

const ALIASES: Record<'provider' | 'base_url' | 'key' | 'model', string[]> = {
  provider: ['type', 'provider', 'platform'],
  base_url: ['base_url', 'baseUrl', 'baseurl', 'base', 'url', 'endpoint'],
  key: ['key', 'api_key', 'apiKey', 'apikey', 'token'],
  model: ['model'],
}

const PROVIDERS: Record<string, PrefillProvider> = {
  claude: 'claude',
  anthropic: 'claude',
  openai: 'openai',
  gemini: 'gemini',
  google: 'gemini',
  image: 'image',
  images: 'image',
}

const MAX_LENGTH = 2048

function pick(params: URLSearchParams, names: string[]): string | undefined {
  for (const name of names) {
    const value = params.get(name)?.trim()
    if (value) return value.slice(0, MAX_LENGTH)
  }
  return undefined
}

function fromParams(params: URLSearchParams): CheckPrefill {
  const provider = PROVIDERS[pick(params, ALIASES.provider)?.toLowerCase() ?? '']
  return {
    provider,
    base_url: pick(params, ALIASES.base_url),
    key: pick(params, ALIASES.key),
    model: pick(params, ALIASES.model),
  }
}

// Fragment values win over query values, field by field.
export function parsePrefill(search: string, hash: string): CheckPrefill {
  const query = fromParams(new URLSearchParams(search))
  const fragment = fromParams(new URLSearchParams(hash.replace(/^#/, '')))
  const merged: CheckPrefill = {}
  for (const field of Object.keys(ALIASES) as Array<keyof typeof ALIASES>) {
    const value = fragment[field] ?? query[field]
    if (value) (merged as Record<string, string>)[field] = value
  }
  return merged
}

export function hasCredentials(prefill: CheckPrefill): boolean {
  return !!(prefill.base_url || prefill.key)
}

// The address without any prefill parameter; other parameters are kept. A
// fragment that only carried prefill values is dropped.
export function strippedLocation(pathname: string, search: string, hash: string): string {
  const names = Object.values(ALIASES).flat()
  const query = new URLSearchParams(search)
  for (const name of names) query.delete(name)
  const rest = query.toString()
  const fragment = hash.replace(/^#/, '')
  let tail = ''
  if (fragment && !fragment.includes('=')) tail = `#${fragment}`
  else if (fragment) {
    const params = new URLSearchParams(fragment)
    for (const name of names) params.delete(name)
    const left = params.toString()
    if (left) tail = `#${left}`
  }
  return pathname + (rest ? `?${rest}` : '') + tail
}

let consumed: CheckPrefill | undefined
let retest: CheckPrefill | undefined

// A retest from the records list hands its rebuilt form to the home page.
export const RETEST_EVENT = 'modelsexam:retest'

export function setRetestPrefill(prefill: CheckPrefill) {
  retest = prefill
  if (typeof window !== 'undefined') window.dispatchEvent(new Event(RETEST_EVENT))
}

export function takeRetestPrefill(): CheckPrefill | undefined {
  const value = retest
  retest = undefined
  return value
}

// Read the link once per page load (safe under StrictMode double renders) and
// clean the address bar. A link that only names a model is left as it is,
// since the model name is not secret.
export function consumePrefill(): CheckPrefill {
  if (consumed) return consumed
  if (typeof window === 'undefined') return {}
  const { pathname, search, hash } = window.location
  consumed = parsePrefill(search, hash)
  if (consumed.key || consumed.base_url || consumed.provider) {
    window.history.replaceState(window.history.state, '', strippedLocation(pathname, search, hash))
  }
  return consumed
}

// sk-abcd…wxyz: enough to recognise the key, not enough to use it.
export function maskKey(key: string): string {
  const value = key.trim()
  if (value.length <= 8) return '•'.repeat(Math.max(value.length, 4))
  const head = value.slice(0, Math.min(6, Math.floor(value.length / 4)))
  return `${head}••••${value.slice(-4)}`
}

// The link a relay puts in its console. Values go in the fragment so the key
// never reaches a server log; empty fields are left out.
export function buildPrefillLink(origin: string, prefill: CheckPrefill): string {
  const params = new URLSearchParams()
  if (prefill.provider && prefill.provider !== 'claude') params.set('type', prefill.provider)
  if (prefill.base_url?.trim()) params.set('base_url', prefill.base_url.trim())
  if (prefill.key?.trim()) params.set('key', prefill.key.trim())
  if (prefill.model?.trim()) params.set('model', prefill.model.trim())
  const fragment = params.toString()
  return `${origin.replace(/\/+$/, '')}/${fragment ? `#${fragment}` : ''}`
}
