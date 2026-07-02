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
import { useQueryClient, useIsFetching } from '@tanstack/react-query'
import { useNavigate, getRouteApi } from '@tanstack/react-router'
import type { Table } from '@tanstack/react-table'
import { Eye, EyeOff } from 'lucide-react'
import { useState, useCallback, useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useIsAdmin } from '@/hooks/use-admin'

import {
  LOG_TYPE_ALL_VALUE,
  LOG_TYPE_FILTERS,
  LOG_TYPE_TOPUP_VALUE,
  TOPUP_CHANNEL_META,
} from '../constants'
import { buildSearchParams } from '../lib/filter'
import { getDefaultTimeRange } from '../lib/utils'
import type { CommonLogFilters, TopupClientFilters, TopupKind } from '../types'
import { CommonLogsStats } from './common-logs-stats'
import { CompactDateTimeRangePicker } from './compact-date-time-range-picker'
import {
  LogsFilterField,
  LogsFilterInput,
  LogsFilterToolbar,
} from './logs-filter-toolbar'
import { useUsageLogsContext } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

type LogTypeValue = (typeof LOG_TYPE_FILTERS)[number]['value']
const logTypeValueSet = new Set<string>(
  LOG_TYPE_FILTERS.map((type) => type.value)
)

type CommonLogDraft = {
  sourceKey: string
  filters: CommonLogFilters
  logType: LogTypeValue
}

function isLogTypeValue(value: string): value is LogTypeValue {
  return logTypeValueSet.has(value)
}

function getLogTypeValue(value: unknown): LogTypeValue {
  return Array.isArray(value) &&
    value.length === 1 &&
    typeof value[0] === 'string' &&
    isLogTypeValue(value[0])
    ? value[0]
    : LOG_TYPE_ALL_VALUE
}

function buildSearchSourceKey(values: {
  startTime?: unknown
  endTime?: unknown
  channel?: unknown
  model?: unknown
  token?: unknown
  group?: unknown
  username?: unknown
  requestId?: unknown
  upstreamRequestId?: unknown
  type?: unknown
}) {
  return [
    values.startTime,
    values.endTime,
    values.channel,
    values.model,
    values.token,
    values.group,
    values.username,
    values.requestId,
    values.upstreamRequestId,
    Array.isArray(values.type) ? values.type.join(',') : values.type,
  ]
    .map((value) => String(value ?? ''))
    .join('\u001f')
}

interface CommonLogsFilterBarProps<TData> {
  table: Table<TData>
  viewToggle?: ReactNode
  showViewOptions?: boolean
}

