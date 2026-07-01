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
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useRef, useState } from 'react'

import { DEFAULT_LOGS_DATA } from '../constants'
import type { UsageLog } from '../data/schema'
import type { LogCategory } from '../types'
import { fetchLogsByCategory } from './utils'

const DEFAULT_LIVE_INTERVAL_MS = 5000
// Cap the live buffer so a long-running tail cannot grow memory without bound.
const LIVE_BUFFER_LIMIT = 200
// How long a freshly-arrived row is flagged for the one-shot enter animation.
const RECENT_FLASH_MS = 1600

interface UseInfiniteLogsConfig {
  logCategory: LogCategory
  isAdmin: boolean
  pageSize: number
  searchParams: Record<string, unknown>
  columnFilters?: Array<{ id: string; value: unknown }>
  /** Admin-only: exclude logs produced by Admin/Root role users. */
  excludeAdmin?: boolean
  /** Master switch for auto-refresh (live row insertion). */
  liveEnabled?: boolean
  /** Poll interval in ms when live refresh is active. */
  liveIntervalMs?: number
  /** Temporarily suspend polling (e.g. user is browsing history, not at top). */
  livePaused?: boolean
}

export function useInfiniteLogs(config: UseInfiniteLogsConfig) {
  const {
    liveEnabled = false,
    liveIntervalMs = DEFAULT_LIVE_INTERVAL_MS,
    livePaused = false,
  } = config

  const baseKey = [
    config.logCategory,
    config.isAdmin,
    config.pageSize,
    config.columnFilters,
    config.searchParams,
    config.excludeAdmin,
  ]

  const query = useInfiniteQuery({
    queryKey: ['logs-infinite', ...baseKey],
    initialPageParam: 1,
    queryFn: async ({ pageParam }) => {
      const result = await fetchLogsByCategory({
        logCategory: config.logCategory,
        isAdmin: config.isAdmin,
        page: pageParam,
        pageSize: config.pageSize,
        searchParams: config.searchParams,
        columnFilters: config.columnFilters ?? [],
        excludeAdmin: config.excludeAdmin,
      })

      if (!result?.success) {
        throw new Error(result?.message || 'Failed to load logs')
      }

      return result.data || DEFAULT_LOGS_DATA
    },
    getNextPageParam: (lastPage, allPages, lastPageParam) => {
      const loaded = allPages.reduce(
        (sum, page) => sum + (page.items?.length || 0),
        0
      )
      if (loaded < (lastPage.total || 0)) return lastPageParam + 1
      return undefined
    },
  })

  const historicalLogs = useMemo(
    () => query.data?.pages.flatMap((page) => page.items as UsageLog[]) ?? [],
    [query.data]
  )
  const total = query.data?.pages[0]?.total ?? 0

  // Baseline = newest id at the time the first page loaded. It stays fixed
  // because page 1 is never refetched here, so historical rows (id <= baseline)
  // and live rows (id > baseline) never overlap.
  const baselineId = useMemo(() => {
    const firstPage = query.data?.pages[0]?.items as UsageLog[] | undefined
    if (!firstPage?.length) return 0
    return firstPage.reduce((max, item) => (item.id > max ? item.id : max), 0)
  }, [query.data])

  // Dedicated head query: only ever fetches page 1 to discover new rows.
  const liveActive = liveEnabled && !livePaused
  const liveQuery = useQuery({
    queryKey: ['logs-live', ...baseKey],
    enabled: liveEnabled && query.isSuccess,
    queryFn: async () => {
      const result = await fetchLogsByCategory({
        logCategory: config.logCategory,
        isAdmin: config.isAdmin,
        page: 1,
        pageSize: config.pageSize,
        searchParams: config.searchParams,
        columnFilters: config.columnFilters ?? [],
        excludeAdmin: config.excludeAdmin,
      })
      if (!result?.success) {
        throw new Error(result?.message || 'Failed to load logs')
      }
      return result.data || DEFAULT_LOGS_DATA
    },
    // Dynamic interval: paused -> off. React Query also skips while a fetch is
    // in flight and (by default) while the tab is hidden, so polls never stack.
    refetchInterval: liveActive ? liveIntervalMs : false,
    refetchIntervalInBackground: false,
  })

  // Accumulating buffer of newly-arrived rows, prepended on top of history.
  const [liveBuffer, setLiveBuffer] = useState<UsageLog[]>([])
  // Ids that arrived in the last poll, used to play a one-shot enter animation.
  const [recentIds, setRecentIds] = useState<Set<number>>(() => new Set())
  const knownIdsRef = useRef<Set<number>>(new Set())
  const flashTimersRef = useRef<number[]>([])

  // Reset the buffer whenever the underlying filter/category/baseline changes.
  const baselineKey = `${config.logCategory}:${baselineId}`
  useEffect(() => {
    setLiveBuffer([])
    setRecentIds(new Set())
    knownIdsRef.current = new Set()
  }, [baselineKey])

  useEffect(() => {
    const items = liveQuery.data?.items as UsageLog[] | undefined
    if (!items?.length || baselineId === 0) return
    const fresh = items.filter(
      (row) => row.id > baselineId && !knownIdsRef.current.has(row.id)
    )
    if (fresh.length === 0) return
    for (const row of fresh) knownIdsRef.current.add(row.id)

    setLiveBuffer((prev) => {
      const merged = [...fresh, ...prev]
      merged.sort((a, b) => b.id - a.id)
      return merged.slice(0, LIVE_BUFFER_LIMIT)
    })

    const freshIds = fresh.map((row) => row.id)
    setRecentIds((prev) => {
      const next = new Set(prev)
      for (const id of freshIds) next.add(id)
      return next
    })
    const timer = window.setTimeout(() => {
      setRecentIds((prev) => {
        if (freshIds.every((id) => !prev.has(id))) return prev
        const next = new Set(prev)
        for (const id of freshIds) next.delete(id)
        return next
      })
    }, RECENT_FLASH_MS)
    flashTimersRef.current.push(timer)
  }, [liveQuery.data, baselineId])

  useEffect(
    () => () => {
      for (const timer of flashTimersRef.current) window.clearTimeout(timer)
    },
    []
  )

  const liveRows = useMemo(
    () => liveBuffer.filter((row) => row.id > baselineId),
    [liveBuffer, baselineId]
  )

  const logs = useMemo(
    () => [...liveRows, ...historicalLogs],
    [liveRows, historicalLogs]
  )

  return {
    ...query,
    logs,
    total,
    liveCount: liveRows.length,
    recentIds,
    refetchLive: liveQuery.refetch,
    isLiveFetching: liveQuery.isFetching,
  }
}
