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
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Progress } from '@/components/ui/progress'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { UsageLog } from '../data/schema'
import type { TopupInfo } from '../lib/parse-topup'

interface TopupOrderDetailProps {
  log: UsageLog | null
  topupInfo: TopupInfo | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

function formatPayAmount(amount: number | null): string {
  if (amount == null) return '-'
  return amount.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 6,
  })
}

function DetailItem(props: {
  label: string
  value: React.ReactNode
  className?: string
}) {
  return (
    <div className={cn('bg-muted/30 rounded-md px-3 py-2', props.className)}>
      <div className='text-muted-foreground mb-1 text-[11px] leading-none font-medium'>
        {props.label}
      </div>
      <div className='min-w-0 text-sm leading-5 font-medium break-words'>
        {props.value}
      </div>
    </div>
  )
}

export function TopupOrderDetail(props: TopupOrderDetailProps) {
  const { t } = useTranslation()
  const log = props.log
  const topupInfo = props.topupInfo

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[85dvh] overflow-y-auto sm:max-w-xl'>
        <DialogHeader>
          <DialogTitle>{t('Top-up order details')}</DialogTitle>
          <DialogDescription>
            {t('This log records a completed top-up entry.')}
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-4'>
          <div className='border-border/70 bg-success/5 rounded-lg border p-3'>
            <div className='mb-2 flex items-center justify-between gap-3'>
              <div className='text-sm font-medium'>{t('Payment progress')}</div>
              <StatusBadge
                label={t('Completed')}
                icon={CheckCircle2}
                variant='green'
                copyable={false}
              />
            </div>
            <Progress value={100} className='[&_[data-slot=progress-indicator]]:bg-success' />
          </div>

          <div className='grid grid-cols-1 gap-2 sm:grid-cols-2'>
            <DetailItem
              label={t('Order time')}
              value={formatTimestampToDate(log?.created_at)}
            />
            <DetailItem
              label={t('Completed')}
              value={
                <StatusBadge
                  label={t('Yes')}
                  icon={CheckCircle2}
                  variant='green'
                  copyable={false}
                />
              }
            />
            <DetailItem
              label={t('Payment channel')}
              value={
                topupInfo ? (
                  <StatusBadge
                    label={t(topupInfo.channelLabelKey)}
                    variant={topupInfo.channelVariant}
                    copyable={false}
                  />
                ) : (
                  '-'
                )
              }
            />
            <DetailItem
              label={t('Payment amount')}
              value={
                <span className='font-mono tabular-nums'>
                  {formatPayAmount(topupInfo?.payAmount ?? null)}
                </span>
              }
            />
            <DetailItem
              label={t('Recharge quota')}
              value={
                <span className='font-mono tabular-nums'>
                  {topupInfo?.rechargeQuotaText || '-'}
                </span>
              }
            />
            {topupInfo?.planTitle && (
              <DetailItem label={t('Plan')} value={topupInfo.planTitle} />
            )}
          </div>

          <DetailItem
            label={t('Raw content')}
            value={
              <span className='text-muted-foreground text-xs leading-5'>
                {topupInfo?.raw || log?.content || '-'}
              </span>
            }
            className='bg-transparent px-0 py-0'
          />
        </div>
      </DialogContent>
    </Dialog>
  )
}
