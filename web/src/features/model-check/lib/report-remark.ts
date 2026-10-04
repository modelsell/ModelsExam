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
import type {
  CheckHistoryDetail,
  CheckHistoryPage,
  ClaudeCheckReport,
  HistoryDetail,
} from '../types'

// An explicitly cleared remark stays empty; only old reports use the channel name.
export function reportRemark(report: {
  remark?: string | null
  channel_name: string
}): string {
  return report.remark ?? report.channel_name
}

export function withSavedRemark(
  report: ClaudeCheckReport | null,
  saved?: HistoryDetail
): ClaudeCheckReport | null {
  // OpenAI and image rows share the detail query key but carry no Claude remark.
  if (
    !report ||
    saved?.run.transport === 'openai_api' ||
    saved?.run.transport === 'image_api'
  )
    return report
  if (saved?.report.id !== report.id) return report
  return { ...report, remark: (saved as CheckHistoryDetail).report.remark }
}

export function updateHistoryRemark(
  page: CheckHistoryPage | undefined,
  saved: CheckHistoryDetail
): CheckHistoryPage | undefined {
  if (!page) return page
  return {
    ...page,
    items: page.items.map((item) =>
      item.id === saved.run.id ? { ...item, remark: saved.run.remark } : item
    ),
  }
}
