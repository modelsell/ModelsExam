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

export type SiteLabel = {
  /** Site name read from the tested site; the host name when none was found. */
  name: string
  /** The tested endpoint, without credentials or query. */
  url: string
  host: string
  description: string
}

// What a record says about the tested site. Older runs have no stored name or
// description, so the host name stands in for the name.
export function siteLabel(run: {
  channel_name?: string | null
  site_description?: string | null
  endpoint?: string | null
}): SiteLabel {
  const url = run.endpoint ?? ''
  let host = ''
  try {
    host = new URL(url).host
  } catch {
    host = ''
  }
  return {
    name: run.channel_name?.trim() || host || '—',
    url,
    host,
    description: run.site_description?.trim() ?? '',
  }
}
