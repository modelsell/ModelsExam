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
  CheckPlanItem,
  ClaudeCheckOptions,
  ClaudeCheckReport,
} from '../types'
import { presentPlanItem, presentProbe } from './report-presentation'

export const CHECK_IDS = [
  'basic',
  'system',
  'stream',
  'tool',
  'max_tokens',
  'stop_sequence',
  'multi_turn',
  'error_validation',
  'thinking',
  'signature_replay',
  'cache',
  'source',
  'token_count',
  'billing',
  'prompt_integrity',
] as const

export const STAGES = [
  'connection',
  'protocol',
  'capabilities',
  'reliability',
  'boundaries',
] as const

export function createCheckPlan(
  options?: ClaudeCheckOptions,
  version = 20
): CheckPlanItem[] {
  if (options?.suite === 'focused') {
    const plan: CheckPlanItem[] = [
      { id: 'basic', stage: 'identity', kind: 'assertion', selected: true },
      {
        id: 'model_consistency',
        stage: 'identity',
        kind: 'observation',
        selected: true,
      },
      {
        id: 'performance_sampling',
        stage: 'performance',
        kind: 'boundary',
        selected: true,
      },
      ...(version >= 19
        ? [
            {
              id: 'usage_token_integrity',
              stage: 'integrity',
              kind: 'boundary' as const,
              selected: true,
            },
          ]
        : []),
      {
        id: 'capability_benchmark',
        stage: 'identity',
        kind: 'boundary',
        selected: true,
      },
    ]
    if (version < 17 && options.fingerprint)
      plan.push({
        id: 'behavior_fingerprint',
        stage: 'identity',
        kind: 'boundary',
        selected: true,
      })
    if (options.cache)
      plan.push(
        ...(version < 17
          ? [
              {
                id: 'cache',
                stage: 'integrity',
                kind: 'observation' as const,
                selected: true,
              },
            ]
          : []),
        {
          id: 'cache_token_audit',
          stage: 'integrity',
          kind: 'boundary',
          selected: true,
        }
      )
    if (options.prompt_audit)
      plan.push({
        id: 'prompt_integrity',
        stage: 'integrity',
        kind: 'boundary',
        selected: true,
      })
    for (const id of ['vision', 'pdf'] as const)
      if (options[id])
        plan.push({ id, stage: 'identity', kind: 'assertion', selected: true })
    if (version !== 17 && options.bedrock) plan.push(...bedrockPlan(version))
    return plan
  }
  const plan: CheckPlanItem[] = []
  const add = (
    stage: string,
    kind: CheckPlanItem['kind'],
    selected: boolean,
    ids: string[]
  ) => {
    plan.push(...ids.map((id) => ({ id, stage, kind, selected })))
  }
  add('connection', 'assertion', true, ['basic'])
  add('connection', 'observation', true, [
    'model_echo',
    'source',
    'token_count',
    'cold_cache',
  ])
  add('protocol', 'assertion', true, [
    'system',
    'stream',
    'tool',
    'max_tokens',
    'stop_sequence',
    'multi_turn',
    version < 3 ? 'error_validation' : 'zero_output',
  ])
  add('protocol', 'observation', true, ['error_shape'])
  add(
    version >= 5 ? 'capabilities' : 'protocol',
    'assertion',
    !!options?.vision,
    ['vision']
  )
  if (version >= 5) add('capabilities', 'assertion', !!options?.pdf, ['pdf'])
  add('capabilities', 'assertion', !!options?.thinking, [
    'thinking',
    'thinking_stream',
    'signature_replay',
  ])
  add('capabilities', 'observation', !!options?.cache, ['cache'])
  if (version >= 8)
    add('reliability', 'boundary', !!options?.cache, ['cache_token_audit'])
  add('reliability', 'observation', !!options?.repeat, ['repeatability'])
  add('reliability', 'assertion', true, ['usage_fields', 'aws_usage'])
  if (version >= 6) add('reliability', 'assertion', true, ['model_consistency'])
  if (version >= 5) {
    add('reliability', 'assertion', !!options?.stream_comparison, [
      'stream_comparison',
      'stream_stop_reason',
    ])
  } else if (version >= 3) {
    const selected = !!(
      options?.veridrop ||
      options?.pdf ||
      options?.stream_comparison
    )
    add('capabilities', 'assertion', selected, ['pdf'])
    add('protocol', 'assertion', selected, ['structured_tool'])
    add('reliability', 'assertion', selected, ['stream_comparison'])
    add('reliability', 'observation', selected, ['reference_baseline'])
  }
  add('boundaries', 'boundary', true, ['billing', 'prompt_integrity'])
  if (version >= 7 && options?.benchmark)
    add('reliability', 'boundary', true, ['capability_benchmark'])
  if (version >= 7 && options?.performance)
    add('reliability', 'boundary', true, ['performance_sampling'])
  if (version >= 7 && version < 17 && options?.fingerprint)
    add('reliability', 'boundary', true, ['behavior_fingerprint'])
  if (version !== 17 && options?.bedrock) plan.push(...bedrockPlan(version))
  return plan
}

