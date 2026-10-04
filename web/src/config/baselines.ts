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
// Official baselines: the same exam run directly on the provider's own
// endpoint. A baseline is only shown as a result once `reportId` points to a
// real, public check report; until then the row says "reference run pending".
// Maintainers add the id after running the suite on the official endpoint.
// Never invent a result here.

export type BaselineProvider = 'anthropic' | 'bedrock' | 'openai' | 'openai-image'

export type OfficialBaseline = {
  id: string
  provider: BaselineProvider
  /** The official endpoint host the reference run is sent to. */
  endpoint: string
  /** Public check report id of the reference run. */
  reportId?: string
}

export const OFFICIAL_BASELINES: OfficialBaseline[] = [
  { id: 'anthropic', provider: 'anthropic', endpoint: 'api.anthropic.com' },
  { id: 'bedrock', provider: 'bedrock', endpoint: 'bedrock-runtime.<region>.amazonaws.com' },
  { id: 'openai', provider: 'openai', endpoint: 'api.openai.com' },
  { id: 'openai-image', provider: 'openai-image', endpoint: 'api.openai.com (images)' },
]
