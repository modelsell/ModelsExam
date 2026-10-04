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
import { useEffect, useMemo, useSyncExternalStore, type ComponentProps, type MouseEvent } from 'react'
import { parseRoute, type Route } from './route'

const NAV_EVENT = 'modelsexam:navigate'

function subscribe(callback: () => void) {
  window.addEventListener('popstate', callback)
  window.addEventListener(NAV_EVENT, callback)
  return () => {
    window.removeEventListener('popstate', callback)
    window.removeEventListener(NAV_EVENT, callback)
  }
}

const snapshot = () => window.location.pathname + window.location.search

export function useRoute(): Route {
  const current = useSyncExternalStore(subscribe, snapshot, () => '/')
  return useMemo(() => {
    const at = current.indexOf('?')
    return at < 0 ? parseRoute(current) : parseRoute(current.slice(0, at), current.slice(at))
  }, [current])
}

export function navigate(to: string, options?: { replace?: boolean }) {
  const url = new URL(to, window.location.href)
  const same = url.pathname + url.search === window.location.pathname + window.location.search
  if (options?.replace) window.history.replaceState(null, '', url)
  else if (!same) window.history.pushState(null, '', url)
  window.dispatchEvent(new Event(NAV_EVENT))
  // Land on the anchor if there is one, otherwise at the top of the new page.
  window.setTimeout(() => {
    const target = url.hash ? document.getElementById(url.hash.slice(1)) : null
    if (target) target.scrollIntoView()
    else if (!same || !url.hash) window.scrollTo(0, 0)
  }, 0)
}

// A real <a href>: crawlers and "open in new tab" work, and a plain click is
// handled in-app without a reload.
export function Link({ to, onClick, ...rest }: ComponentProps<'a'> & { to: string }) {
  const handle = (event: MouseEvent<HTMLAnchorElement>) => {
    onClick?.(event)
    if (
      event.defaultPrevented ||
      event.button !== 0 ||
      event.metaKey ||
      event.ctrlKey ||
      event.shiftKey ||
      event.altKey ||
      rest.target === '_blank'
    ) {
      return
    }
    event.preventDefault()
    navigate(to)
  }
  return <a {...rest} href={to} onClick={handle} />
}

// Old shared links (/?history_id=…) are rewritten to /reports/<id>.
export function useLegacyReportRedirect(route: Route) {
  useEffect(() => {
    if (route.name === 'report' && route.id && window.location.search.includes('history_id')) {
      navigate(`/reports/${encodeURIComponent(route.id)}`, { replace: true })
    }
  }, [route])
}
