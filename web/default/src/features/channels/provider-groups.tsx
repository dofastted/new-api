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
import { Link } from '@tanstack/react-router'
import {
  ArrowDown,
  ArrowUp,
  GitBranch,
  Info,
  Plus,
  Route,
  Save,
  Search,
  Trash2,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { getChannels } from '@/features/channels/api'
import { CHANNEL_TYPES } from '@/features/channels/constants'
import { channelsQueryKeys } from '@/features/channels/lib'
import {
  getAdvancedCustomIncomingPathLabel,
  parseAdvancedCustomConfig,
} from '@/features/channels/lib/advanced-custom'
import {
  PROVIDER_GROUP_STATUS,
  createProviderGroup,
  deleteProviderGroup,
  getProviderGroupChannels,
  getProviderGroups,
  providerGroupQueryKeys,
  updateProviderGroup,
  updateProviderGroupChannels,
  type ProviderGroup,
  type ProviderGroupChannel,
} from '@/features/channels/provider-group-api'
import type { Channel } from '@/features/channels/types'

const PROVIDER_GROUP_CHANNEL_PAGE_SIZE = 10000

type MembershipState = {
  enabled: boolean
  priority: number
}

type MetadataDraft = {
  display_name: string
  description: string
  usage_ratio: number
  status: number
}

function parseModelCount(value: string | null | undefined): number {
  if (!value) return 0
  return value
    .split(',')
    .map((model) => model.trim())
    .filter(Boolean).length
}

function getChannelRouteLabels(channel: Channel): string[] {
  const advancedCustom = parseAdvancedCustomConfig(channel.settings)
  const routes = advancedCustom?.advanced_routes || []
  if (routes.length === 0) return ['All routes']
  return [
    ...new Set(
      routes
        .map((route) => route.incoming_path?.trim())
        .filter((route): route is string => Boolean(route))
        .map(getAdvancedCustomIncomingPathLabel)
    ),
  ]
}

function getChannelTypeLabel(channel: Channel): string {
  return (
    CHANNEL_TYPES[channel.type as keyof typeof CHANNEL_TYPES] ||
    `#${channel.type}`
  )
}

function buildProviderGroupUpdatePayload(
  group: ProviderGroup,
  patch: Partial<ProviderGroup> = {}
): Partial<ProviderGroup> {
  return {
    display_name: group.display_name || group.name,
    description: group.description,
    usage_ratio: group.usage_ratio || 1,
    status: group.status,
    is_auto: group.is_auto,
    sort_order: group.sort_order,
    ...patch,
  }
}

function buildMembershipState(
  channels: Channel[],
  memberships: ProviderGroupChannel[]
): Record<number, MembershipState> {
  const membershipByChannel = new Map(
    memberships.map((item) => [item.channel_id, item])
  )
  const state: Record<number, MembershipState> = {}
  for (const channel of channels) {
    const membership = membershipByChannel.get(channel.id)
    state[channel.id] = membership
      ? {
          enabled: membership.enabled,
          priority: membership.priority ?? channel.priority ?? 0,
        }
      : {
          enabled: false,
          priority: channel.priority ?? 0,
        }
  }
  return state
}

export function ProviderGroups() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const groupsQuery = useQuery({
    queryKey: providerGroupQueryKeys.list(),
    queryFn: getProviderGroups,
  })
  const channelsQuery = useQuery({
    queryKey: channelsQueryKeys.list({
      scope: 'provider-groups',
      page_size: PROVIDER_GROUP_CHANNEL_PAGE_SIZE,
    }),
    queryFn: () => getChannels({ page_size: PROVIDER_GROUP_CHANNEL_PAGE_SIZE }),
  })

  const groups = useMemo<ProviderGroup[]>(
    () => groupsQuery.data?.data ?? [],
    [groupsQuery.data]
  )
  const channels = useMemo<Channel[]>(
    () => channelsQuery.data?.data?.items ?? [],
    [channelsQuery.data]
  )

  const [selectedId, setSelectedId] = useState<number | null>(null)
  const selectedGroup = groups.find((group) => group.id === selectedId) ?? null

  const orderMutation = useMutation({
    mutationFn: async (
      updates: Array<{ group: ProviderGroup; sortOrder: number }>
    ) => {
      await Promise.all(
        updates.map(async (update) => {
          const response = await updateProviderGroup(
            update.group.id,
            buildProviderGroupUpdatePayload(update.group, {
              sort_order: update.sortOrder,
            })
          )
          if (!response.success) {
            throw new Error(
              response.message || t('Failed to update provider group')
            )
          }
        })
      )
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: providerGroupQueryKeys.list(),
      })
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const moveGroup = (index: number, direction: -1 | 1) => {
    const target = index + direction
    if (target < 0 || target >= groups.length) return
    const currentGroup = groups[index]
    const targetGroup = groups[target]
    if (!currentGroup || !targetGroup) return
    orderMutation.mutate([
      { group: currentGroup, sortOrder: targetGroup.sort_order },
      { group: targetGroup, sortOrder: currentGroup.sort_order },
    ])
  }

  useEffect(() => {
    if (groups.length === 0) {
      setSelectedId(null)
      return
    }
    setSelectedId((current) => {
      if (current && groups.some((group) => group.id === current)) {
        return current
      }
      return groups[0].id
    })
  }, [groups])

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Provider groups')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button variant='outline' render={<Link to='/providers/auto' />}>
          <Route className='size-4' aria-hidden='true' />
          {t('Auto routing rules')}
        </Button>
        <CreateProviderGroupButton
          existingNames={groups.map((group) => group.name)}
          onCreated={(group) => {
            queryClient.invalidateQueries({
              queryKey: providerGroupQueryKeys.list(),
            })
            setSelectedId(group.id)
          }}
        />
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-4'>
          <Card className='border-primary/20 bg-primary/5'>
            <CardHeader>
              <CardTitle>{t('Provider routing groups')}</CardTitle>
              <CardDescription>
                {t(
                  'Provider groups are the routing targets API keys select. User level groups such as default, vip, and premium stay in billing settings and are not shown here.'
                )}
              </CardDescription>
            </CardHeader>
          </Card>

          <div className='grid min-h-0 flex-1 gap-4 xl:grid-cols-[minmax(340px,420px)_minmax(0,1fr)]'>
            <Card className='min-h-0'>
              <CardHeader>
                <CardTitle>{t('Provider groups')}</CardTitle>
                <CardDescription>
                  {t('Online groups can be selected by API keys.')}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <StaticDataTable
                  data={groups}
                  getRowKey={(row) => row.id}
                  emptyContent={t('No provider groups yet.')}
                  getRowClassName={(row) =>
                    row.id === selectedId ? 'bg-primary/5' : undefined
                  }
                  columns={[
                    {
                      id: 'name',
                      header: t('Provider group'),
                      className: 'min-w-44',
                      cell: (row, index) => (
                        <div className='flex items-start gap-2'>
                          <div className='flex flex-col gap-1 pt-0.5'>
                            <Button
                              variant='ghost'
                              size='icon-xs'
                              aria-label={t('Move up')}
                              disabled={index === 0 || orderMutation.isPending}
                              onClick={() => moveGroup(index, -1)}
                            >
                              <ArrowUp className='size-3' aria-hidden='true' />
                            </Button>
                            <Button
                              variant='ghost'
                              size='icon-xs'
                              aria-label={t('Move down')}
                              disabled={
                                index === groups.length - 1 ||
                                orderMutation.isPending
                              }
                              onClick={() => moveGroup(index, 1)}
                            >
                              <ArrowDown
                                className='size-3'
                                aria-hidden='true'
                              />
                            </Button>
                          </div>
                          <button
                            type='button'
                            className='focus-visible:ring-ring/50 flex min-w-0 flex-1 flex-col gap-1 rounded-md text-left outline-none focus-visible:ring-3'
                            onClick={() => setSelectedId(row.id)}
                          >
                            <span className='flex items-center gap-2 font-medium'>
                              {row.display_name || row.name}
                              {row.is_auto && (
                                <StatusBadge
                                  label='auto'
                                  variant='success'
                                  size='sm'
                                  copyable={false}
                                />
                              )}
                            </span>
                            <span className='text-muted-foreground text-xs'>
                              {row.name}
                            </span>
                          </button>
                        </div>
                      ),
                    },
                    {
                      id: 'ratio',
                      header: t('Usage ratio'),
                      className: 'w-24',
                      cell: (row) => (
                        <span className='text-sm tabular-nums'>
                          {row.usage_ratio}x
                        </span>
                      ),
                    },
                    {
                      id: 'status',
                      header: t('Status'),
                      className: 'w-24',
                      cell: (row) =>
                        row.status === PROVIDER_GROUP_STATUS.enabled ? (
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
                        ),
                    },
                  ]}
                />
              </CardContent>
            </Card>

            {selectedGroup ? (
              <ProviderGroupDetail
                key={selectedGroup.id}
                group={selectedGroup}
                channels={channels}
                onChanged={() => {
                  queryClient.invalidateQueries({
                    queryKey: providerGroupQueryKeys.list(),
                  })
                }}
                onDeleted={() => {
                  queryClient.invalidateQueries({
                    queryKey: providerGroupQueryKeys.list(),
                  })
                  setSelectedId(null)
                }}
              />
            ) : (
              <Card className='min-h-0'>
                <CardContent className='text-muted-foreground flex h-40 items-center justify-center rounded-lg border border-dashed text-sm'>
                  <GitBranch className='mr-2 size-4' aria-hidden='true' />
                  {t('Create or select a provider group to begin.')}
                </CardContent>
              </Card>
            )}
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function CreateProviderGroupButton({
  existingNames,
  onCreated,
}: {
  existingNames: string[]
  onCreated: (group: ProviderGroup) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [ratio, setRatio] = useState('1')

  const createMutation = useMutation({
    mutationFn: async () => {
      const trimmed = name.trim()
      if (!trimmed) {
        throw new Error(t('Provider group name is required.'))
      }
      if (existingNames.includes(trimmed)) {
        throw new Error(t('Provider group name must be unique.'))
      }
      const response = await createProviderGroup({
        name: trimmed,
        display_name: displayName.trim() || trimmed,
        usage_ratio: Number.parseFloat(ratio) || 1,
        status: PROVIDER_GROUP_STATUS.enabled,
      })
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Failed to create provider group')
        )
      }
      return response.data
    },
    onSuccess: (group) => {
      toast.success(t('Provider group created'))
      setOpen(false)
      setName('')
      setDisplayName('')
      setRatio('1')
      onCreated(group)
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  return (
    <>
      <Button onClick={() => setOpen(true)}>
        <Plus className='size-4' aria-hidden='true' />
        {t('Add provider group')}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Add provider group')}</DialogTitle>
            <DialogDescription>
              {t(
                'The name is the stable identifier used by API keys and logs. It cannot be changed later.'
              )}
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4'>
            <label className='block space-y-1.5 text-sm'>
              <span className='font-medium'>{t('Provider group name')}</span>
              <Input
                value={name}
                placeholder='gpt'
                onChange={(event) => setName(event.target.value)}
              />
            </label>
            <label className='block space-y-1.5 text-sm'>
              <span className='font-medium'>{t('Display name')}</span>
              <Input
                value={displayName}
                onChange={(event) => setDisplayName(event.target.value)}
              />
            </label>
            <label className='block space-y-1.5 text-sm'>
              <span className='font-medium'>{t('Usage ratio')}</span>
              <Input
                type='number'
                step='0.01'
                min='0'
                value={ratio}
                onChange={(event) => setRatio(event.target.value)}
              />
            </label>
          </div>
          <DialogFooter>
            <Button variant='outline' onClick={() => setOpen(false)}>
              {t('Cancel')}
            </Button>
            <Button
              onClick={() => createMutation.mutate()}
              disabled={createMutation.isPending}
            >
              {createMutation.isPending ? t('Saving...') : t('Create')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function ProviderGroupDetail({
  group,
  channels,
  onChanged,
  onDeleted,
}: {
  group: ProviderGroup
  channels: Channel[]
  onChanged: () => void
  onDeleted: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [metadata, setMetadata] = useState<MetadataDraft>({
    display_name: group.display_name,
    description: group.description,
    usage_ratio: group.usage_ratio,
    status: group.status,
  })
  const [confirmDelete, setConfirmDelete] = useState(false)

  const membershipQuery = useQuery({
    queryKey: providerGroupQueryKeys.channels(group.id),
    queryFn: () => getProviderGroupChannels(group.id),
  })

  const [membership, setMembership] = useState<Record<number, MembershipState>>(
    {}
  )
  const [membershipDirty, setMembershipDirty] = useState(false)
  const [providerFilter, setProviderFilter] = useState('')
  const [pendingProviderId, setPendingProviderId] = useState('')

  useEffect(() => {
    if (membershipDirty) return
    setMembership(
      buildMembershipState(channels, membershipQuery.data?.data ?? [])
    )
  }, [channels, membershipQuery.data, membershipDirty])

  const metadataMutation = useMutation({
    mutationFn: async () => {
      const response = await updateProviderGroup(
        group.id,
        buildProviderGroupUpdatePayload(group, {
          display_name: metadata.display_name.trim() || group.name,
          description: metadata.description,
          usage_ratio: metadata.usage_ratio,
          status: metadata.status,
        })
      )
      if (!response.success) {
        throw new Error(
          response.message || t('Failed to update provider group')
        )
      }
    },
    onSuccess: () => {
      toast.success(t('Provider group updated'))
      onChanged()
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const membershipMutation = useMutation({
    mutationFn: async () => {
      const items: ProviderGroupChannel[] = channels
        .filter((channel) => membership[channel.id]?.enabled)
        .map((channel, index) => {
          const state = membership[channel.id]
          return {
            provider_group_id: group.id,
            channel_id: channel.id,
            priority: state.priority,
            weight: null,
            route_types: '',
            enabled: true,
            sort_order: index,
          }
        })
      const response = await updateProviderGroupChannels(group.id, items)
      if (!response.success) {
        throw new Error(response.message || t('Failed to update members'))
      }
    },
    onSuccess: () => {
      toast.success(t('Provider group members saved'))
      setMembershipDirty(false)
      queryClient.invalidateQueries({
        queryKey: providerGroupQueryKeys.channels(group.id),
      })
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: async () => {
      const response = await deleteProviderGroup(group.id)
      if (!response.success) {
        throw new Error(
          response.message || t('Failed to delete provider group')
        )
      }
    },
    onSuccess: () => {
      toast.success(t('Provider group deleted'))
      setConfirmDelete(false)
      onDeleted()
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const updateMembership = (
    channelId: number,
    patch: Partial<MembershipState>
  ) => {
    setMembershipDirty(true)
    setMembership((current) => ({
      ...current,
      [channelId]: { ...current[channelId], ...patch },
    }))
  }

  const memberCount = channels.filter(
    (channel) => membership[channel.id]?.enabled
  ).length

  const selectedChannels = channels.filter(
    (channel) => membership[channel.id]?.enabled
  )
  const selectedChannelIds = new Set(
    selectedChannels.map((channel) => channel.id)
  )
  const providerKeyword = providerFilter.trim().toLowerCase()
  const availableChannels = channels.filter((channel) => {
    if (selectedChannelIds.has(channel.id)) return false
    if (!providerKeyword) return true
    return [
      channel.name,
      String(channel.id),
      getChannelTypeLabel(channel),
      channel.models,
    ]
      .join(' ')
      .toLowerCase()
      .includes(providerKeyword)
  })

  const addPendingProvider = () => {
    const channelId = Number.parseInt(pendingProviderId, 10)
    if (!channelId) return
    updateMembership(channelId, { enabled: true })
    setPendingProviderId('')
  }

  const removeProvider = (channelId: number) => {
    updateMembership(channelId, { enabled: false })
  }
  return (
    <Card className='min-h-0'>
      <CardHeader>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div>
            <CardTitle>{group.display_name || group.name}</CardTitle>
            <CardDescription>{group.name}</CardDescription>
          </div>
          <Button
            variant='outline'
            size='sm'
            className='text-destructive'
            onClick={() => setConfirmDelete(true)}
          >
            <Trash2 className='size-4' aria-hidden='true' />
            {t('Delete')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className='space-y-6'>
        <div className='grid gap-3 lg:grid-cols-[minmax(0,1fr)_140px]'>
          <label className='space-y-1.5 text-sm'>
            <span className='font-medium'>{t('Display name')}</span>
            <Input
              value={metadata.display_name}
              onChange={(event) =>
                setMetadata((current) => ({
                  ...current,
                  display_name: event.target.value,
                }))
              }
            />
          </label>
          <label className='space-y-1.5 text-sm'>
            <span className='font-medium'>{t('Usage ratio')}</span>
            <Input
              type='number'
              step='0.01'
              min='0'
              value={metadata.usage_ratio}
              onChange={(event) =>
                setMetadata((current) => ({
                  ...current,
                  usage_ratio: Number.parseFloat(event.target.value) || 0,
                }))
              }
            />
          </label>
        </div>
        <label className='block space-y-1.5 text-sm'>
          <span className='font-medium'>{t('Description')}</span>
          <Textarea
            rows={2}
            value={metadata.description}
            onChange={(event) =>
              setMetadata((current) => ({
                ...current,
                description: event.target.value,
              }))
            }
          />
        </label>
        <div className='flex items-center justify-between rounded-lg border p-3'>
          <div className='space-y-0.5'>
            <div className='text-sm font-medium'>{t('Online')}</div>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Offline groups are hidden from API key selection. Keys already bound to an offline group fail with "分组已下线".'
              )}
            </p>
          </div>
          <Switch
            checked={metadata.status === PROVIDER_GROUP_STATUS.enabled}
            onCheckedChange={(checked) =>
              setMetadata((current) => ({
                ...current,
                status: checked
                  ? PROVIDER_GROUP_STATUS.enabled
                  : PROVIDER_GROUP_STATUS.disabled,
              }))
            }
          />
        </div>
        <div className='flex justify-end'>
          <Button
            onClick={() => metadataMutation.mutate()}
            disabled={metadataMutation.isPending}
          >
            <Save className='size-4' aria-hidden='true' />
            {metadataMutation.isPending ? t('Saving...') : t('Save details')}
          </Button>
        </div>

        {group.is_auto ? (
          <div className='text-muted-foreground flex items-start gap-2 rounded-lg border border-dashed p-3 text-sm'>
            <Info className='mt-0.5 size-4 shrink-0' aria-hidden='true' />
            <span>
              {t(
                'The auto group has no direct providers. Its routing candidates are configured per route type on the Auto routing rules page.'
              )}
            </span>
          </div>
        ) : (
          <div className='space-y-3'>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <div>
                <div className='text-sm font-medium'>{t('Providers')}</div>
                <p className='text-muted-foreground text-xs'>
                  {t(
                    'Add providers to this group. Route support is read from each provider configuration automatically; only priority is configured here.'
                  )}{' '}
                  {t('{{count}} provider(s) selected', { count: memberCount })}
                </p>
              </div>
              <Button
                size='sm'
                onClick={() => membershipMutation.mutate()}
                disabled={membershipMutation.isPending || !membershipDirty}
              >
                <Save className='size-4' aria-hidden='true' />
                {membershipMutation.isPending
                  ? t('Saving...')
                  : t('Save members')}
              </Button>
            </div>
            <div className='bg-muted/20 grid gap-3 rounded-lg border p-3 lg:grid-cols-[minmax(0,1fr)_220px_auto]'>
              <div className='relative'>
                <Search className='text-muted-foreground absolute top-1/2 left-3 size-4 -translate-y-1/2' />
                <Input
                  value={providerFilter}
                  placeholder={t(
                    'Search providers by name, ID, type, or model...'
                  )}
                  className='pl-9'
                  onChange={(event) => setProviderFilter(event.target.value)}
                />
              </div>
              <NativeSelect
                value={pendingProviderId}
                onChange={(event) => setPendingProviderId(event.target.value)}
              >
                <NativeSelectOption value=''>
                  {t('Choose a provider to add')}
                </NativeSelectOption>
                {availableChannels.map((channel) => (
                  <NativeSelectOption
                    key={channel.id}
                    value={String(channel.id)}
                  >
                    #{channel.id} · {channel.name}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
              <Button
                variant='outline'
                onClick={addPendingProvider}
                disabled={!pendingProviderId}
              >
                <Plus className='size-4' aria-hidden='true' />
                {t('Add provider')}
              </Button>
            </div>
            <StaticDataTable
              data={selectedChannels}
              getRowKey={(row) => row.id}
              emptyContent={t('No providers selected yet. Add one above.')}
              columns={[
                {
                  id: 'provider',
                  header: t('Provider'),
                  className: 'min-w-64',
                  cell: (row) => (
                    <div className='space-y-1'>
                      <div className='flex flex-wrap items-center gap-2'>
                        <span className='font-medium'>{row.name}</span>
                        <StatusBadge
                          label={getChannelTypeLabel(row)}
                          variant='info'
                          size='sm'
                          copyable={false}
                        />
                      </div>
                      <div className='text-muted-foreground text-xs'>
                        ID {row.id} · {parseModelCount(row.models)}{' '}
                        {t('model(s)')}
                      </div>
                    </div>
                  ),
                },
                {
                  id: 'routes',
                  header: t('Auto-detected routes'),
                  className: 'min-w-64',
                  cell: (row) => (
                    <div className='flex flex-wrap gap-1.5'>
                      {getChannelRouteLabels(row).map((route) => (
                        <StatusBadge
                          key={route}
                          label={t(route)}
                          variant='neutral'
                          size='sm'
                          copyable={false}
                        />
                      ))}
                    </div>
                  ),
                },
                {
                  id: 'priority',
                  header: t('Priority'),
                  className: 'w-28',
                  cell: (row) => (
                    <Input
                      type='number'
                      value={membership[row.id]?.priority ?? 0}
                      onChange={(event) =>
                        updateMembership(row.id, {
                          priority:
                            Number.parseInt(event.target.value, 10) || 0,
                        })
                      }
                    />
                  ),
                },
                {
                  id: 'actions',
                  header: t('Actions'),
                  className: 'w-24',
                  cell: (row) => (
                    <Button
                      variant='ghost'
                      size='sm'
                      className='text-destructive'
                      onClick={() => removeProvider(row.id)}
                    >
                      <Trash2 className='size-4' aria-hidden='true' />
                      {t('Remove')}
                    </Button>
                  ),
                },
              ]}
            />
          </div>
        )}
      </CardContent>

      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title={t('Delete provider group')}
        desc={t(
          'Deleting this provider group does not rewrite existing API keys. Keys still bound to it will fail with "分组已下线" until they are moved to an online group.'
        )}
        destructive
        confirmText={t('Delete')}
        isLoading={deleteMutation.isPending}
        handleConfirm={() => deleteMutation.mutate()}
      />
    </Card>
  )
}
