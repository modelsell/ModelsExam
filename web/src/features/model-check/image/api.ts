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
import { t } from 'i18next'
import { getCommonHeaders } from '@/lib/api'
import { readImageStream } from './lib/stream'
import type {
  ImageCheckEvent,
  ImageCheckReport,
  ImageCheckTarget,
} from './types'

export async function streamImageCheck(
  target: ImageCheckTarget,
  signal: AbortSignal,
  onEvent: (event: ImageCheckEvent) => void
): Promise<void> {
  const response = await fetch('/api/model_check/image', {
    method: 'POST',
    credentials: 'include',
    cache: 'no-store',
    signal,
    headers: { ...getCommonHeaders(), Accept: 'text/event-stream' },
    body: JSON.stringify(target),
  })
  if (
    !response.ok ||
    !response.headers.get('content-type')?.includes('text/event-stream')
  ) {
    const data: { message?: string; detail?: string } = await response
      .json()
      .catch(() => ({}))
    const message = data.message ? t(data.message) : `HTTP ${response.status}`
    throw new Error(data.detail ? `${message}\n${data.detail}` : message)
  }
  if (!response.body) throw new Error('Progress stream is unavailable')
  await readImageStream(response.body, onEvent)
}

function saveBlob(content: string, type: string, name: string): void {
  const url = URL.createObjectURL(new Blob([content], { type }))
  const link = document.createElement('a')
  link.href = url
  link.download = name
  link.click()
  URL.revokeObjectURL(url)
}

// Neither key is in the report: the server redacts both before sending.
export function downloadImageReportJSON(report: ImageCheckReport): void {
  // Thumbnails are live-only previews; leave them out of the export.
  const images = report.images.map(({ thumb: _thumb, ...rest }) => rest)
  saveBlob(
    JSON.stringify({ ...report, images }, null, 2),
    'application/json',
    `image-check-${report.id}.json`
  )
}

export function downloadImageReportMarkdown(
  report: ImageCheckReport,
  markdown: string
): void {
  saveBlob(markdown, 'text/markdown', `image-check-${report.id}.md`)
}
