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
import { useTokenAuditLabels } from '../hooks/use-token-audit-labels'
import type { PromptInjectionReport } from '../types'

export function PromptInjectionEvidence(props: {
  evidence: PromptInjectionReport
  paper?: boolean
  practical?: boolean
}) {
  const { t } = useTranslation()
  const labels = useTokenAuditLabels(props.evidence.sampling ? 4 : 3)
  const codes: Record<string, string> = {
    prompt_behavior_only: t(
      'Only instruction behavior was measured; no input comparison is available'
    ),
    same_endpoint_aligned: t('Reported input agrees with same-endpoint counts'),
    same_endpoint_input_deviation: t(
      'Reported input differs from same-endpoint counts'
    ),
    prompt_samples_unstable: t(
      'Repeated input counts or request profiles vary; no injection conclusion'
    ),
    prompt_samples_incomplete: t(
      'Prompt repetitions are incomplete; no injection conclusion'
    ),
    injection_unverified: t(
      'No independent prompt evidence; injection remains unverified'
    ),
    local_prompt_changed: t('Local outbound prompt content was changed'),
    reference_aligned: t(
      'No extra input observed relative to the saved references'
    ),
    reference_fixed_excess: t(
      'Stable extra input relative to references; possible added prompt or inflated usage'
    ),
    reference_extra_input: t(
      'Extra input observed in multiple reference comparisons'
    ),
    reference_input_deviation: t(
      'Input counts differ from the saved reference'
    ),
    references_disagree: t(
      'Reference conclusions disagree; inspect every comparison'
    ),
    reference_incomplete: t(
      'Reference comparison incomplete or request parameters differ'
    ),
    repeat_input_changed: t(
      'Identical requests reported different input counts'
    ),
    same_endpoint_extra_input: t(
      'Reported input exceeds same-endpoint counting; origin remains unverified'
    ),
  }
  const className = props.paper ? 'mc-muted' : 'text-muted-foreground text-xs'
  const signed = (value: number | null) =>
    value == null ? '—' : `${value > 0 ? '+' : ''}${value}`
  return (
    <>
      <p>
        <b>{codes[props.evidence.code] || props.evidence.code}</b>
      </p>
      {props.evidence.sampling?.map((group) => (
        <p key={group.id} className={className}>
          {labels.names[group.id] || group.id} ·{' '}
          {t(
            'Samples {{valid}}/{{planned}} · Median {{median}} · Range {{min}}–{{max}}',
            {
              ...group,
              median: group.median ?? '—',
              min: group.min ?? '—',
              max: group.max ?? '—',
            }
          )}
          {!!group.outliers && (
            <> · {t('Outlier samples: {{count}}', { count: group.outliers })}</>
          )}
        </p>
      ))}
      <p className={className}>
        {t(
          props.evidence.sampling
            ? 'Known system input delta {{system}}'
            : 'Repeat input delta {{repeat}} · Known system input delta {{system}}',
          {
            repeat: signed(props.evidence.repeat_delta),
            system: signed(props.evidence.system_delta),
          }
        )}
      </p>
      <p className={className}>
        {t(
          'References compared {{count}}; incompatible references {{incompatible}}',
          {
            count: props.evidence.references.length,
            incompatible: props.evidence.incompatible_references,
          }
        )}
      </p>
      <p className={className}>
        {t(
          props.practical
            ? 'Saved references are optional. Matching prompts can be compared across test versions; unrelated cases are ignored.'
            : 'Behavior and same-endpoint counts cannot prove a clean channel. References must use this test version, the same model and matching requests.'
        )}
      </p>
      <p className={className}>
        {t(
          'Reference alignment only compares reported usage; forged counters and same-length rewrites can still evade this test.'
        )}
      </p>
      {!props.practical && !props.evidence.references.length && (
        <p className={className}>
          {t(
            'Run these cases on a trusted direct channel and manually save the report as a baseline, then rerun this target.'
          )}
        </p>
      )}
      {!!props.evidence.references.length && (
        <details>
          <summary>{t('Prompt reference evidence')}</summary>
          {props.evidence.references.map((ref) => (
            <div key={ref.id}>
              <p className={className}>
                <b>{ref.type}</b> · {codes[ref.code] || ref.code}
                {props.evidence.selected_reference_id === ref.id && (
                  <> · {t('Reference used for scoring')}</>
                )}
              </p>
              <p className={className}>
                {t('Source report')}: {ref.report_id}
              </p>
              {ref.pairs.map((pair) => (
                <p key={pair.id} className={className}>
                  {labels.names[pair.id] || pair.id}: {pair.expected} →{' '}
                  {pair.actual} · Δ {signed(pair.difference)} · ±
                  {pair.tolerance}
                </p>
              ))}
            </div>
          ))}
        </details>
      )}
    </>
  )
}
