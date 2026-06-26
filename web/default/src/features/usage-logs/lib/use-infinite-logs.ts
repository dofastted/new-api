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
import { useInfiniteQuery } from '@tanstack/react-query'

import { DEFAULT_LOGS_DATA } from '../constants'
import type { UsageLog } from '../data/schema'
import type { LogCategory } from '../types'
import { fetchLogsByCategory } from './utils'

interface UseInfiniteLogsConfig {
  logCategory: LogCategory
  isAdmin: boolean
  pageSize: number
  searchParams: Record<string, unknown>
  columnFilters?: Array<{ id: string; value: unknown }>
}

export function useInfiniteLogs(config: UseInfiniteLogsConfig) {
  const query = useInfiniteQuery({
    queryKey: [
      'logs-infinite',
      config.logCategory,
      config.isAdmin,
      config.pageSize,
      config.columnFilters,
      config.searchParams,
    ],
    initialPageParam: 1,
    queryFn: async ({ pageParam }) => {
      const result = await fetchLogsByCategory({
        logCategory: config.logCategory,
        isAdmin: config.isAdmin,
        page: pageParam,
        pageSize: config.pageSize,
        searchParams: config.searchParams,
        columnFilters: config.columnFilters ?? [],
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

  const logs =
    query.data?.pages.flatMap((page) => page.items as UsageLog[]) ?? []
  const total = query.data?.pages[0]?.total ?? 0

  return { ...query, logs, total }
}
