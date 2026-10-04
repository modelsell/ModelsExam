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

// Same rules as internal/badge.NormalizeDomain on the server: a public domain
// name, lower-case, without scheme, path or port. Returns null when invalid.
const LABEL = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/

export function normalizeDomain(input: string): string | null {
  let s = input.trim().toLowerCase()
  s = s.replace(/^https?:\/\//, '')
  s = s.split(/[/?#]/)[0]
  s = s.slice(s.lastIndexOf('@') + 1)
  const colon = s.lastIndexOf(':')
  if (colon >= 0) s = s.slice(0, colon)
  s = s.replace(/\.$/, '')
  if (!s || s.length > 253) return null
  const labels = s.split('.')
  if (labels.length < 2 || !labels.every((l) => LABEL.test(l))) return null
  if (/^[0-9]+$/.test(labels[labels.length - 1])) return null
  return s
}

const trim = (origin: string) => origin.replace(/\/+$/, '')

// auto follows the visitor's light or dark setting.
export type BadgeTheme = 'auto' | 'light' | 'dark'

export function scriptSnippet(origin: string, domain: string, theme: BadgeTheme = 'auto'): string {
  const attr = theme === 'auto' ? '' : ` data-theme="${theme}"`
  return `<script async src="${trim(origin)}/embed/${domain}.js"${attr}></script>`
}

// Optional: put this where the badge should sit. Without it the badge floats
// in the bottom-right corner.
export function slotSnippet(): string {
  return '<div id="modelsexam-badge"></div>'
}

const themeQuery = (theme: BadgeTheme) => (theme === 'auto' ? '' : `?theme=${theme}`)

export function imageSnippet(origin: string, domain: string, theme: BadgeTheme = 'auto'): string {
  const o = trim(origin)
  return `<a href="${o}/sites/${domain}" target="_blank" rel="noopener"><img src="${o}/badge/${domain}.svg${themeQuery(theme)}" alt="ModelsExam badge for ${domain}" height="28"></a>`
}

export function markdownSnippet(origin: string, domain: string, theme: BadgeTheme = 'auto'): string {
  const o = trim(origin)
  return `[![ModelsExam badge for ${domain}](${o}/badge/${domain}.svg${themeQuery(theme)})](${o}/sites/${domain})`
}
