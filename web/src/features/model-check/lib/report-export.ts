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
import { REPORT_SHEET_CSS } from './report-sheet-style'

const escapeHTML = (value: string) =>
  value.replace(
    /[&<>"']/g,
    (char) =>
      ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[
        char
      ]!
  )
export function reportHTML(markup: string, title: string, language: string) {
  // Markup must come from the React-rendered report, never raw report values.
  return `<!doctype html><html lang="${escapeHTML(language)}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src data:; base-uri 'none'; form-action 'none'"><title>${escapeHTML(title)}</title><style>${REPORT_SHEET_CSS}body{margin:0;padding:24px;background:#edf1f3}body>.mc-report{max-width:1120px;margin:auto}@media(max-width:760px){body{padding:8px}}@media print{body{padding:0}body>.mc-report{max-width:none}}</style></head><body>${markup}</body></html>`
}
export function captureReportHTML(
  element: HTMLElement,
  title: string,
  language: string
) {
  const clone = element.querySelector('.mc-report')?.cloneNode(true) as
    | HTMLElement
    | undefined
  if (!clone) throw new Error('Report document is unavailable')
  clone
    .querySelectorAll('style,script,button,iframe,input,link')
    .forEach((node) => node.remove())
  // Preserve expanded sections. HTML retains collapsed evidence for inspection;
  // printing follows the user's current detail visibility.
  return reportHTML(clone.outerHTML, title, language)
}
export function downloadReportHTML(html: string, id: string) {
  const url = URL.createObjectURL(
    new Blob([html], { type: 'text/html;charset=utf-8' })
  )
  const link = document.createElement('a')
  link.href = url
  link.download = `modelsexam-${id.replace(/[^a-zA-Z0-9_-]/g, '_')}.html`
  link.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
export function printReportHTML(html: string) {
  const frame = document.createElement('iframe')
  const url = URL.createObjectURL(
    new Blob([html], { type: 'text/html;charset=utf-8' })
  )
  frame.title = 'Model check report'
  frame.style.cssText =
    'position:fixed;width:1px;height:1px;bottom:0;right:0;border:0;'
  const cleanup = () => {
    frame.remove()
    URL.revokeObjectURL(url)
  }
  frame.onload = () => {
    const target = frame.contentWindow
    if (!target) {
      cleanup()
      return
    }
    target.addEventListener('afterprint', cleanup, { once: true })
    target.focus()
    target.print()
  }
  frame.onerror = cleanup
  frame.src = url
  document.body.appendChild(frame)
  setTimeout(cleanup, 120000)
}
