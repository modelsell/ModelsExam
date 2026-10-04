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
import type { HistoryDetail, ImageHistoryDetail } from '../../types'
import type { ImageRunState } from '../types'
import { interruptImageRun, normalizeImageReport } from './run-state'

export function isImageHistory(
  detail: HistoryDetail
): detail is ImageHistoryDetail {
  return detail.run.transport === 'image_api'
}

// A stored image run renders through the same view as a live one. History
// carries no thumbnails, so the gallery shows metadata only.
export function imageHistoryState(detail: ImageHistoryDetail): ImageRunState {
  const report = normalizeImageReport({
    ...detail.report,
    score: detail.report.score ?? detail.run.score ?? null,
  })
  const state: ImageRunState = {
    phase: report.cancelled ? 'cancelled' : 'complete',
    report,
    markdown: detail.markdown ?? null,
    activeProbe: null,
    error: null,
  }
  if (detail.run.status === 'running')
    return {
      ...state,
      phase: 'running',
      markdown: null,
      activeProbe: detail.run.active_probe || null,
    }
  if (detail.run.status === 'interrupted')
    return {
      ...interruptImageRun(state, false, detail.run.duration_ms, null),
      markdown: state.markdown,
    }
  return state
}
