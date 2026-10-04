import type {
  ClaudeCheckReport,
  ClaudeCheckSample,
  ComparisonBaseline,
} from '../types'
import { selectedReportBaseline } from './baseline-selection'
import { isCountProbe } from './check-plan'
import { median } from './report-metrics'
import { presentProbe } from './report-presentation'
import { totalInput } from './report-sheet'
import {
  isUsageTokenProbe,
  usageTokenCounters,
  usageTokenBaseline,
} from './usage-token-integrity'

export type BaselineLedgerCode =
  | 'matched'
  | 'no_baseline'
  | 'snapshot_unavailable'
  | 'no_sample'
  | 'parameters_differ'
  | 'reference_usage_unavailable'
  | 'current_usage_unavailable'

export function selectedLedgerBaseline(
  report: ClaudeCheckReport
): ComparisonBaseline | undefined {
  return selectedReportBaseline(report)
}

// Only independent repeats of the same question share a reference median.
// Cache write/read stages remain distinct even when their request is identical.
function question(probe: string): string {
  probe = presentProbe(probe)
  if (/^prompt_audit_(minimal_r[1-3]|floor_a(?:_r[1-3])?|floor_b)$/.test(probe))
    return 'prompt_audit_minimal'
  if (/^performance_\d+$/.test(probe)) return 'performance'
  if (/^fingerprint_.+_\d+$/.test(probe)) return probe.replace(/_\d+$/, '')
  return probe
}

function ledgerInput(sample: ClaudeCheckSample): number | null {
  if (isUsageTokenProbe(sample.probe))
    return usageTokenCounters(sample).total_input
  const value = totalInput(sample)
  const counts = [
    sample.usage.input_tokens,
    sample.usage.cache_creation_input_tokens,
    sample.usage.cache_read_input_tokens,
  ]
  return value !== null &&
    Number.isSafeInteger(value) &&
    counts.every((count) => Number.isSafeInteger(count))
    ? value
    : null
}

export function baselineLedgerComparison(
  report: ClaudeCheckReport,
  sample: ClaudeCheckSample
): {
  reference: number | null
  difference: number | null
  samples: number
  code: BaselineLedgerCode
} {
  const empty = (code: BaselineLedgerCode) => ({
    reference: null,
    difference: null,
    samples: 0,
    code,
  })
  if (!report.options?.baseline_id) return empty('no_baseline')
  const baseline = isUsageTokenProbe(sample.probe)
    ? usageTokenBaseline(report)
    : selectedLedgerBaseline(report)
  if (!baseline) return empty('snapshot_unavailable')
  if (isCountProbe(sample.probe)) return empty('no_sample')
  const candidates = baseline.samples.filter(
    (item) =>
      !isCountProbe(item.probe) &&
      question(item.probe) === question(sample.probe)
  )
  if (!candidates.length) return empty('no_sample')
  const profilePresent = isUsageTokenProbe(sample.probe)
    ? !!sample.request_profile?.trim()
    : !!sample.request_profile
  const matching = candidates.filter(
    (item) => profilePresent && item.request_profile === sample.request_profile
  )
  if (!matching.length) return empty('parameters_differ')
  const values = matching.flatMap((item) => {
    const value = ledgerInput(item)
    return value == null ? [] : [value]
  })
  const reference = median(values)
  if (reference == null) return empty('reference_usage_unavailable')
  const actual = ledgerInput(sample)
  return {
    reference,
    difference: actual == null ? null : actual - reference,
    samples: values.length,
    code: actual == null ? 'current_usage_unavailable' : 'matched',
  }
}
