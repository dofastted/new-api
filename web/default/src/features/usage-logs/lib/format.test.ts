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

import {
  calculateLogBilledQuotaParts,
  formatModelName,
  getLogBilledCostLabels,
} from './format'

import type { UsageLog } from '../data/schema'

function makeLog(
  modelName: string,
  other: Record<string, unknown> = {}
): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: 0,
    type: 2,
    content: '',
    username: 'tester',
    token_name: 'key',
    model_name: modelName,
    quota: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    use_time: 0,
    is_stream: false,
    channel: 0,
    channel_name: '',
    token_id: 0,
    group: 'default',
    ip: '',
    other: JSON.stringify(other),
  } as UsageLog
}

describe('formatModelName', () => {
  test('marks mapped models and exposes the upstream model', () => {
    const result = formatModelName(
      makeLog('gpt-4o', {
        upstream_model: 'gpt-4o-2024-08-06',
        is_model_mapped: true,
      })
    )

    assert.equal(result.name, 'gpt-4o')
    assert.equal(result.isMapped, true)
    assert.equal(result.actualModel, 'gpt-4o-2024-08-06')
  })

  test('returns unmapped when no upstream model is present', () => {
    const result = formatModelName(makeLog('gpt-4o'))

    assert.equal(result.name, 'gpt-4o')
    assert.equal(result.isMapped, false)
    assert.equal(result.actualModel, undefined)
  })

  test('honors is_model_mapped flag even when upstream equals request model', () => {
    const result = formatModelName(
      makeLog('gpt-4o', { upstream_model: 'gpt-4o', is_model_mapped: true })
    )

    assert.equal(result.isMapped, true)
    assert.equal(result.actualModel, 'gpt-4o')
  })
})

describe('calculateLogBilledQuotaParts', () => {
  test('computes actual billed quota after cache and group ratio', () => {
    // prompt=1000, cache=800, completion=100, model=1, completion=3, cache=0.25, group=0.1
    // base input = 200, inputQuota = round(200*0.1)=20
    // output = round(100*3*0.1)=30
    // cache read = round(800*0.25*0.1)=20
    const parts = calculateLogBilledQuotaParts(
      { prompt_tokens: 1000, completion_tokens: 100 },
      {
        model_ratio: 1,
        completion_ratio: 3,
        cache_ratio: 0.25,
        cache_tokens: 800,
        group_ratio: 0.1,
      }
    )

    assert.deepEqual(parts, {
      inputQuota: 20,
      outputQuota: 30,
      cacheReadQuota: 20,
      cacheWriteQuota: 0,
    })
  })

  test('returns null for per-call billing', () => {
    const parts = calculateLogBilledQuotaParts(
      { prompt_tokens: 10, completion_tokens: 1 },
      { model_price: 0.05, model_ratio: 1 }
    )
    assert.equal(parts, null)
  })

  test('returns null when model ratio missing', () => {
    const parts = calculateLogBilledQuotaParts(
      { prompt_tokens: 10, completion_tokens: 1 },
      { completion_ratio: 3, cache_ratio: 0.25 }
    )
    assert.equal(parts, null)
  })
})

describe('getLogBilledCostLabels', () => {
  test('formats non-zero billed fragments', () => {
    const labels = getLogBilledCostLabels(
      { prompt_tokens: 1000, completion_tokens: 100 },
      {
        model_ratio: 1,
        completion_ratio: 3,
        cache_ratio: 0.25,
        cache_tokens: 800,
        group_ratio: 0.1,
      }
    )

    assert.ok(labels.input)
    assert.ok(labels.output)
    assert.ok(labels.cacheRead)
    assert.equal(labels.tokensLine, `${labels.input} / ${labels.output}`)
    assert.equal(labels.cacheLine, labels.cacheRead)
  })
})
