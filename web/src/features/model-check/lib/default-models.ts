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
// Models offered as one-click choices before (or without) a live list from the
// endpoint. These are common names, not a claim about what any site serves.
export type ModelKind = 'claude' | 'openai' | 'image'

export const DEFAULT_MODELS: Record<ModelKind, string[]> = {
  claude: [
    'claude-opus-5-5',
    'claude-sonnet-5-5',
    'claude-haiku-4-5-20251001',
    'claude-sonnet-4-5',
    'claude-opus-4-1',
  ],
  openai: ['gpt-5', 'gpt-5-mini', 'gpt-4.1', 'gpt-4o', 'gpt-4o-mini'],
  image: ['gpt-image-2', 'gpt-image-1', 'gpt-image-1-mini'],
}

// Error codes from POST /api/model_check/models.
export type ModelListCode =
  | 'url'
  | 'unreachable'
  | 'auth'
  | 'not_found'
  | 'status'
  | 'format'

export interface ModelListResult {
  models: string[]
  total: number
  filtered: boolean
}
