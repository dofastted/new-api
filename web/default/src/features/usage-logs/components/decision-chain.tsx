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
import { GitBranch } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'

import {
  chainStatusColors,
  chainStatusIcon,
  chainStepStatus,
  formatChainChannel,
  formatChainToken,
  hasChainDecision,
} from '../lib/decision-chain'
import type { ChannelChainEntry } from '../types'

interface DecisionChainProps {
  chain: ChannelChainEntry[] | undefined
}

function StepRow(props: { label: string; value: React.ReactNode }) {
  return (
    <div className='grid min-w-0 grid-cols-[5.25rem_minmax(0,1fr)] gap-2 text-xs sm:grid-cols-[7rem_minmax(0,1fr)] sm:gap-3'>
      <span className='text-muted-foreground min-w-0'>{props.label}</span>
      <span className='max-w-full min-w-0 break-all font-mono text-xs'>
        {props.value}
      </span>
    </div>
  )
}

/**
 * Dialog-level decision-chain view: renders the backend `channel_chain` as a
 * vertical list of step cards (one per entry). Adapted from claude-code-hub's
 * `LogicTraceTab` + `StepCard`, rebuilt on new-api primitives and the existing
 * `channel_chain` payload (a subset of cch's ProviderChain — no
 * priority_levels / candidates, so we render only what exists).
 */
export function DecisionChain(props: DecisionChainProps) {
  const { t } = useTranslation()
  const chain = props.chain ?? []

  if (chain.length === 0) {
    return (
      <div className='text-muted-foreground py-6 text-center'>
        <GitBranch className='mx-auto mb-2 size-7 opacity-40' />
        <p className='text-xs'>{t('No decision data')}</p>
      </div>
    )
  }

  return (
    <div className='min-w-0 space-y-2'>
      <Label className='text-xs font-semibold'>
        <GitBranch className='size-3.5' aria-hidden='true' />
        {t('Decision Chain')}
      </Label>
      <div className='bg-muted/30 min-w-0 space-y-2 overflow-hidden rounded-md border p-2.5'>
        {chain.map((entry, index) => {
          const status = chainStepStatus(entry)
          const Icon = chainStatusIcon(status)
          const colors = chainStatusColors(status)
          const isLast = index === chain.length - 1
          const stepTitle =
            entry.attempt != null && entry.attempt > 1
              ? `${t('Attempt')} ${entry.attempt}`
              : formatChainToken(entry.reason, t)
          return (
            <div
              key={`${entry.channel_id ?? 'ch'}-${entry.attempt ?? entry.retry_index ?? 0}-${entry.reason ?? 'step'}`}
              className='flex min-w-0 gap-2'
            >
              {/* timeline node + connector */}
              <div className='flex flex-col items-center'>
                <div
                  className={cn(
                    'flex size-6 shrink-0 items-center justify-center rounded-full border',
                    colors.bg
                  )}
                >
                  <Icon className={cn('size-3', colors.text)} />
                </div>
                {!isLast && (
                  <div className='bg-border min-h-[6px] w-0.5 flex-1' />
                )}
              </div>
              {/* step body */}
              <div
                className={cn(
                  'bg-background/60 min-w-0 flex-1 rounded border p-2',
                  isLast && 'mb-0'
                )}
              >
                <div className='flex min-w-0 flex-wrap items-center gap-1.5'>
                  <span className='text-xs font-medium'>
                    {stepTitle}
                  </span>
                  <StatusBadge
                    label={formatChainChannel(entry)}
                    variant='neutral'
                    size='sm'
                    copyable={false}
                  />
                  <StatusBadge
                    label={formatChainToken(entry.selection, t)}
                    variant={colors.badge as StatusVariant}
                    size='sm'
                    copyable={false}
                  />
                </div>
                <div className='mt-1.5 min-w-0 space-y-1'>
                  {entry.group && (
                    <StepRow label={t('Group')} value={entry.group} />
                  )}
                  {entry.endpoint && (
                    <StepRow label={t('Endpoint')} value={entry.endpoint} />
                  )}
                  {entry.circuit_state && (
                    <StepRow
                      label={t('Circuit State')}
                      value={formatChainToken(entry.circuit_state, t)}
                    />
                  )}
                  {entry.error_code && (
                    <StepRow label={t('Error Code')} value={entry.error_code} />
                  )}
                  {entry.error_category && (
                    <StepRow
                      label={t('Error Category')}
                      value={entry.error_category}
                    />
                  )}
                  {hasChainDecision(entry.decision) && (
                    <StepRow
                      label={t('Decision')}
                      value={[
                        entry.decision?.priority != null &&
                          `${t('Priority')}: ${entry.decision.priority}`,
                        entry.decision?.weight != null &&
                          `${t('Weight')}: ${entry.decision.weight}`,
                        entry.decision?.total_weight != null &&
                          `${t('Total Weight')}: ${entry.decision.total_weight}`,
                        entry.decision?.selected_probability != null &&
                          `${t('Probability')}: ${entry.decision.selected_probability}`,
                        entry.decision?.filtered_candidates != null &&
                          `${t('Candidates')}: ${entry.decision.filtered_candidates}/${entry.decision.total_candidates ?? '-'}`,
                      ]
                        .filter(Boolean)
                        .join(' · ')}
                    />
                  )}
                </div>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