export function CommonLogsFilterBar<TData>(
  props: CommonLogsFilterBarProps<TData>
) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const searchParams = route.useSearch()
  const isAdmin = useIsAdmin()
  const {
    sensitiveVisible,
    setSensitiveVisible,
    topupClientFilters,
    setTopupClientFilters,
    excludeAdminUsers,
    setExcludeAdminUsers,
  } = useUsageLogsContext()
  const fetchingLogs = useIsFetching({ queryKey: ['logs'] })

  const searchState = useMemo<CommonLogDraft>(() => {
    const { start, end } = getDefaultTimeRange()
    const sourceValues = {
      startTime: searchParams.startTime,
      endTime: searchParams.endTime,
      channel: searchParams.channel,
      model: searchParams.model,
      token: searchParams.token,
      group: searchParams.group,
      username: searchParams.username,
      requestId: searchParams.requestId,
      upstreamRequestId: searchParams.upstreamRequestId,
      type: searchParams.type,
    }
    const filters: CommonLogFilters = {
      startTime: searchParams.startTime
        ? new Date(searchParams.startTime)
        : start,
      endTime: searchParams.endTime ? new Date(searchParams.endTime) : end,
      channel: searchParams.channel || undefined,
      model: searchParams.model || undefined,
      token: searchParams.token || undefined,
      group: searchParams.group || undefined,
      username: searchParams.username || undefined,
      requestId: searchParams.requestId || undefined,
      upstreamRequestId: searchParams.upstreamRequestId || undefined,
    }
    return {
      sourceKey: buildSearchSourceKey(sourceValues),
      filters,
      logType: getLogTypeValue(searchParams.type),
    }
  }, [
    searchParams.startTime,
    searchParams.endTime,
    searchParams.channel,
    searchParams.model,
    searchParams.token,
    searchParams.group,
    searchParams.username,
    searchParams.requestId,
    searchParams.upstreamRequestId,
    searchParams.type,
  ])
  const [draft, setDraft] = useState<CommonLogDraft>(() => searchState)
  const activeDraft =
    draft.sourceKey === searchState.sourceKey ? draft : searchState
  const filters = activeDraft.filters
  const logType = activeDraft.logType

  const handleChange = useCallback(
    (field: keyof CommonLogFilters, value: Date | string | undefined) => {
      setDraft((current) => {
        const base =
          current.sourceKey === searchState.sourceKey ? current : searchState
        return {
          sourceKey: searchState.sourceKey,
          filters: { ...base.filters, [field]: value },
          logType: base.logType,
        }
      })
    },
    [searchState]
  )

  const handleApply = useCallback(() => {
    const filterParams = buildSearchParams(filters, 'common')
    navigate({
      to: '/usage-logs/$section',
      params: { section: 'common' },
      search: {
        ...filterParams,
        type: [logType],
        page: 1,
      },
    })
    queryClient.invalidateQueries({ queryKey: ['logs'] })
    queryClient.invalidateQueries({ queryKey: ['usage-logs-stats'] })
  }, [filters, logType, navigate, queryClient])

  const handleReset = useCallback(() => {
    const { start, end } = getDefaultTimeRange()
    const resetFilters: CommonLogFilters = { startTime: start, endTime: end }
    const resetSearch = {
      type: [LOG_TYPE_ALL_VALUE],
      startTime: start.getTime(),
      endTime: end.getTime(),
    }
    setDraft({
      sourceKey: buildSearchSourceKey(resetSearch),
      filters: resetFilters,
      logType: LOG_TYPE_ALL_VALUE,
    })

    navigate({
      to: '/usage-logs/$section',
      params: { section: 'common' },
      search: {
        page: 1,
        ...resetSearch,
      },
    })
    queryClient.invalidateQueries({ queryKey: ['logs'] })
    queryClient.invalidateQueries({ queryKey: ['usage-logs-stats'] })
  }, [navigate, queryClient])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter') handleApply()
    },
    [handleApply]
  )

  const hasExpandedFilters =
    !!filters.token ||
    !!filters.username ||
    !!filters.channel ||
    !!filters.requestId ||
    !!filters.upstreamRequestId

  const hasTypeFilter = logType !== LOG_TYPE_ALL_VALUE
  const hasAdditionalFilters =
    !!filters.model || !!filters.group || hasTypeFilter || hasExpandedFilters

  const expandedFilterCount = [
    filters.token,
    isAdmin ? filters.username : undefined,
    isAdmin ? filters.channel : undefined,
    filters.requestId,
    filters.upstreamRequestId,
  ].filter(Boolean).length
  const sensitiveType = sensitiveVisible ? 'text' : 'password'
  const logTypeItems = useMemo(
    () =>
      LOG_TYPE_FILTERS.map((type) => ({
        value: type.value,
        label: t(type.label),
      })),
    [t]
  )
  const logTypeLabel =
    logTypeItems.find((type) => type.value === logType)?.label ?? t('All Types')

  const isTopupMode = logType === LOG_TYPE_TOPUP_VALUE

  const topupChannelItems = useMemo(
    () =>
      (Object.keys(TOPUP_CHANNEL_META) as TopupKind[])
        .filter((kind) => kind !== 'unknown')
        .map((kind) => ({
          kind,
          label: t(TOPUP_CHANNEL_META[kind].labelKey),
        })),
    [t]
  )

  const updateTopupFilters = useCallback(
    (patch: Partial<TopupClientFilters>) => {
      setTopupClientFilters({ ...topupClientFilters, ...patch })
    },
    [setTopupClientFilters, topupClientFilters]
  )

  const statsBar = (
    <div className='flex flex-wrap items-center gap-2'>
      <CommonLogsStats />
    </div>
  )
  const sensitiveToggle = (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            onClick={() => setSensitiveVisible(!sensitiveVisible)}
            aria-label={sensitiveVisible ? t('Hide') : t('Show')}
            className='text-muted-foreground hover:text-foreground size-7'
          />
        }
      >
        {sensitiveVisible ? <Eye /> : <EyeOff />}
      </TooltipTrigger>
      <TooltipContent>
        {sensitiveVisible ? t('Hide') : t('Show')}
      </TooltipContent>
    </Tooltip>
  )
  const excludeAdminToggle = isAdmin ? (
    <label className='text-muted-foreground flex cursor-pointer items-center gap-1.5 text-xs'>
      <Switch
        checked={excludeAdminUsers}
        onCheckedChange={(checked) => setExcludeAdminUsers(Boolean(checked))}
      />
      <span className='hidden sm:inline'>{t('Exclude root user')}</span>
    </label>
  ) : null
  const toolbarActionStart = (
    <>
      {sensitiveToggle}
      {excludeAdminToggle}
    </>
  )

  const dateRangeFilter = (
    <LogsFilterField wide>
      <CompactDateTimeRangePicker
        start={filters.startTime}
        end={filters.endTime}
        onChange={({ start, end }) => {
          handleChange('startTime', start)
          handleChange('endTime', end)
        }}
      />
    </LogsFilterField>
  )
  const modelFilter = (
    <LogsFilterField>
      <LogsFilterInput
        placeholder={t('Model Name')}
        value={filters.model || ''}
        onChange={(e) => handleChange('model', e.target.value)}
        onKeyDown={handleKeyDown}
      />
    </LogsFilterField>
  )
  const groupFilter = (
    <LogsFilterField>
      <LogsFilterInput
        placeholder={t('Group')}
        type={sensitiveType}
        value={filters.group || ''}
        onChange={(e) => handleChange('group', e.target.value)}
        onKeyDown={handleKeyDown}
      />
    </LogsFilterField>
  )
  const typeFilter = (
    <LogsFilterField>
      <Select
        items={logTypeItems}
        value={logType}
        onValueChange={(value) => {
          const nextLogType =
            value !== null && isLogTypeValue(value) ? value : LOG_TYPE_ALL_VALUE
          setDraft((current) => {
            const base =
              current.sourceKey === searchState.sourceKey
                ? current
                : searchState
            return {
              sourceKey: searchState.sourceKey,
              filters: base.filters,
              logType: nextLogType,
            }
          })
        }}
      >
        <SelectTrigger>
          <SelectValue>{logTypeLabel}</SelectValue>
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {LOG_TYPE_FILTERS.map((type) => (
              <SelectItem key={type.value} value={type.value}>
                {t(type.label)}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </LogsFilterField>
  )
  const advancedFilters = (
    <>
      <LogsFilterField>
        <LogsFilterInput
          placeholder={t('Token Name')}
          type={sensitiveType}
          value={filters.token || ''}
          onChange={(e) => handleChange('token', e.target.value)}
          onKeyDown={handleKeyDown}
        />
      </LogsFilterField>
      {isAdmin && (
        <LogsFilterField>
          <LogsFilterInput
            placeholder={t('Username')}
            type={sensitiveType}
            value={filters.username || ''}
            onChange={(e) => handleChange('username', e.target.value)}
            onKeyDown={handleKeyDown}
          />
        </LogsFilterField>
      )}
      {isAdmin && (
        <LogsFilterField>
          <LogsFilterInput
            placeholder={t('Channel ID')}
            value={filters.channel || ''}
            onChange={(e) => handleChange('channel', e.target.value)}
            onKeyDown={handleKeyDown}
          />
        </LogsFilterField>
      )}
      <LogsFilterField>
        <LogsFilterInput
          placeholder={t('Request ID')}
          value={filters.requestId || ''}
          onChange={(e) => handleChange('requestId', e.target.value)}
          onKeyDown={handleKeyDown}
        />
      </LogsFilterField>
      <LogsFilterField>
        <LogsFilterInput
          placeholder={t('Upstream Request ID')}
          value={filters.upstreamRequestId || ''}
          onChange={(e) => handleChange('upstreamRequestId', e.target.value)}
          onKeyDown={handleKeyDown}
        />
      </LogsFilterField>
    </>
  )

  // --- Topup-mode client-side filters (no URL params; current page only) ---
  const topupChannelFilter = (
    <LogsFilterField wide>
      <div className='flex min-w-0 flex-col gap-1'>
        <span className='text-muted-foreground text-[11px] leading-none font-medium'>
          {t('Payment channel')}
        </span>
        <ToggleGroup
          value={topupClientFilters.channels}
          onValueChange={(value) => {
            const next = Array.isArray(value)
              ? (value.filter(
                  (v): v is TopupKind => typeof v === 'string'
                ) as TopupKind[])
              : []
            updateTopupFilters({ channels: next })
          }}
          variant='outline'
          size='sm'
          spacing={0}
          aria-label={t('Payment channel')}
          className='flex flex-wrap'
        >
          {topupChannelItems.map((item) => (
            <ToggleGroupItem
              key={item.kind}
              value={item.kind}
              aria-label={item.label}
            >
              {item.label}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </div>
    </LogsFilterField>
  )

  const topupPlanFilter = (
    <LogsFilterField>
      <LogsFilterInput
        placeholder={t('Plan name')}
        value={topupClientFilters.planContains}
        onChange={(e) => updateTopupFilters({ planContains: e.target.value })}
      />
    </LogsFilterField>
  )

  const topupAmountFilter = (
    <LogsFilterField wide>
      <div className='flex min-w-0 items-center gap-2'>
        <LogsFilterInput
          type='number'
          placeholder={t('Min amount')}
          value={topupClientFilters.minAmount ?? ''}
          onChange={(e) =>
            updateTopupFilters({
              minAmount: e.target.value === '' ? null : Number(e.target.value),
            })
          }
          className='w-full'
        />
        <span className='text-muted-foreground shrink-0 text-xs'>—</span>
        <LogsFilterInput
          type='number'
          placeholder={t('Max amount')}
          value={topupClientFilters.maxAmount ?? ''}
          onChange={(e) =>
            updateTopupFilters({
              maxAmount: e.target.value === '' ? null : Number(e.target.value),
            })
          }
          className='w-full'
        />
      </div>
    </LogsFilterField>
  )

  const topupClientHint = (
    <span className='text-muted-foreground text-[11px]'>
      {t('Client-side filter (current page only)')}
    </span>
  )

  const topupPrimaryFilters = (
    <>
      {dateRangeFilter}
      {topupChannelFilter}
      {topupPlanFilter}
      {topupAmountFilter}
      {isAdmin && (
        <LogsFilterField>
          <LogsFilterInput
            placeholder={t('Username')}
            type={sensitiveType}
            value={filters.username || ''}
            onChange={(e) => handleChange('username', e.target.value)}
            onKeyDown={handleKeyDown}
          />
        </LogsFilterField>
      )}
      {topupClientHint}
    </>
  )

  // In topup mode the Type selector is locked to Topup (an exit affordance is
  // provided via reset, which returns to All Types). We render a disabled,
  // fixed-value Select so the layout stays consistent with the default bar.
  const topupTypeFilter = (
    <LogsFilterField>
      <Select value={LOG_TYPE_TOPUP_VALUE} items={logTypeItems} disabled>
        <SelectTrigger>
          <SelectValue>{t('Top-up')}</SelectValue>
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {LOG_TYPE_FILTERS.map((type) => (
              <SelectItem key={type.value} value={type.value}>
                {t(type.label)}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </LogsFilterField>
  )

  // Reset in topup mode also clears the client-side topup filters.
  const handleTopupReset = useCallback(() => {
    handleReset()
    setTopupClientFilters({
      channels: [],
      planContains: '',
      minAmount: null,
      maxAmount: null,
    })
  }, [handleReset, setTopupClientFilters])

  const topupActiveCount =
    topupClientFilters.channels.length +
    (topupClientFilters.planContains.trim() !== '' ? 1 : 0) +
    (topupClientFilters.minAmount != null ? 1 : 0) +
    (topupClientFilters.maxAmount != null ? 1 : 0)

  if (isTopupMode) {
    return (
      <LogsFilterToolbar
        table={props.table}
        stats={statsBar}
        actionStart={toolbarActionStart}
        actionEnd={props.viewToggle}
        showViewOptions={props.showViewOptions}
        primaryFilters={
          <>
            {topupTypeFilter}
            {topupPrimaryFilters}
          </>
        }
        advancedFilters={null}
        mobilePinnedFilters={dateRangeFilter}
        mobileFilters={
          <>
            {topupTypeFilter}
            {topupChannelFilter}
            {topupPlanFilter}
            {topupAmountFilter}
            {topupClientHint}
          </>
        }
        mobileFilterCount={topupActiveCount}
        hasAdvancedActiveFilters={false}
        advancedFilterCount={0}
        hasActiveFilters
        onSearch={handleApply}
        searchLoading={fetchingLogs > 0}
        onReset={handleTopupReset}
      />
    )
  }

  return (
    <LogsFilterToolbar
      table={props.table}
      stats={statsBar}
      actionStart={toolbarActionStart}
      actionEnd={props.viewToggle}
      showViewOptions={props.showViewOptions}
      primaryFilters={
        <>
          {dateRangeFilter}
          {modelFilter}
          {groupFilter}
          {typeFilter}
        </>
      }
      advancedFilters={advancedFilters}
      mobilePinnedFilters={dateRangeFilter}
      mobileFilters={
        <>
          {modelFilter}
          {groupFilter}
          {typeFilter}
          {advancedFilters}
        </>
      }
      mobileFilterCount={
        [filters.model, filters.group, hasTypeFilter].filter(Boolean).length +
        expandedFilterCount
      }
      hasAdvancedActiveFilters={hasExpandedFilters}
      advancedFilterCount={expandedFilterCount}
      hasActiveFilters={hasAdditionalFilters}
      onSearch={handleApply}
      searchLoading={fetchingLogs > 0}
      onReset={handleReset}
    />
  )
}
