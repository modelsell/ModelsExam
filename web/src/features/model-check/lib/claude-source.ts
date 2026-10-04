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
// Where a Claude check is sent. The three sources are checked with the same
// exam but are different things, so the form and the records name them
// separately: the vendor's own API, AWS Bedrock, or anything else (a relay).
export type ClaudeSource = 'official' | 'aws' | 'relay'

export const OFFICIAL_BASE_URL = 'https://api.anthropic.com'
export const AWS_BASE_URL_EXAMPLE = 'https://bedrock-mantle.us-east-1.api.aws'

export function sourceOfEndpoint(
  endpoint: string | null | undefined,
  transport?: string
): ClaudeSource {
  if (transport === 'bedrock_runtime') return 'aws'
  let host = ''
  try {
    host = new URL(endpoint ?? '').hostname.toLowerCase()
  } catch {
    return 'relay'
  }
  if (host === 'api.anthropic.com') return 'official'
  if (
    (host.startsWith('bedrock-mantle.') && host.endsWith('.api.aws')) ||
    (host.startsWith('bedrock-runtime.') && host.endsWith('.amazonaws.com'))
  )
    return 'aws'
  return 'relay'
}

// Badge colours: blue for the vendor, amber for AWS, neutral for the rest.
export const SOURCE_STYLE: Record<ClaudeSource, string> = {
  official:
    'border-[#b9cdf5] bg-[#e6eefc] text-[#1d4fb8] dark:border-[#24457f] dark:bg-[#0f2347] dark:text-[#8db4ff]',
  aws: 'border-[#f0cf9a] bg-[#fbefd6] text-[#8a4f00] dark:border-[#6b5116] dark:bg-[#33270a] dark:text-[#fbbf24]',
  relay: 'bg-muted text-muted-foreground',
}
