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

export function useUsageTokenLabels() {
  const { t } = useTranslation()
  const names: Record<string, string> = {
    usage_token_integrity: t('Usage token integrity'),
    usage_tokens_en_30: t('English · {{count}} words', { count: 30 }),
    usage_tokens_en_300: t('English · {{count}} words', { count: 300 }),
    usage_tokens_en_1500: t('English · {{count}} words', { count: 1500 }),
    usage_tokens_zh_1000: t('Chinese · {{count}} characters', { count: 1000 }),
  }
  const codes: Record<string, string> = {
    not_applicable: t('Not checked in this historical run'),
    not_collected: t('Not collected'),
    ambiguous_sample: t('Duplicate samples; usage is not scored.'),
    invalid_response: t('A valid HTTP 200 response is required.'),
    missing_input: t('Input token count is missing.'),
    invalid_usage: t('Token counts must be nonnegative safe integers.'),
    usage_overflow: t('The total exceeds the safe integer range.'),
    nonpositive_input: t(
      'Total input must be positive for these nonempty prompts.'
    ),
    observed: t('Reported usage'),
    not_selected: t('No baseline selected for this run.'),
    baseline_unavailable: t('Selected baseline snapshot unavailable'),
    baseline_incomplete: t(
      'Four matching baseline samples are required for a consistency score.'
    ),
    baseline_complete: t(
      'All four requests match the selected baseline profiles.'
    ),
    no_sample: t('No matching baseline sample'),
    parameters_differ: t('Baseline request parameters differ'),
    reference_usage_unavailable: t('Baseline usage unavailable'),
    usage_incomplete: t('Four valid usage samples are required for scoring.'),
    usage_anomaly: t(
      'Token growth or baseline consistency differs; inspect the recorded counts.'
    ),
    usage_consistent: t(
      'The measured token growth and available baseline comparison are consistent.'
    ),
  }
  return { names, codes }
}
