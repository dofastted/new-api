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
import {
  AlertCircle,
  ArrowUp,
  Database,
  List,
  Loader2,
  RefreshCw,
  Rows3,
} from 'lucide-react'
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useUsageLogsContext } from '@/features/usage-logs/components/usage-logs-provider'
import { cn } from '@/lib/utils'

import {
  COMPACT_STREAM_COLUMN_ORDER,
  DEFAULT_STREAM_COLUMN_SETTINGS,
  getUsageLogsDensityStorageKey,
  getUsageLogsStreamColumnsStorageKey,
  parseStreamColumnSettings,
  SIMPLE_USER_REQUEST_LOG_TYPES,
  STREAM_CUSTOMIZABLE_COLUMNS,
  USAGE_LOGS_DENSITY,
  type StreamColumnSettings,
  type UsageLogsDensity,
} from '../constants'
import type { UsageLog } from '../data/schema'
import type { TopupInfo } from '../lib/parse-topup'
import { applyTopupClientFilters, isTopupTypeFilter } from '../lib/topup-filter'
import { useInfiniteLogs } from '../lib/use-infinite-logs'
import type { LogCategory } from '../types'
import { DetailsDialog } from './dialogs/details-dialog'
import { TopupOrderDetail } from './topup-order-detail'
import { UsageLogsErrorAnalysisBar } from './usage-logs-error-analysis-bar'
import { UsageLogsStreamColumnManager } from './usage-logs-stream-column-manager'
import { UsageLogsStreamHeader } from './usage-logs-stream-header'
import { UsageLogsStreamRow } from './usage-logs-stream-row'

interface UsageLogsStreamViewProps {
  logCategory: LogCategory
  isAdmin: boolean
  pageSize: number
  searchParams: Record<string, unknown>
  columnFilters: Array<{ id: string; value: unknown }>
  toolbar: ReactNode
  simplifiedUserView?: boolean
}

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

const ROW_HEIGHT: Record<UsageLogsDensity, number> = {
  [USAGE_LOGS_DENSITY.COMFORTABLE]: 44,
  [USAGE_LOGS_DENSITY.COMPACT]: 32,
}


function UsageLogsStreamSkeleton() {
  return (
    <div className='divide-border/40 divide-y'>
      {SKELETON_ROW_KEYS.map((key) => (
        <div key={key} className='flex h-[44px] items-center gap-3 px-3'>
          <Skeleton className='h-3 w-24 rounded' />
          <Skeleton className='h-5 w-16 rounded-full' />
          <Skeleton className='h-4 flex-1 rounded' />
          <Skeleton className='h-4 w-20 rounded' />
        </div>
      ))}
    </div>
  )
}

