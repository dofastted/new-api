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
import { ChevronRight, Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
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

interface ChannelChainPopoverProps {
  chain: ChannelChainEntry[]
  /** Final channel name to show as the trigger label. */
  finalChannelName: string
  className?: string
}

/**
 * Inline channel-chain affordance for a stream row.
 *
 * - empty chain → plain name, no popover/tooltip
 * - single entry → name with a tooltip summarizing the decision context
 * - multiple entries → `N×` badge + name trigger opening a popover that lists
 *   the chain as a vertical timeline (one node per entry)
 *
 * Pattern adapted from claude-code-hub's `provider-chain-popover.tsx`, rebuilt
 * on new-api's Base UI primitives and the existing `channel_chain` payload.
 */
export function ChannelChainPopover(props: ChannelChainPopoverProps) {
  const { t } = useTranslation()
  const { chain, finalChannelName } = props
  const displayName = finalChannelName || '-'

  if (chain.length === 0) {
    return (
      <span
        className={cn('text-muted-foreground truncate text-xs', props.className)}
        title={displayName}
      >
        {displayName}
      </span>
    )
  }

  if (chain.length === 1) {
    const entry = chain[0]
    const status = chainStepStatus(entry)
    const Icon = chainStatusIcon(status)
    const colors = chainStatusColors(status)
    const decision = entry.decision
    return (
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger
            render={
              <span
                className={cn(
                  'flex min-w-0 cursor-help items-center gap-1 text-xs',
                  props.className
                )}
              >
                <Icon className={cn('size-3 shrink-0', colors.text)} />
                <span className='truncate' dir='auto'>
                  {displayName}
                </span>
              </span>
            }
          />
          <TooltipContent side='bottom' align='start' className='max-w-[300px]'>
            <div className='space-y-1.5'>
              <div className='text-xs font-medium'>{displayName}</div>
              <div className='flex flex-wrap items-center gap-1 text-[10px]'>
                <StatusBadge
                  label={formatChainToken(entry.selection, t)}
                  variant='neutral'
                  size='sm'
                  copyable={false}
                />
                {entry.attempt != null && entry.attempt > 1 && (
                  <span className='text-muted-foreground'>
                    {t('Attempt')} {entry.attempt}
                  </span>
                )}
                {entry.circuit_state && (
                  <span className='text-muted-foreground'>
                    {formatChainToken(entry.circuit_state, t)}
                  </span>
                )}
              </div>
              {hasChainDecision(decision) && (
                <div className='grid grid-cols-2 gap-x-3 gap-y-0.5 pt-1 text-[10px]'>
                  {decision?.priority != null && (
                    <div>
                      <span className='text-muted-foreground'>
                        {t('Priority')}:
                      </span>{' '}
                      <span className='font-mono'>P{decision.priority}</span>
                    </div>
                  )}
                  {decision?.weight != null && (
                    <div>
                      <span className='text-muted-foreground'>
                        {t('Weight')}:
                      </span>{' '}
                      <span className='font-mono'>{decision.weight}</span>
                    </div>
                  )}
                  {decision?.selected_probability != null && (
                    <div>
                      <span className='text-muted-foreground'>
                        {t('Probability')}:
                      </span>{' '}
                      <span className='font-mono'>
                        {decision.selected_probability}
                      </span>
                    </div>
                  )}
                  {decision?.filtered_candidates != null && (
                    <div>
                      <span className='text-muted-foreground'>
                        {t('Candidates')}:
                      </span>{' '}
                      <span className='font-mono'>
                        {decision.filtered_candidates}/
                        {decision.total_candidates ?? '-'}
                      </span>
                    </div>
                  )}
                </div>
              )}
            </div>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
    )
  }

  // Multiple entries → popover with a vertical timeline.
  const requestCount = chain.length
  return (
    <Popover>
      <PopoverTrigger
        render={
          <button
            type='button'
            className={cn(
              'flex min-w-0 items-center gap-1 text-xs hover:text-foreground',
              props.className
            )}
            aria-label={`${displayName} - ${requestCount}${t('times')}`}
          >
            <StatusBadge
              label={String(requestCount)}
              variant='neutral'
              size='sm'
              copyable={false}
              className='shrink-0 px-1'
            />
            <span className='truncate' dir='auto'>
              {displayName}
            </span>
            <Info className='text-muted-foreground size-3 shrink-0' />
          </button>
        }
      />
      <PopoverContent align='start' className='w-[340px] max-w-[calc(100vw-2rem)] p-0'>
        <div className='border-b p-3'>
          <div className='flex items-center justify-between'>
            <h4 className='text-sm font-semibold'>
              {t('Channel Chain')}
            </h4>
            <StatusBadge
              label={`${requestCount} ${t('times')}`}
              variant='neutral'
              size='sm'
              copyable={false}
            />
          </div>
        </div>
        <div className='max-h-[300px] space-y-0 overflow-y-auto p-3'>
          {chain.map((entry, index) => {
            const status = chainStepStatus(entry)
            const Icon = chainStatusIcon(status)
            const colors = chainStatusColors(status)
            const isLast = index === chain.length - 1
            return (
              <div
                key={`${entry.channel_id ?? 'ch'}-${entry.attempt ?? entry.retry_index ?? 0}-${entry.reason ?? 'step'}`}
                className='relative flex gap-2'
              >
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
                    <div className='bg-border min-h-[8px] w-0.5 flex-1' />
                  )}
                </div>
                <div className={cn('min-w-0 flex-1', isLast ? 'pb-0' : 'pb-3')}>
                  <div className='flex flex-wrap items-center gap-1.5'>
                    <span className='truncate text-xs font-medium'>
                      {formatChainChannel(entry)}
                    </span>
                    <StatusBadge
                      label={formatChainToken(entry.reason, t)}
                      variant={colors.badge}
                      size='sm'
                      copyable={false}
                    />
                    {entry.attempt != null && entry.attempt > 1 && (
                      <span className='text-muted-foreground text-[10px]'>
                        {t('Attempt')} {entry.attempt}
                      </span>
                    )}
                  </div>
                  {entry.error_code && (
                    <p className='text-muted-foreground mt-0.5 line-clamp-1 text-[10px]'>
                      {entry.error_code}
                      {entry.error_category ? ` · ${entry.error_category}` : ''}
                    </p>
                  )}
                  {entry.selection && (
                    <div className='text-muted-foreground mt-0.5 flex items-center gap-1 text-[10px]'>
                      <ChevronRight className='size-2.5' />
                      {formatChainToken(entry.selection, t)}
                      {entry.endpoint && (
                        <span className='truncate'> · {entry.endpoint}</span>
                      )}
                    </div>
                  )}
                </div>
              </div>
            )
          })}
        </div>
        <div className='bg-muted/30 border-t p-2'>
          <p className='text-muted-foreground text-center text-[10px]'>
            {t('Click row for full decision chain')}
          </p>
        </div>
      </PopoverContent>
    </Popover>
  )
}
