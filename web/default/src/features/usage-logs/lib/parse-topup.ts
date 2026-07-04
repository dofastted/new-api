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
import type { StatusBadgeProps } from '@/components/status-badge'

import { TOPUP_CHANNEL_META } from '../constants'
import type { UsageLog } from '../data/schema'
import type { LogOtherData, TopupKind } from '../types'
import { parseLogOther } from './format'

export interface TopupInfo {
  kind: TopupKind
  channelLabelKey: string
  channelVariant: StatusBadgeProps['variant']
  rechargeQuotaText: string | null
  payAmount: number | null
  planTitle: string | null
  quotaDelta: number | null
  balanceAfter: number | null
  completed: true
  raw: string
}

interface ExtractedTopupFields {
  kind: TopupKind
  rechargeQuotaText: string | null
  payAmount: number | null
  planTitle: string | null
}

const AMOUNT_PATTERN = '([+-]?(?:\\d+(?:\\.\\d+)?|\\.\\d+))'

function readFiniteNumber(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

function createTopupInfo(
  raw: string,
  fields: ExtractedTopupFields,
  other?: LogOtherData | null
): TopupInfo {
  const meta = TOPUP_CHANNEL_META[fields.kind]
  return {
    kind: fields.kind,
    channelLabelKey: meta.labelKey,
    channelVariant: meta.variant,
    rechargeQuotaText: fields.rechargeQuotaText,
    payAmount: readFiniteNumber(other?.topup?.pay_amount) ?? fields.payAmount,
    planTitle: fields.planTitle,
    quotaDelta: readFiniteNumber(other?.topup?.quota_delta),
    balanceAfter: readFiniteNumber(other?.topup?.balance_after),
    completed: true,
    raw,
  }
}

function createUnknownTopupInfo(
  raw: string,
  other?: LogOtherData | null
): TopupInfo {
  return createTopupInfo(raw, {
    kind: 'unknown',
    rechargeQuotaText: null,
    payAmount: null,
    planTitle: null,
  }, other)
}

function normalizeMethod(method: string | undefined): string {
  return method?.trim().toLowerCase().replaceAll('-', '_') ?? ''
}

function kindFromPaymentMethod(method: string | undefined): TopupKind {
  const normalized = normalizeMethod(method)
  if (!normalized) return 'unknown'
  if (normalized === 'stripe') return 'stripe'
  if (normalized === 'creem') return 'creem'
  if (normalized === 'waffo') return 'waffo'
  if (normalized === 'waffo_pancake') return 'waffo_pancake'
  if (normalized === 'admin') return 'admin'
  if (normalized === 'balance') return 'balance_sub'
  if (normalized === 'epay') return 'online'
  return 'online'
}

function kindFromOther(other: LogOtherData | null): TopupKind {
  const adminInfo = other?.admin_info
  const paymentMethod = kindFromPaymentMethod(adminInfo?.payment_method)
  if (paymentMethod !== 'unknown') return paymentMethod
  return kindFromPaymentMethod(adminInfo?.callback_payment_method)
}

function kindFromContent(content: string): TopupKind {
  if (content.includes('使用余额购买订阅成功')) return 'balance_sub'
  if (content.includes('订阅购买成功')) return 'subscription'
  if (content.includes('Waffo Pancake')) return 'waffo_pancake'
  if (content.includes('Waffo')) return 'waffo'
  if (content.includes('Creem')) return 'creem'
  if (content.includes('管理员补单')) return 'admin'
  if (content.includes('在线充值')) return 'online'
  return 'unknown'
}

function resolveKind(
  preferredKind: TopupKind,
  contentKind: TopupKind
): TopupKind {
  if (contentKind === 'subscription' || contentKind === 'balance_sub') {
    return contentKind
  }
  if (preferredKind !== 'unknown') return preferredKind
  return contentKind
}

function parsePayAmount(value: string | undefined): number | null {
  if (!value) return null
  const amount = Number(value)
  return Number.isFinite(amount) ? amount : null
}

function matchTopupAmounts(
  content: string
): { quota: string; pay: number } | null {
  const match = content.match(
    new RegExp(
      `(?:充值金额|充值额度)\\s*[:：]\\s*(.+?)\\s*[，,]\\s*支付金额\\s*[:：]\\s*${AMOUNT_PATTERN}`
    )
  )
  if (!match) return null
  const pay = parsePayAmount(match[2])
  if (pay == null) return null
  return { quota: match[1].trim(), pay }
}

function matchSubscription(content: string): {
  planTitle: string
  pay: number
} | null {
  const match = content.match(
    new RegExp(
      `订阅购买成功\\s*[，,]\\s*套餐\\s*[:：]\\s*(.+?)\\s*[，,]\\s*支付金额\\s*[:：]\\s*${AMOUNT_PATTERN}\\s*[，,]\\s*支付方式\\s*[:：]\\s*(.+)$`
    )
  )
  if (!match) return null
  const pay = parsePayAmount(match[2])
  if (pay == null) return null
  return { planTitle: match[1].trim(), pay }
}

function matchBalanceSubscription(content: string): {
  planTitle: string
  pay: number
  quota: string
} | null {
  const match = content.match(
    new RegExp(
      `使用余额购买订阅成功\\s*[，,]\\s*套餐\\s*[:：]\\s*(.+?)\\s*[，,]\\s*支付金额\\s*[:：]\\s*${AMOUNT_PATTERN}\\s*[，,]\\s*扣除额度\\s*[:：]\\s*(.+)$`
    )
  )
  if (!match) return null
  const pay = parsePayAmount(match[2])
  if (pay == null) return null
  return { planTitle: match[1].trim(), pay, quota: match[3].trim() }
}

function extractTopupFields(
  content: string,
  preferredKind: TopupKind
): ExtractedTopupFields | null {
  const contentKind = kindFromContent(content)
  const kind = resolveKind(preferredKind, contentKind)

  if (kind === 'subscription') {
    const subscription = matchSubscription(content)
    if (!subscription) return null
    return {
      kind,
      rechargeQuotaText: null,
      payAmount: subscription.pay,
      planTitle: subscription.planTitle,
    }
  }

  if (kind === 'balance_sub') {
    const subscription = matchBalanceSubscription(content)
    if (!subscription) return null
    return {
      kind,
      rechargeQuotaText: subscription.quota,
      payAmount: subscription.pay,
      planTitle: subscription.planTitle,
    }
  }

  if (kind === 'unknown') return null
  const amounts = matchTopupAmounts(content)
  if (!amounts) return null

  return {
    kind,
    rechargeQuotaText: amounts.quota,
    payAmount: amounts.pay,
    planTitle: null,
  }
}

export function parseTopup(
  log: Pick<UsageLog, 'content' | 'other'>
): TopupInfo {
  const raw = log.content || ''
  const other = parseLogOther(log.other)
  const preferredKind = kindFromOther(other)
  const fields = extractTopupFields(raw, preferredKind)

  if (!fields) return createUnknownTopupInfo(raw, other)
  return createTopupInfo(raw, fields, other)
}
