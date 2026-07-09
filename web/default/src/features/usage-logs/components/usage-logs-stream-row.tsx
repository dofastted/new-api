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
import { CheckCircle2, FileSearch, Gauge, Globe, Image } from 'lucide-react'
import { memo } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  formatLogQuota,
  formatTimestampToDate,
  formatUseTime,
} from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  LOG_TYPE_ENUM,
  SIMPLE_USER_STREAM_COLUMNS,
  STREAM_COLUMNS,
  type StreamColumnId,
} from '../constants'
import type { UsageLog } from '../data/schema'
import {
  formatModelName,
  getLogBilledCostLabels,
  getResponseTimeColor,
  parseLogOther,
} from '../lib/format'
import { parseTopup, type TopupInfo } from '../lib/parse-topup'
import {
  getLogTypeConfig,
  isDisplayableLogType,
  isTimingLogType,
} from '../lib/utils'
import type { ChannelChainEntry, LogOtherData } from '../types'
import { ChannelChainPopover } from './channel-chain-popover'
import { ModelBadge } from './model-badge'

interface UsageLogsStreamRowProps {
  log: UsageLog
  isAdmin: boolean
  sensitiveVisible: boolean
  isNew?: boolean
  /** Compact display density: caller passes the compact column subset. */
  compact?: boolean
  /** Ordinary-user fixed stream view: safe summary columns only, no details affordance. */
  simplifiedUserView?: boolean
  /**
   * Visible customizable columns, in display order (already filtered for
   * admin-only columns and user hide/reorder preferences). Time and Model are
   * fixed anchors and always render first regardless of this list.
   */
  columnOrder: StreamColumnId[]
  onTopupClick: (log: UsageLog, topupInfo: TopupInfo) => void
  onRowClick?: (log: UsageLog) => void
}

const logTypeRowTint: Record<number, string> = {
  [LOG_TYPE_ENUM.TOPUP]:
    'bg-cyan-50/25 dark:bg-cyan-950/10 border-cyan-200/40 dark:border-cyan-900/25',
  [LOG_TYPE_ENUM.ERROR]:
    'bg-rose-50/40 dark:bg-rose-950/20 border-rose-200/50 dark:border-rose-900/30',
  [LOG_TYPE_ENUM.REFUND]:
    'bg-blue-50/30 dark:bg-blue-950/15 border-blue-200/50 dark:border-blue-900/30',
}

function maskSensitive(value: string, visible: boolean): string {
  if (!value) return '-'
  return visible ? value : '••••'
}

function formatPaymentAmount(amount: number | null): string {
  if (amount == null) return '-'
  return amount.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 6,
  })
}

function getLogTypeVariant(variant: string): StatusVariant {
  return variant === 'default' ? 'neutral' : (variant as StatusVariant)
}

function TopupContent(props: { topupInfo: TopupInfo; userText?: string }) {
  const { t } = useTranslation()
  const info = props.topupInfo
  const quotaText =
    info.rechargeQuotaText ??
    (info.quotaDelta != null ? formatLogQuota(info.quotaDelta) : null)

  if (info.kind === 'unknown') {
    return (
      <div className='flex min-w-0 items-center gap-2 overflow-hidden'>
        <span className='text-muted-foreground min-w-0 flex-1 truncate'>
          {info.raw}
        </span>
        {props.userText && (
          <span className='text-muted-foreground max-w-[8rem] shrink-0 truncate text-xs'>
            {t('User')}: {props.userText}
          </span>
        )}
        {quotaText && (
          <span className='border-border/70 bg-background/70 inline-flex h-6 min-w-0 shrink-0 items-center rounded-md border px-2 font-mono text-xs tabular-nums'>
            <span className='truncate'>{quotaText}</span>
          </span>
        )}
        {info.balanceAfter != null && (
          <span className='text-muted-foreground shrink-0 font-mono text-xs tabular-nums'>
            {t('Balance')}: {formatLogQuota(info.balanceAfter)}
          </span>
        )}
      </div>
    )
  }

  return (
    <div className='flex min-w-0 items-center gap-2 overflow-hidden'>
      <StatusBadge
        label={t(info.channelLabelKey)}
        variant={info.channelVariant}
        copyable={false}
      />
      {quotaText && (
        <span className='border-border/70 bg-background/70 inline-flex h-6 min-w-0 items-center rounded-md border px-2 font-mono text-xs tabular-nums'>
          <span className='truncate'>{quotaText}</span>
        </span>
      )}
      {props.userText && (
        <span className='text-muted-foreground max-w-[8rem] shrink-0 truncate text-xs'>
          {t('User')}: {props.userText}
        </span>
      )}
      {info.planTitle && (
        <span className='text-foreground min-w-0 truncate text-xs font-medium'>
          {info.planTitle}
        </span>
      )}
      <span className='text-muted-foreground shrink-0 font-mono text-xs tabular-nums'>
        {t('Paid')}: {formatPaymentAmount(info.payAmount)}
      </span>
      {info.balanceAfter != null && (
        <span className='text-muted-foreground shrink-0 font-mono text-xs tabular-nums'>
          {t('Balance')}: {formatLogQuota(info.balanceAfter)}
        </span>
      )}
      <StatusBadge
        label={t('Completed')}
        icon={CheckCircle2}
        variant='green'
        copyable={false}
        className='shrink-0'
      />
    </div>
  )
}

