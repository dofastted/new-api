/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { LOG_TYPE_ENUM, LOG_TYPE_TOPUP_VALUE } from '../constants'
import type { UsageLog } from '../data/schema'
import { EMPTY_TOPUP_CLIENT_FILTERS, type TopupClientFilters } from '../types'
import { parseTopup } from './parse-topup'

/**
 * Whether the URL search params select topup-only mode (type === '1').
 * The route serializes `type` as a single-element array.
 */
export function isTopupTypeFilter(
  searchParams: Record<string, unknown>
): boolean {
  const type = searchParams.type
  if (Array.isArray(type)) {
    return type.length === 1 && type[0] === LOG_TYPE_TOPUP_VALUE
  }
  return type === LOG_TYPE_TOPUP_VALUE
}

/**
 * Whether the client-side topup filter is effectively empty (no narrowing).
 * Used to skip the filter pass entirely when nothing is configured.
 */
export function isTopupClientFilterEmpty(filters: TopupClientFilters): boolean {
  return (
    filters.channels.length === 0 &&
    filters.planContains.trim() === '' &&
    (filters.minAmount == null || Number.isNaN(filters.minAmount)) &&
    (filters.maxAmount == null || Number.isNaN(filters.maxAmount))
  )
}

function matchesAmount(
  pay: number | null,
  filters: TopupClientFilters
): boolean {
  if (pay == null) {
    // A topup with no parseable amount only matches when no amount bound is set.
    return (
      (filters.minAmount == null || Number.isNaN(filters.minAmount)) &&
      (filters.maxAmount == null || Number.isNaN(filters.maxAmount))
    )
  }
  const min = filters.minAmount
  const max = filters.maxAmount
  if (min != null && !Number.isNaN(min) && pay < min) return false
  if (max != null && !Number.isNaN(max) && pay > max) return false
  return true
}

/**
 * Narrow an already-loaded page of logs by the topup client-side filters.
 *
 * The backend log list endpoint cannot filter by payment channel / amount /
 * quota / plan (those are derived from `content` / `other` at display time),
 * so this runs purely on the client over the current page of results.
 */
export function applyTopupClientFilters(
  logs: UsageLog[],
  filters: TopupClientFilters
): UsageLog[] {
  if (isTopupClientFilterEmpty(filters)) return logs
  const planQuery = filters.planContains.trim().toLowerCase()
  const channels = filters.channels
  return logs.filter((log) => {
    if (log.type !== LOG_TYPE_ENUM.TOPUP) return true
    const info = parseTopup(log)
    if (channels.length > 0 && !channels.includes(info.kind)) return false
    if (
      planQuery !== '' &&
      (info.planTitle ?? '').toLowerCase().includes(planQuery) === false
    ) {
      return false
    }
    if (!matchesAmount(info.payAmount, filters)) return false
    return true
  })
}

export { EMPTY_TOPUP_CLIENT_FILTERS }