export function getCheckPlan(
  report?: ClaudeCheckReport | null,
  preview?: ClaudeCheckOptions
): CheckPlanItem[] {
  if (report?.plan?.length) return report.plan.map(presentPlanItem)
  const plan = createCheckPlan(
    report?.options ?? preview,
    report?.version ?? 20
  )
  if (!report || report.version >= 2) return plan
  const legacy = new Set<string>(CHECK_IDS)
  // Version 1 had no persisted plan. Keep its original scope, including
  // explicitly returned unknown checks, without inventing version 2 results.
  const ids = new Set([...legacy, ...report.checks.map((check) => check.id)])
  return [...ids].map((id) => {
    const item = plan.find((item) => item.id === id) ?? {
      id,
      stage: 'boundaries',
      kind: 'observation' as const,
      selected: true,
    }
    const check = report.checks.find((check) => check.id === id)
    return {
      ...item,
      selected: check ? check.code !== 'not_requested' : item.selected,
    }
  })
}

export function probeCheckID(probe?: string | null): string | null {
  if (!probe) return null
  probe = presentProbe(probe)
  if (probe.startsWith('comparison_')) return 'stream_comparison'
  if (probe.startsWith('prompt_audit_')) return 'prompt_integrity'
  if (probe.startsWith('cache_')) return 'cache'
  if (probe.startsWith('repeat_')) return 'repeatability'
  if (probe.startsWith('fingerprint_')) return 'behavior_fingerprint'
  if (probe.startsWith('benchmark_')) return 'capability_benchmark'
  if (probe.startsWith('performance_')) return 'performance_sampling'
  if (probe.startsWith('usage_tokens_')) return 'usage_token_integrity'
  return probe
}

export function samplesForCheck(report: ClaudeCheckReport | null, id: string) {
  return (
    report?.samples.filter((sample) => {
      if (['system', 'source', 'model_echo', 'cold_cache'].includes(id))
        return (
          sample.probe ===
          (report.checks.find((check) => check.id === id)?.evidence
            ?.baseline_probe ||
            report.baseline_probe ||
            'basic')
        )
      if (id === 'error_shape')
        return ['zero_output', 'error_validation'].includes(sample.probe)
      if (['usage_fields', 'aws_usage', 'model_consistency'].includes(id))
        return sample.http_status === 200 && !isCountProbe(sample.probe)
      if (id === 'cache_token_audit') return sample.probe.startsWith('cache_')
      if (id === 'stream_stop_reason')
        return probeCheckID(sample.probe) === 'stream_comparison'
      return probeCheckID(sample.probe) === id
    }) ?? []
  )
}

export function requestBudget(
  options: ClaudeCheckOptions,
  version = 20
): number {
  const cacheRequests = version >= 10 ? 8 : 3
  let promptRequests = version >= 10 ? 18 : 6
  if (version >= 12) promptRequests = 3
  if (options.suite === 'focused')
    return (
      (version >= 16 ? 11 : version >= 15 ? 17 : 10) +
      (version >= 19 ? 4 : 0) +
      (version < 17 && options.fingerprint ? 20 : 0) +
      (options.cache ? cacheRequests : 0) +
      (options.prompt_audit ? promptRequests : 0) +
      (options.vision ? 1 : 0) +
      (options.pdf ? 1 : 0) +
      (version !== 17 && options.bedrock ? bedrockPlan(version).length : 0)
    )

  return (
    8 +
    (options.thinking ? 3 : 0) +
    (options.cache ? 3 : 0) +
    (options.repeat ? 3 : 0) +
    (options.vision ? 1 : 0) +
    (options.pdf || options.veridrop ? 1 : 0) +
    (options.stream_comparison || options.veridrop ? 2 : 0) +
    (version < 17 && options.fingerprint ? 40 : 0) +
    (options.benchmark ? (version >= 16 ? 7 : 12) : 0) +
    (options.performance ? 3 : 0) +
    (options.prompt_audit ? 6 : 0) +
    (version !== 17 && options.bedrock ? bedrockPlan(version).length : 0)
  )
}

export function isCountProbe(probe: string): boolean {
  return (
    probe === 'token_count' ||
    (probe.startsWith('prompt_audit_') && probe.endsWith('_count'))
  )
}

function bedrockPlan(version: number): CheckPlanItem[] {
  return [
    'bedrock_role',
    'bedrock_beta',
    'bedrock_web_search',
    'bedrock_web_fetch',
    'bedrock_code_execution',
    'bedrock_advisor',
    'bedrock_sampling',
    ...(version >= 20 ? ['bedrock_signature'] : []),
  ].map((id) => ({ id, stage: 'bedrock', kind: 'observation', selected: true }))
}

// Main report and check cards show scoring targets; raw historical requests
// remain in the ledger even when their diagnostic category has been retired.
export const SCORED_CHECK_IDS = new Set([
  'basic',
  'model_consistency',
  'model_echo',
  'stream',
  'tool',
  'structured_tool',
  'max_tokens',
  'stop_sequence',
  'vision',
  'pdf',
  'capability_benchmark',
  'performance_sampling',
  'usage_token_integrity',
  'cache_token_audit',
  'prompt_integrity',
])
