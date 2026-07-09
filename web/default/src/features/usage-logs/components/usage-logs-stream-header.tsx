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
import { GripVertical } from 'lucide-react'
import { useState, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import {
  SIMPLE_USER_STREAM_COLUMNS,
  STREAM_COLUMNS,
  STREAM_CUSTOMIZABLE_COLUMNS,
  type StreamColumnId,
  type StreamColumnSettings,
} from '../constants'

interface UsageLogsStreamHeaderProps {
  isAdmin: boolean
  compact?: boolean
  simplifiedUserView?: boolean
  /** Visible customizable columns, in display order. */
  columnOrder: StreamColumnId[]
  /** When set with onColumnSettingsChange, customizable headers become draggable. */
  columnSettings?: StreamColumnSettings
  onColumnSettingsChange?: (next: StreamColumnSettings) => void
}

const LABEL_KEY_BY_ID: Record<StreamColumnId, string> = Object.fromEntries(
  STREAM_CUSTOMIZABLE_COLUMNS.map((column) => [column.id, column.labelKey])
) as Record<StreamColumnId, string>

/**
 * Column header row for the stream list. Mirrors `STREAM_COLUMNS` and the
 * caller-supplied column order exactly, so headers stay aligned with the
 * cells rendered by `UsageLogsStreamRow`. In detailed mode, customizable
 * headers can be drag-reordered in place.
 */
export function UsageLogsStreamHeader(props: UsageLogsStreamHeaderProps) {
  const { t } = useTranslation()
  const [draggedId, setDraggedId] = useState<StreamColumnId | null>(null)
  const [dropTargetId, setDropTargetId] = useState<StreamColumnId | null>(null)

  const reorderEnabled =
    !props.compact &&
    !props.simplifiedUserView &&
    !!props.columnSettings &&
    !!props.onColumnSettingsChange

  const moveColumn = (sourceId: StreamColumnId, targetId: StreamColumnId) => {
    if (!props.columnSettings || !props.onColumnSettingsChange) return
    if (sourceId === targetId) return
    const order = [...props.columnSettings.order]
    const sourceIndex = order.indexOf(sourceId)
    if (sourceIndex === -1 || !order.includes(targetId)) return
    order.splice(sourceIndex, 1)
    const nextTargetIndex = order.indexOf(targetId)
    if (nextTargetIndex === -1) return
    order.splice(nextTargetIndex, 0, sourceId)
    props.onColumnSettingsChange({ ...props.columnSettings, order })
  }

  const handleDragStart = (event: DragEvent, id: StreamColumnId) => {
    setDraggedId(id)
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', id)
  }

  const handleDragOver = (event: DragEvent, id: StreamColumnId) => {
    if (!draggedId || draggedId === id) return
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
    if (dropTargetId !== id) setDropTargetId(id)
  }

  const handleDrop = (event: DragEvent, targetId: StreamColumnId) => {
    event.preventDefault()
    if (draggedId) moveColumn(draggedId, targetId)
    setDraggedId(null)
    setDropTargetId(null)
  }

  const handleDragEnd = () => {
    setDraggedId(null)
    setDropTargetId(null)
  }

  if (props.simplifiedUserView) {
    return (
      <div className='border-border/60 bg-muted/40 text-muted-foreground flex min-w-0 items-center gap-2 border-b px-2 py-1.5 text-[11px] font-medium tracking-wide'>
        <span className={SIMPLE_USER_STREAM_COLUMNS.model}>{t('Model')}</span>
        <span className={SIMPLE_USER_STREAM_COLUMNS.key}>{t('Key')}</span>
        <span className={SIMPLE_USER_STREAM_COLUMNS.group}>{t('Group')}</span>
        <span
          className={cn('text-right', SIMPLE_USER_STREAM_COLUMNS.performance)}
        >
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

  const orderedColumns = props.columnOrder
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
      {orderedColumns.map((id) => {
        const right = alignRight.has(id)
        const label = t(LABEL_KEY_BY_ID[id])
        if (!reorderEnabled) {
          return (
            <span
              key={id}
              className={cn(right && 'text-right', STREAM_COLUMNS[id])}
            >
              {label}
            </span>
          )
        }

        const isDragging = draggedId === id
        const isDropTarget = dropTargetId === id && !isDragging
        return (
          <span
            key={id}
            draggable
            onDragStart={(event) => handleDragStart(event, id)}
            onDragOver={(event) => handleDragOver(event, id)}
            onDrop={(event) => handleDrop(event, id)}
            onDragEnd={handleDragEnd}
            title={t('Drag to reorder')}
            className={cn(
              'inline-flex min-w-0 items-center gap-0.5 select-none',
              right ? 'justify-end' : 'justify-start',
              'cursor-grab active:cursor-grabbing',
              STREAM_COLUMNS[id],
              isDragging && 'opacity-40',
              isDropTarget && 'text-primary'
            )}
          >
            <GripVertical className='text-muted-foreground/45 size-3 shrink-0' />
            <span className='truncate'>{label}</span>
          </span>
        )
      })}
    </div>
  )
}
