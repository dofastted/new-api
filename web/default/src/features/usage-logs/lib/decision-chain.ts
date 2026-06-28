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
import { type LucideIcon, CheckCircle2, Circle, RefreshCw, XCircle } from 'lucide-react'

import type { ChannelChainEntry } from '../types'

/**
 * Visual status for a single channel-chain step, derived from the backend
 * `channel_chain` entry. Mirrors claude-code-hub's provider-chain semantics
 * but uses only the fields new-api records (reason / error_code /
 * circuit_state / attempt).
 */
export type ChainStepStatus = 'success' | 'failure' | 'retry' | 'pending'

/**
 * Map a channel-chain entry to a coarse visual status.
 *
 * - `failure` reason, any `error_code`, or an `open` circuit → failure
 * - `retry_selected` reason, `attempt > 1`, or a non-zero `retry_index` → retry
 * - `selected` / `affinity_reuse` without error → success
 * - anything else → pending
 */
export function chainStepStatus(entry: ChannelChainEntry): ChainStepStatus {
  if (
    entry.reason === 'failure' ||
    !!entry.error_code ||
    entry.circuit_state === 'open'
  ) {
    return 'failure'
  }
  if (
    entry.reason === 'retry_selected' ||
    (entry.attempt != null && entry.attempt > 1) ||
    (entry.retry_index != null && entry.retry_index > 0)
  ) {
    return 'retry'
  }
  if (entry.reason === 'selected' || entry.reason === 'affinity_reuse') {
    return 'success'
  }
  return 'pending'
}

const STATUS_ICON: Record<ChainStepStatus, LucideIcon> = {
  success: CheckCircle2,
  failure: XCircle,
  retry: RefreshCw,
  pending: Circle,
}

export function chainStatusIcon(status: ChainStepStatus): LucideIcon {
  return STATUS_ICON[status]
}

interface ChainStatusColors {
  text: string
  bg: string
  badge: 'green' | 'red' | 'yellow' | 'neutral'
}

const STATUS_COLORS: Record<ChainStepStatus, ChainStatusColors> = {
  success: {
    text: 'text-emerald-600 dark:text-emerald-400',
    bg: 'bg-emerald-50 dark:bg-emerald-950/30',
    badge: 'green',
  },
  failure: {
    text: 'text-rose-600 dark:text-rose-400',
    bg: 'bg-rose-50 dark:bg-rose-950/30',
    badge: 'red',
  },
  retry: {
    text: 'text-amber-600 dark:text-amber-400',
    bg: 'bg-amber-50 dark:bg-amber-950/30',
    badge: 'yellow',
  },
  pending: {
    text: 'text-muted-foreground',
    bg: 'bg-muted/40',
    badge: 'neutral',
  },
}

export function chainStatusColors(status: ChainStepStatus): ChainStatusColors {
  return STATUS_COLORS[status]
}

/**
 * Render a channel-chain entry's channel as `#id name` (or just one of them).
 */
export function formatChainChannel(entry: ChannelChainEntry): string {
  const id = entry.channel_id
  const name = entry.channel_name
  if (id != null && name) return `#${id} ${name}`
  if (id != null) return `#${id}`
  return name || '-'
}

/**
 * Whether an entry carries any decision metadata worth showing
 * (priority / weight / probability / candidates). Mirrors the guard used in
 * `details-dialog.tsx` so both surfaces stay consistent.
 */
export function hasChainDecision(
  decision: ChannelChainEntry['decision']
): boolean {
  if (!decision) return false
  return Object.values(decision).some(
    (value) => value != null && value !== 0 && value !== ''
  )
}

/**
 * Stable mapping from a language-independent channel-chain token (reason /
 * selection / circuit_state) to its i18n label key. Shared by the inline
 * popover and the dialog-level decision-chain view so the two surfaces stay
 * consistent. Kept in sync with the backend `service/channel_chain.go`
 * reason/selection constants and the legacy table in `details-dialog.tsx`.
 */
const CHAIN_TOKEN_LABELS: Record<string, string> = {
  // reasons
  selected: 'Selected',
  affinity_reuse: 'Affinity Reuse',
  retry_selected: 'Retry Selected',
  failure: 'Failure',
  // selections
  affinity: 'Affinity',
  weighted: 'Weighted',
  specific_channel: 'Specific Channel',
  relay_failure: 'Relay Failure',
  // circuit states
  closed: 'Closed',
  open: 'Open',
  half_open: 'Half Open',
}

function normalizeChainToken(value: string): string {
  return value
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

/**
 * Render a localized label for a channel-chain token (reason / selection /
 * circuit_state). Falls back to a humanized form when the token is unknown.
 */
export function formatChainToken(
  value: string | undefined,
  t: (key: string) => string
): string {
  if (!value) return '-'
  return t(CHAIN_TOKEN_LABELS[value] ?? normalizeChainToken(value))
}
