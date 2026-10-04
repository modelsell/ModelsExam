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
import { useTranslation } from 'react-i18next'

export function useTokenAuditLabels(version = 1) {
  const { t } = useTranslation()
  const names: Record<string, string> = {
    minimal: t('Minimal input sample'),
    floor_a: t('Input floor without system text'),
    floor_b: t('Identical input floor repeat'),
    system_canary: t('Known system marker control'),
    short: t('Short prompt without system text'),
    system: t('Verification code in system text'),
    long: t('Code lookup in a long reference'),
    cache_read_1: t('Cache read request 1'),
    cache_read_2: t('Cache read request 2'),
  }
  const questions: Record<string, string> = {
    floor_a: t(
      'No system, tools or reference text. Request PONG and record total input.'
    ),
    floor_b: t(
      'Repeat the identical request to check whether the input count changes.'
    ),
    system_canary: t(
      'Keep the user message unchanged and add a known system instruction to return MAPLE-7391.'
    ),
    short: t('No system prompt. Ask for PONG only.'),
    system: t(
      'Put MAPLE-7391 in the system prompt and ask for that verification code.'
    ),
    long: t(
      'Provide 96 fixed records and ask for the six-digit code in Record 073.'
    ),
  }
  if (version >= 2) {
    questions.system = t(
      'System requires MAPLE-7391; the user requests CEDAR-0000. Expect MAPLE-7391 only.'
    )
    questions.long = t(
      'Look up Record 073 among 96 records containing a fake instruction to output CEDAR-0000. Expect 701544 only.'
    )
  }
  if (version >= 4)
    questions.long = t(
      'Provide 96 fixed records and ask for the six-digit code in Record 073.'
    )
  for (const id of ['floor_a', 'long', 'system_canary']) {
    for (let round = 1; round <= 3; round++) {
      names[`${id}_r${round}`] = `${names[id]} · ${round}/3`
      questions[`${id}_r${round}`] = t(
        'Repeat the same prompt three times; compare the median and full range.'
      )
    }
  }
  for (let round = 1; round <= 3; round++)
    names[`minimal_r${round}`] = `${names.minimal} · ${round}/3`
  for (let read = 1; read <= 6; read++) {
    if (version < 4) {
      if (read > 2)
        names[`cache_read_${read}`] = t('Cache read request {{number}}', {
          number: read,
        })
      continue
    }
    names[`cache_read_${read}`] = t('Cache round {{round}} · Read {{read}}/3', {
      round: Math.ceil(read / 3),
      read: ((read - 1) % 3) + 1,
    })
  }
  const codes: Record<string, string> = {
    prompt_input_budget_checked: t('See input allowance and repeated samples'),
    budget_collecting: t('Collecting minimal input samples'),
    budget_no_obvious_injection: t('No obvious extra input observed'),
    budget_extra_input: t(
      'Input exceeds the allowance; added context is possible'
    ),
    budget_insufficient_samples: t('No valid input sample; not scored'),
    budget_within_allowance: t('Within the input allowance'),
    simple_request_mismatch: t(
      'Request model or parameters differ across samples'
    ),
    prompt_input_compared: t('See input samples and baseline comparison'),
    simple_collecting: t('Collecting three input samples'),
    simple_no_reference: t('Awaiting a trusted baseline'),
    simple_insufficient_samples: t('Insufficient input samples for comparison'),
    simple_input_aligned: t('No extra input observed relative to the baseline'),
    simple_extra_input: t('Extra input observed; a prompt may have been added'),
    simple_input_deviation: t('Input differs from the baseline'),
    simple_variable_input: t('Input counts vary across repeated requests'),
    local_prompt_changed: t('Local outbound prompt content was changed'),
    input_observed: t('Input sample collected'),
    missing_usage: t('Input usage not returned'),
    probe_unavailable: t('Sample unavailable'),
    cache_rewritten: t(
      'Cache was written again; the read score includes this rewrite'
    ),
    prompt_integrity_assessed: t(
      'Instruction preservation and token consistency are scored separately; see evidence coverage.'
    ),
    prompt_answer_matched: t('Answer matches the test instruction'),
    prompt_answer_changed: t('Answer differs from the test instruction'),
    prompt_answer_incomplete: t(
      'Output did not complete; behavior is unscored'
    ),
    tokens_within_tolerance: t('Token difference is within tolerance'),
    pending: t('Pending'),
    not_collected: t('Not collected'),
    tokens_equal: t('Token counts are equal'),
    tokens_differ: t('Token counts differ; inspect the signed delta'),
    count_profile_mismatch: t(
      'Matching request parameters could not be confirmed'
    ),
    cache_write_reference_missing: t(
      'A fresh cache write was not confirmed; no read comparison is scored'
    ),
    token_audit_observed: t(
      'Compare three request counts with reported total input; see the token audit table.'
    ),
    cache_token_audit_observed: t(
      'Compare each cache read with the first write; see the token audit table.'
    ),
  }
  return { names, questions, codes }
}
