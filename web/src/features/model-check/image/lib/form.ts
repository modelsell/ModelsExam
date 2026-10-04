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
import type { ImageCheckOptions } from '../types'

export const imageCheckSchema = z
  .object({
    base_url: z.string().trim().max(2048),
    key: z.string().trim().max(8192),
    verify_key: z.string().trim().max(8192),
    model: z.string().trim().min(1).max(200),
    suite: z.enum(['basic', 'standard', 'full']),
    provenance: z.boolean(),
    baseline: z.boolean(),
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
    const needsVerify = value.provenance || value.baseline
    if (
      (needsVerify && value.verify_key.length < 4) ||
      /[\r\n]/.test(value.verify_key)
    )
      ctx.addIssue({
        code: 'custom',
        path: ['verify_key'],
        message: 'Enter an official OpenAI API key',
      })
  })

export type ImageCheckForm = z.infer<typeof imageCheckSchema>

export function imageCheckOptions(values: ImageCheckForm): ImageCheckOptions {
  return {
    model: values.model,
    suite: values.suite,
    provenance: values.provenance,
    baseline: values.baseline,
  }
}

export type ImageBudget = {
  requests: number
  images: number
  verifyCalls: number
}

// Estimate for the default capability profile (gpt-image family). The server
// computes the exact bound from the real profile and returns it in the report
// limits; this only sets expectations before the run starts.
export function imageBudget(
  options: Pick<ImageCheckOptions, 'suite' | 'provenance' | 'baseline'>
): ImageBudget {
  const std = options.suite !== 'basic'
  const full = options.suite === 'full'
  let requests = 0
  let images = 0
  let verifyCalls = 0
  const spend = (r: number, i: number, v: number) => {
    requests += r
    images += i
    verifyCalls += v
  }
  spend(1, 1, 0) // generate_basic (shared by shape, size and solid color)
  spend(1, 0, 0) // error_shape
  if (std) {
    spend(3, 3, 0) // split, centered, count
    spend(2, 2, 0) // output_format
    spend(full ? 3 : 2, full ? 3 : 2, 0) // size matrix
    spend(1, 1, 0) // background
    spend(1, 2, 0) // n=2
  }
  if (full) {
    spend(3, 3, 0) // quality levels
    spend(1, 1, 0) // stream
    spend(1, 1, 0) // invalid size
    spend(2, 2, 0) // edit, mask edit
  }
  // A baseline comparison implies the provenance stage on the server.
  if (options.provenance || options.baseline) {
    spend(0, 0, 2)
    if (full) spend(0, 0, 2)
  }
  if (options.baseline) spend(1, 1, 1)
  return { requests, images, verifyCalls }
}
