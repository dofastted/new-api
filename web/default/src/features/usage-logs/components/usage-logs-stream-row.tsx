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
import { CheckCircle2 } from 'lucide-react'
import { memo } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import {
  formatLogQuota,
  formatTimestampToDate,
  formatTokens,
  formatUseTime,
} from '@/lib/format'
import { cn } from '@/lib/utils'

import { LOG_TYPE_ENUM } from '../constants'
import type { UsageLog } from '../data/schema'
import { ChannelChainPopover } from './channel-chain-popover'
import { parseLogOther } from '../lib/format'
import { parseTopup, type TopupInfo } from '../lib/parse-topup'
import { getLogTypeConfig, isDisplayableLogType } from '../lib/utils'
import type { ChannelChainEntry } from '../types'

interface UsageLogsStreamRowProps {
  log: UsageLog
  isAdmin: boolean
  sensitiveVisible: boolean
  isNew?: boolean
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

function SecondaryChip(props: { label: string; value: string }) {
  if (!props.value || props.value === '-') return null
  return (
    <span className='text-muted-foreground inline-flex items-center gap-1 text-[11px]'>
      <span className='text-muted-foreground/70'>{props.label}</span>
      <span className='font-mono tabular-nums'>{props.value}</span>
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
  const totalTokens = (log.prompt_tokens || 0) + (log.completion_tokens || 0)
  const other = parseLogOther(log.other)
  const channelChain: ChannelChainEntry[] = Array.isArray(other?.channel_chain)
    ? other.channel_chain
    : []

  const isTopup = log.type === LOG_TYPE_ENUM.TOPUP
  const userText = props.isAdmin
    ? maskSensitive(
        log.username || String(log.user_id || ''),
        props.sensitiveVisible
      )
    : '-'
  const groupText = maskSensitive(log.group, props.sensitiveVisible)
  const channelText =
    log.channel_name || (log.channel ? String(log.channel) : '-')
  const quotaText = isDisplayableLogType(log.type)
    ? formatLogQuota(log.quota)
    : '-'
  const finalChannelName = channelText

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
    <div className='flex min-w-0 flex-1 flex-col gap-0.5'>
      {/* Line 1: time · type · primary id · chain popover */}
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
        {props.isAdmin && !isTopup && (
          <ChannelChainPopover
            chain={channelChain}
            finalChannelName={finalChannelName}
            className='shrink-0'
          />
        )}
      </div>

      {/* Line 2: muted secondary chips */}
      <div className='flex min-w-0 flex-wrap items-center gap-x-3 gap-y-0.5 pl-[7rem]'>
        {props.isAdmin && (
          <SecondaryChip label={t('User')} value={userText} />
        )}
        {!isTopup && (
          <>
            <SecondaryChip label={t('Tokens')} value={formatTokens(totalTokens)} />
            <SecondaryChip label={t('Cost')} value={quotaText} />
            <SecondaryChip label={t('Time')} value={formatUseTime(log.use_time)} />
            <SecondaryChip label={t('Group')} value={groupText} />
          </>
        )}
        {props.isAdmin && (
          <SecondaryChip
            label='IP'
            value={maskSensitive(log.ip, props.sensitiveVisible)}
          />
        )}
      </div>
    </div>
  )

  const className = cn(
    'border-border/40 hover:bg-accent/50 flex h-[52px] w-full items-center border-b border-l-2 border-l-transparent px-2 text-[13px] transition-colors',
    rowTint,
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
