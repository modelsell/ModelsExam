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
import type { RefObject } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { Button } from '@/components/ui/button'
import {
  captureReportHTML,
  downloadReportHTML,
  printReportHTML,
} from '../../lib/report-export'
import { downloadOpenAIReportJSON, downloadOpenAIReportMarkdown } from '../api'
import type { OpenAICheckReport } from '../types'

// Exports are disabled while collecting: partial reports are provisional.
export function OpenAIReportActions(props: {
  report: OpenAICheckReport | null
  markdown: string | null
  sheet: RefObject<HTMLDivElement | null>
  running: boolean
}) {
  const { t, i18n } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const { report, markdown } = props
  const disabled = !report || props.running
  const exportDocument = (print: boolean) => {
    if (!report || !props.sheet.current) return
    try {
      const html = captureReportHTML(
        props.sheet.current,
        `${t('Model check report')} · ${report.model}`,
        i18n.resolvedLanguage || 'en'
      )
      if (print) printReportHTML(html)
      else downloadReportHTML(html, report.id)
    } catch {
      toast.error(t('Could not export the report document'))
    }
  }
  return (
    <div className='flex flex-wrap gap-2'>
      <Button
        variant='outline'
        size='sm'
        disabled={disabled}
        onClick={() => exportDocument(false)}
      >
        {t('Download HTML report')}
      </Button>
      <Button
        variant='outline'
        size='sm'
        disabled={disabled}
        onClick={() => exportDocument(true)}
      >
        {t('Print / Save PDF')}
      </Button>
      <Button
        variant='outline'
        size='sm'
        disabled={disabled}
        onClick={() => report && downloadOpenAIReportJSON(report)}
      >
        {t('Export JSON')}
      </Button>
      <Button
        variant='outline'
        size='sm'
        disabled={disabled || !markdown}
        onClick={() =>
          report && markdown && downloadOpenAIReportMarkdown(report, markdown)
        }
      >
        {t('Download Markdown')}
      </Button>
      <Button
        variant='outline'
        size='sm'
        disabled={disabled || !markdown}
        onClick={() => markdown && void copyToClipboard(markdown)}
      >
        {t('Copy Markdown')}
      </Button>
    </div>
  )
}
