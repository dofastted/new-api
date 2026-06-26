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
import { useVirtualizer } from '@tanstack/react-virtual'
import { AlertCircle, Database, Loader2 } from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { useUsageLogsContext } from '@/features/usage-logs/components/usage-logs-provider'
import { cn } from '@/lib/utils'

import type { UsageLog } from '../data/schema'
import type { TopupInfo } from '../lib/parse-topup'
import { useInfiniteLogs } from '../lib/use-infinite-logs'
import type { LogCategory } from '../types'
import { TopupOrderDetail } from './topup-order-detail'
import { UsageLogsStreamRow } from './usage-logs-stream-row'

interface UsageLogsStreamViewProps {
  logCategory: LogCategory
  isAdmin: boolean
  pageSize: number
  searchParams: Record<string, unknown>
  columnFilters: Array<{ id: string; value: unknown }>
  toolbar: ReactNode
}

const HEADER_HEIGHT_CLASS = 'h-8'
const SKELETON_ROW_KEYS = [
  'stream-skeleton-a',
  'stream-skeleton-b',
  'stream-skeleton-c',
  'stream-skeleton-d',
  'stream-skeleton-e',
  'stream-skeleton-f',
  'stream-skeleton-g',
  'stream-skeleton-h',
] as const

function HeaderCell(props: { children: ReactNode; className?: string }) {
  return (
    <div
      className={cn(
        'min-w-0 truncate px-2 text-left text-[11px] leading-none font-medium',
        props.className
      )}
    >
      {props.children}
    </div>
  )
}

function UsageLogsStreamSkeleton() {
  return (
    <div className='divide-border/40 divide-y'>
      {SKELETON_ROW_KEYS.map((key) => (
        <div key={key} className='flex h-[52px] items-center gap-2 px-2'>
          <Skeleton className='h-4 w-28 rounded' />
          <Skeleton className='h-5 w-16 rounded-full' />
          <Skeleton className='h-4 w-24 rounded' />
          <Skeleton className='h-4 w-32 rounded' />
          <Skeleton className='h-4 flex-1 rounded' />
        </div>
      ))}
    </div>
  )
}

