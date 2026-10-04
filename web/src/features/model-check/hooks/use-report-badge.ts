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
import { useQuery } from '@tanstack/react-query'
import { getCheckHistoryReport } from '../api'
import { badgeTier } from '../lib/badge'

// Looks up the badge of one public report by id, for baselines
// that point at a report. Returns undefined while loading or if it is gone.
export function useReportBadge(reportId?: string) {
  const query = useQuery({
    queryKey: ['model-check-report-badge', reportId],
    queryFn: () => getCheckHistoryReport(reportId!),
    enabled: !!reportId,
    retry: false,
    staleTime: 5 * 60_000,
  })
  const run = query.data?.run
  return {
    loading: !!reportId && query.isPending,
    missing: !!reportId && query.isError,
    badge: run ? { tier: badgeTier(run.status, run.score), score: run.score } : undefined,
  }
}