// Rare, occasionally-present billing add-ons. Rendered as icon-only chips so
// the common case (none of these apply) costs zero row width.
function ExtraBillingIcons(props: { other: LogOtherData | null }) {
  const { t } = useTranslation()
  const other = props.other
  if (!other) return null

  const items: Array<{
    key: string
    icon: typeof Globe
    label: string
  }> = []
  if (other.web_search) {
    items.push({ key: 'web_search', icon: Globe, label: t('Web search') })
  }
  if (other.file_search) {
    items.push({
      key: 'file_search',
      icon: FileSearch,
      label: t('File search'),
    })
  }
  if (other.image_generation_call) {
    items.push({
      key: 'image_generation',
      icon: Image,
      label: t('Image generation'),
    })
  }
  if (other.reasoning_effort) {
    items.push({
      key: 'reasoning_effort',
      icon: Gauge,
      label: `${t('Reasoning effort')}: ${other.reasoning_effort}`,
    })
  }
  if (items.length === 0) return null

  return (
    <span className='inline-flex shrink-0 items-center gap-1'>
      {items.map((item) => (
        <Tooltip key={item.key}>
          <TooltipTrigger
            render={
              <span className='text-muted-foreground/70 inline-flex size-4 items-center justify-center' />
            }
          >
            <item.icon className='size-3.5' />
          </TooltipTrigger>
          <TooltipContent>{item.label}</TooltipContent>
        </Tooltip>
      ))}
    </span>
  )
}

// Group names encode the upstream provider (e.g. "claude-max", "codex"), so
// give the well-known providers a fixed brand-ish accent instead of the
// generic name-hash color; unrecognized groups keep the hash-based color.
function getGroupAccentVariant(group: string): StatusVariant | undefined {
  const g = group.toLowerCase()
  if (g.includes('claude') || g.includes('anthropic')) {
    return 'orange'
  }
  if (g.includes('codex') || g.includes('gpt') || g.includes('openai')) {
    return 'neutral'
  }
  return undefined
}

function formatGroupRatio(ratio: number): string {
  return `${Number(ratio.toFixed(2))}x`
}

function GroupChip(props: { group: string; ratio?: number }) {
  const variant = getGroupAccentVariant(props.group)
  return (
    <span className='inline-flex max-w-full min-w-0 items-center gap-1'>
      <StatusBadge
        label={props.group}
        variant={variant}
        autoColor={variant ? undefined : props.group}
        copyable={false}
        showDot={false}
        className='h-5 max-w-full truncate rounded-md px-1.5 text-[11px]'
      />
      {props.ratio != null && (
        <span className='text-muted-foreground/70 shrink-0 font-mono text-[10px] tabular-nums'>
          {formatGroupRatio(props.ratio)}
        </span>
      )}
    </span>
  )
}

