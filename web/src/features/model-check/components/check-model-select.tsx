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
import { useEffect, useMemo, useRef, useState, type Ref } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'
import { fetchModelList } from '../api'
import {
  DEFAULT_MODELS,
  type ModelKind,
  type ModelListCode,
  type ModelListResult,
} from '../lib/default-models'

const COLLAPSED = 12
const DEBOUNCE_MS = 1200

type State =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'ok'; result: ModelListResult }
  | { status: 'error'; code: ModelListCode }

function validBase(raw: string): boolean {
  try {
    const u = new URL(raw.trim())
    return u.protocol === 'https:' || u.protocol === 'http:'
  } catch {
    return false
  }
}

// Model choice as buttons. The common models for the check type are shown (and
// the first one selected) straight away. Once a base URL and key are entered,
// the endpoint's own /v1/models list replaces them. If that cannot be read the
// defaults stay, and a model name can still be typed.
export function CheckModelSelect(props: {
  value: string
  onChange: (value: string) => void
  onBlur: () => void
  inputRef: Ref<HTMLInputElement>
  kind?: ModelKind
  // A fixed list (a configured channel) instead of the endpoint lookup.
  models?: string[]
  baseUrl?: string
  apiKey?: string
  id?: string
  disabled: boolean
  invalid: boolean
}) {
  const { t } = useTranslation()
  const { value, onChange } = props
  const kind = props.kind ?? 'claude'
  const defaults = DEFAULT_MODELS[kind]
  const [state, setState] = useState<State>({ status: 'idle' })
  const [expanded, setExpanded] = useState(false)
  const [query, setQuery] = useState('')
  const abort = useRef<AbortController | null>(null)
  // Once the visitor edits the name by hand, an empty box stays empty.
  const typed = useRef(false)
  useEffect(() => {
    typed.current = false
  }, [kind])
  const base = props.baseUrl ?? ''
  const key = props.apiKey ?? ''
  const canFetch = !props.models && validBase(base) && key.trim().length >= 4

  const load = async () => {
    abort.current?.abort()
    const ctl = new AbortController()
    abort.current = ctl
    setState({ status: 'loading' })
    try {
      const res = await fetchModelList(
        { kind, base_url: base.trim(), key: key.trim() },
        ctl.signal
      )
      if (ctl.signal.aborted) return
      setState(
        res.ok
          ? { status: 'ok', result: res.data }
          : { status: 'error', code: res.code }
      )
    } catch {
      // aborted by a newer request
    }
  }
  const loadRef = useRef(load)
  loadRef.current = load

  // Any change to the endpoint drops the old list and, once both fields are
  // filled in, reads the new one after a short pause.
  useEffect(() => {
    abort.current?.abort()
    setState({ status: 'idle' })
    if (!canFetch) return
    const timer = setTimeout(() => void loadRef.current(), DEBOUNCE_MS)
    return () => clearTimeout(timer)
  }, [base, key, kind, canFetch])
  useEffect(() => () => abort.current?.abort(), [])

  const live = state.status === 'ok' ? state.result.models : undefined
  const list = useMemo(
    () => props.models ?? live ?? defaults,
    [props.models, live, defaults]
  )
  const fromEndpoint = !!(props.models ?? live)

  // The first model is selected by default; a fetched list keeps the current
  // choice if it is on the list, otherwise moves to its first entry.
  useEffect(() => {
    if (!list.length) return
    if (!value) {
      if (!typed.current) onChange(list[0])
    }
    else if (live && !live.includes(value) && defaults.includes(value))
      onChange(live[0])
  }, [list, live, value, defaults, onChange])

  const shown = useMemo(() => {
    const q = query.trim().toLowerCase()
    const filtered = q ? list.filter((m) => m.toLowerCase().includes(q)) : list
    return expanded || q ? filtered : filtered.slice(0, COLLAPSED)
  }, [list, query, expanded])
  const hidden = !expanded && !query.trim() ? list.length - COLLAPSED : 0

  const note = (() => {
    if (state.status === 'loading') return t('Reading the model list…')
    if (state.status === 'ok')
      return state.result.filtered
        ? t('{{count}} models found on this endpoint.', {
            count: state.result.models.length,
          })
        : t(
            'This endpoint listed {{count}} models, none that look like this check type. Showing all of them.',
            { count: state.result.models.length }
          )
    if (state.status === 'error') {
      const why: Record<ModelListCode, string> = {
        auth: t('The endpoint rejected the key.'),
        not_found: t('The endpoint has no /v1/models list.'),
        url: t('Could not use that Base URL.'),
        unreachable: t('Could not reach the endpoint.'),
        status: t('The endpoint returned an error.'),
        format: t('The model list was not in a known format.'),
      }
      return `${why[state.code]} ${t('Showing common models instead; you can also type a model name.')}`
    }
    return canFetch || props.models
      ? ''
      : t('Enter the Base URL and API key to load this endpoint’s models. Common models are shown until then.')
  })()

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <p className='text-muted-foreground text-xs font-medium'>
          {fromEndpoint ? t('Models on this endpoint') : t('Common models')}
        </p>
        {!props.models && (
          <Button
            type='button'
            size='sm'
            variant='outline'
            disabled={!canFetch || props.disabled || state.status === 'loading'}
            onClick={() => void load()}
          >
            {state.status === 'loading' && <Spinner />}
            {t('Load model list')}
          </Button>
        )}
      </div>
      {list.length > COLLAPSED && (
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={t('Filter models')}
          aria-label={t('Filter models')}
          autoComplete='off'
          spellCheck={false}
          disabled={props.disabled}
        />
      )}
      <div
        role='radiogroup'
        aria-label={t('Model')}
        aria-invalid={props.invalid}
        className='flex flex-wrap gap-2'
      >
        {shown.map((model) => {
          const selected = model === value
          return (
            <button
              key={model}
              type='button'
              role='radio'
              aria-checked={selected}
              disabled={props.disabled}
              onClick={() => {
                typed.current = false
                onChange(model)
              }}
              className={cn(
                'focus-visible:ring-ring max-w-full rounded-md border px-2.5 py-1.5 font-mono text-xs break-all transition-colors focus-visible:ring-2 focus-visible:outline-none disabled:opacity-50',
                selected
                  ? 'border-foreground bg-foreground text-background'
                  : 'bg-card hover:bg-accent'
              )}
            >
              {model}
            </button>
          )
        })}
        {query.trim() && shown.length === 0 && (
          <p className='text-muted-foreground text-xs'>{t('No model matches.')}</p>
        )}
        {hidden > 0 && (
          <button
            type='button'
            onClick={() => setExpanded(true)}
            className='text-muted-foreground hover:text-foreground rounded-md border border-dashed px-2.5 py-1.5 text-xs'
          >
            {t('Show all {{count}}', { count: list.length })}
          </button>
        )}
      </div>
      {note && (
        <p
          role='status'
          className={cn(
            'text-xs leading-5',
            state.status === 'error' ? 'text-warning' : 'text-muted-foreground'
          )}
        >
          {note}
        </p>
      )}
      <div className='flex flex-col gap-1.5'>
        <label
          htmlFor={props.id ?? 'check-model'}
          className='text-muted-foreground text-xs'
        >
          {t('Or type a model name')}
        </label>
        <Input
          id={props.id ?? 'check-model'}
          ref={props.inputRef}
          value={value}
          onChange={(e) => {
            typed.current = true
            onChange(e.target.value)
          }}
          onBlur={props.onBlur}
          placeholder={t('Select or enter a model name')}
          aria-invalid={props.invalid}
          autoComplete='off'
          spellCheck={false}
          disabled={props.disabled}
        />
      </div>
    </div>
  )
}
