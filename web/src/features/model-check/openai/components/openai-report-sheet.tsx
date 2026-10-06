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
import { REPORT_SHEET_CSS } from '../../lib/report-sheet-style'
import { useOpenAILabels } from '../hooks/use-openai-labels'
import { sheetData } from '../lib/sheet'
import type { OpenAICheckReport } from '../types'
import { OpenAISheetBody } from './openai-sheet-body'
import { OpenAISheetHero } from './openai-sheet-hero'

// Printable paper report. It reuses the Claude report styles (.mc-report) so
// both providers export the same looking HTML / PDF document.
export function OpenAIReportSheet(props: {
  report: OpenAICheckReport
  running: boolean
  phaseLabel: string
  durationMS: number
}) {
  const { t } = useTranslation()
  const labels = useOpenAILabels()
  const { report } = props
  const data = sheetData(report, props.running)
  return (
    <article className='mc-report' aria-label={t('Model check report')}>
      <style>{REPORT_SHEET_CSS}</style>
      <header className='mc-title'>
        <div>
          <small>
            MODELSEXAM / {report.provider === 'gemini' ? 'GEMINI' : 'OPENAI'} / v
            {report.version}
          </small>
          <h2>{t('Model check report')}</h2>
          <p>
            {report.model}
            {report.options?.suite
              ? ` · ${t(report.options.suite === 'basic' ? 'Basic' : report.options.suite === 'full' ? 'Full' : 'Standard')}`
              : ''}
          </p>
        </div>
        <div className='mc-status'>
          <p>{props.phaseLabel}</p>
          <small>{new Date(report.started_at).toLocaleString()}</small>
          <small>{report.endpoint ||
              (report.provider === 'gemini' ? 'Gemini API' : 'OpenAI API')}</small>
        </div>
      </header>
      {props.running && (
        <p className='mc-provisional'>
          {t('Live results are provisional until collection finishes.')}
        </p>
      )}
      {(report.cancelled || report.stop_reason) && (
        <p className='mc-provisional'>
          {t('Stopped before all checks ran')} ·{' '}
          {labels.code(report.stop_reason || 'cancelled')}
        </p>
      )}
      <OpenAISheetHero report={report} data={data} />
      <OpenAISheetBody
        report={report}
        data={data}
        running={props.running}
        durationMS={props.durationMS}
      />
    </article>
  )
}
