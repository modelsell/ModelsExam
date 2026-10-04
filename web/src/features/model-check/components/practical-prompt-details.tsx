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
import type { PromptAssessment, PromptInjectionReport } from '../types'
import { PromptInjectionEvidence } from './prompt-injection-evidence'

export function PracticalPromptDetails(props: {
  assessment: PromptAssessment
  injection?: PromptInjectionReport
  paper?: boolean
}) {
  const { t } = useTranslation()
  const a = props.assessment
  const sources: Record<string, string> = {
    unavailable: t('No input reference'),
    same_endpoint: t('Same-endpoint token counting'),
    saved_reference: t('Saved prompt reference'),
    mixed: t('Saved references and same-endpoint counting'),
  }
  const className = props.paper ? 'mc-muted' : 'text-muted-foreground text-xs'
  return (
    <>
      <p className={className}>
        {t(
          'Instruction preservation {{behavior}}/100 · Input count consistency {{tokens}}/100',
          {
            behavior: a.behavior_score ?? '—',
            tokens: a.token_score ?? '—',
          }
        )}
      </p>
      <p className={className}>
        {t(
          'Coverage {{coverage}}% · Responses {{behavior}}/9 · Input cases {{tokens}}/3',
          {
            coverage: a.coverage,
            behavior: a.behavior_measured,
            tokens: a.token_measured,
          }
        )}
      </p>
      <p className={className}>{sources[a.token_source || 'unavailable']}</p>
      <p className={className}>
        {t(
          'The score uses available checks only. Missing evidence reduces coverage, not the score. Full marks do not prove the absence of hidden prompts.'
        )}
      </p>
      {props.injection && (
        <PromptInjectionEvidence
          evidence={props.injection}
          paper={props.paper}
          practical
        />
      )}
    </>
  )
}
