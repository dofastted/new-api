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
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import {
  COMPACT_STREAM_COLUMN_ORDER,
  SIMPLE_USER_STREAM_COLUMNS,
  STREAM_COLUMNS,
  STREAM_CUSTOMIZABLE_COLUMNS,
  type StreamColumnId,
} from '../constants'

interface UsageLogsStreamHeaderProps {
  isAdmin: boolean
  compact?: boolean
  simplifiedUserView?: boolean
  /** Visible customizable columns, in display order. Ignored when `compact`. */
  columnOrder: StreamColumnId[]
}

const LABEL_KEY_BY_ID: Record<StreamColumnId, string> = Object.fromEntries(
  STREAM_CUSTOMIZABLE_COLUMNS.map((column) => [column.id, column.labelKey])
) as Record<StreamColumnId, string>

/**
 * Column header row for the stream list. Mirrors `STREAM_COLUMNS` and the
 * caller-supplied column order exactly, so headers stay aligned with the
 * cells rendered by `UsageLogsStreamRow`.
 */
export function UsageLogsStreamHeader(props: UsageLogsStreamHeaderProps) {
  const { t } = useTranslation()

  if (props.simplifiedUserView) {
    return (
      <div className='border-border/60 bg-muted/40 text-muted-foreground flex min-w-0 items-center gap-2 border-b px-2 py-1.5 text-[11px] font-medium tracking-wide'>
        <span className={SIMPLE_USER_STREAM_COLUMNS.model}>{t('Model')}</span>
        <span className={SIMPLE_USER_STREAM_COLUMNS.key}>{t('Key')}</span>
        <span className={SIMPLE_USER_STREAM_COLUMNS.group}>{t('Group')}</span>
        <span className={cn('text-right', SIMPLE_USER_STREAM_COLUMNS.performance)}>
          {t('Performance')}
        </span>
        <span className={cn('text-right', SIMPLE_USER_STREAM_COLUMNS.input)}>
          {t('Input')}
        </span>
        <span className={cn('text-right', SIMPLE_USER_STREAM_COLUMNS.output)}>
          {t('Output')}
        </span>
        <span className={cn('text-right', SIMPLE_USER_STREAM_COLUMNS.cache)}>
          {t('Cache')}
        </span>
        <span className={cn('text-right', SIMPLE_USER_STREAM_COLUMNS.cost)}>
          {t('Cost')}
        </span>
      </div>
    )
  }

  const orderedColumns = props.compact
    ? COMPACT_STREAM_COLUMN_ORDER
    : props.columnOrder
  const alignRight = new Set<StreamColumnId>([
    'tokens',
    'cache',
    'cost',
    'performance',
  ])

  return (
    <div className='border-border/60 bg-muted/40 text-muted-foreground flex min-w-0 items-center gap-2 border-b px-2 py-1.5 text-[11px] font-medium tracking-wide'>
      <span className={STREAM_COLUMNS.time}>{t('Time')}</span>
      <span className={STREAM_COLUMNS.model}>{t('Model')}</span>
      {orderedColumns.map((id) => (
        <span
          key={id}
          className={cn(alignRight.has(id) && 'text-right', STREAM_COLUMNS[id])}
        >
          {t(LABEL_KEY_BY_ID[id])}
        </span>
      ))}
    </div>
  )
}
