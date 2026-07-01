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
import { GripVertical, RotateCcw, SlidersHorizontal } from 'lucide-react'
import { useState, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import {
  DEFAULT_STREAM_COLUMN_SETTINGS,
  STREAM_CUSTOMIZABLE_COLUMNS,
  type StreamColumnId,
  type StreamColumnSettings,
} from '../constants'

interface UsageLogsStreamColumnManagerProps {
  isAdmin: boolean
  settings: StreamColumnSettings
  onChange: (next: StreamColumnSettings) => void
}

/**
 * "Manage columns" popover for comfortable-density stream rows: check to
 * show/hide a column, drag the handle to reorder. Time and Model are the
 * row's fixed anchors and aren't offered here.
 */
export function UsageLogsStreamColumnManager(
  props: UsageLogsStreamColumnManagerProps
) {
  const { t } = useTranslation()
  const [draggedId, setDraggedId] = useState<StreamColumnId | null>(null)

  const visibleDefs = STREAM_CUSTOMIZABLE_COLUMNS.filter(
    (column) => props.isAdmin || !column.adminOnly
  )
  const orderedDefs = props.settings.order
    .map((id) => visibleDefs.find((column) => column.id === id))
    .filter((column): column is (typeof visibleDefs)[number] => !!column)

  const toggleColumn = (id: StreamColumnId, checked: boolean) => {
    const hidden = checked
      ? props.settings.hidden.filter((hiddenId) => hiddenId !== id)
      : [...props.settings.hidden, id]
    props.onChange({ ...props.settings, hidden })
  }

  const moveColumn = (sourceId: StreamColumnId, targetId: StreamColumnId) => {
    if (sourceId === targetId) return
    const order = [...props.settings.order]
    const sourceIndex = order.indexOf(sourceId)
    const targetIndex = order.indexOf(targetId)
    if (sourceIndex === -1 || targetIndex === -1) return
    order.splice(sourceIndex, 1)
    order.splice(order.indexOf(targetId), 0, sourceId)
    props.onChange({ ...props.settings, order })
  }

  const handleDragStart = (event: DragEvent, id: StreamColumnId) => {
    setDraggedId(id)
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', id)
  }

  const handleDragOver = (event: DragEvent) => {
    if (draggedId) event.preventDefault()
  }

  const handleDrop = (event: DragEvent, targetId: StreamColumnId) => {
    event.preventDefault()
    if (draggedId) moveColumn(draggedId, targetId)
    setDraggedId(null)
  }

  const handleReset = () => {
    props.onChange(DEFAULT_STREAM_COLUMN_SETTINGS)
  }

  return (
    <Popover>
      <Tooltip>
        <TooltipTrigger
          render={
            <PopoverTrigger
              render={
                <Button
                  variant='outline'
                  size='icon'
                  aria-label={t('Manage columns')}
                  className='size-8'
                />
              }
            />
          }
        >
          <SlidersHorizontal className='size-3.5' />
        </TooltipTrigger>
        <TooltipContent>{t('Manage columns')}</TooltipContent>
      </Tooltip>
      <PopoverContent align='end' className='w-64 p-2'>
        <div className='mb-1.5 flex items-center justify-between px-1'>
          <span className='text-muted-foreground text-xs font-medium'>
            {t('Manage columns')}
          </span>
          <Button
            variant='ghost'
            size='sm'
            className='text-muted-foreground h-6 gap-1 px-1.5 text-xs'
            onClick={handleReset}
          >
            <RotateCcw className='size-3' />
            {t('Reset')}
          </Button>
        </div>
        <div className='flex flex-col gap-0.5'>
          {orderedDefs.map((column) => {
            const checked = !props.settings.hidden.includes(column.id)
            return (
              <div
                key={column.id}
                draggable
                onDragStart={(event) => handleDragStart(event, column.id)}
                onDragOver={handleDragOver}
                onDrop={(event) => handleDrop(event, column.id)}
                onDragEnd={() => setDraggedId(null)}
                className={cn(
                  'hover:bg-accent/60 flex items-center gap-2 rounded-md px-1.5 py-1.5',
                  draggedId === column.id && 'opacity-40'
                )}
              >
                <GripVertical className='text-muted-foreground/60 size-3.5 shrink-0 cursor-grab active:cursor-grabbing' />
                <Checkbox
                  id={`stream-column-${column.id}`}
                  checked={checked}
                  onCheckedChange={(next) =>
                    toggleColumn(column.id, Boolean(next))
                  }
                />
                <Label
                  htmlFor={`stream-column-${column.id}`}
                  className='flex-1 cursor-pointer text-xs font-normal'
                >
                  {t(column.labelKey)}
                </Label>
              </div>
            )
          })}
        </div>
      </PopoverContent>
    </Popover>
  )
}