function CostChip(props: { quota: number; subscription?: boolean }) {
  const { t } = useTranslation()
  return (
    <span
      className={cn(
        'border-border/80 inline-flex h-5 items-center rounded-md border px-1.5 font-mono text-[11px] font-semibold tabular-nums',
        props.subscription
          ? 'border-violet-300/60 bg-violet-50/40 !text-violet-600 dark:border-violet-900/40 dark:bg-violet-950/15 dark:!text-violet-400'
          : 'bg-amber-50/45 !text-amber-600 dark:bg-amber-950/15 dark:!text-amber-400 border-amber-200/55 dark:border-amber-900/40'
      )}
      title={
        props.subscription
          ? t('Deducted by subscription')
          : t('Cost charged to balance')
      }
    >
      {formatLogQuota(props.quota)}
    </span>
  )
}

function TokensCell(props: {
  prompt: number
  completion: number
  billingLine?: string
  showBilling?: boolean
}) {
  return (
    <div className='flex flex-col items-end font-mono text-[11px] leading-tight tabular-nums'>
      <span>{props.prompt.toLocaleString()}</span>
      <span className='text-muted-foreground'>
        {props.completion.toLocaleString()}
      </span>
      {props.showBilling && props.billingLine && (
        <span
          className='text-muted-foreground/70 max-w-full truncate text-[10px]'
          title={props.billingLine}
        >
          {props.billingLine}
        </span>
      )}
    </div>
  )
}

function CacheCell(props: {
  read: number
  write: number
  billingLine?: string
  showBilling?: boolean
}) {
  if (props.read === 0 && props.write === 0 && !props.billingLine) {
    return (
      <span className='text-muted-foreground/40 font-mono text-[11px]'>-</span>
    )
  }
  return (
    <div className='flex flex-col items-end font-mono text-[11px] leading-tight tabular-nums'>
      <span className='text-chart-3/80'>
        {props.write > 0 ? `↑${props.write.toLocaleString()}` : '-'}
      </span>
      <span className='text-sky-600/80 dark:text-sky-400/80'>
        {props.read > 0 ? `↓${props.read.toLocaleString()}` : '-'}
      </span>
      {props.showBilling && props.billingLine && (
        <span
          className='text-muted-foreground/70 max-w-full truncate text-[10px]'
          title={props.billingLine}
        >
          {props.billingLine}
        </span>
      )}
    </div>
  )
}


const performanceColorMap: Record<string, string> = {
  success: 'text-emerald-600 dark:text-emerald-400',
  warning: 'text-amber-600 dark:text-amber-400',
  danger: 'text-red-600 dark:text-red-400',
}

function PerformanceCell(props: {
  seconds: number
  completionTokens: number
  stream: boolean
  frt?: number
}) {
  if (props.seconds <= 0) {
    return (
      <span className='text-muted-foreground/40 font-mono text-[11px]'>-</span>
    )
  }
  const variant = getResponseTimeColor(props.seconds, props.completionTokens)
  const tps =
    props.completionTokens > 0
      ? Math.round(props.completionTokens / props.seconds)
      : null
  let secondaryLine: string | null = null
  if (tps != null) {
    secondaryLine = `${tps} t/s`
  } else if (props.stream && props.frt) {
    secondaryLine = formatUseTime(props.frt / 1000)
  }
  return (
    <div className='flex flex-col items-end font-mono text-[11px] leading-tight tabular-nums'>
      <span className={cn('font-semibold', performanceColorMap[variant])}>
        {formatUseTime(props.seconds)}
      </span>
      {secondaryLine && (
        <span className='text-muted-foreground/70 text-[10px]'>
          {secondaryLine}
        </span>
      )}
    </div>
  )
}

