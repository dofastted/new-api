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
import { api } from '@/lib/api'

export type RiskEvent = {
  id: number
  user_id: number
  created_at: number
  source: string
  action: string
  detail: string
  snippet: string
  score: number
  request_id: string
  model_name: string
  ip: string
}

export type RiskEventQuery = {
  page: number
  page_size: number
  user_id?: number
  source?: string
  action?: string
  start_timestamp?: number
  end_timestamp?: number
}

type ApiEnvelope<T> = {
  success: boolean
  message: string
  data: T
}

type PageResult<T> = {
  page: number
  page_size: number
  total: number
  items: T[]
}

export async function getRiskEvents(query: RiskEventQuery) {
  const params: Record<string, string | number> = {
    page: query.page,
    page_size: query.page_size,
  }
  if (query.user_id) params.user_id = query.user_id
  if (query.source) params.source = query.source
  if (query.action) params.action = query.action
  if (query.start_timestamp) params.start_timestamp = query.start_timestamp
  if (query.end_timestamp) params.end_timestamp = query.end_timestamp

  const res = await api.get<ApiEnvelope<PageResult<RiskEvent>>>(
    '/api/abuse_guard/events',
    { params }
  )
  return res.data
}

export async function unbanAbuseUser(userId: number) {
  const res = await api.post<ApiEnvelope<null>>(
    `/api/abuse_guard/unban/${userId}`
  )
  return res.data
}

export async function resetAbuseScore(userId: number) {
  const res = await api.post<ApiEnvelope<null>>(
    `/api/abuse_guard/reset/${userId}`
  )
  return res.data
}
