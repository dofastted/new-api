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
import { Columns3, ListTree } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { USAGE_LOGS_VIEW, type UsageLogsView } from '../constants'

interface UsageLogsViewToggleProps {
  value: UsageLogsView
  onValueChange: (value: UsageLogsView) => void
}

export function UsageLogsViewToggle(props: UsageLogsViewToggleProps) {
  const { t } = useTranslation()

  const handleValueChange = (value: string | string[] | null) => {
    const nextValue = Array.isArray(value) ? value.at(-1) : value
    if (
      nextValue === USAGE_LOGS_VIEW.TABLE ||
      nextValue === USAGE_LOGS_VIEW.STREAM
    ) {
      props.onValueChange(nextValue)
    }
  }

  return (
    <ToggleGroup
      value={[props.value]}
      onValueChange={handleValueChange}
      variant='outline'
      size='sm'
      spacing={0}
      aria-label={t('Usage logs view style')}
    >
      <Tooltip>
        <TooltipTrigger
          render={
            <ToggleGroupItem
              value={USAGE_LOGS_VIEW.TABLE}
              aria-label={t('Table view')}
            />
          }
        >
          <Columns3 className='size-3.5' />
          <span className='hidden sm:inline'>{t('Table')}</span>
        </TooltipTrigger>
        <TooltipContent>{t('Table view')}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger
          render={
            <ToggleGroupItem
              value={USAGE_LOGS_VIEW.STREAM}
              aria-label={t('Stream view')}
            />
          }
        >
          <ListTree className='size-3.5' />
          <span className='hidden sm:inline'>{t('Stream')}</span>
        </TooltipTrigger>
        <TooltipContent>{t('Stream view')}</TooltipContent>
      </Tooltip>
    </ToggleGroup>
  )
}
