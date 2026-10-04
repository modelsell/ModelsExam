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
import type { OpenAICheckEvent } from '../types'

const EVENT_TYPES = ['start', 'done', 'probe_start', 'sample', 'check']

// SSE frames may span UTF-8 chunks and may contain multiple data lines.
// Comment frames (": keep-alive") carry no data and are skipped.
export async function readOpenAIStream(
  stream: ReadableStream<Uint8Array>,
  onEvent: (event: OpenAICheckEvent) => void
): Promise<void> {
  const reader = stream.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let finished = false
  try {
    while (!finished) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      if (buffer.length > 4 * 1024 * 1024)
        throw new Error('Progress event exceeds size limit')
      let match: RegExpExecArray | null
      while ((match = /\r?\n\r?\n/.exec(buffer))) {
        const frame = buffer.slice(0, match.index)
        buffer = buffer.slice(match.index + match[0].length)
        const data = frame
          .split(/\r?\n/)
          .filter((line) => line.startsWith('data:'))
          .map((line) => line.slice(5).trimStart())
          .join('\n')
        if (!data) continue
        const event: OpenAICheckEvent = JSON.parse(data)
        if (!EVENT_TYPES.includes(event.type))
          throw new Error('Invalid progress event')
        onEvent(event)
        if (event.type === 'done') {
          finished = true
          break
        }
      }
    }
    if (!finished)
      throw new Error('Progress stream ended before the report completed')
  } finally {
    await reader.cancel().catch(() => undefined)
    reader.releaseLock()
  }
}
