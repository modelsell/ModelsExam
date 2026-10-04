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
import type { CheckHistoryStatus } from '../types'

// The badge is a verdict on ONE run. It is derived only from the run's status
// and its compatibility score, which counts assertions alone. Observations
// (latency, cache) and provenance results are shown in the report but never
// change a badge.
export type BadgeTier =
  | 'conformant'
  | 'mostly'
  | 'review'
  | 'incomplete'
  | 'running'
  | 'unscored'

export const BADGE_CONFORMANT_MIN = 100
export const BADGE_MOSTLY_MIN = 80

const INCOMPLETE_STATUSES: CheckHistoryStatus[] = [
  'stopped',
  'cancelled',
  'interrupted',
]

export function badgeTier(
  status: CheckHistoryStatus,
  score: number | null | undefined
): BadgeTier {
  if (status === 'running') return 'running'
  // A run that stopped early has not been fully checked: no verdict.
  if (INCOMPLETE_STATUSES.includes(status)) return 'incomplete'
  if (score == null) return 'unscored'
  if (score >= BADGE_CONFORMANT_MIN) return 'conformant'
  if (score >= BADGE_MOSTLY_MIN) return 'mostly'
  return 'review'
}