export function UsageLogsStreamView(props: UsageLogsStreamViewProps) {
  const { t } = useTranslation()
  const parentRef = useRef<HTMLDivElement | null>(null)
  const { sensitiveVisible } = useUsageLogsContext()
  const [selectedTopup, setSelectedTopup] = useState<{
    log: UsageLog
    topupInfo: TopupInfo
  } | null>(null)

  const query = useInfiniteLogs({
    logCategory: props.logCategory,
    isAdmin: props.isAdmin,
    pageSize: props.pageSize,
    searchParams: props.searchParams,
    columnFilters: props.columnFilters,
  })

  const rowVirtualizer = useVirtualizer({
    count: query.logs.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 52,
    overscan: 10,
  })

  const virtualItems = rowVirtualizer.getVirtualItems()
  const lastVirtualItem = virtualItems.at(-1)
  const {
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    logs,
  } = query

  useEffect(() => {
    if (!lastVirtualItem) return
    if (lastVirtualItem.index < logs.length - 5) return
    if (!hasNextPage || isFetchingNextPage) return
    void fetchNextPage()
  }, [
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    lastVirtualItem,
    logs.length,
  ])

  const header = useMemo(
    () => (
      <div
        className={cn(
          'bg-muted/30 text-muted-foreground sticky top-0 z-10 flex min-w-max items-center border-b',
          HEADER_HEIGHT_CLASS
        )}
      >
        <HeaderCell className='w-[8.5rem] shrink-0'>{t('Time')}</HeaderCell>
        <HeaderCell className='w-[5.8rem] shrink-0'>{t('Type')}</HeaderCell>
        {props.isAdmin && (
          <HeaderCell className='w-[7rem] shrink-0'>{t('User')}</HeaderCell>
        )}
        <HeaderCell className='w-[7rem] shrink-0'>{t('Token')}</HeaderCell>
        <HeaderCell className='w-[10rem] shrink-0'>{t('Model')}</HeaderCell>
        {props.isAdmin && (
          <HeaderCell className='w-[7rem] shrink-0'>{t('Channel')}</HeaderCell>
        )}
        <HeaderCell className='w-[5.8rem] shrink-0'>{t('Tokens')}</HeaderCell>
        <HeaderCell className='w-[7rem] shrink-0'>{t('Cost')}</HeaderCell>
        <HeaderCell className='w-[6rem] shrink-0'>{t('Group')}</HeaderCell>
        <HeaderCell className='min-w-[15rem] flex-1'>{t('Details')}</HeaderCell>
        <HeaderCell className='w-[7rem] shrink-0'>{t('IP')}</HeaderCell>
      </div>
    ),
    [props.isAdmin, t]
  )

  return (
    <div className='flex h-full min-h-0 flex-col gap-3'>
      {props.toolbar}
      <div className='border-border/70 bg-card min-h-0 flex-1 overflow-hidden rounded-lg border'>
        <div ref={parentRef} className='h-full overflow-auto'>
          <div className='min-w-[980px]'>
            {header}

            {query.isLoading && <UsageLogsStreamSkeleton />}

            {query.isError && (
              <div className='p-6'>
                <Empty className='border-none p-0'>
                  <EmptyHeader>
                    <EmptyMedia variant='icon'>
                      <AlertCircle className='size-6' />
                    </EmptyMedia>
                    <EmptyTitle>{t('Failed to load logs')}</EmptyTitle>
                    <EmptyDescription>
                      {query.error?.message || t('Please try again later.')}
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
              </div>
            )}

            {!query.isLoading && !query.isError && query.logs.length === 0 && (
              <div className='p-6'>
                <Empty className='border-none p-0'>
                  <EmptyHeader>
                    <EmptyMedia variant='icon'>
                      <Database className='size-6' />
                    </EmptyMedia>
                    <EmptyTitle>{t('No Logs Found')}</EmptyTitle>
                    <EmptyDescription>
                      {t(
                        'No usage logs available. Logs will appear here once API calls are made.'
                      )}
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
              </div>
            )}

            {!query.isLoading && !query.isError && query.logs.length > 0 && (
              <div
                className='relative min-w-max'
                style={{ height: rowVirtualizer.getTotalSize() }}
              >
                {virtualItems.map((virtualRow) => {
                  const log = query.logs[virtualRow.index]
                  if (!log) return null
                  return (
                    <div
                      key={log.id}
                      className='absolute top-0 left-0 w-full'
                      style={{
                        height: virtualRow.size,
                        transform: `translateY(${virtualRow.start}px)`,
                      }}
                    >
                      <UsageLogsStreamRow
                        log={log}
                        isAdmin={props.isAdmin}
                        sensitiveVisible={sensitiveVisible}
                        onTopupClick={(nextLog, topupInfo) =>
                          setSelectedTopup({ log: nextLog, topupInfo })
                        }
                      />
                    </div>
                  )
                })}
              </div>
            )}
          </div>

          {query.isFetchingNextPage && (
            <div className='border-border/40 bg-card/90 sticky bottom-0 flex h-9 items-center justify-center gap-2 border-t text-xs'>
              <Loader2 className='size-3.5 animate-spin' />
              <span>{t('Loading more logs')}</span>
            </div>
          )}

          {!query.hasNextPage && query.logs.length > 0 && (
            <div className='border-border/40 bg-card/90 sticky bottom-0 flex h-8 items-center justify-center border-t'>
              <StatusBadge
                label={t('All matching logs loaded')}
                variant='neutral'
                copyable={false}
                size='sm'
              />
            </div>
          )}
        </div>
      </div>

      <TopupOrderDetail
        log={selectedTopup?.log ?? null}
        topupInfo={selectedTopup?.topupInfo ?? null}
        open={selectedTopup != null}
        onOpenChange={(open) => {
          if (!open) setSelectedTopup(null)
        }}
      />
    </div>
  )
}
