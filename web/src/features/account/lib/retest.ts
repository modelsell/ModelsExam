import type { ZodObject, ZodType } from 'zod'
import type { Provider } from '../api'
import { modelCheckFields, modelCheckOptions, type ModelCheckForm } from '../../model-check/lib/form'
import { requestBudget } from '../../model-check/lib/check-plan'
import {
  openAICheckFields,
  openAICheckOptions,
  openAIRequestBudget,
  type OpenAICheckForm,
} from '../../model-check/openai/lib/form'
import {
  geminiCheckFields,
  geminiCheckOptions,
  geminiRequestBudget,
  type GeminiCheckForm,
} from '../../model-check/gemini/lib/form'
import {
  imageBudget,
  imageCheckFields,
  imageCheckOptions,
  type ImageCheckForm,
} from '../../model-check/image/lib/form'
import type { BaselineListItem } from '../../model-check/types'
import { normalizeBaseURL } from './credentials'

// A retest rebuilds a check from a stored report: provider from the
// transport, Base URL from the endpoint, model and options from the report.
// Options go through today's form fields, so unknown or invalid ones are
// dropped and the run uses current defaults for anything missing.

export const TRANSPORT_PROVIDER: Record<string, Provider> = {
  openai_api: 'openai',
  gemini_api: 'gemini',
  image_api: 'image',
}

export function providerOf(transport: string): Provider {
  return TRANSPORT_PROVIDER[transport] ?? 'claude'
}

// Form values a new check of each kind starts with (as in the forms).
export const FORM_DEFAULTS = {
  claude: {
    cache: true,
    thinking: false,
    repeat: false,
    vision: false,
    pdf: false,
    stream_comparison: false,
    benchmark: true,
    performance: true,
    prompt_audit: false,
    bedrock: false,
    performance_tolerance: 25,
    baseline_id: '',
    baseline_type: '',
  },
  openai: {
    suite: 'standard',
    responses: false,
    vision: false,
    logprobs: false,
    limit_param: 'max_completion_tokens',
  },
  gemini: { suite: 'standard', vision: false },
  image: { suite: 'standard', provenance: false, baseline: false },
} as const

const FIELDS: Record<Provider, ZodObject> = {
  claude: modelCheckFields,
  openai: openAICheckFields,
  gemini: geminiCheckFields,
  image: imageCheckFields,
}

// Never copied from a report: connection fields and secrets.
const NOT_OPTIONS = new Set(['base_url', 'key', 'verify_key', 'credential_id', 'mode', 'model'])

export function normalizeOptions(
  provider: Provider,
  saved: Record<string, unknown> | undefined,
  baselines: BaselineListItem[] = []
): Record<string, unknown> {
  const shape = FIELDS[provider].shape as Record<string, ZodType>
  const values: Record<string, unknown> = { ...FORM_DEFAULTS[provider] }
  for (const [name, value] of Object.entries(saved ?? {})) {
    if (NOT_OPTIONS.has(name) || !(name in shape)) continue
    if (shape[name].safeParse(value).success) values[name] = value
  }
  if (provider === 'claude') {
    // A comparison baseline that no longer exists is dropped.
    const id = typeof values.baseline_id === 'string' ? values.baseline_id : ''
    if (!id || !baselines.some((b) => b.id === id)) {
      values.baseline_id = ''
      values.baseline_type = ''
    }
  }
  if (provider === 'image') {
    // Provenance needs an official key the retest does not have.
    values.provenance = false
    values.baseline = false
  }
  return values
}

export type RetestPlan = {
  provider: Provider
  base_url: string
  model: string
  /** Form values to prefill. */
  values: Record<string, unknown>
  /** Options for the check API. */
  options: Record<string, unknown>
  requests: number
  suite: string
}

type StoredReport = {
  model?: string
  endpoint?: string
  options?: Record<string, unknown> | null
}

export function planRetest(
  run: { transport: string; endpoint: string; model: string },
  report: StoredReport | undefined,
  baselines: BaselineListItem[] = []
): RetestPlan | null {
  const provider = providerOf(run.transport)
  const base = normalizeBaseURL(provider, report?.endpoint || run.endpoint)
  const model = (report?.options?.model as string | undefined) || report?.model || run.model
  if (!base || !model) return null
  const values = normalizeOptions(provider, report?.options ?? undefined, baselines)
  const { options, requests, suite } = checkOptions(provider, model, values, baselines)
  return { provider, base_url: base, model, values, options, requests, suite }
}

// API options and request budget for form values of one kind of check.
export function checkOptions(
  provider: Provider,
  model: string,
  values: Record<string, unknown>,
  baselines: BaselineListItem[] = []
): { options: Record<string, unknown>; requests: number; suite: string } {
  const form = { ...values, model, base_url: '', key: '' }
  switch (provider) {
    case 'claude': {
      const reference = baselines.find((b) => b.id === values.baseline_id)
      const claude = modelCheckOptions({ ...form, mode: 'endpoint' } as ModelCheckForm, reference)
      return { options: claude, requests: requestBudget(claude), suite: 'focused' }
    }
    case 'openai': {
      const openai = openAICheckOptions(form as OpenAICheckForm)
      return { options: openai, requests: openAIRequestBudget(openai), suite: openai.suite ?? 'standard' }
    }
    case 'gemini': {
      const gemini = geminiCheckOptions(form as GeminiCheckForm)
      return { options: gemini, requests: geminiRequestBudget(gemini), suite: gemini.suite ?? 'standard' }
    }
    default: {
      const image = imageCheckOptions({ ...form, verify_key: '' } as ImageCheckForm)
      return { options: image, requests: imageBudget(image).requests, suite: image.suite ?? 'standard' }
    }
  }
}

// Estimated upstream requests per day of a schedule.
export function requestsPerDay(requestsPerRun: number, intervalMinutes: number): number {
  return Math.ceil((24 * 60) / intervalMinutes) * requestsPerRun
}

export function scoreDelta(score?: number | null, previous?: number | null): number | null {
  if (score == null || previous == null) return null
  return score - previous
}
