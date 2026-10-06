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
import z from 'zod'
import type { GeminiCheckOptions } from '../types'

// Mirrors pkg/geminicheck.ValidModel: an id that stays inside models/{model}.
export const GEMINI_MODEL_PATTERN = /^(models\/)?[A-Za-z0-9._-]+$/

export const geminiCheckSchema = z
  .object({
    base_url: z.string().trim().max(2048),
    key: z.string().trim().max(8192),
    model: z.string().trim().min(1).max(200),
    suite: z.enum(['basic', 'standard', 'full']),
    vision: z.boolean(),
  })
  .superRefine((value, ctx) => {
    try {
      const url = new URL(value.base_url)
      if (
        !['http:', 'https:'].includes(url.protocol) ||
        url.username ||
        url.password ||
        url.search ||
        url.hash
      )
        throw new Error('url')
    } catch {
      ctx.addIssue({
        code: 'custom',
        path: ['base_url'],
        message:
          'Enter a valid Base URL without credentials or query parameters',
      })
    }
    if (value.key.length < 4 || /[\r\n]/.test(value.key))
      ctx.addIssue({
        code: 'custom',
        path: ['key'],
        message: 'Enter a valid API key',
      })
    if (
      !GEMINI_MODEL_PATTERN.test(value.model) ||
      ['.', '..'].includes(value.model.replace(/^models\//, ''))
    )
      ctx.addIssue({
        code: 'custom',
        path: ['model'],
        message: 'Enter a model name',
      })
  })
export type GeminiCheckForm = z.infer<typeof geminiCheckSchema>

export function geminiCheckOptions(values: GeminiCheckForm): GeminiCheckOptions {
  return { model: values.model, suite: values.suite, vision: values.vision }
}

// Upper bound of upstream requests; mirrors pkg/geminicheck.MaxRequests.
export function geminiRequestBudget(
  options: Pick<GeminiCheckOptions, 'suite' | 'vision'>
): number {
  let total = 4 // model resource, generateContent, stream, error shape
  if (options.suite !== 'basic') total += 12 // system, 4 inputs, 2 tools, round trip x2, 2 formats, countTokens
  if (options.suite === 'full') total += 5 // candidates, thinking, NONE, streamed call, parallel calls
  if (options.vision) total += 1
  return total
}
