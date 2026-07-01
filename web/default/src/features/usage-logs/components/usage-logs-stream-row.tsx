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

import { LOG_TYPE_ENUM } from '../constants'
import type { UsageLog } from '../data/schema'
import {
  formatModelName,
  getFirstResponseTimeColor,
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
  /** Compact display density: hides the secondary metadata line. */
  compact?: boolean
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

function TopupContent(props: { topupInfo: TopupInfo }) {
  const { t } = useTranslation()
  const info = props.topupInfo

  if (info.kind === 'unknown') {
    return <span className='text-muted-foreground truncate'>{info.raw}</span>
  }

  return (
    <div className='flex min-w-0 items-center gap-2 overflow-hidden'>
      <StatusBadge
        label={t(info.channelLabelKey)}
        variant={info.channelVariant}
        copyable={false}
      />
      {info.rechargeQuotaText && (
        <span className='border-border/70 bg-background/70 inline-flex h-6 min-w-0 items-center rounded-md border px-2 font-mono text-xs tabular-nums'>
          <span className='truncate'>{info.rechargeQuotaText}</span>
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

function SecondaryChip(props: {
  label: string
  value: string
  accent?: string
  mono?: boolean
}) {
  if (!props.value || props.value === '-') return null
  return (
    <span className='text-muted-foreground inline-flex items-center gap-1 text-[11px]'>
      <span className='text-muted-foreground/65'>{props.label}</span>
      <span
        className={cn(
          'inline-flex rounded px-0.5 tabular-nums',
          props.mono && 'font-mono',
          props.accent
        )}
      >
        {props.value}
      </span>
    </span>
  )
}

const timingBgMap: Record<string, string> = {
  success:
    'border border-emerald-200/45 bg-emerald-50/35 !text-emerald-600 dark:border-emerald-900/40 dark:bg-emerald-950/15 dark:!text-emerald-400',
  warning:
    'border border-amber-200/50 bg-amber-50/35 !text-amber-600 dark:border-amber-900/40 dark:bg-amber-950/15 dark:!text-amber-400',
  danger:
    'border border-rose-200/55 bg-rose-50/35 !text-red-600 dark:border-rose-900/40 dark:bg-rose-950/15 dark:!text-red-400',
  neutral:
    'border border-border/60 bg-muted/30 dark:border-border/40 dark:bg-muted/20',
}

function TimingChip(props: {
  seconds: number
  completionTokens: number
  stream: boolean
  frt?: number
}) {
  const variant = getResponseTimeColor(props.seconds, props.completionTokens)
  const tps =
    props.seconds > 0 && props.completionTokens > 0
      ? Math.round(props.completionTokens / props.seconds)
      : null
  const frtVariant = props.frt
    ? getFirstResponseTimeColor(props.frt / 1000)
    : null
  return (
    <span className='inline-flex items-center gap-1'>
      <StatusBadge
        label={formatUseTime(props.seconds)}
        variant={variant as StatusVariant}
        copyable={false}
        showDot={false}
        className={cn(
          'h-5 rounded-md px-1 font-mono text-[11px] !text-foreground',
          timingBgMap[variant]
        )}
      />
      {props.stream && props.frt != null && props.frt > 0 && (
        <StatusBadge
          label={formatUseTime(props.frt / 1000)}
          variant={(frtVariant ?? 'neutral') as StatusVariant}
          copyable={false}
          showDot={false}
          className={cn(
            'h-5 rounded-md px-1 font-mono text-[11px] !text-foreground',
            timingBgMap[frtVariant ?? 'neutral']
          )}
        />
      )}
      {tps != null && (
        <span className='text-muted-foreground/70 font-mono text-[10px] tabular-nums'>
          {tps}
          <span className='text-muted-foreground/45'> t/s</span>
        </span>
      )}
    </span>
  )
}

function TokensChip(props: {
  prompt: number
  completion: number
  cacheRead: number
  cacheWrite: number
}) {
  const { t } = useTranslation()
  const total = props.prompt + props.completion
  if (total === 0) return null
  return (
    <span className='inline-flex items-center gap-1 text-[11px]'>
      <span
        className='bg-chart-1/10 text-chart-1 inline-flex rounded px-1 font-mono tabular-nums'
        title={`${t('Input tokens')}: ${props.prompt.toLocaleString()} / ${t(
          'Output tokens'
        )}: ${props.completion.toLocaleString()}`}
      >
        {props.prompt.toLocaleString()} / {props.completion.toLocaleString()}
      </span>
      {props.cacheRead > 0 && (
        <span className='font-mono text-sky-600/70 tabular-nums dark:text-sky-400/70'>
          ↓{props.cacheRead.toLocaleString()}
        </span>
      )}
      {props.cacheWrite > 0 && (
        <span className='text-chart-3/70 font-mono tabular-nums'>
          ↑{props.cacheWrite.toLocaleString()}
        </span>
      )}
    </span>
  )
}

function ErrorBadge(props: { label: string }) {
  return (
    <StatusBadge
      label={props.label}
      variant='red'
      copyable={false}
      showDot={false}
      className='h-5 max-w-[10rem] shrink-0 truncate rounded-md px-1.5 font-mono text-[11px]'
    />
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
    <span className='inline-flex min-w-0 shrink-0 items-center gap-1'>
      <StatusBadge
        label={props.group}
        variant={variant}
        autoColor={variant ? undefined : props.group}
        copyable={false}
        showDot={false}
        className='h-5 max-w-[8rem] truncate rounded-md px-1.5 text-[11px]'
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
  const lastChainEntry = channelChain.at(-1)
  const errorBadgeLabel =
    lastChainEntry?.error_code || lastChainEntry?.error_category || undefined
  const userText = props.isAdmin
    ? maskSensitive(
        log.username || String(log.user_id || ''),
        props.sensitiveVisible
      )
    : '-'
  const groupText = maskSensitive(log.group, props.sensitiveVisible)
  const channelText =
    log.channel_name || (log.channel ? String(log.channel) : '-')
  const finalChannelName = channelText
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
  const modelInfo = formatModelName(log)

  // Primary identifier content depends on log type.
  let primaryContent: React.ReactNode
  if (isTopup) {
    primaryContent = topupInfo ? (
      <div className='min-w-0 flex-1'>
        <TopupContent topupInfo={topupInfo} />
      </div>
    ) : (
      <span className='text-muted-foreground min-w-0 flex-1 truncate text-xs'>
        {log.content || '-'}
      </span>
    )
  } else if (isDisplayable && modelInfo.name) {
    // No flex-1 here: the badge has no truncation need, and letting it grow
    // just pushes the group/channel/cost badges away from it, leaving a
    // visible dead gap in the middle of the row.
    primaryContent = (
      <ModelBadge
        modelName={modelInfo.name}
        actualModel={modelInfo.actualModel}
      />
    )
  } else {
    const fallbackContent = log.content ? log.content.slice(0, 80) : '-'
    const primaryId = log.model_name || fallbackContent
    primaryContent = (
      <span
        className='text-foreground min-w-0 flex-1 truncate text-xs'
        title={log.content || log.model_name || ''}
      >
        {primaryId}
      </span>
    )
  }

  const rowBody = (
    <div className='flex min-w-0 flex-1 flex-col gap-1'>
      {/* Line 1: time · type · model · chain popover */}
      <div className='flex min-w-0 items-center gap-2'>
        <span className='text-muted-foreground w-[7rem] shrink-0 font-mono text-[11px] tabular-nums'>
          {formatTimestampToDate(log.created_at)}
        </span>
        <StatusBadge
          label={t(logTypeConfig.label)}
          variant={getLogTypeVariant(logTypeConfig.color)}
          copyable={false}
          className='shrink-0'
        />
        {primaryContent}
        {isError && errorBadgeLabel && <ErrorBadge label={errorBadgeLabel} />}
        {!props.compact && <ExtraBillingIcons other={other} />}
        {!isTopup && isDisplayable && (
          <GroupChip group={groupText} ratio={effectiveGroupRatio} />
        )}
        {props.isAdmin && !isTopup && !props.compact && (
          <ChannelChainPopover
            chain={channelChain}
            finalChannelName={finalChannelName}
            className='shrink-0'
          />
        )}
        {/* Emphasis chips on the right: cost · tokens · timing */}
        {!isTopup && isDisplayable && (
          <div className='flex shrink-0 items-center gap-2'>
            <CostChip quota={log.quota} subscription={isSubscription} />
            {!props.compact && (
              <>
                <TokensChip
                  prompt={log.prompt_tokens}
                  completion={log.completion_tokens}
                  cacheRead={cacheReadTokens}
                  cacheWrite={cacheWriteTokens}
                />
                {isTiming && (
                  <TimingChip
                    seconds={log.use_time}
                    completionTokens={log.completion_tokens}
                    stream={log.is_stream}
                    frt={other?.frt}
                  />
                )}
              </>
            )}
          </div>
        )}
      </div>

      {/* Line 2: muted secondary metadata */}
      {!props.compact && (
        <div className='flex min-w-0 flex-wrap items-center gap-x-3 gap-y-0.5 pl-[7rem]'>
          {props.isAdmin && (
            <SecondaryChip label={t('User')} value={userText} />
          )}
          {!isTopup && isDisplayable && props.isAdmin && (
            <SecondaryChip label={t('Channel')} value={channelText} mono />
          )}
          {!isTopup && isDisplayable && (
            <SecondaryChip
              label={`${t('Stream')} / ${t('Non-stream')}`}
              value={log.is_stream ? t('Stream') : t('Non-stream')}
            />
          )}
          {props.isAdmin && (
            <SecondaryChip
              label='IP'
              value={maskSensitive(log.ip, props.sensitiveVisible)}
              mono
            />
          )}
        </div>
      )}
    </div>
  )

  const className = cn(
    'border-border/40 hover:bg-accent/50 flex w-full items-center border-b border-l-2 border-l-transparent px-2 text-[13px] transition-colors',
    props.compact ? 'h-[36px]' : 'h-[56px]',
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
