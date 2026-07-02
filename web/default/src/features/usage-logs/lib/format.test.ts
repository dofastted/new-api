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

import { formatModelName } from './format'

import type { UsageLog } from '../data/schema'

function makeLog(modelName: string, other: Record<string, unknown> = {}): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: 1000,
    type: 2,
    content: '',
    username: '',
    token_name: '',
    model_name: modelName,
    quota: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    use_time: 0,
    is_stream: false,
    channel: 0,
    channel_name: '',
    token_id: 0,
    group: '',
    ip: '',
    other: JSON.stringify(other),
    request_id: '',
    upstream_request_id: '',
  }
}

describe('formatModelName', () => {
  test('marks mapped when upstream_model differs from request model', () => {
    const result = formatModelName(
      makeLog('gpt-4o', { upstream_model: 'gpt-4o-2024-08-06' })
    )

    assert.equal(result.name, 'gpt-4o')
    assert.equal(result.isMapped, true)
    assert.equal(result.actualModel, 'gpt-4o-2024-08-06')
  })

  test('does not mark mapped when upstream_model equals request model', () => {
    const result = formatModelName(
      makeLog('gpt-4o', { upstream_model: 'gpt-4o' })
    )

    assert.equal(result.name, 'gpt-4o')
    assert.equal(result.isMapped, false)
    assert.equal(result.actualModel, undefined)
  })

  test('upstream_model_name takes priority over upstream_model', () => {
    const result = formatModelName(
      makeLog('gpt-4o', {
        upstream_model: 'gpt-4o-2024-08-06',
        upstream_model_name: 'azure-gpt-4o-mini',
      })
    )

    assert.equal(result.isMapped, true)
    assert.equal(result.actualModel, 'azure-gpt-4o-mini')
  })

  test('falls back to upstream_model when upstream_model_name is empty', () => {
    const result = formatModelName(
      makeLog('gpt-4o', {
        upstream_model_name: '',
        upstream_model: 'gpt-4o-2024-08-06',
      })
    )

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