export function UsageLogsStreamView(props: UsageLogsStreamViewProps) {
  const { t } = useTranslation()
  const parentRef = useRef<HTMLDivElement | null>(null)
  const { sensitiveVisible, topupClientFilters, excludeAdminUsers } =
    useUsageLogsContext()
  const [selectedTopup, setSelectedTopup] = useState<{
    log: UsageLog
    topupInfo: TopupInfo
  } | null>(null)
  const [selectedLog, setSelectedLog] = useState<UsageLog | null>(null)
  const [autoRefresh, setAutoRefresh] = useState(true)
  // Live rows only insert while the user is at the top; once they scroll down to
  // browse history, polling pauses so the scroll position never jumps.
  const [isAtTop, setIsAtTop] = useState(true)

  const shouldUseSimplifiedUserView =
    props.simplifiedUserView === true &&
    !props.isAdmin &&
    props.logCategory === 'common'

  const densityStorageKey = getUsageLogsDensityStorageKey(props.isAdmin)
  const [density, setDensity] = useState<UsageLogsDensity>(
    USAGE_LOGS_DENSITY.COMFORTABLE
  )
  useEffect(() => {
    if (typeof window === 'undefined') return
    const stored = window.localStorage.getItem(densityStorageKey)
    if (
      stored === USAGE_LOGS_DENSITY.COMFORTABLE ||
      stored === USAGE_LOGS_DENSITY.COMPACT
    ) {
      setDensity(stored)
    }
  }, [densityStorageKey])
  const handleDensityChange = useCallback(
    (next: UsageLogsDensity) => {
      setDensity(next)
      if (typeof window !== 'undefined') {
        window.localStorage.setItem(densityStorageKey, next)
      }
    },
    [densityStorageKey]
  )
  const effectiveDensity = shouldUseSimplifiedUserView
    ? USAGE_LOGS_DENSITY.COMFORTABLE
    : density
  const isCompact =
    !shouldUseSimplifiedUserView && density === USAGE_LOGS_DENSITY.COMPACT

  const columnsStorageKey = getUsageLogsStreamColumnsStorageKey(props.isAdmin)
  const [columnSettings, setColumnSettings] = useState<StreamColumnSettings>(
    DEFAULT_STREAM_COLUMN_SETTINGS
  )
  useEffect(() => {
    if (typeof window === 'undefined') return
    setColumnSettings(
      parseStreamColumnSettings(window.localStorage.getItem(columnsStorageKey))
    )
  }, [columnsStorageKey])
  const handleColumnSettingsChange = useCallback(
    (next: StreamColumnSettings) => {
      setColumnSettings(next)
      if (typeof window !== 'undefined') {
        window.localStorage.setItem(columnsStorageKey, JSON.stringify(next))
      }
    },
    [columnsStorageKey]
  )
  const effectiveColumnOrder = useMemo(
    () =>
      columnSettings.order.filter((id) => {
        if (columnSettings.hidden.includes(id)) return false
        const def = STREAM_CUSTOMIZABLE_COLUMNS.find(
          (column) => column.id === id
        )
        return !(def?.adminOnly && !props.isAdmin)
      }),
    [columnSettings, props.isAdmin]
  )
  const compactColumnOrder = useMemo(
    () =>
      COMPACT_STREAM_COLUMN_ORDER.filter((id) => {
        if (id === 'type' && columnSettings.hidden.includes(id)) return false
        const def = STREAM_CUSTOMIZABLE_COLUMNS.find(
          (column) => column.id === id
        )
        return !(def?.adminOnly && !props.isAdmin)
      }),
    [columnSettings.hidden, props.isAdmin]
  )
  const renderedColumnOrder = isCompact
    ? compactColumnOrder
    : effectiveColumnOrder
  const compactTypeVisible = !columnSettings.hidden.includes('type')
  const handleCompactTypeVisibilityChange = useCallback(
    (checked: boolean) => {
      const hiddenWithoutType = columnSettings.hidden.filter(
        (id) => id !== 'type'
      )
      const nextHidden: StreamColumnSettings['hidden'] = checked
        ? hiddenWithoutType
        : [...hiddenWithoutType, 'type']
      handleColumnSettingsChange({ ...columnSettings, hidden: nextHidden })
    },
    [columnSettings, handleColumnSettingsChange]
  )

  const query = useInfiniteLogs({
    logCategory: props.logCategory,
    isAdmin: props.isAdmin,
    pageSize: props.pageSize,
    searchParams: props.searchParams,
    columnFilters: props.columnFilters,
    excludeAdmin: excludeAdminUsers,
    liveEnabled: autoRefresh,
    livePaused: !isAtTop,
  })

  // Topup mode narrows the already-loaded page on the client (the backend
  // cannot filter by payment channel / amount / plan). Other modes render the
  // full page unchanged.
  const isTopupMode =
    !shouldUseSimplifiedUserView && isTopupTypeFilter(props.searchParams)
  const visibleLogs = useMemo(() => {
    const baseLogs = isTopupMode
      ? applyTopupClientFilters(query.logs, topupClientFilters)
      : query.logs
    if (!shouldUseSimplifiedUserView) return baseLogs
    return baseLogs.filter((log) => SIMPLE_USER_REQUEST_LOG_TYPES.includes(log.type))
  }, [isTopupMode, query.logs, shouldUseSimplifiedUserView, topupClientFilters])

  const rowVirtualizer = useVirtualizer({
    count: visibleLogs.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT[effectiveDensity],
    overscan: 10,
  })

  // Row height is fixed per density mode; force a remeasure so previously
  // cached row offsets don't linger after switching comfortable <-> compact.
  useEffect(() => {
    rowVirtualizer.measure()
  }, [effectiveDensity, rowVirtualizer])

  const virtualItems = rowVirtualizer.getVirtualItems()
  const lastVirtualItem = virtualItems.at(-1)
  const {
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    logs,
    liveCount,
    recentIds,
    refetchLive,
    isLiveFetching,
  } = query

  useEffect(() => {
    const el = parentRef.current
    if (!el) return
    const onScroll = () => setIsAtTop(el.scrollTop <= 4)
    onScroll()
    el.addEventListener('scroll', onScroll, { passive: true })
    return () => el.removeEventListener('scroll', onScroll)
  }, [])

  const scrollToTop = useCallback(() => {
    parentRef.current?.scrollTo({ top: 0, behavior: 'smooth' })
  }, [])

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

  return (
    <div className='flex h-full min-h-0 flex-col gap-3'>
      {props.toolbar}

      <div className='flex flex-wrap items-center gap-3 text-xs'>
        <label className='flex cursor-pointer items-center gap-2'>
          <Switch
            checked={autoRefresh}
            onCheckedChange={(checked) => setAutoRefresh(Boolean(checked))}
          />
          <span className='text-muted-foreground'>{t('Auto refresh')}</span>
        </label>

        <Button
          variant='outline'
          size='sm'
          onClick={() => void refetchLive()}
          disabled={isLiveFetching}
        >
          <RefreshCw
            className={cn('size-3.5', isLiveFetching && 'animate-spin')}
          />
          {t('Refresh')}
        </Button>

        {autoRefresh && isAtTop && (
          <span className='inline-flex items-center gap-1.5 font-medium text-emerald-600 dark:text-emerald-400'>
            <span className='relative flex size-2'>
              <span
                className='absolute inset-0 animate-ping rounded-full bg-emerald-500 opacity-75'
                style={{ animationDuration: '1.5s' }}
              />
              <span className='relative size-2 rounded-full bg-emerald-500' />
            </span>
            {t('Live')}
          </span>
        )}

        {autoRefresh && !isAtTop && (
          <span className='text-muted-foreground inline-flex items-center gap-1.5'>
            <span className='bg-muted-foreground/50 size-2 rounded-full' />
            {t('Paused while browsing history')}
          </span>
        )}

        {liveCount > 0 && (
          <Button
            variant='ghost'
            size='sm'
            className='text-primary'
            onClick={scrollToTop}
          >
            <ArrowUp className='size-3.5' />
            {t('{{count}} new', { count: liveCount })}
          </Button>
        )}

        {!shouldUseSimplifiedUserView && (
          <div className='ms-auto flex items-center gap-2'>
            {isCompact ? (
              <label className='border-border/70 bg-background/70 text-muted-foreground flex h-8 items-center gap-2 rounded-md border px-2 text-xs'>
                <Switch
                  size='sm'
                  checked={compactTypeVisible}
                  onCheckedChange={handleCompactTypeVisibilityChange}
                  aria-label={t('Type')}
                />
                {t('Type')}
              </label>
            ) : (
              <UsageLogsStreamColumnManager
                isAdmin={props.isAdmin}
                settings={columnSettings}
                onChange={handleColumnSettingsChange}
              />
            )}

            <ToggleGroup
              value={[density]}
              onValueChange={(value) => {
                const next = Array.isArray(value) ? value.at(-1) : value
                if (
                  next === USAGE_LOGS_DENSITY.COMFORTABLE ||
                  next === USAGE_LOGS_DENSITY.COMPACT
                ) {
                  handleDensityChange(next)
                }
              }}
              variant='outline'
              size='sm'
              spacing={0}
              aria-label={t('Display density')}
            >
              <Tooltip>
                <TooltipTrigger
                  render={
                    <ToggleGroupItem
                      value={USAGE_LOGS_DENSITY.COMFORTABLE}
                      aria-label={t('Detailed')}
                    />
                  }
                >
                  <Rows3 className='size-3.5' />
                  <span className='hidden sm:inline'>{t('Detailed')}</span>
                </TooltipTrigger>
                <TooltipContent>{t('Detailed')}</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <ToggleGroupItem
                      value={USAGE_LOGS_DENSITY.COMPACT}
                      aria-label={t('Compact')}
                    />
                  }
                >
                  <List className='size-3.5' />
                  <span className='hidden sm:inline'>{t('Compact')}</span>
                </TooltipTrigger>
                <TooltipContent>{t('Compact')}</TooltipContent>
              </Tooltip>
            </ToggleGroup>
          </div>
        )}
      </div>

      {!shouldUseSimplifiedUserView && <UsageLogsErrorAnalysisBar logs={logs} />}

      <div className='border-border/70 bg-card flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border'>
        {!isTopupMode && (
          <UsageLogsStreamHeader
            isAdmin={props.isAdmin}
            compact={isCompact}
            simplifiedUserView={shouldUseSimplifiedUserView}
            columnOrder={renderedColumnOrder}
          />
        )}
        <div ref={parentRef} className='min-h-0 flex-1 overflow-auto'>
          <div>
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

            {!query.isLoading && !query.isError && visibleLogs.length === 0 && (
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

            {!query.isLoading && !query.isError && visibleLogs.length > 0 && (
              <div
                className='relative min-w-max'
                style={{ height: rowVirtualizer.getTotalSize() }}
              >
                {virtualItems.map((virtualRow) => {
                  const log = visibleLogs[virtualRow.index]
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
                        isNew={recentIds.has(log.id)}
                        compact={isCompact}
                        simplifiedUserView={shouldUseSimplifiedUserView}
                        columnOrder={renderedColumnOrder}
                        onTopupClick={(nextLog, topupInfo) =>
                          setSelectedTopup({ log: nextLog, topupInfo })
                        }
                        onRowClick={
                          shouldUseSimplifiedUserView ? undefined : setSelectedLog
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

      {!shouldUseSimplifiedUserView && (
        <TopupOrderDetail
          log={selectedTopup?.log ?? null}
          topupInfo={selectedTopup?.topupInfo ?? null}
          open={selectedTopup != null}
          onOpenChange={(open) => {
            if (!open) setSelectedTopup(null)
          }}
        />
      )}

      {!shouldUseSimplifiedUserView && selectedLog && (
        <DetailsDialog
          log={selectedLog}
          isAdmin={props.isAdmin}
          open={selectedLog != null}
          onOpenChange={(open) => {
            if (!open) setSelectedLog(null)
          }}
        />
      )}
    </div>
  )
}
