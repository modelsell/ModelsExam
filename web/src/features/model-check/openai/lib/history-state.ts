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
import type { HistoryDetail, OpenAIHistoryDetail } from '../../types'
import type { OpenAIRunState } from '../types'
import { interruptOpenAIRun } from './run-state'

export function isOpenAIHistory(
  detail: HistoryDetail
): detail is OpenAIHistoryDetail {
  return (
    detail.run.transport === 'openai_api' ||
    detail.run.transport === 'gemini_api'
  )
}

// A stored OpenAI run is shown by the same view as a live one. Reports saved
// mid-run (or interrupted) can lack arrays, so they are normalized first.
export function openAIHistoryState(
  detail: OpenAIHistoryDetail
): OpenAIRunState {
  const report = {
    ...detail.report,
    checks: detail.report.checks ?? [],
    samples: detail.report.samples ?? [],
    plan: detail.report.plan ?? [],
    score: detail.report.score ?? detail.run.score ?? null,
  }
  const state: OpenAIRunState = {
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
      ...interruptOpenAIRun(state, false, detail.run.duration_ms, null),
      markdown: state.markdown,
    }
  return state
}
