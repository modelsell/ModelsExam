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
import type { CheckHistoryDetail, RunState } from '../types'
import { presentReport, presentProbe } from './report-presentation'
import { interruptRun } from './run-state'

// Historical reports use the same view state and components as a live run.
export function historyRunState(detail: CheckHistoryDetail): RunState {
  const state: RunState = {
    phase: 'complete',
    report: presentReport(detail.report),
    activeProbe: null,
    error: null,
  }
  if (detail.run.status === 'running')
    return {
      ...state,
      phase: 'running',
      activeProbe: presentProbe(detail.run.active_probe) || null,
    }
  if (
    detail.run.status === 'cancelled' ||
    detail.run.status === 'interrupted'
  ) {
    return interruptRun(
      state,
      detail.run.status === 'cancelled',
      detail.run.duration_ms,
      null
    )
  }
  return state
}
