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
import type { BaselineListItem, ClaudeCheckOptions } from '../types'
import { baselineSelectionOptions } from './baseline-selection'

export const modelCheckSchema = z
  .object({
    base_url: z.string().trim().max(2048),
    key: z.string().trim().max(8192),
    model: z.string().trim().min(1).max(200),
    cache: z.boolean(),
    thinking: z.boolean(),
    repeat: z.boolean(),
    vision: z.boolean(),
    pdf: z.boolean(),
    stream_comparison: z.boolean(),
    fingerprint: z.boolean().optional(),
    benchmark: z.boolean().optional(),
    performance: z.boolean().optional(),
    performance_tolerance: z.number().min(0).max(100).optional(),
    prompt_audit: z.boolean().optional(),
    bedrock: z.boolean().optional(),
    baseline_id: z.string().trim().max(80).optional(),
    baseline_type: z.string().trim().max(40).optional(),
    mode: z.enum(['endpoint', 'channel']),
  })
  .superRefine((value, ctx) => {
    if (!!value.baseline_id !== !!value.baseline_type)
      ctx.addIssue({
        code: 'custom',
        path: ['baseline_id'],
        message: 'Select a saved baseline for the chosen type',
      })
    if (value.mode !== 'endpoint') return
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
  })
export type ModelCheckForm = z.infer<typeof modelCheckSchema>

export function modelCheckOptions(
  values: ModelCheckForm,
  baseline?: BaselineListItem
): ClaudeCheckOptions {
  return {
    suite: 'focused',
    model: values.model,
    cache: values.cache,
    thinking: false,
    benchmark: true,
    performance: true,
    vision: values.vision,
    pdf: values.pdf,
    prompt_audit: !!values.prompt_audit,
    bedrock: !!values.bedrock,
    performance_tolerance: values.performance_tolerance ?? 25,
    ...baselineSelectionOptions(baseline),
  }
}

export const baselineTypeSchema = z.object({
  type: z
    .string()
    .trim()
    .min(1)
    .max(40)
    .regex(/^[\p{L}\p{N} _-]+$/u),
})
