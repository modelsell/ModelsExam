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
import type {
  OpenAICheck,
  OpenAICheckEvent,
  OpenAICheckReport,
  OpenAICheckStatus,
  OpenAIRunState,
} from '../types'

export const INITIAL_OPENAI_RUN: OpenAIRunState = {
  phase: 'idle',
  report: null,
  markdown: null,
  activeProbe: null,
  error: null,
}

// Share of passed assertions among decided assertions, rounded half up. This
// mirrors pkg/openaicheck.Score so the live number matches the final report.
// Observations, inconclusive and skipped checks never move it.
export function openAIScore(checks: OpenAICheck[]): number | null {
  let pass = 0
  let fail = 0
  for (const check of checks) {
    if (check.kind !== 'assertion') continue
    if (check.status === 'pass') pass++
    if (check.status === 'fail') fail++
  }
  const decided = pass + fail
  if (decided === 0) return null
  return Math.floor((pass * 100 + Math.floor(decided / 2)) / decided)
}

export function summarizeOpenAI(report: OpenAICheckReport): OpenAICheckReport {
  const summary: Record<OpenAICheckStatus, number> = {
    pass: 0,
    fail: 0,
    inconclusive: 0,
    skipped: 0,
  }
  for (const check of report.checks) summary[check.status]++
  return { ...report, summary, score: openAIScore(report.checks) }
}

function normalize(report: OpenAICheckReport): OpenAICheckReport {
  return {
    ...report,
    checks: report.checks ?? [],
    samples: report.samples ?? [],
    plan: report.plan ?? [],
    score: report.score ?? null,
  }
}

export function applyOpenAIEvent(
  state: OpenAIRunState,
  event: OpenAICheckEvent
): OpenAIRunState {
  if (event.type === 'start')
    return {
      phase: 'running',
      report: normalize(event.report),
      markdown: null,
      activeProbe: null,
      error: null,
    }
  if (event.type === 'done')
    return {
      phase: event.report.cancelled ? 'cancelled' : 'complete',
      report: normalize(event.report),
      markdown: event.markdown ?? null,
      activeProbe: null,
      error: null,
    }
  if (!state.report)
    throw new Error('Progress arrived before the report started')
  if (event.type === 'probe_start')
    return { ...state, activeProbe: event.probe }
  if (event.type === 'sample')
    return {
      ...state,
      report: {
        ...state.report,
        samples: [...state.report.samples, event.sample],
      },
    }
  return {
    ...state,
    report: summarizeOpenAI({
      ...state.report,
      checks: [
        ...state.report.checks.filter((check) => check.id !== event.check.id),
        event.check,
      ],
    }),
  }
}

// interruptOpenAIRun closes a run that ended without a "done" event. Planned
// checks that never ran are recorded as skipped so the report stays complete.
export function interruptOpenAIRun(
  state: OpenAIRunState,
  cancelled: boolean,
  duration: number,
  error: string | null
): OpenAIRunState {
  let report = state.report
  if (report) {
    const finished = new Set(report.checks.map((check) => check.id))
    const missing: OpenAICheck[] = (report.plan ?? [])
      .filter((item) => !finished.has(item.id))
      .map((item) => {
        let code = 'interrupted'
        if (!item.selected) code = 'not_requested'
        else if (cancelled) code = 'cancelled'
        return {
          id: item.id,
          stage: item.stage,
          kind: item.kind,
          status: 'skipped' as const,
          code,
        }
      })
    report = summarizeOpenAI({
      ...report,
      cancelled: true,
      duration_ms: duration,
      checks: [...report.checks, ...missing],
    })
  }
  return {
    phase: cancelled ? 'cancelled' : 'error',
    report,
    markdown: null,
    activeProbe: null,
    error,
  }
}
