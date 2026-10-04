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
  ImageCheck,
  ImageCheckEvent,
  ImageCheckReport,
  ImageCheckStatus,
  ImageRunState,
} from '../types'

export const INITIAL_IMAGE_RUN: ImageRunState = {
  phase: 'idle',
  report: null,
  markdown: null,
  activeProbe: null,
  error: null,
}

// Share of passed assertions among decided assertions, rounded half up. Mirrors
// pkg/imagecheck.Score: observations and provenance checks never move it.
export function imageScore(checks: ImageCheck[]): number | null {
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

export function summarizeImage(report: ImageCheckReport): ImageCheckReport {
  const summary: Record<ImageCheckStatus, number> = {
    pass: 0,
    fail: 0,
    inconclusive: 0,
    skipped: 0,
  }
  for (const check of report.checks) summary[check.status]++
  return { ...report, summary, score: imageScore(report.checks) }
}

export function normalizeImageReport(
  report: ImageCheckReport
): ImageCheckReport {
  return {
    ...report,
    checks: report.checks ?? [],
    samples: report.samples ?? [],
    images: report.images ?? [],
    plan: report.plan ?? [],
    score: report.score ?? null,
  }
}

export function applyImageEvent(
  state: ImageRunState,
  event: ImageCheckEvent
): ImageRunState {
  if (event.type === 'start')
    return {
      phase: 'running',
      report: normalizeImageReport(event.report),
      markdown: null,
      activeProbe: null,
      error: null,
    }
  if (event.type === 'done')
    return {
      phase: event.report.cancelled ? 'cancelled' : 'complete',
      report: normalizeImageReport(event.report),
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
    report: summarizeImage({
      ...state.report,
      checks: [
        ...state.report.checks.filter((check) => check.id !== event.check.id),
        event.check,
      ],
    }),
  }
}

// Closes a run that ended without a "done" event; unrun planned checks are
// recorded as skipped so the report stays complete.
export function interruptImageRun(
  state: ImageRunState,
  cancelled: boolean,
  duration: number,
  error: string | null
): ImageRunState {
  let report = state.report
  if (report) {
    const finished = new Set(report.checks.map((check) => check.id))
    const missing: ImageCheck[] = (report.plan ?? [])
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
    report = summarizeImage({
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
