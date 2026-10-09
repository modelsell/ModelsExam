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
import type { OpenAICheckOptions } from '../types'

// The plain fields, without cross-field rules: retests normalize saved
// options through them.
export const openAICheckFields = z.object({
  base_url: z.string().trim().max(2048),
  key: z.string().trim().max(8192),
  // A saved key replaces key (signed-in accounts).
  credential_id: z.string().max(64).optional(),
  model: z.string().trim().min(1).max(200),
  suite: z.enum(['basic', 'standard', 'full']),
  responses: z.boolean(),
  vision: z.boolean(),
  logprobs: z.boolean(),
  limit_param: z.enum(['max_completion_tokens', 'max_tokens']),
})
export const openAICheckSchema = openAICheckFields.superRefine((value, ctx) => {
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
  if (!value.credential_id && (value.key.length < 4 || /[\r\n]/.test(value.key)))
    ctx.addIssue({
      code: 'custom',
      path: ['key'],
      message: 'Enter a valid API key',
    })
})
export type OpenAICheckForm = z.infer<typeof openAICheckSchema>

export function openAICheckOptions(
  values: OpenAICheckForm
): OpenAICheckOptions {
  const full = values.suite === 'full'
  return {
    model: values.model,
    suite: values.suite,
    // The full suite always includes Responses and logprobs on the server.
    responses: full || values.responses,
    vision: values.vision,
    logprobs: full || values.logprobs,
    limit_param: values.limit_param,
  }
}

// Upper bound of upstream requests; mirrors pkg/openaicheck.MaxRequests.
export function openAIRequestBudget(
  options: Pick<
    OpenAICheckOptions,
    'suite' | 'responses' | 'vision' | 'logprobs'
  >
): number {
  const full = options.suite === 'full'
  const standard = options.suite !== 'basic'
  let total = 4 // models_list, chat_basic, chat_stream, error_shape
  if (standard) total += 11 // system, 4 inputs, 2 tools, tool round trip x2, 2 formats
  if (full) total += 4 // n, tool_choice none, tool stream, parallel tools
  if (full || options.logprobs) total += 1
  if (options.vision) total += 1
  if (full || options.responses) total += 8 // 5 single probes, round trip x2, structured
  if (full) total += 2 // previous_response_id
  return total
}
