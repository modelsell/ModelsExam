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
import type { CheckEvent, ClaudeCheckReport, RunState } from '../types'
import { getCheckPlan } from './check-plan'
import { presentEvent } from './report-presentation'

export { CHECK_IDS } from './check-plan'

export const INITIAL_RUN: RunState = {
  phase: 'idle',
  report: null,
  activeProbe: null,
  error: null,
}

export function summarize(report: ClaudeCheckReport): ClaudeCheckReport {
  const summary = { pass: 0, fail: 0, inconclusive: 0, skipped: 0 }
  for (const check of report.checks) summary[check.status]++
  return { ...report, summary }
}

export function applyCheckEvent(state: RunState, event: CheckEvent): RunState {
  event = presentEvent(event)
  if (event.type === 'start')
    return {
      phase: 'running',
      report: event.report,
      activeProbe: null,
      error: null,
    }
  if (event.type === 'done')
    return {
      phase: event.report.cancelled ? 'cancelled' : 'complete',
      report: event.report,
      activeProbe: null,
      error: null,
    }
  if (!state.report)
    throw new Error('Progress arrived before the report started')
  if (event.type === 'probe_start')
    return { ...state, activeProbe: event.probe }
  if (event.type === 'token_audit')
    return {
      ...state,
      report: { ...state.report, token_audit: event.token_audit },
    }
  if (event.type === 'fingerprint')
    return {
      ...state,
      report: { ...state.report, fingerprint: event.fingerprint },
    }
  if (event.type === 'benchmark')
    return { ...state, report: { ...state.report, benchmark: event.benchmark } }
  if (event.type === 'sample')
    return {
      ...state,
      report: {
        ...state.report,
        samples: [...state.report.samples, event.sample],
      },
    }
  if (event.type === 'check') {
    return {
      ...state,
      report: summarize({
        ...state.report,
        checks: [
          ...state.report.checks.filter((check) => check.id !== event.check.id),
          event.check,
        ],
      }),
    }
  }
  return state
}

export function interruptRun(
  state: RunState,
  cancelled: boolean,
  duration: number,
  error: string | null
): RunState {
  let report = state.report
  if (report) {
    const completed = new Set(report.checks.map((check) => check.id))
    report = summarize({
      ...report,
      cancelled: true,
      duration_ms: duration,
      checks: [
        ...report.checks,
        ...getCheckPlan(report)
          .filter((item) => !completed.has(item.id))
          .map((item) => ({
            id: item.id,
            status: 'skipped' as const,
            code: !item.selected
              ? 'not_requested'
              : cancelled
                ? 'cancelled'
                : 'interrupted',
          })),
      ],
    })
  }
  return {
    phase: cancelled ? 'cancelled' : 'error',
    report,
    activeProbe: null,
    error,
  }
}
