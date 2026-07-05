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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import {
  getRiskEvents,
  resetAbuseScore,
  unbanAbuseUser,
  type RiskEvent,
} from './api'

const ACTION_VARIANT: Record<
  string,
  'default' | 'secondary' | 'destructive' | 'outline'
> = {
  blocked: 'destructive',
  temp_ban: 'destructive',
  perm_ban: 'destructive',
  flagged: 'secondary',
  forced_review: 'outline',
}

const SOURCE_LABEL_KEY: Record<string, string> = {
  sync_word: 'Sync word',
  sync_pattern: 'Sync pattern',
  moderation: 'Moderation review',
  penalty: 'Penalty',
}

const ACTION_LABEL_KEY: Record<string, string> = {
  blocked: 'Blocked',
  flagged: 'Flagged',
  forced_review: 'Forced review',
  temp_ban: 'Temporary ban',
  perm_ban: 'Permanent ban',
}

const PAGE_SIZE = 20

export function RiskEvents() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [userId, setUserId] = useState('')
  const [source, setSource] = useState('')
  const [action, setAction] = useState('')

  const query = useQuery({
    queryKey: ['risk-events', page, userId, source, action],
    queryFn: () =>
      getRiskEvents({
        page,
        page_size: PAGE_SIZE,
        user_id: userId ? Number(userId) : undefined,
        source: source || undefined,
        action: action || undefined,
      }),
  })

  const unban = useMutation({
    mutationFn: (uid: number) => unbanAbuseUser(uid),
    onSuccess: () => {
      toast.success(t('User unbanned'))
      queryClient.invalidateQueries({ queryKey: ['risk-events'] })
    },
  })
  const reset = useMutation({
    mutationFn: (uid: number) => resetAbuseScore(uid),
    onSuccess: () => toast.success(t('Score reset')),
  })

  const events: RiskEvent[] = query.data?.data?.items ?? []
  const total = query.data?.data?.total ?? 0
  const maxPage = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Risk Events')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mb-4 flex flex-wrap items-end gap-3'>
          <div className='w-40'>
            <label className='text-muted-foreground mb-1 block text-xs'>
              {t('User ID')}
            </label>
            <Input
              value={userId}
              onChange={(e) => {
                setUserId(e.target.value)
                setPage(1)
              }}
              placeholder={t('User ID')}
            />
          </div>
          <div className='w-44'>
            <label className='text-muted-foreground mb-1 block text-xs'>
              {t('Source')}
            </label>
            <NativeSelect
              value={source}
              onChange={(e) => {
                setSource(e.target.value)
                setPage(1)
              }}
            >
              <NativeSelectOption value=''>{t('All')}</NativeSelectOption>
              <NativeSelectOption value='sync_word'>
                {t('Sync word')}
              </NativeSelectOption>
              <NativeSelectOption value='sync_pattern'>
                {t('Sync pattern')}
              </NativeSelectOption>
              <NativeSelectOption value='moderation'>
                {t('Moderation review')}
              </NativeSelectOption>
              <NativeSelectOption value='penalty'>
                {t('Penalty')}
              </NativeSelectOption>
            </NativeSelect>
          </div>
          <div className='w-44'>
            <label className='text-muted-foreground mb-1 block text-xs'>
              {t('Action')}
            </label>
            <NativeSelect
              value={action}
              onChange={(e) => {
                setAction(e.target.value)
                setPage(1)
              }}
            >
              <NativeSelectOption value=''>{t('All')}</NativeSelectOption>
              <NativeSelectOption value='blocked'>
                {t('Blocked')}
              </NativeSelectOption>
              <NativeSelectOption value='flagged'>
                {t('Flagged')}
              </NativeSelectOption>
              <NativeSelectOption value='forced_review'>
                {t('Forced review')}
              </NativeSelectOption>
              <NativeSelectOption value='temp_ban'>
                {t('Temporary ban')}
              </NativeSelectOption>
              <NativeSelectOption value='perm_ban'>
                {t('Permanent ban')}
              </NativeSelectOption>
            </NativeSelect>
          </div>
        </div>

        <div className='rounded-md border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Time')}</TableHead>
                <TableHead>{t('User ID')}</TableHead>
                <TableHead>{t('Source')}</TableHead>
                <TableHead>{t('Action')}</TableHead>
                <TableHead>{t('Score')}</TableHead>
                <TableHead>{t('Model')}</TableHead>
                <TableHead>{t('Detail')}</TableHead>
                <TableHead>{t('Operations')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {events.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={8}
                    className='text-muted-foreground py-8 text-center'
                  >
                    {query.isLoading ? t('Loading...') : t('No records')}
                  </TableCell>
                </TableRow>
              ) : (
                events.map((ev) => (
                  <TableRow key={ev.id}>
                    <TableCell className='whitespace-nowrap'>
                      {new Date(ev.created_at * 1000).toLocaleString()}
                    </TableCell>
                    <TableCell>{ev.user_id}</TableCell>
                    <TableCell>
                      {t(SOURCE_LABEL_KEY[ev.source] ?? ev.source)}
                    </TableCell>
                    <TableCell>
                      <Badge variant={ACTION_VARIANT[ev.action] ?? 'outline'}>
                        {t(ACTION_LABEL_KEY[ev.action] ?? ev.action)}
                      </Badge>
                    </TableCell>
                    <TableCell>{ev.score}</TableCell>
                    <TableCell>{ev.model_name}</TableCell>
                    <TableCell
                      className='max-w-xs truncate'
                      title={`${ev.detail}\n${ev.snippet}`}
                    >
                      {ev.detail}
                    </TableCell>
                    <TableCell className='space-x-2 whitespace-nowrap'>
                      <Button
                        size='sm'
                        variant='outline'
                        onClick={() => unban.mutate(ev.user_id)}
                        disabled={unban.isPending}
                      >
                        {t('Unban')}
                      </Button>
                      <Button
                        size='sm'
                        variant='ghost'
                        onClick={() => reset.mutate(ev.user_id)}
                        disabled={reset.isPending}
                      >
                        {t('Reset score')}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>

        <div className='mt-4 flex items-center justify-end gap-2'>
          <span className='text-muted-foreground text-sm'>
            {t('Page')} {page} / {maxPage} · {total}
          </span>
          <Button
            size='sm'
            variant='outline'
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            disabled={page <= 1}
          >
            {t('Previous')}
          </Button>
          <Button
            size='sm'
            variant='outline'
            onClick={() => setPage((p) => Math.min(maxPage, p + 1))}
            disabled={page >= maxPage}
          >
            {t('Next')}
          </Button>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
