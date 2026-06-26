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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { parseTopup } from './parse-topup'

function makeLog(content: string, adminInfo?: Record<string, unknown>) {
  return {
    content,
    other: adminInfo ? JSON.stringify({ admin_info: adminInfo }) : '',
  }
}

const templateCases: Array<{
  name: string
  log: ReturnType<typeof makeLog>
  kind: string
  rechargeQuotaText: string | null
  payAmount: number
  planTitle: string | null
}> = [
  {
    name: 'epay online top-up',
    log: makeLog(
      '使用在线充值成功，充值金额: ＄20.000000 额度，支付金额：20.000000',
      {
        payment_method: 'alipay',
        callback_payment_method: 'epay',
      }
    ),
    kind: 'online',
    rechargeQuotaText: '＄20.000000 额度',
    payAmount: 20,
    planTitle: null,
  },
  {
    name: 'stripe top-up',
    log: makeLog('使用在线充值成功，充值金额: $30.000000 额度，支付金额：30', {
      payment_method: 'stripe',
      callback_payment_method: 'stripe',
    }),
    kind: 'stripe',
    rechargeQuotaText: '$30.000000 额度',
    payAmount: 30,
    planTitle: null,
  },
  {
    name: 'admin completed top-up',
    log: makeLog(
      '管理员补单成功，充值金额: ＄5.500000 额度，支付金额：5.500000',
      {
        payment_method: 'admin',
        callback_payment_method: 'admin',
      }
    ),
    kind: 'admin',
    rechargeQuotaText: '＄5.500000 额度',
    payAmount: 5.5,
    planTitle: null,
  },
  {
    name: 'creem top-up',
    log: makeLog('使用Creem充值成功，充值额度: 100000，支付金额：12.34', {
      payment_method: 'creem',
      callback_payment_method: 'creem',
    }),
    kind: 'creem',
    rechargeQuotaText: '100000',
    payAmount: 12.34,
    planTitle: null,
  },
  {
    name: 'waffo top-up',
    log: makeLog('Waffo充值成功，充值额度: ＄9.000000 额度，支付金额: 9.01', {
      payment_method: 'waffo',
      callback_payment_method: 'waffo',
    }),
    kind: 'waffo',
    rechargeQuotaText: '＄9.000000 额度',
    payAmount: 9.01,
    planTitle: null,
  },
  {
    name: 'waffo pancake top-up',
    log: makeLog(
      'Waffo Pancake充值成功，充值额度: ＄8.000000 额度，支付金额: 8.02'
    ),
    kind: 'waffo_pancake',
    rechargeQuotaText: '＄8.000000 额度',
    payAmount: 8.02,
    planTitle: null,
  },
  {
    name: 'subscription purchase',
    log: makeLog(
      '订阅购买成功，套餐: Pro 月付，支付金额: 19.99，支付方式: stripe',
      {
        payment_method: 'stripe',
      }
    ),
    kind: 'subscription',
    rechargeQuotaText: null,
    payAmount: 19.99,
    planTitle: 'Pro 月付',
  },
  {
    name: 'balance subscription purchase',
    log: makeLog(
      '使用余额购买订阅成功，套餐: Team，支付金额: 0.00，扣除额度: 200000'
    ),
    kind: 'balance_sub',
    rechargeQuotaText: '200000',
    payAmount: 0,
    planTitle: 'Team',
  },
]

describe('parseTopup', () => {
  for (const templateCase of templateCases) {
    test(`parses ${templateCase.name}`, () => {
      const result = parseTopup(templateCase.log)

      assert.equal(result.kind, templateCase.kind)
      assert.equal(result.rechargeQuotaText, templateCase.rechargeQuotaText)
      assert.equal(result.payAmount, templateCase.payAmount)
      assert.equal(result.planTitle, templateCase.planTitle)
      assert.equal(result.completed, true)
      assert.equal(result.raw, templateCase.log.content)
    })
  }

  test('prefers structured payment method over online content keyword', () => {
    const result = parseTopup(
      makeLog('使用在线充值成功，充值金额: ＄30.000000 额度，支付金额：30', {
        payment_method: 'stripe',
        callback_payment_method: 'epay',
      })
    )

    assert.equal(result.kind, 'stripe')
  })

  test('accepts full-width and half-width colons around amounts', () => {
    const result = parseTopup(
      makeLog('使用Creem充值成功，充值额度：＄0.010000 额度，支付金额: 0.01')
    )

    assert.equal(result.kind, 'creem')
    assert.equal(result.rechargeQuotaText, '＄0.010000 额度')
    assert.equal(result.payAmount, 0.01)
  })

  test('falls back to raw content when required fields are missing', () => {
    const content = 'Waffo充值成功，支付金额: 9.01'
    const result = parseTopup(makeLog(content, { payment_method: 'waffo' }))

    assert.equal(result.kind, 'unknown')
    assert.equal(result.rechargeQuotaText, null)
    assert.equal(result.payAmount, null)
    assert.equal(result.raw, content)
  })

  test('falls back to raw content for unknown templates', () => {
    const content = '充值已完成，但这不是已知模板'
    const result = parseTopup(makeLog(content))

    assert.equal(result.kind, 'unknown')
    assert.equal(result.rechargeQuotaText, null)
    assert.equal(result.payAmount, null)
    assert.equal(result.raw, content)
  })
})
