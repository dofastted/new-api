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

import { STREAM_COLUMNS } from '../constants'

interface UsageLogsStreamHeaderProps {
  isAdmin: boolean
  compact?: boolean
}

/**
 * Column header row for the stream list. Mirrors `STREAM_COLUMNS` (defined in
 * `usage-logs-stream-row.tsx`) exactly, including which columns are hidden
 * for non-admin users / compact density, so headers stay aligned with the
 * cells below them.
 */
export function UsageLogsStreamHeader(props: UsageLogsStreamHeaderProps) {
  const { t } = useTranslation()

  return (
    <div className='border-border/60 bg-muted/40 text-muted-foreground flex min-w-0 items-center gap-2 border-b px-2 py-1.5 text-[11px] font-medium tracking-wide'>
      <span className={cn(STREAM_COLUMNS.time)}>{t('Time')}</span>
      <span className={STREAM_COLUMNS.type}>{t('Type')}</span>
      {props.isAdmin && !props.compact && (
        <span className={STREAM_COLUMNS.user}>{t('User')}</span>
      )}
      <span className={STREAM_COLUMNS.group}>{t('Group')}</span>
      {props.isAdmin && !props.compact && (
        <span className={STREAM_COLUMNS.channel}>{t('Channel')}</span>
      )}
      <span className={STREAM_COLUMNS.model}>{t('Model')}</span>
      {!props.compact && (
        <>
          <span className={cn('text-right', STREAM_COLUMNS.tokens)}>
            {t('Tokens')}
          </span>
          <span className={cn('text-right', STREAM_COLUMNS.cache)}>
            {t('Cache')}
          </span>
        </>
      )}
      <span className={cn('text-right', STREAM_COLUMNS.cost)}>{t('Cost')}</span>
      {!props.compact && (
        <span className={cn('text-right', STREAM_COLUMNS.performance)}>
          {t('Performance')}
        </span>
      )}
    </div>
  )
}
