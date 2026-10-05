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
import { ImageCheckReportView } from '../image/components/image-check-report'
import { imageHistoryState, isImageHistory } from '../image/lib/history-state'
import { OpenAICheckReportView } from '../openai/components/openai-check-report'
import { isOpenAIHistory, openAIHistoryState } from '../openai/lib/history-state'
import { badgeTier } from '../lib/badge'
import { historyRunState } from '../lib/history-state'
import type { HistoryDetail } from '../types'
import { CheckReport } from './check-report'
import { ReportBadge } from './report-badge'

// A stored report: the badge for this run and the full report for the
// protocol that produced it. Reports are private, so there is no share link.
export function ReportDetail({ detail }: { detail: HistoryDetail }) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-col gap-6'>
      <div className='bg-card flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border p-4'>
        <ReportBadge
          size='md'
          tier={badgeTier(detail.run.status, detail.run.score)}
          score={detail.run.score}
        />
        <p className='text-muted-foreground min-w-0 flex-1 text-xs leading-5'>
          {t(
            'This badge covers this run only. Observations and provenance results do not change it.'
          )}
        </p>
      </div>
      {isImageHistory(detail) ? (
        <ImageCheckReportView
          state={imageHistoryState(detail)}
          elapsed={detail.run.duration_ms}
        />
      ) : isOpenAIHistory(detail) ? (
        <OpenAICheckReportView
          state={openAIHistoryState(detail)}
          elapsed={detail.run.duration_ms}
        />
      ) : (
        <CheckReport
          state={historyRunState(detail)}
          elapsed={detail.run.duration_ms}
        />
      )}
    </div>
  )
}
