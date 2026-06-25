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

export const PROVIDER_GROUP_STATUS = {
  disabled: 0,
  enabled: 1,
} as const

export const PROVIDER_ROUTE_TYPES = [
  'completions',
  'responses',
  'messages',
  'other',
] as const

export type ProviderRouteType = (typeof PROVIDER_ROUTE_TYPES)[number]

export type ProviderGroup = {
  id: number
  name: string
  display_name: string
  description: string
  status: number
  usage_ratio: number
  is_auto: boolean
  sort_order: number
  created_time: number
  updated_time: number
}

export type ProviderGroupChannel = {
  id?: number
  provider_group_id: number
  group_name?: string
  channel_id: number
  priority: number | null
  weight: number | null
  route_types: string
  enabled: boolean
  sort_order: number
  created_time?: number
  updated_time?: number
}

export type ProviderGroupAutoRule = {
  id?: number
  route_type: ProviderRouteType
  candidate_group: string
  sort_order: number
  enabled: boolean
  created_time?: number
  updated_time?: number
}

export type ProviderGroupApiResponse<T = unknown> = {
  success: boolean
  message?: string
  data?: T
}

/**
 * Parse the JSON-encoded route_types string from the backend into a typed list.
 * An empty or invalid value means the membership serves every route type.
 */
export function parseRouteTypes(
  value: string | null | undefined
): ProviderRouteType[] {
  if (!value) return [...PROVIDER_ROUTE_TYPES]
  try {
    const parsed: unknown = JSON.parse(value)
    if (!Array.isArray(parsed)) return [...PROVIDER_ROUTE_TYPES]
    const allowed = new Set<string>(PROVIDER_ROUTE_TYPES)
    const result = parsed.filter(
      (item): item is ProviderRouteType =>
        typeof item === 'string' && allowed.has(item)
    )
    return result.length > 0 ? result : [...PROVIDER_ROUTE_TYPES]
  } catch {
    return [...PROVIDER_ROUTE_TYPES]
  }
}

export function serializeRouteTypes(routeTypes: ProviderRouteType[]): string {
  const ordered = PROVIDER_ROUTE_TYPES.filter((type) =>
    routeTypes.includes(type)
  )
  return JSON.stringify(ordered)
}

export const providerGroupQueryKeys = {
  all: ['provider-groups'] as const,
  list: () => [...providerGroupQueryKeys.all, 'list'] as const,
  channels: (id: number) =>
    [...providerGroupQueryKeys.all, 'channels', id] as const,
  autoRules: () => [...providerGroupQueryKeys.all, 'auto-rules'] as const,
}

export async function getProviderGroups(): Promise<
  ProviderGroupApiResponse<ProviderGroup[]>
> {
  const res = await api.get('/api/provider-group/')
  return res.data
}

export async function createProviderGroup(
  payload: Partial<ProviderGroup>
): Promise<ProviderGroupApiResponse<ProviderGroup>> {
  const res = await api.post('/api/provider-group/', payload)
  return res.data
}

export async function updateProviderGroup(
  id: number,
  payload: Partial<ProviderGroup>
): Promise<ProviderGroupApiResponse<ProviderGroup>> {
  const res = await api.put(`/api/provider-group/${id}`, payload)
  return res.data
}

export async function deleteProviderGroup(
  id: number
): Promise<ProviderGroupApiResponse> {
  const res = await api.delete(`/api/provider-group/${id}`)
  return res.data
}

export async function getProviderGroupChannels(
  id: number
): Promise<ProviderGroupApiResponse<ProviderGroupChannel[]>> {
  const res = await api.get(`/api/provider-group/${id}/channels`)
  return res.data
}

export async function updateProviderGroupChannels(
  id: number,
  items: ProviderGroupChannel[]
): Promise<ProviderGroupApiResponse<ProviderGroupChannel[]>> {
  const res = await api.put(`/api/provider-group/${id}/channels`, { items })
  return res.data
}

export async function getProviderGroupAutoRules(): Promise<
  ProviderGroupApiResponse<ProviderGroupAutoRule[]>
> {
  const res = await api.get('/api/provider-group/auto-rules')
  return res.data
}

export async function updateProviderGroupAutoRules(
  items: ProviderGroupAutoRule[]
): Promise<ProviderGroupApiResponse<ProviderGroupAutoRule[]>> {
  const res = await api.put('/api/provider-group/auto-rules', { items })
  return res.data
}
