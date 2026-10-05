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
import { t } from 'i18next'
import { api, getCommonHeaders } from '@/lib/api'
import { readCheckStream } from './lib/event-stream'
import { reportDownloadData } from './lib/report-download'
import type {
  CheckEvent,
  CheckTarget,
  ClaudeCheckReport,
  CheckHistoryDetail,
  HistoryDetail,
  CheckHistoryPage,
  BaselineListItem,
} from './types'
import type { ModelKind, ModelListCode, ModelListResult } from './lib/default-models'

export async function getBaselines(): Promise<BaselineListItem[]> {
  const { data } = await api.get<{
    success: boolean
    data: BaselineListItem[]
    message?: string
  }>('/api/model_check/baselines')
  if (!data.success)
    throw new Error(t(data.message || 'Could not load comparison baselines'))
  return data.data
}

export async function saveBaseline(
  reportID: string,
  type: string
): Promise<BaselineListItem> {
  const { data } = await api.post<{
    success: boolean
    data: BaselineListItem
    message?: string
  }>('/api/model_check/baselines', { report_id: reportID, type })
  if (!data.success)
    throw new Error(t(data.message || 'Could not save the comparison baseline'))
  return data.data
}

export async function getCheckHistory(
  params: { page: number; model?: string; status?: string },
  signal?: AbortSignal
): Promise<CheckHistoryPage> {
  const { data } = await api.get<{
    success: boolean
    data: CheckHistoryPage
    message?: string
  }>('/api/model_check/history', {
    params: { ...params, page_size: 20 },
    signal,
  })
  if (!data.success)
    throw new Error(data.message || 'Failed to load check history')
  return data.data
}

export async function getCheckHistoryReport(
  id: string,
  signal?: AbortSignal
): Promise<HistoryDetail> {
  const { data } = await api.get<{
    success: boolean
    data: HistoryDetail
    message?: string
  }>(`/api/model_check/history/${encodeURIComponent(id)}`, { signal })
  if (!data.success)
    throw new Error(data.message || 'Failed to load check report')
  return data.data
}

export async function updateCheckReportRemark(
  id: string,
  remark: string
): Promise<CheckHistoryDetail> {
  try {
    const { data } = await api.patch<{
      success: boolean
      data: CheckHistoryDetail
      message?: string
    }>(`/api/model_check/history/${encodeURIComponent(id)}/remark`, { remark })
    if (!data.success)
      throw new Error(t(data.message || 'Failed to save report remark'))
    return data.data
  } catch (error) {
    const response = (error as { response?: { data?: { message?: string } } })
      ?.response?.data
    if (response?.message)
      throw new Error(t(response.message), { cause: error })
    throw error
  }
}

export async function streamModelCheck(
  target: CheckTarget,
  signal: AbortSignal,
  onEvent: (event: CheckEvent) => void
): Promise<void> {
  let path = '/api/model_check'
  if ('channel_id' in target)
    path = `/api/channel/${target.channel_id}/claude_check`
  const response = await fetch(path, {
    method: 'POST',
    credentials: 'include',
    cache: 'no-store',
    signal,
    headers: { ...getCommonHeaders(), Accept: 'text/event-stream' },
    body: JSON.stringify(target),
  })
  if (
    !response.ok ||
    !response.headers.get('content-type')?.includes('text/event-stream')
  ) {
    const data: { message?: string; detail?: string } = await response
      .json()
      .catch(() => ({}))
    const message = data.message ? t(data.message) : `HTTP ${response.status}`
    throw new Error(data.detail ? `${message}\n${data.detail}` : message)
  }
  if (!response.body) throw new Error('Progress stream is unavailable')
  await readCheckStream(response.body, onEvent)
}

export function downloadCheckReport(report: ClaudeCheckReport): void {
  const url = URL.createObjectURL(
    new Blob([JSON.stringify(reportDownloadData(report), null, 2)], {
      type: 'application/json',
    })
  )
  const link = document.createElement('a')
  link.href = url
  link.download = `model-check-${report.id}.json`
  link.click()
  URL.revokeObjectURL(url)
}

export type BadgeData = {
  domain: string
  status: 'ok' | 'stale' | 'none'
  tier?: 'conformant' | 'mostly' | 'review'
  score?: number
  model?: string
  protocol?: 'claude' | 'openai' | 'image'
  report_id?: string
  /** Unix milliseconds. */
  checked_at?: number
  expires_at?: number
}

// The latest badge for a domain: the newest completed check whose endpoint
// host is that domain or a subdomain of it.
export async function getBadge(domain: string): Promise<BadgeData> {
  const { data } = await api.get<{
    success: boolean
    data: BadgeData
    message?: string
  }>(`/api/badge/${encodeURIComponent(domain)}`)
  if (!data.success) throw new Error(data.message || 'Failed to load badge')
  return data.data
}

// Asks the endpoint for its model list through the server. A refusal comes back
// as { ok: false, code } so the form can keep its default models.
export async function fetchModelList(
  input: { kind: ModelKind; base_url: string; key: string },
  signal?: AbortSignal
): Promise<{ ok: true; data: ModelListResult } | { ok: false; code: ModelListCode }> {
  try {
    const { data } = await api.post<{
      success: boolean
      data?: ModelListResult
      code?: ModelListCode
    }>('/api/model_check/models', input, { signal })
    if (data.success && data.data) return { ok: true, data: data.data }
    return { ok: false, code: data.code ?? 'unreachable' }
  } catch (error) {
    if (signal?.aborted) throw error
    return { ok: false, code: 'unreachable' }
  }
}