function UsageLogsStreamRowInner(props: UsageLogsStreamRowProps) {
  const { t } = useTranslation()
  const log = props.log
  const logTypeConfig = getLogTypeConfig(log.type)
  const topupInfo =
    log.type === LOG_TYPE_ENUM.TOPUP ? parseTopup(log) : undefined
  const rowTint = logTypeRowTint[log.type] ?? ''
  const other = parseLogOther(log.other)
  const channelChain: ChannelChainEntry[] = Array.isArray(other?.channel_chain)
    ? other.channel_chain
    : []

  const isTopup = log.type === LOG_TYPE_ENUM.TOPUP
  const isError = log.type === LOG_TYPE_ENUM.ERROR
  const isDisplayable = isDisplayableLogType(log.type)
  const isTiming = isTimingLogType(log.type)
  const userText = props.isAdmin
    ? maskSensitive(
        log.username || String(log.user_id || ''),
        props.sensitiveVisible
      )
    : '-'
  const groupText = maskSensitive(log.group, props.sensitiveVisible)
  const channelText =
    log.channel_name || (log.channel ? String(log.channel) : '-')
  const userGroupRatio = other?.user_group_ratio
  const isUserGroupRatio =
    userGroupRatio != null &&
    Number.isFinite(userGroupRatio) &&
    userGroupRatio !== -1
  const effectiveGroupRatio = isUserGroupRatio
    ? userGroupRatio
    : other?.group_ratio

  const cacheReadTokens = other?.cache_tokens || 0
  const cacheWrite5m = other?.cache_creation_tokens_5m || 0
  const cacheWrite1h = other?.cache_creation_tokens_1h || 0
  const hasSplitCache = cacheWrite5m > 0 || cacheWrite1h > 0
  const cacheWriteTokens = hasSplitCache
    ? cacheWrite5m + cacheWrite1h
    : other?.cache_creation_tokens || 0
  const isSubscription = other?.billing_source === 'subscription'
  const billedCosts = getLogBilledCostLabels(log, other)
  const showBillingPrices = !props.compact
  const modelInfo = formatModelName(log)

  if (props.simplifiedUserView) {
    const className = cn(
      'border-border/40 hover:bg-accent/40 flex min-h-[44px] w-full items-center border-b border-l-2 border-l-transparent px-2 py-1 text-[13px] transition-colors',
      rowTint,
      isError && 'border-l-rose-500/70 dark:border-l-rose-400/60',
      props.isNew && 'usage-log-row-new'
    )

    return (
      <div className={className}>
        <div className='flex min-w-0 flex-1 items-center gap-2'>
          <div
            className={cn(
              'flex min-w-0 items-center gap-1.5 overflow-hidden',
              SIMPLE_USER_STREAM_COLUMNS.model
            )}
          >
            {modelInfo.name ? (
              <ModelBadge
                modelName={modelInfo.name}
                actualModel={modelInfo.actualModel}
              />
            ) : (
              <span
                className='text-foreground truncate text-xs'
                title={log.content || ''}
              >
                {log.content ? log.content.slice(0, 80) : '-'}
              </span>
            )}
          </div>
          <span
            className={cn(
              'text-muted-foreground truncate text-[11px]',
              SIMPLE_USER_STREAM_COLUMNS.key
            )}
            title={props.sensitiveVisible ? log.token_name : undefined}
          >
            {maskSensitive(log.token_name, props.sensitiveVisible)}
          </span>
          <div className={cn('min-w-0', SIMPLE_USER_STREAM_COLUMNS.group)}>
            <GroupChip group={groupText} ratio={effectiveGroupRatio} />
          </div>
          <div
            className={cn(
              'text-right',
              SIMPLE_USER_STREAM_COLUMNS.performance
            )}
          >
            {isTiming ? (
              <PerformanceCell
                seconds={log.use_time}
                completionTokens={log.completion_tokens}
                stream={log.is_stream}
                frt={other?.frt}
              />
            ) : (
              <span className='text-muted-foreground/40 font-mono text-[11px]'>
                -
              </span>
            )}
          </div>
          <div className={cn('text-right', SIMPLE_USER_STREAM_COLUMNS.input)}>
            <div className='flex flex-col items-end font-mono text-[11px] leading-tight tabular-nums'>
              <span>
                {log.prompt_tokens > 0
                  ? log.prompt_tokens.toLocaleString()
                  : '-'}
              </span>
              {billedCosts.input && (
                <span
                  className='text-muted-foreground/70 max-w-full truncate text-[10px]'
                  title={billedCosts.input}
                >
                  {billedCosts.input}
                </span>
              )}
            </div>
          </div>
          <div className={cn('text-right', SIMPLE_USER_STREAM_COLUMNS.output)}>
            <div className='flex flex-col items-end font-mono text-[11px] leading-tight tabular-nums'>
              <span>
                {log.completion_tokens > 0
                  ? log.completion_tokens.toLocaleString()
                  : '-'}
              </span>
              {billedCosts.output && (
                <span
                  className='text-muted-foreground/70 max-w-full truncate text-[10px]'
                  title={billedCosts.output}
                >
                  {billedCosts.output}
                </span>
              )}
            </div>
          </div>
          <div className={cn('text-right', SIMPLE_USER_STREAM_COLUMNS.cache)}>
            <div className='flex flex-col items-end font-mono text-[11px] leading-tight tabular-nums'>
              <span>
                {cacheReadTokens + cacheWriteTokens > 0
                  ? (cacheReadTokens + cacheWriteTokens).toLocaleString()
                  : '-'}
              </span>
              {billedCosts.cacheLine && (
                <span
                  className='text-muted-foreground/70 max-w-full truncate text-[10px]'
                  title={billedCosts.cacheLine}
                >
                  {billedCosts.cacheLine}
                </span>
              )}
            </div>
          </div>
          <div className={cn('text-right', SIMPLE_USER_STREAM_COLUMNS.cost)}>
            <CostChip quota={log.quota} subscription={isSubscription} />
          </div>
        </div>
      </div>
    )
  }

  let rowBody: React.ReactNode

  if (isTopup) {
    rowBody = (
      <div className='flex min-w-0 flex-1 items-center gap-2'>
        <span className='text-muted-foreground w-[7.5rem] shrink-0 font-mono text-[11px] tabular-nums'>
          {formatTimestampToDate(log.created_at)}
        </span>
        <StatusBadge
          label={t(logTypeConfig.label)}
          variant={getLogTypeVariant(logTypeConfig.color)}
          copyable={false}
          className='shrink-0'
        />
        {topupInfo ? (
          <div className='min-w-0 flex-1'>
            <TopupContent
              topupInfo={topupInfo}
              userText={props.isAdmin ? userText : undefined}
            />
          </div>
        ) : (
          <span className='text-muted-foreground min-w-0 flex-1 truncate text-xs'>
            {log.content || '-'}
          </span>
        )}
      </div>
    )
  } else if (isDisplayable) {
    // A real column grid: every cell has a flex-grow share (see STREAM_COLUMNS,
    // shared with UsageLogsStreamHeader), so leftover width is always absorbed
    // proportionally by the visible columns instead of collecting as a gap.
    // Time and Model are fixed anchors; the rest render in the caller-supplied
    // (user-customized) order.
    const renderColumnCell = (id: StreamColumnId): React.ReactNode => {
      switch (id) {
        case 'type':
          return (
            <div key={id} className={cn('min-w-0', STREAM_COLUMNS.type)}>
              <StatusBadge
                label={t(logTypeConfig.label)}
                variant={getLogTypeVariant(logTypeConfig.color)}
                copyable={false}
              />
            </div>
          )
        case 'user':
          if (!props.isAdmin) return null
          return (
            <div
              key={id}
              className={cn(
                'min-w-0 truncate text-[11px] text-muted-foreground',
                STREAM_COLUMNS.user
              )}
              title={userText}
            >
              {userText}
            </div>
          )
        case 'group':
          return (
            <div key={id} className={cn('min-w-0', STREAM_COLUMNS.group)}>
              <GroupChip group={groupText} ratio={effectiveGroupRatio} />
            </div>
          )
        case 'channel':
          if (!props.isAdmin) return null
          return (
            <div key={id} className={cn('min-w-0', STREAM_COLUMNS.channel)}>
              <ChannelChainPopover
                chain={channelChain}
                finalChannelName={channelText}
                className='min-w-0'
              />
            </div>
          )
        case 'tokens':
          return (
            <div key={id} className={cn('text-right', STREAM_COLUMNS.tokens)}>
              <TokensCell
                prompt={log.prompt_tokens}
                completion={log.completion_tokens}
                billingLine={billedCosts.tokensLine}
                showBilling={showBillingPrices}
              />
            </div>
          )
        case 'cache':
          return (
            <div key={id} className={cn('text-right', STREAM_COLUMNS.cache)}>
              <CacheCell
                read={cacheReadTokens}
                write={cacheWriteTokens}
                billingLine={billedCosts.cacheLine}
                showBilling={showBillingPrices}
              />
            </div>
          )
        case 'cost':
          return (
            <div key={id} className={cn('text-right', STREAM_COLUMNS.cost)}>
              <CostChip quota={log.quota} subscription={isSubscription} />
            </div>
          )
        case 'performance':
          return (
            <div
              key={id}
              className={cn('text-right', STREAM_COLUMNS.performance)}
            >
              {isTiming ? (
                <PerformanceCell
                  seconds={log.use_time}
                  completionTokens={log.completion_tokens}
                  stream={log.is_stream}
                  frt={other?.frt}
                />
              ) : (
                <span className='text-muted-foreground/40 font-mono text-[11px]'>
                  -
                </span>
              )}
            </div>
          )
        default:
          return null
      }
    }

    const orderedColumns = props.columnOrder

    rowBody = (
      <div className='flex min-w-0 flex-1 items-center gap-2'>
        <span
          className={cn(
            'text-muted-foreground font-mono text-[11px] tabular-nums',
            STREAM_COLUMNS.time
          )}
        >
          {formatTimestampToDate(log.created_at)}
        </span>
        <div
          className={cn(
            'flex min-w-0 items-center gap-1.5 overflow-hidden',
            STREAM_COLUMNS.model
          )}
        >
          {modelInfo.name ? (
            <ModelBadge
              modelName={modelInfo.name}
              actualModel={modelInfo.actualModel}
            />
          ) : (
            <span
              className='text-foreground truncate text-xs'
              title={log.content || ''}
            >
              {log.content ? log.content.slice(0, 80) : '-'}
            </span>
          )}
          {!props.compact && <ExtraBillingIcons other={other} />}
        </div>
        {orderedColumns.map(renderColumnCell)}
      </div>
    )
  } else {
    const fallbackContent = log.content ? log.content.slice(0, 80) : '-'
    const primaryId = log.model_name || fallbackContent
    rowBody = (
      <div className='flex min-w-0 flex-1 items-center gap-2'>
        <span className='text-muted-foreground w-[7.5rem] shrink-0 font-mono text-[11px] tabular-nums'>
          {formatTimestampToDate(log.created_at)}
        </span>
        <StatusBadge
          label={t(logTypeConfig.label)}
          variant={getLogTypeVariant(logTypeConfig.color)}
          copyable={false}
          className='shrink-0'
        />
        <span
          className='text-foreground min-w-0 flex-1 truncate text-xs'
          title={log.content || log.model_name || ''}
        >
          {primaryId}
        </span>
      </div>
    )
  }

  const className = cn(
    'border-border/40 hover:bg-accent/50 flex w-full items-center border-b border-l-2 border-l-transparent px-2 text-[13px] transition-colors',
    props.compact ? 'h-[32px]' : 'min-h-[44px] py-1',
    rowTint,
    isError && 'border-l-rose-500/70 dark:border-l-rose-400/60',
    props.isNew && 'usage-log-row-new',
    (topupInfo || props.onRowClick) && 'cursor-pointer'
  )

  if (isTopup && topupInfo) {
    return (
      <button
        type='button'
        className={cn(className, 'text-left')}
        onClick={() => props.onTopupClick(log, topupInfo)}
      >
        {rowBody}
      </button>
    )
  }

  if (props.onRowClick) {
    return (
      <button
        type='button'
        className={cn(className, 'text-left')}
        onClick={() => props.onRowClick?.(log)}
      >
        {rowBody}
      </button>
    )
  }

  return <div className={className}>{rowBody}</div>
}

export const UsageLogsStreamRow = memo(UsageLogsStreamRowInner)
