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
import { CheckCircle2 } from 'lucide-react'
import { memo } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import {
  formatLogQuota,
  formatTimestampToDate,
  formatTokens,
} from '@/lib/format'
import { cn } from '@/lib/utils'

import { LOG_TYPE_ENUM } from '../constants'
import type { UsageLog } from '../data/schema'
import { parseTopup, type TopupInfo } from '../lib/parse-topup'
import { getLogTypeConfig, isDisplayableLogType } from '../lib/utils'

interface UsageLogsStreamRowProps {
  log: UsageLog
  isAdmin: boolean
  sensitiveVisible: boolean
  onTopupClick: (log: UsageLog, topupInfo: TopupInfo) => void
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

function StreamCell(props: {
  children: React.ReactNode
  className?: string
  title?: string
}) {
  return (
    <div
      className={cn(
        'min-w-0 truncate px-2 text-left leading-tight',
        props.className
      )}
      title={props.title}
    >
      {props.children}
    </div>
  )
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

function UsageLogsStreamRowInner(props: UsageLogsStreamRowProps) {
  const { t } = useTranslation()
  const log = props.log
  const logTypeConfig = getLogTypeConfig(log.type)
  const topupInfo =
    log.type === LOG_TYPE_ENUM.TOPUP ? parseTopup(log) : undefined
  const rowTint = logTypeRowTint[log.type] ?? ''
  const totalTokens = (log.prompt_tokens || 0) + (log.completion_tokens || 0)
  const userText = props.isAdmin
    ? maskSensitive(log.username || String(log.user_id || ''), props.sensitiveVisible)
    : '-'
  const tokenText = maskSensitive(log.token_name, props.sensitiveVisible)
  const groupText = maskSensitive(log.group, props.sensitiveVisible)
  const channelText =
    log.channel_name || (log.channel ? String(log.channel) : '-')
  const quotaText = isDisplayableLogType(log.type) ? formatLogQuota(log.quota) : '-'

  const rowContent = (
    <>
      <StreamCell className='w-[8.5rem] shrink-0 font-mono text-xs tabular-nums'>
        {formatTimestampToDate(log.created_at)}
      </StreamCell>
      <StreamCell className='w-[5.8rem] shrink-0'>
        <StatusBadge
          label={t(logTypeConfig.label)}
          variant={getLogTypeVariant(logTypeConfig.color)}
          copyable={false}
        />
      </StreamCell>
      {props.isAdmin && (
        <StreamCell className='w-[7rem] shrink-0 text-xs' title={userText}>
          {userText}
        </StreamCell>
      )}
      <StreamCell className='w-[7rem] shrink-0 text-xs' title={tokenText}>
        {tokenText}
      </StreamCell>
      <StreamCell className='w-[10rem] shrink-0 text-xs' title={log.model_name}>
        {log.model_name || '-'}
      </StreamCell>
      {props.isAdmin && (
        <StreamCell className='w-[7rem] shrink-0 text-xs' title={channelText}>
          {channelText}
        </StreamCell>
      )}
      <StreamCell className='w-[5.8rem] shrink-0 font-mono text-xs tabular-nums'>
        {formatTokens(totalTokens)}
      </StreamCell>
      <StreamCell className='w-[7rem] shrink-0 font-mono text-xs tabular-nums'>
        {quotaText}
      </StreamCell>
      <StreamCell className='w-[6rem] shrink-0 text-xs' title={groupText}>
        {groupText}
      </StreamCell>
      <StreamCell
        className='min-w-[15rem] flex-1 text-xs'
        title={topupInfo ? topupInfo.raw : log.content}
      >
        {topupInfo ? <TopupContent topupInfo={topupInfo} /> : log.content || '-'}
      </StreamCell>
      <StreamCell className='w-[7rem] shrink-0 font-mono text-xs tabular-nums'>
        {maskSensitive(log.ip, props.sensitiveVisible)}
      </StreamCell>
    </>
  )

  const className = cn(
    'border-border/40 flex h-[52px] w-full items-center border-b border-l-2 border-l-transparent text-[13px] transition-colors hover:bg-accent/50',
    rowTint,
    topupInfo && 'cursor-pointer'
  )

  if (topupInfo) {
    return (
      <button
        type='button'
        className={cn(className, 'text-left')}
        onClick={() => props.onTopupClick(log, topupInfo)}
      >
        {rowContent}
      </button>
    )
  }

  return <div className={className}>{rowContent}</div>
}

export const UsageLogsStreamRow = memo(UsageLogsStreamRowInner)
