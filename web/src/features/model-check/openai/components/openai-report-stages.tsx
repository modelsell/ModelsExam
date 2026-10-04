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
import { useOpenAILabels } from '../hooks/use-openai-labels'
import { checkIdForProbe, OPENAI_STAGES } from '../lib/catalog'
import type { OpenAICheckReport } from '../types'
import { OpenAICheckRow } from './openai-check-row'

export function OpenAIReportStages(props: {
  report: OpenAICheckReport
  activeProbe: string | null
  live: boolean
}) {
  const { t } = useTranslation()
  const labels = useOpenAILabels()
  const report = props.report
  const done = new Map(report.checks.map((check) => [check.id, check]))
  const activeId = props.activeProbe ? checkIdForProbe(props.activeProbe) : null
  // Report without a plan (older server): show finished checks only.
  const plan =
    report.plan && report.plan.length > 0
      ? report.plan
      : report.checks.map((check) => ({ ...check, selected: true }))
  const known = new Set(OPENAI_STAGES.map((stage) => stage.id))
  const stages = [
    ...OPENAI_STAGES,
    ...[...new Set(plan.map((item) => item.stage))]
      .filter((id) => !known.has(id))
      .map((id) => ({ id, title: id })),
  ]
  return (
    <div className='flex flex-col gap-4'>
      {stages.map((stage) => {
        const items = plan.filter((item) => item.stage === stage.id)
        if (items.length === 0) return null
        const finished = items.filter((item) => done.has(item.id)).length
        return (
          <section key={stage.id} className='rounded-xl border'>
            <header className='flex items-center justify-between gap-3 border-b px-4 py-3'>
              <h3 className='text-sm font-semibold'>
                {labels.stage(stage.id)}
              </h3>
              <span className='text-muted-foreground text-xs'>
                {t('{{done}} / {{total}}', {
                  done: finished,
                  total: items.length,
                })}
              </span>
            </header>
            <div className='divide-y'>
              {items.map((item) => (
                <OpenAICheckRow
                  key={item.id}
                  id={item.id}
                  kind={item.kind}
                  selected={item.selected}
                  check={done.get(item.id)}
                  running={
                    props.live && activeId === item.id && !done.has(item.id)
                  }
                  samples={report.samples.filter(
                    (sample) => checkIdForProbe(sample.probe) === item.id
                  )}
                />
              ))}
            </div>
          </section>
        )
      })}
    </div>
  )
}
