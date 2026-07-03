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
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowRight, FileWarning } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { StatusBadge } from '@/components/status-badge'
import {
  PROVIDER_GROUP_STATUS,
  type ProviderGroup,
  getProviderGroups,
  providerGroupQueryKeys,
} from '@/features/channels/provider-group-api'
import { SettingsSection } from '../components/settings-section'

/**
 * SettingsModule: billing-group-pricing
 * Reason: surfaces the channel-routing provider groups (the authoritative
 * source of routing membership, enabled status, and the API-call billing
 * multiplier) alongside user-level billing groups so admins understand that
 * `/providers/groups` is the single writable source for provider group ratios.
 * Read-only on this page by design; edits live on the groups page.
 */
export function ProviderGroupsOverviewSection() {
  const { t } = useTranslation()

  const groupsQuery = useQuery({
    queryKey: providerGroupQueryKeys.list(),
    queryFn: getProviderGroups,
  })

  const groups =
    (groupsQuery.data?.data ?? []).filter(
      (group: ProviderGroup) => group.is_auto !== true
    ) ?? []

  return (
    <SettingsSection title={t('Provider groups (usage billing)')}>
      <Card className='border-primary/20 bg-primary/5'>
        <CardHeader>
          <CardTitle>{t('Provider group usage ratio is the billing ratio')}</CardTitle>
          <CardDescription>
            {t(
              'Each provider group aggregates providers and is what API keys select. The usage ratio shown here is the single multiplier used for API-call billing when that provider group is selected. Edit it only on the Provider groups page.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-3'>
          {(() => {
            if (groupsQuery.isLoading) {
              return (
                <p className='text-muted-foreground text-sm'>
                  {t('Loading provider groups...')}
                </p>
              )
            }
            if (groups.length === 0) {
              return (
                <div className='text-muted-foreground flex items-center gap-2 text-sm'>
                  <FileWarning className='size-4' aria-hidden='true' />
                  {t('No provider groups yet. Create one on the Provider groups page.')}
                </div>
              )
            }
            return (
              <ul className='divide-border divide-y rounded-lg border'>
                {groups.map((group) => (
                  <li
                    key={group.id}
                    className='flex flex-wrap items-center justify-between gap-2 px-3 py-2'
                  >
                    <div className='flex flex-col gap-0.5'>
                      <span className='font-medium'>
                        {group.display_name || group.name}
                      </span>
                      <span className='text-muted-foreground text-xs'>
                        {group.name}
                      </span>
                    </div>
                    <div className='flex items-center gap-3'>
                      <span className='text-sm tabular-nums'>
                        {t('Usage billing ratio')}: {group.usage_ratio}x
                      </span>
                      {group.status === PROVIDER_GROUP_STATUS.enabled ? (
                        <StatusBadge
                          label={t('Online')}
                          variant='success'
                          size='sm'
                          copyable={false}
                        />
                      ) : (
                        <StatusBadge
                          label={t('Offline')}
                          variant='neutral'
                          size='sm'
                          copyable={false}
                        />
                      )}
                    </div>
                  </li>
                ))}
              </ul>
            )
          })()}
          <div className='flex justify-end'>
            <Button variant='outline' size='sm' render={<Link to='/providers/groups' />}>
              {t('Manage provider groups')}
              <ArrowRight className='size-4' aria-hidden='true' />
            </Button>
          </div>
        </CardContent>
      </Card>
    </SettingsSection>
  )
}