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
export type RouteName =
  | 'home'
  | 'records'
  | 'report'
  | 'baselines'
  | 'getbadge'
  | 'site'
  | 'method'
  | 'integrate'
  | 'login'
  | 'register'
  | 'account'
  | 'keys'
  | 'schedules'
  | 'notfound'

export type Route = { name: RouteName; id?: string }

export const ROUTE_PATHS = {
  home: '/',
  records: '/records',
  baselines: '/baselines',
  getbadge: '/get-badge',
  method: '/method',
  integrate: '/integrate',
  login: '/login',
  register: '/register',
  account: '/account',
  keys: '/keys',
  schedules: '/schedules',
} as const

// Pure URL parsing, kept free of React so it can be unit tested. The server
// (internal/seo) knows the same paths; keep the two lists in sync.
export function parseRoute(pathname: string, search = ''): Route {
  // Old shared links used /?history_id=<id>; they still open that report.
  const legacy = new URLSearchParams(search).get('history_id')
  if (legacy) return { name: 'report', id: legacy }
  const path = pathname.replace(/\/+$/, '') || '/'
  for (const name of Object.keys(ROUTE_PATHS) as Array<keyof typeof ROUTE_PATHS>) {
    if (ROUTE_PATHS[name] === path) return { name }
  }
  const site = /^\/sites\/([A-Za-z0-9.-]+)$/.exec(path)
  if (site) return { name: 'site', id: site[1].toLowerCase() }
  const match = /^\/reports\/([^/]+)$/.exec(path)
  if (match) {
    try {
      return { name: 'report', id: decodeURIComponent(match[1]) }
    } catch {
      return { name: 'notfound' }
    }
  }
  return { name: 'notfound' }
}

export function reportPath(id: string): string {
  return `/reports/${encodeURIComponent(id)}`
}

export function routePath(route: Route): string {
  if (route.name === 'report' && route.id) return reportPath(route.id)
  if (route.name === 'site' && route.id) return `/sites/${route.id}`
  if (route.name === 'notfound') return '/'
  return ROUTE_PATHS[route.name as keyof typeof ROUTE_PATHS] ?? '/'
}
