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
import type {
  PromptAssessment,
  PromptInjectionReport,
  TokenComparison,
} from '../types'
import { BudgetPromptDetails } from './budget-prompt-details'
import { PracticalPromptDetails } from './practical-prompt-details'
import { PromptInjectionEvidence } from './prompt-injection-evidence'
import { SimplePromptDetails } from './simple-prompt-details'

export function PromptIntegrityDetails(props: {
  assessment: PromptAssessment
  paper?: boolean
  injection?: PromptInjectionReport
  samples?: TokenComparison[]
}) {
  const { t } = useTranslation()
  const className = props.paper ? 'mc-muted' : 'text-muted-foreground text-xs'
  if (props.assessment.scoring === 'input_budget')
    return <BudgetPromptDetails {...props} />
  if (props.assessment.scoring === 'input_consistency')
    return <SimplePromptDetails {...props} />
  if (props.assessment.scoring === 'measured_checks')
    return <PracticalPromptDetails {...props} />
  return (
    <>
      {props.injection && (
        <PromptInjectionEvidence
          evidence={props.injection}
          paper={props.paper}
        />
      )}
      <p className={className}>
        {t(
          props.assessment.token_source
            ? 'Instruction preservation {{behavior}} / 30 · Reference token consistency {{tokens}} / 70'
            : 'Instruction preservation {{behavior}} / 30 · Token consistency {{tokens}} / 70',
          {
            behavior: props.assessment.behavior_points,
            tokens: props.assessment.token_points,
          }
        )}
      </p>
      <p className={className}>
        {t(
          'Evidence coverage {{coverage}}% · Behavior {{behavior}} / {{planned}} · Token pairs {{tokens}} / 3',
          {
            coverage: props.assessment.coverage,
            planned: props.assessment.behavior_planned ?? 3,
            behavior: props.assessment.behavior_measured,
            tokens: props.assessment.token_measured,
          }
        )}
      </p>
      <p className={className}>
        {t(
          'Unmeasured layers earn no evidence points. Behavior alone earns at most 30; this is not a probability of an injection-free channel.'
        )}
      </p>
      <p className={className}>
        {t(
          'Prompt token tolerance is the larger of 2 tokens or 1% of the estimate. This is a product threshold, not an official guarantee.'
        )}
      </p>
    </>
  )
}
