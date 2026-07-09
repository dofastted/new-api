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
  STREAM_CUSTOMIZABLE_COLUMNS,
  type StreamColumnId,
  type StreamColumnSettings,
} from '../constants'

interface UsageLogsStreamColumnTabsProps {
  isAdmin: boolean
  settings: StreamColumnSettings
  onChange: (next: StreamColumnSettings) => void
}

/**
 * Horizontal tab strip for stream columns. Drag tabs to reorder; click a tab
 * to toggle visibility. Mirrors the popover manager but stays always visible.
 */
export function UsageLogsStreamColumnTabs(
  props: UsageLogsStreamColumnTabsProps
) {
  const { t } = useTranslation()
  const [draggedId, setDraggedId] = useState<StreamColumnId | null>(null)
  const [dropTargetId, setDropTargetId] = useState<StreamColumnId | null>(null)

  const visibleDefs = STREAM_CUSTOMIZABLE_COLUMNS.filter(
    (column) => props.isAdmin || !column.adminOnly
  )
  const orderedDefs = props.settings.order
    .map((id) => visibleDefs.find((column) => column.id === id))
    .filter((column): column is (typeof visibleDefs)[number] => !!column)

  const moveColumn = (sourceId: StreamColumnId, targetId: StreamColumnId) => {
    if (sourceId === targetId) return
    const order = [...props.settings.order]
    const sourceIndex = order.indexOf(sourceId)
    const targetIndex = order.indexOf(targetId)
    if (sourceIndex === -1 || targetIndex === -1) return
    order.splice(sourceIndex, 1)
    // After removal, recompute target index so insert lands before the drop target.
    const nextTargetIndex = order.indexOf(targetId)
    if (nextTargetIndex === -1) return
    order.splice(nextTargetIndex, 0, sourceId)
    props.onChange({ ...props.settings, order })
  }

  const toggleColumn = (id: StreamColumnId) => {
    const isHidden = props.settings.hidden.includes(id)
    const hidden = isHidden
      ? props.settings.hidden.filter((hiddenId) => hiddenId !== id)
      : [...props.settings.hidden, id]
    // Keep at least one visible customizable column.
    const remainingVisible = orderedDefs.some(
      (column) => column.id !== id && !hidden.includes(column.id)
    )
    if (!isHidden && !remainingVisible) return
    props.onChange({ ...props.settings, hidden })
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

  return (
    <div
      className='border-border/60 bg-muted/20 flex min-w-0 items-center gap-1 overflow-x-auto border-b px-2 py-1.5'
      role='toolbar'
      aria-label={t('Manage columns')}
    >
      <span className='text-muted-foreground me-1 shrink-0 text-[11px] font-medium whitespace-nowrap'>
        {t('Columns')}
      </span>
      <div className='flex min-w-0 flex-1 items-center gap-1'>
        {orderedDefs.map((column) => {
          const active = !props.settings.hidden.includes(column.id)
          const isDragging = draggedId === column.id
          const isDropTarget = dropTargetId === column.id && !isDragging
          return (
            <button
              key={column.id}
              type='button'
              draggable
              onDragStart={(event) => handleDragStart(event, column.id)}
              onDragOver={(event) => handleDragOver(event, column.id)}
              onDrop={(event) => handleDrop(event, column.id)}
              onDragEnd={handleDragEnd}
              onClick={() => toggleColumn(column.id)}
              title={
                active
                  ? t('Drag to reorder, click to hide')
                  : t('Drag to reorder, click to show')
              }
              className={cn(
                'inline-flex h-7 shrink-0 items-center gap-1 rounded-md border px-2 text-[11px] font-medium transition-colors select-none',
                'cursor-grab active:cursor-grabbing',
                active
                  ? 'border-border bg-background text-foreground shadow-xs'
                  : 'border-transparent bg-transparent text-muted-foreground/70 hover:bg-accent/50',
                isDragging && 'opacity-40',
                isDropTarget && 'border-primary/60 ring-primary/30 ring-1'
              )}
            >
              <GripVertical className='text-muted-foreground/50 size-3 shrink-0' />
              <span className='whitespace-nowrap'>{t(column.labelKey)}</span>
            </button>
          )
        })}
      </div>
    </div>
  )
}
