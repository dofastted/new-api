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
import { Link } from '@tanstack/react-router'
import {
  Activity,
  ArrowRight,
  ChevronDown,
  ListChecks,
  Route,
  ShieldAlert,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'

import { ChannelsDialogs } from './components/channels-dialogs'
import { ChannelsPrimaryButtons } from './components/channels-primary-buttons'
import { ChannelsProvider } from './components/channels-provider'
import { ChannelsTable } from './components/channels-table'

const providerFlowSteps = [
  {
    titleKey: 'Create provider channels',
    descriptionKey: 'Add GPT, Claude, GLM, or other upstream API credentials.',
    icon: Route,
  },
  {
    titleKey: 'Order fallback priority',
    descriptionKey:
      'Higher priority is tried first; weight balances traffic inside the same priority.',
    icon: ListChecks,
  },
  {
    titleKey: 'Define failure policy',
    descriptionKey:
      'Retry and disable status codes decide when a provider is skipped or fused.',
    icon: ShieldAlert,
  },
  {
    titleKey: 'Observe routing decisions',
    descriptionKey:
      'Usage log details show selected provider, retries, and final failure reasons.',
    icon: Activity,
  },
] as const

const managementLinks = [
  {
    titleKey: 'Retry policy',
    descriptionKey: 'Configure retry count and automatic retry status codes.',
    to: '/system-settings/operations/$section',
    params: { section: 'monitoring' },
  },
  {
    titleKey: 'Provider groups',
    descriptionKey:
      'Manage provider group membership, priority, route types, and usage ratio.',
    to: '/providers/groups',
    params: {},
  },
  {
    titleKey: 'Auto routing rules',
    descriptionKey:
      'Configure auto candidate provider groups and order per route type.',
    to: '/providers/auto',
    params: {},
  },
  {
    titleKey: 'Channel affinity',
    descriptionKey: 'Configure sticky provider rules and cache behavior.',
    to: '/system-settings/models/$section',
    params: { section: 'channel-affinity' },
  },
] as const

function ProviderManagementOverview() {
  const { t } = useTranslation()

  return (
    <Collapsible defaultOpen={false} className='group/provider-overview'>
      <CollapsibleTrigger
        render={
          <Button
            variant='ghost'
            className='bg-card hover:bg-accent/50 h-auto w-full justify-between gap-2 border px-3 py-2 text-left'
            aria-label={t('Toggle provider routing overview')}
          />
        }
      >
        <span className='flex min-w-0 items-center gap-2'>
          <span className='bg-primary/10 text-primary flex size-7 shrink-0 items-center justify-center rounded-md'>
            <Route className='size-4' aria-hidden='true' />
          </span>
          <span className='text-sm font-medium'>
            {t('Provider routing center')}
          </span>
        </span>
        <ChevronDown className='text-muted-foreground size-4 shrink-0 transition-transform duration-200 group-data-[open]/provider-overview:rotate-180' />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className='grid gap-3 xl:grid-cols-[minmax(0,1fr)_360px]'>
          <Card className='border-primary/20 bg-primary/5 ring-primary/10'>
            <CardHeader>
              <div className='flex flex-wrap items-start justify-between gap-3'>
                <div className='space-y-1'>
                  <CardTitle>{t('Provider routing center')}</CardTitle>
                  <CardDescription>
                    {t(
                      'Manage upstream providers in one place: credentials, model coverage, group access, priority, weight, and failure handling.'
                    )}
                  </CardDescription>
                </div>
                <StatusBadge
                  label={t('Admin only')}
                  variant='info'
                  size='lg'
                  copyable={false}
                />
              </div>
            </CardHeader>
            <CardContent>
              <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
                {providerFlowSteps.map((step) => {
                  const Icon = step.icon
                  return (
                    <div
                      key={step.titleKey}
                      className='bg-background/70 rounded-lg border p-3'
                    >
                      <div className='flex items-center gap-2'>
                        <span className='bg-muted text-muted-foreground flex size-8 items-center justify-center rounded-md'>
                          <Icon className='size-4' aria-hidden='true' />
                        </span>
                        <div className='text-sm font-medium'>
                          {t(step.titleKey)}
                        </div>
                      </div>
                      <p className='text-muted-foreground mt-2 text-xs leading-5'>
                        {t(step.descriptionKey)}
                      </p>
                    </div>
                  )
                })}
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('Routing controls')}</CardTitle>
              <CardDescription>
                {t(
                  'Global policies that decide when failed providers are retried, skipped, or disabled.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent className='space-y-2'>
              {managementLinks.map((item) => (
                <Button
                  key={item.titleKey}
                  variant='outline'
                  className='h-auto w-full justify-between gap-3 px-3 py-2 text-left'
                  render={<Link to={item.to} params={item.params} />}
                >
                  <span className='min-w-0 space-y-0.5'>
                    <span className='block text-sm font-medium'>
                      {t(item.titleKey)}
                    </span>
                    <span className='text-muted-foreground block text-xs whitespace-normal'>
                      {t(item.descriptionKey)}
                    </span>
                  </span>
                  <ArrowRight className='size-4 shrink-0' aria-hidden='true' />
                </Button>
              ))}
            </CardContent>
          </Card>
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

export function Providers() {
  const { t } = useTranslation()

  return (
    <ChannelsProvider
      labels={{
        createTitle: 'Create Provider',
        editTitle: 'Edit Provider',
        createDescription:
          'Add a new provider by providing the necessary information.',
        editDescription:
          "Update provider configuration and click save when you're done.",
      }}
    >
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>{t('Providers')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <ChannelsPrimaryButtons createLabel='Create Provider' />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex h-full min-h-0 flex-col gap-3'>
            <ProviderManagementOverview />
            <div className='min-h-0 flex-1'>
              <ChannelsTable
                emptyTitle='No Providers Found'
                emptyDescription='No providers available. Create GPT, Claude, GLM, or other upstream providers to start routing requests.'
                searchPlaceholder='Filter providers by name, ID, or key...'
              />
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ChannelsDialogs />
    </ChannelsProvider>
  )
}
