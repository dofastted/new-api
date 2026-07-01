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
import { AlertTriangle } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { LOG_TYPE_ENUM } from '../constants'
import type { UsageLog } from '../data/schema'
import { parseLogOther } from '../lib/format'

interface UsageLogsErrorAnalysisBarProps {
  logs: UsageLog[]
}

interface RankedEntry {
  label: string
  count: number
}

const TOP_N = 3

function rankEntries(counts: Map<string, number>): RankedEntry[] {
  return [...counts.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, TOP_N)
    .map(([label, count]) => ({ label, count }))
}

function incrementCount(counts: Map<string, number>, key: string | undefined) {
  if (!key) return
  counts.set(key, (counts.get(key) || 0) + 1)
}

function RankGroup(props: { title: string; entries: RankedEntry[] }) {
  if (props.entries.length === 0) return null
  return (
    <div className='flex min-w-0 items-center gap-1.5'>
      <span className='text-muted-foreground/70 shrink-0 text-[11px]'>
        {props.title}
      </span>
      <div className='flex min-w-0 flex-wrap items-center gap-1'>
        {props.entries.map((entry) => (
          <span
            key={entry.label}
            className='border-border/60 bg-background/70 inline-flex max-w-[10rem] items-center gap-1 truncate rounded px-1.5 py-0.5 text-[11px]'
            title={entry.label}
          >
            <span className='truncate'>{entry.label}</span>
            <span className='text-muted-foreground font-mono tabular-nums'>
              {entry.count}
            </span>
          </span>
        ))}
      </div>
    </div>
  )
}

export function UsageLogsErrorAnalysisBar(
  props: UsageLogsErrorAnalysisBarProps
) {
  const { t } = useTranslation()

  const analysis = useMemo(() => {
    const errorLogs = props.logs.filter(
      (log) => log.type === LOG_TYPE_ENUM.ERROR
    )
    if (errorLogs.length === 0) return null

    const channelCounts = new Map<string, number>()
    const modelCounts = new Map<string, number>()
    const categoryCounts = new Map<string, number>()

    for (const log of errorLogs) {
      const channelLabel =
        log.channel_name || (log.channel ? `#${log.channel}` : undefined)
      incrementCount(channelCounts, channelLabel)
      incrementCount(modelCounts, log.model_name || undefined)

      const other = parseLogOther(log.other)
      const chain = Array.isArray(other?.channel_chain)
        ? other.channel_chain
        : []
      const lastEntry = chain.at(-1)
      incrementCount(
        categoryCounts,
        lastEntry?.error_category || lastEntry?.error_code
      )
    }

    return {
      total: props.logs.length,
      errorCount: errorLogs.length,
      errorRate: errorLogs.length / props.logs.length,
      topChannels: rankEntries(channelCounts),
      topModels: rankEntries(modelCounts),
      topCategories: rankEntries(categoryCounts),
    }
  }, [props.logs])

  if (!analysis) return null

  return (
    <div
      className={cn(
        'flex flex-wrap items-center gap-x-4 gap-y-1.5 rounded-lg border px-3 py-2 text-xs',
        'border-rose-200/60 bg-rose-50/30 dark:border-rose-900/40 dark:bg-rose-950/15'
      )}
    >
      <div className='flex shrink-0 items-center gap-1.5 font-medium text-rose-600 dark:text-rose-400'>
        <AlertTriangle className='size-3.5' />
        <span>
          {t('{{count}} errors', { count: analysis.errorCount })} ·{' '}
          {(analysis.errorRate * 100).toFixed(1)}%
        </span>
      </div>

      <RankGroup title={t('Top channels')} entries={analysis.topChannels} />
      <RankGroup title={t('Top models')} entries={analysis.topModels} />
      <RankGroup
        title={t('Top error types')}
        entries={analysis.topCategories}
      />

      <span className='text-muted-foreground/60 ms-auto shrink-0 text-[11px]'>
        {t('Based on currently loaded logs')}
      </span>
    </div>
  )
}
