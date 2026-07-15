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
import {
  ArrowDown,
  ArrowUp,
  ChevronDown,
  GitBranch,
  GripVertical,
  MoreHorizontal,
  Plus,
  Save,
  Search,
  Trash2,
} from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
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
import { Combobox } from '@/components/ui/combobox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { getChannels, searchChannels } from '@/features/channels/api'
import { NumericSpinnerInput } from '@/features/channels/components/numeric-spinner-input'
import { CHANNEL_TYPES } from '@/features/channels/constants'
import {
  PROVIDER_GROUP_STATUS,
  createProviderGroup,
  deleteProviderGroup,
  getProviderGroupChannels,
  getProviderGroups,
  providerGroupQueryKeys,
  updateProviderGroupConfiguration,
  type ProviderGroup,
  type ProviderGroupChannel,
  type ProviderGroupChannelDetail,
  type ProviderGroupChannelInfo,
} from '@/features/channels/provider-group-api'
import { AutoRulesEditor } from '@/features/channels/provider-group-auto-rules'
import { FormNavigationGuard } from '@/features/system-settings/components/form-navigation-guard'
import { useDebounce } from '@/hooks'
import { cn } from '@/lib/utils'

type MembershipState = {
  enabled: boolean
  priority: number
  weight: number
  sortOrder: number
  channel: ProviderGroupChannelInfo
}

type MetadataDraft = {
  display_name: string
  description: string
  usage_ratio: number
  status: number
}

type PendingNavigation = { type: 'select'; id: number } | { type: 'create' }

function parseModelCount(value: string | null | undefined): number {
  if (!value) return 0
  return value
    .split(',')
    .map((model) => model.trim())
    .filter(Boolean).length
}

function getChannelTypeLabel(channel: ProviderGroupChannelInfo): string {
  return (
    CHANNEL_TYPES[channel.type as keyof typeof CHANNEL_TYPES] ||
    `#${channel.type}`
  )
}

function buildMetadataDraft(group: ProviderGroup): MetadataDraft {
  return {
    display_name: group.display_name || group.name,
    description: group.description || '',
    usage_ratio: group.usage_ratio || 1,
    status: group.status,
  }
}

function metadataEqual(a: MetadataDraft, b: MetadataDraft): boolean {
  return (
    a.display_name === b.display_name &&
    a.description === b.description &&
    a.usage_ratio === b.usage_ratio &&
    a.status === b.status
  )
}

function buildMembershipState(
  memberships: ProviderGroupChannelDetail[]
): Record<number, MembershipState> {
  // Routing authority: higher priority first; sort_order is the stable tie-breaker.
  const ordered = [...memberships].sort(
    (a, b) =>
      (b.priority ?? 0) - (a.priority ?? 0) ||
      (a.sort_order ?? 0) - (b.sort_order ?? 0) ||
      a.channel_id - b.channel_id
  )
  const state: Record<number, MembershipState> = {}
  ordered.forEach((member, index) => {
    state[member.channel_id] = {
      enabled: member.enabled,
      priority: member.priority ?? 0,
      weight: member.weight ?? 0,
      sortOrder: index,
      channel: member.channel,
    }
  })
  return state
}

function membershipSnapshot(
  membership: Record<number, MembershipState>
): string {
  const enabled = Object.entries(membership)
    .filter(([, value]) => value.enabled)
    .map(([id, value]) => ({
      id: Number(id),
      priority: value.priority,
      weight: value.weight,
      sortOrder: value.sortOrder,
    }))
    .sort((a, b) => a.sortOrder - b.sortOrder || a.id - b.id)
  return JSON.stringify(enabled)
}

function usesDefaultListPriorities(
  orderedIds: number[],
  membership: Record<number, MembershipState>
): boolean {
  return orderedIds.every(
    (id, index) => membership[id]?.priority === orderedIds.length - index
  )
}

function applyListOrder(
  orderedIds: number[],
  current: Record<number, MembershipState>
): Record<number, MembershipState> {
  const next = { ...current }
  orderedIds.forEach((id, index) => {
    next[id] = {
      ...next[id],
      enabled: true,
      sortOrder: index,
    }
  })
  return next
}

function applyListOrderPriorities(
  orderedIds: number[],
  current: Record<number, MembershipState>
): Record<number, MembershipState> {
  const next = { ...current }
  orderedIds.forEach((id, index) => {
    next[id] = {
      ...next[id],
      enabled: true,
      sortOrder: index,
      // Top of the list is tried first: higher priority for earlier items.
      priority: orderedIds.length - index,
    }
  })
  return next
}

export function ProviderGroups() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const groupsQuery = useQuery({
    queryKey: providerGroupQueryKeys.list(),
    queryFn: getProviderGroups,
  })

  const groups = useMemo<ProviderGroup[]>(
    () => groupsQuery.data?.data ?? [],
    [groupsQuery.data]
  )

  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [groupFilter, setGroupFilter] = useState('')
  const [detailDirty, setDetailDirty] = useState(false)
  const [detailVersion, setDetailVersion] = useState(0)
  const [createOpen, setCreateOpen] = useState(false)
  const [pendingNavigation, setPendingNavigation] =
    useState<PendingNavigation | null>(null)
  const addProviderFocusRequest = useRef(0)

  const selectedGroup = groups.find((group) => group.id === selectedId) ?? null

  const filteredGroups = useMemo(() => {
    const keyword = groupFilter.trim().toLowerCase()
    if (!keyword) return groups
    return groups.filter((group) =>
      [group.display_name, group.name, group.description]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
        .includes(keyword)
    )
  }, [groups, groupFilter])

  const autoGroups = filteredGroups.filter((group) => group.is_auto)
  const ordinaryGroups = filteredGroups.filter((group) => !group.is_auto)

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

  const requestSelect = (id: number) => {
    if (id === selectedId) return
    if (detailDirty) {
      setPendingNavigation({ type: 'select', id })
      return
    }
    setSelectedId(id)
  }

  const requestCreate = () => {
    if (detailDirty) {
      setPendingNavigation({ type: 'create' })
      return
    }
    setCreateOpen(true)
  }

  const confirmPendingNavigation = () => {
    if (!pendingNavigation) return
    if (pendingNavigation.type === 'select') {
      setSelectedId(pendingNavigation.id)
    } else {
      setDetailVersion((version) => version + 1)
      setCreateOpen(true)
    }
    setPendingNavigation(null)
    setDetailDirty(false)
  }

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Provider groups')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <CreateProviderGroupButton
          existingNames={groups.map((group) => group.name)}
          open={createOpen}
          onOpenChange={setCreateOpen}
          onRequestOpen={requestCreate}
          onCreated={(group) => {
            queryClient.invalidateQueries({
              queryKey: providerGroupQueryKeys.list(),
            })
            setSelectedId(group.id)
            setDetailDirty(false)
            addProviderFocusRequest.current += 1
          }}
        />
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <FormNavigationGuard when={detailDirty} />
        <div className='grid min-h-0 flex-1 gap-3 overflow-y-auto lg:grid-cols-[minmax(260px,320px)_minmax(0,1fr)] lg:grid-rows-1 lg:overflow-hidden'>
          <Card size='sm' className='min-h-[320px] lg:h-full lg:min-h-0'>
            <CardHeader className='shrink-0 space-y-3 border-b'>
              <div className='flex items-center justify-between gap-3'>
                <div>
                  <CardTitle>{t('Provider groups')}</CardTitle>
                  <CardDescription>
                    {t('Select a group to manage providers and settings.')}
                  </CardDescription>
                </div>
                <span className='bg-muted text-muted-foreground rounded-md px-2 py-1 text-xs font-medium tabular-nums'>
                  {groups.length}
                </span>
              </div>
              <div className='relative'>
                <Search className='text-muted-foreground absolute top-1/2 left-3 size-4 -translate-y-1/2' />
                <Input
                  value={groupFilter}
                  onChange={(event) => setGroupFilter(event.target.value)}
                  placeholder={t('Search groups...')}
                  className='pl-9'
                  aria-label={t('Search groups')}
                />
              </div>
            </CardHeader>
            <CardContent className='min-h-0 flex-1 space-y-3 overflow-y-auto px-2 py-3'>
              {autoGroups.length > 0 && (
                <div className='space-y-1'>
                  <div className='text-muted-foreground px-2 text-[11px] font-medium tracking-wide uppercase'>
                    {t('Auto')}
                  </div>
                  {autoGroups.map((group) => (
                    <GroupNavItem
                      key={group.id}
                      group={group}
                      selected={group.id === selectedId}
                      onSelect={() => requestSelect(group.id)}
                    />
                  ))}
                </div>
              )}
              <div className='space-y-1'>
                {autoGroups.length > 0 && (
                  <div className='text-muted-foreground px-2 text-[11px] font-medium tracking-wide uppercase'>
                    {t('Groups')}
                  </div>
                )}
                {ordinaryGroups.length === 0 && autoGroups.length === 0 && (
                  <p className='text-muted-foreground px-2 py-6 text-center text-sm'>
                    {t('No provider groups yet.')}
                  </p>
                )}
                {ordinaryGroups.length === 0 && autoGroups.length > 0 && (
                  <p className='text-muted-foreground px-2 py-3 text-sm'>
                    {t('No matching groups.')}
                  </p>
                )}
                {ordinaryGroups.map((group) => (
                  <GroupNavItem
                    key={group.id}
                    group={group}
                    selected={group.id === selectedId}
                    onSelect={() => requestSelect(group.id)}
                  />
                ))}
              </div>
            </CardContent>
          </Card>

          {selectedGroup ? (
            <ProviderGroupDetail
              key={`${selectedGroup.id}:${detailVersion}`}
              group={selectedGroup}
              focusAddProviderToken={addProviderFocusRequest.current}
              onDirtyChange={setDetailDirty}
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
                setDetailDirty(false)
              }}
            />
          ) : (
            <Card size='sm' className='min-h-[320px] lg:h-full lg:min-h-0'>
              <CardContent className='text-muted-foreground flex h-full min-h-64 items-center justify-center text-sm'>
                <GitBranch className='mr-2 size-4' aria-hidden='true' />
                {t('Create or select a provider group to begin.')}
              </CardContent>
            </Card>
          )}
        </div>

        <ConfirmDialog
          open={pendingNavigation !== null}
          onOpenChange={(open) => {
            if (!open) setPendingNavigation(null)
          }}
          title={t('Unsaved changes')}
          desc={t(
            'You have unsaved changes. Discard them and continue, or stay on this group.'
          )}
          confirmText={t('Discard changes')}
          cancelBtnText={t('Stay here')}
          destructive
          handleConfirm={confirmPendingNavigation}
        />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function GroupNavItem({
  group,
  selected,
  onSelect,
}: {
  group: ProviderGroup
  selected: boolean
  onSelect: () => void
}) {
  const { t } = useTranslation()
  const routingEnabled = group.status === PROVIDER_GROUP_STATUS.enabled

  return (
    <button
      type='button'
      onClick={onSelect}
      className={cn(
        'focus-visible:ring-ring/50 flex w-full cursor-pointer flex-col gap-1 rounded-lg border px-3 py-2 text-left outline-none transition-colors focus-visible:ring-3',
        selected
          ? 'border-primary/30 bg-primary/8'
          : 'hover:bg-muted/50 border-transparent'
      )}
    >
      <div className='flex min-w-0 items-center gap-1.5'>
        <span className='truncate text-sm font-medium'>
          {group.display_name || group.name}
        </span>
        {group.is_auto && (
          <StatusBadge
            label={t('Auto')}
            variant='success'
            size='sm'
            copyable={false}
          />
        )}
      </div>
      <div className='flex min-w-0 items-center justify-between gap-2'>
        <span className='text-muted-foreground truncate font-mono text-[11px]'>
          {group.name}
        </span>
        <StatusBadge
          label={routingEnabled ? t('Routing enabled') : t('Routing disabled')}
          variant={routingEnabled ? 'success' : 'neutral'}
          size='sm'
          copyable={false}
        />
      </div>
    </button>
  )
}

function CreateProviderGroupButton({
  existingNames,
  open,
  onOpenChange,
  onRequestOpen,
  onCreated,
}: {
  existingNames: string[]
  open: boolean
  onOpenChange: (open: boolean) => void
  onRequestOpen: () => void
  onCreated: (group: ProviderGroup) => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [ratio, setRatio] = useState('1')
  const [description, setDescription] = useState('')
  const [showAdvanced, setShowAdvanced] = useState(false)

  const reset = () => {
    setName('')
    setDisplayName('')
    setRatio('1')
    setDescription('')
    setShowAdvanced(false)
  }

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
        description: description.trim(),
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
      onOpenChange(false)
      reset()
      onCreated(group)
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  return (
    <>
      <Button aria-label={t('New provider group')} onClick={onRequestOpen}>
        <Plus className='size-4' aria-hidden='true' />
        <span className='hidden sm:inline'>{t('New provider group')}</span>
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          onOpenChange(next)
          if (!next) reset()
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('New provider group')}</DialogTitle>
            <DialogDescription>
              {t(
                'The group ID is the stable identifier used by API keys and logs. It cannot be changed later.'
              )}
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4'>
            <label className='block space-y-1.5 text-sm'>
              <span className='font-medium'>{t('Group ID')}</span>
              <Input
                value={name}
                placeholder='gpt'
                onChange={(event) => setName(event.target.value)}
              />
            </label>
            <label className='block space-y-1.5 text-sm'>
              <span className='font-medium'>
                {t('Display name')}{' '}
                <span className='text-muted-foreground font-normal'>
                  ({t('optional')})
                </span>
              </span>
              <Input
                value={displayName}
                onChange={(event) => setDisplayName(event.target.value)}
              />
            </label>
            <Collapsible open={showAdvanced} onOpenChange={setShowAdvanced}>
              <CollapsibleTrigger
                render={<Button variant='ghost' size='sm' className='px-0' />}
              >
                <ChevronDown
                  className={cn(
                    'size-4 transition-transform',
                    showAdvanced && 'rotate-180'
                  )}
                  aria-hidden='true'
                />
                {t('Advanced settings')}
              </CollapsibleTrigger>
              <CollapsibleContent className='mt-3 space-y-3'>
                <label className='block space-y-1.5 text-sm'>
                  <span className='font-medium'>{t('Billing multiplier')}</span>
                  <Input
                    type='number'
                    step='0.01'
                    min='0'
                    value={ratio}
                    onChange={(event) => setRatio(event.target.value)}
                  />
                </label>
                <label className='block space-y-1.5 text-sm'>
                  <span className='font-medium'>{t('Description')}</span>
                  <Textarea
                    rows={2}
                    value={description}
                    onChange={(event) => setDescription(event.target.value)}
                  />
                </label>
              </CollapsibleContent>
            </Collapsible>
          </div>
          <DialogFooter>
            <Button variant='outline' onClick={() => onOpenChange(false)}>
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
  focusAddProviderToken,
  onDirtyChange,
  onChanged,
  onDeleted,
}: {
  group: ProviderGroup
  focusAddProviderToken: number
  onDirtyChange: (dirty: boolean) => void
  onChanged: () => void
  onDeleted: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const addProviderInputRef = useRef<HTMLDivElement>(null)

  const [metadata, setMetadata] = useState<MetadataDraft>(() =>
    buildMetadataDraft(group)
  )
  const [baselineMetadata, setBaselineMetadata] = useState<MetadataDraft>(() =>
    buildMetadataDraft(group)
  )
  const [membership, setMembership] = useState<Record<number, MembershipState>>(
    {}
  )
  const [baselineMembership, setBaselineMembership] = useState<
    Record<number, MembershipState>
  >({})
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [advancedTiersOpen, setAdvancedTiersOpen] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [confirmDisableRouting, setConfirmDisableRouting] = useState(false)
  const [confirmClearMembers, setConfirmClearMembers] = useState(false)
  const [pendingStatus, setPendingStatus] = useState<number | null>(null)
  const [draggedChannelId, setDraggedChannelId] = useState<number | null>(null)
  const [dragOverChannelId, setDragOverChannelId] = useState<number | null>(
    null
  )
  const [dragOverPosition, setDragOverPosition] = useState<'before' | 'after'>(
    'before'
  )
  const [providerComboboxKey, setProviderComboboxKey] = useState(0)
  const [providerSearch, setProviderSearch] = useState('')
  const debouncedProviderSearch = useDebounce(providerSearch.trim(), 300)

  const membershipQuery = useQuery({
    queryKey: providerGroupQueryKeys.channels(group.id),
    queryFn: () => getProviderGroupChannels(group.id),
    enabled: !group.is_auto,
  })
  const providerSearchQuery = useQuery({
    queryKey: ['channels', 'provider-group-search', debouncedProviderSearch],
    queryFn: () =>
      debouncedProviderSearch
        ? searchChannels({
            keyword: debouncedProviderSearch,
            p: 1,
            page_size: 20,
            sort_by: 'id',
            sort_order: 'asc',
          })
        : getChannels({
            p: 1,
            page_size: 20,
            sort_by: 'id',
            sort_order: 'asc',
          }),
    enabled: !group.is_auto,
  })

  useEffect(() => {
    const draft = buildMetadataDraft(group)
    setMetadata(draft)
    setBaselineMetadata(draft)
  }, [group])

  useEffect(() => {
    if (group.is_auto) {
      setMembership({})
      setBaselineMembership({})
      return
    }
    if (membershipQuery.data?.data === undefined) return
    const next = buildMembershipState(membershipQuery.data.data)
    setMembership(next)
    setBaselineMembership(next)
  }, [group.is_auto, membershipQuery.data])

  const metadataDirty = !metadataEqual(metadata, baselineMetadata)
  const membershipDirty =
    !group.is_auto &&
    membershipSnapshot(membership) !== membershipSnapshot(baselineMembership)
  const dirty = metadataDirty || membershipDirty

  // Auto owns its own draft via AutoRulesEditor; only report ordinary-group dirty here.
  useEffect(() => {
    if (group.is_auto) return
    onDirtyChange(dirty)
  }, [dirty, group.is_auto, onDirtyChange])

  useEffect(() => {
    if (!focusAddProviderToken || group.is_auto) return
    const input = addProviderInputRef.current?.querySelector('input')
    input?.focus()
  }, [focusAddProviderToken, group.is_auto])

  const selectedChannels = useMemo(() => {
    return Object.values(membership)
      .filter((item) => item.enabled)
      .sort((a, b) => a.sortOrder - b.sortOrder || a.channel.id - b.channel.id)
      .map((item) => item.channel)
  }, [membership])

  const defaultListPriorities = useMemo(
    () =>
      usesDefaultListPriorities(
        selectedChannels.map((channel) => channel.id),
        membership
      ),
    [membership, selectedChannels]
  )

  const providerCandidates = useMemo(
    () => providerSearchQuery.data?.data?.items ?? [],
    [providerSearchQuery.data?.data?.items]
  )
  const availableChannelOptions = useMemo(() => {
    const selectedIds = new Set(selectedChannels.map((channel) => channel.id))
    return providerCandidates
      .filter((channel) => !selectedIds.has(channel.id))
      .map((channel) => ({
        value: String(channel.id),
        label: `#${channel.id} · ${channel.name} · ${getChannelTypeLabel(channel)}`,
      }))
  }, [providerCandidates, selectedChannels])

  const discard = () => {
    setMetadata(baselineMetadata)
    setMembership(baselineMembership)
  }

  const saveMutation = useMutation({
    mutationFn: async () => {
      const payload: {
        metadata?: {
          display_name: string
          description: string
          status: number
          usage_ratio: number
        }
        members?: ProviderGroupChannel[]
      } = {}
      if (metadataDirty) {
        payload.metadata = {
          display_name: metadata.display_name.trim() || group.name,
          description: metadata.description,
          status: metadata.status,
          usage_ratio: metadata.usage_ratio || 1,
        }
      }
      if (membershipDirty) {
        payload.members = selectedChannels.map((channel, index) => {
          const state = membership[channel.id]
          const weightValue = state?.weight ?? 0
          return {
            provider_group_id: group.id,
            channel_id: channel.id,
            priority: state?.priority ?? selectedChannels.length - index,
            // 0 means "use channel default weight" — keep null so routing
            // falls back to the channel weight instead of forcing zero.
            weight: weightValue > 0 ? weightValue : null,
            route_types: '',
            enabled: true,
            sort_order: index,
          }
        })
      }
      const response = await updateProviderGroupConfiguration(group.id, payload)
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Failed to save provider group configuration')
        )
      }
      return response.data
    },
    onSuccess: (result) => {
      toast.success(t('Changes saved'))
      const nextMetadata = buildMetadataDraft(result.group)
      setMetadata(nextMetadata)
      setBaselineMetadata(nextMetadata)
      if (result.members) {
        const nextMembership = buildMembershipState(result.members)
        setMembership(nextMembership)
        setBaselineMembership(nextMembership)
      } else if (membershipDirty === false) {
        // metadata-only save
      } else {
        setBaselineMembership(membership)
      }
      queryClient.invalidateQueries({
        queryKey: providerGroupQueryKeys.channels(group.id),
      })
      onChanged()
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

  const requestSave = () => {
    if (
      membershipDirty &&
      selectedChannels.length === 0 &&
      Object.values(baselineMembership).some((item) => item.enabled)
    ) {
      setConfirmClearMembers(true)
      return
    }
    saveMutation.mutate()
  }

  const updateMembership = (
    channelId: number,
    patch: Partial<MembershipState>
  ) => {
    setMembership((current) => ({
      ...current,
      [channelId]: { ...current[channelId], ...patch },
    }))
  }

  const reorderByIds = (orderedIds: number[]) => {
    if (!defaultListPriorities) return
    setMembership((current) => applyListOrderPriorities(orderedIds, current))
  }

  const reorderMember = (
    sourceId: number,
    targetId: number,
    position: 'before' | 'after'
  ) => {
    const ordered = selectedChannels.map((channel) => channel.id)
    const sourceIndex = ordered.indexOf(sourceId)
    const targetIndex = ordered.indexOf(targetId)
    if (
      sourceIndex === -1 ||
      targetIndex === -1 ||
      sourceIndex === targetIndex
    ) {
      return
    }
    const next = [...ordered]
    const [moved] = next.splice(sourceIndex, 1)
    let insertAt = next.indexOf(targetId)
    if (position === 'after') insertAt += 1
    next.splice(insertAt, 0, moved)
    reorderByIds(next)
  }

  const moveMember = (index: number, direction: -1 | 1) => {
    const target = index + direction
    if (target < 0 || target >= selectedChannels.length) return
    const ordered = selectedChannels.map((channel) => channel.id)
    const [moved] = ordered.splice(index, 1)
    ordered.splice(target, 0, moved)
    reorderByIds(ordered)
  }

  const addProvider = (value: string | null) => {
    if (!value) return
    const channelId = Number.parseInt(value, 10)
    const channel = providerCandidates.find((item) => item.id === channelId)
    if (!channelId || !channel || membership[channelId]?.enabled) return
    const orderedIds = [
      ...selectedChannels.map((selected) => selected.id),
      channelId,
    ]
    setMembership((current) => {
      const currentIds = orderedIds.slice(0, -1)
      const usesDefaultPriorities = usesDefaultListPriorities(
        currentIds,
        current
      )
      const lastChannelId = currentIds.at(-1)
      const appendPriority =
        lastChannelId === undefined
          ? 1
          : (current[lastChannelId]?.priority ?? 1)
      const withEnabled = {
        ...current,
        [channelId]: {
          enabled: true,
          priority: appendPriority,
          weight: current[channelId]?.weight ?? 0,
          sortOrder: orderedIds.length - 1,
          channel: {
            id: channel.id,
            type: channel.type,
            status: channel.status,
            name: channel.name,
            models: channel.models,
          },
        },
      }
      return usesDefaultPriorities
        ? applyListOrderPriorities(orderedIds, withEnabled)
        : applyListOrder(orderedIds, withEnabled)
    })
    setProviderComboboxKey((key) => key + 1)
  }

  const removeProvider = (channelId: number) => {
    const remaining = selectedChannels
      .filter((channel) => channel.id !== channelId)
      .map((channel) => channel.id)
    setMembership((current) => {
      const next = {
        ...current,
        [channelId]: {
          ...current[channelId],
          enabled: false,
          sortOrder: 0,
        },
      }
      return defaultListPriorities
        ? applyListOrderPriorities(remaining, next)
        : applyListOrder(remaining, next)
    })
  }

  const resetDragState = () => {
    setDraggedChannelId(null)
    setDragOverChannelId(null)
    setDragOverPosition('before')
  }

  const handleDragStart = (event: DragEvent, channelId: number) => {
    setDraggedChannelId(channelId)
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', String(channelId))
  }

  const handleDragOver = (event: DragEvent, channelId: number) => {
    if (!draggedChannelId || draggedChannelId === channelId) return
    event.preventDefault()
    const rect = event.currentTarget.getBoundingClientRect()
    const position: 'before' | 'after' =
      event.clientY - rect.top > rect.height / 2 ? 'after' : 'before'
    setDragOverChannelId(channelId)
    setDragOverPosition(position)
    event.dataTransfer.dropEffect = 'move'
  }

  const handleDrop = (event: DragEvent, channelId: number) => {
    event.preventDefault()
    const sourceId =
      draggedChannelId ??
      Number.parseInt(event.dataTransfer.getData('text/plain'), 10)
    if (sourceId && channelId && sourceId !== channelId) {
      const position =
        dragOverChannelId === channelId ? dragOverPosition : 'before'
      reorderMember(sourceId, channelId, position)
    }
    resetDragState()
  }

  const handleRoutingToggle = (checked: boolean) => {
    if (!checked && metadata.status === PROVIDER_GROUP_STATUS.enabled) {
      setPendingStatus(PROVIDER_GROUP_STATUS.disabled)
      setConfirmDisableRouting(true)
      return
    }
    setMetadata((current) => ({
      ...current,
      status: checked
        ? PROVIDER_GROUP_STATUS.enabled
        : PROVIDER_GROUP_STATUS.disabled,
    }))
  }

  const routingEnabled = metadata.status === PROVIDER_GROUP_STATUS.enabled

  return (
    <Card size='sm' className='min-h-[480px] lg:h-full lg:min-h-0'>
      <CardHeader className='shrink-0 border-b'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div className='min-w-0 space-y-1.5'>
            <CardTitle className='truncate'>
              {metadata.display_name || group.name}
            </CardTitle>
            <div className='flex flex-wrap items-center gap-1.5'>
              <span className='bg-muted text-muted-foreground rounded px-1.5 py-0.5 font-mono text-[11px]'>
                {group.name}
              </span>
              <StatusBadge
                label={
                  routingEnabled ? t('Routing enabled') : t('Routing disabled')
                }
                variant={routingEnabled ? 'success' : 'neutral'}
                size='sm'
                copyable={false}
              />
              {dirty && (
                <StatusBadge
                  label={t('Unsaved')}
                  variant='warning'
                  size='sm'
                  copyable={false}
                />
              )}
            </div>
          </div>
          <div className='flex flex-wrap items-center gap-2'>
            {!group.is_auto && (
              <div className='flex items-center gap-2 rounded-lg border px-2 py-1'>
                <span className='text-xs font-medium'>
                  {t('Routing enabled')}
                </span>
                <Switch
                  aria-label={t('Routing enabled')}
                  checked={routingEnabled}
                  onCheckedChange={handleRoutingToggle}
                />
              </div>
            )}
            {!group.is_auto && (
              <>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={discard}
                  disabled={!dirty || saveMutation.isPending}
                >
                  {t('Discard')}
                </Button>
                <Button
                  size='sm'
                  onClick={requestSave}
                  disabled={!dirty || saveMutation.isPending}
                >
                  <Save className='size-4' aria-hidden='true' />
                  {saveMutation.isPending ? t('Saving...') : t('Save changes')}
                </Button>
              </>
            )}
            {!group.is_auto && (
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <Button
                      variant='outline'
                      size='icon-sm'
                      aria-label={t('More actions')}
                    />
                  }
                >
                  <MoreHorizontal className='size-4' aria-hidden='true' />
                </DropdownMenuTrigger>
                <DropdownMenuContent align='end'>
                  <DropdownMenuItem
                    variant='destructive'
                    onClick={() => setConfirmDelete(true)}
                  >
                    <Trash2 className='size-4' aria-hidden='true' />
                    {t('Delete provider group')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </div>
        </div>
      </CardHeader>

      <CardContent className='min-h-0 flex-1 space-y-4 overflow-y-auto'>
        {group.is_auto ? (
          <section className='space-y-3'>
            <div>
              <div className='text-sm font-medium'>
                {t('Auto routing rules')}
              </div>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Providers are not attached directly to Auto. Configure candidate groups per route type instead.'
                )}
              </p>
            </div>
            <AutoRulesEditor embedded compact onDirtyChange={onDirtyChange} />
          </section>
        ) : (
          <>
            <section className='space-y-3'>
              <div>
                <div className='text-sm font-medium'>
                  {t('Providers in this group')}
                </div>
                <p className='text-muted-foreground text-xs'>
                  {defaultListPriorities
                    ? t('Providers are tried from top to bottom.')
                    : t('Routing follows the configured priority tiers.')}{' '}
                  ·{' '}
                  {t('{{count}} provider(s) selected', {
                    count: selectedChannels.length,
                  })}
                </p>
              </div>

              <div ref={addProviderInputRef}>
                <Combobox
                  key={providerComboboxKey}
                  options={availableChannelOptions}
                  value=''
                  onValueChange={addProvider}
                  onSearchValueChange={setProviderSearch}
                  shouldFilter={false}
                  placeholder={t('Search and add a provider...')}
                  searchPlaceholder={t(
                    'Search providers by name, ID, or type...'
                  )}
                  emptyText={t('No providers available to add.')}
                  className='w-full'
                />
              </div>

              {selectedChannels.length === 0 ? (
                <p className='text-muted-foreground rounded-lg border border-dashed p-4 text-center text-sm'>
                  {t('No providers selected yet. Add one above.')}
                </p>
              ) : (
                <ul className='space-y-2'>
                  {selectedChannels.map((channel, index) => {
                    const isDragOver = dragOverChannelId === channel.id
                    return (
                      <li
                        key={channel.id}
                        draggable={defaultListPriorities}
                        onDragStart={(event) =>
                          handleDragStart(event, channel.id)
                        }
                        onDragOver={(event) =>
                          handleDragOver(event, channel.id)
                        }
                        onDrop={(event) => handleDrop(event, channel.id)}
                        onDragEnd={resetDragState}
                        className={cn(
                          'flex flex-wrap items-center gap-2 rounded-lg border p-2',
                          isDragOver &&
                            dragOverPosition === 'before' &&
                            'border-t-primary border-t-2',
                          isDragOver &&
                            dragOverPosition === 'after' &&
                            'border-b-primary border-b-2'
                        )}
                      >
                        <div className='text-muted-foreground flex items-center gap-1'>
                          <GripVertical
                            className={cn(
                              'size-4',
                              defaultListPriorities
                                ? 'cursor-grab active:cursor-grabbing'
                                : 'cursor-not-allowed opacity-40'
                            )}
                            aria-label={t('Drag to reorder')}
                          />
                          <StatusBadge
                            label={String(index + 1)}
                            variant='info'
                            size='sm'
                            copyable={false}
                          />
                        </div>
                        <div className='min-w-0 flex-1'>
                          <div className='truncate text-sm font-medium'>
                            {channel.name}
                          </div>
                          <div className='text-muted-foreground truncate text-[11px]'>
                            ID {channel.id} · {getChannelTypeLabel(channel)} ·{' '}
                            {parseModelCount(channel.models)} {t('model(s)')}
                          </div>
                        </div>
                        {advancedTiersOpen && (
                          <div className='flex flex-wrap items-center gap-2'>
                            <label className='flex items-center gap-1 text-[11px]'>
                              <span className='text-muted-foreground'>
                                {t('Priority')}
                              </span>
                              <NumericSpinnerInput
                                value={membership[channel.id]?.priority ?? 0}
                                onChange={(priority) =>
                                  updateMembership(channel.id, { priority })
                                }
                                min={-999}
                              />
                            </label>
                            <label className='flex items-center gap-1 text-[11px]'>
                              <span className='text-muted-foreground'>
                                {t('Weight')}
                              </span>
                              <NumericSpinnerInput
                                value={membership[channel.id]?.weight ?? 0}
                                onChange={(weight) =>
                                  updateMembership(channel.id, { weight })
                                }
                                min={0}
                              />
                            </label>
                          </div>
                        )}
                        <div className='flex items-center gap-0.5'>
                          <Button
                            variant='ghost'
                            size='icon-sm'
                            aria-label={t('Move up')}
                            disabled={!defaultListPriorities || index === 0}
                            onClick={() => moveMember(index, -1)}
                          >
                            <ArrowUp className='size-4' aria-hidden='true' />
                          </Button>
                          <Button
                            variant='ghost'
                            size='icon-sm'
                            aria-label={t('Move down')}
                            disabled={
                              !defaultListPriorities ||
                              index === selectedChannels.length - 1
                            }
                            onClick={() => moveMember(index, 1)}
                          >
                            <ArrowDown className='size-4' aria-hidden='true' />
                          </Button>
                          <Button
                            variant='ghost'
                            size='icon-sm'
                            aria-label={t('Remove')}
                            className='text-destructive'
                            onClick={() => removeProvider(channel.id)}
                          >
                            <Trash2 className='size-4' aria-hidden='true' />
                          </Button>
                        </div>
                      </li>
                    )
                  })}
                </ul>
              )}

              {!defaultListPriorities && selectedChannels.length > 1 && (
                <p className='text-muted-foreground text-xs'>
                  {t(
                    'Reordering is disabled while custom priority tiers are configured.'
                  )}
                </p>
              )}

              <Collapsible
                open={advancedTiersOpen}
                onOpenChange={setAdvancedTiersOpen}
              >
                <CollapsibleTrigger
                  render={<Button variant='ghost' size='sm' className='px-0' />}
                >
                  <ChevronDown
                    className={cn(
                      'size-4 transition-transform',
                      advancedTiersOpen && 'rotate-180'
                    )}
                    aria-hidden='true'
                  />
                  {t('Advanced routing tiers')}
                </CollapsibleTrigger>
                <CollapsibleContent className='text-muted-foreground mt-2 text-xs'>
                  {t(
                    'Priority and weight control equal-tier load balancing. By default, list order sets fallback order and assigns decreasing priority from top to bottom. Providers with the same priority share a tier and use weight for distribution.'
                  )}
                </CollapsibleContent>
              </Collapsible>
            </section>

            <Collapsible open={settingsOpen} onOpenChange={setSettingsOpen}>
              <div className='rounded-lg border'>
                <CollapsibleTrigger
                  render={
                    <button
                      type='button'
                      className='hover:bg-muted/40 flex w-full items-center justify-between gap-2 px-3 py-2 text-left'
                    />
                  }
                >
                  <div>
                    <div className='text-sm font-medium'>
                      {t('Group settings')}
                    </div>
                    <div className='text-muted-foreground text-xs'>
                      {t(
                        'Display name, billing multiplier, description, and routing status.'
                      )}
                    </div>
                  </div>
                  <ChevronDown
                    className={cn(
                      'text-muted-foreground size-4 shrink-0 transition-transform',
                      settingsOpen && 'rotate-180'
                    )}
                    aria-hidden='true'
                  />
                </CollapsibleTrigger>
                <CollapsibleContent className='space-y-3 border-t px-3 py-3'>
                  <div className='grid gap-3 md:grid-cols-2'>
                    <label className='space-y-1 text-xs'>
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
                    <label className='space-y-1 text-xs'>
                      <span className='font-medium'>
                        {t('Billing multiplier')}
                      </span>
                      <Input
                        type='number'
                        step='0.01'
                        min='0'
                        value={metadata.usage_ratio}
                        onChange={(event) =>
                          setMetadata((current) => ({
                            ...current,
                            usage_ratio:
                              Number.parseFloat(event.target.value) || 0,
                          }))
                        }
                      />
                    </label>
                    <label className='space-y-1 text-xs md:col-span-2'>
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
                  </div>
                </CollapsibleContent>
              </div>
            </Collapsible>
          </>
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
      <ConfirmDialog
        open={confirmDisableRouting}
        onOpenChange={(open) => {
          setConfirmDisableRouting(open)
          if (!open) setPendingStatus(null)
        }}
        title={t('Disable routing for this group?')}
        desc={t(
          'Turning routing off removes this group from live provider selection. API keys still bound to it will fail with "分组已下线" until routing is enabled again or keys are moved.'
        )}
        confirmText={t('Disable routing')}
        destructive
        handleConfirm={() => {
          if (pendingStatus !== null) {
            setMetadata((current) => ({ ...current, status: pendingStatus }))
          }
          setConfirmDisableRouting(false)
          setPendingStatus(null)
        }}
      />
      <ConfirmDialog
        open={confirmClearMembers}
        onOpenChange={setConfirmClearMembers}
        title={t('Remove all providers?')}
        desc={t(
          'Saving with no providers means this group cannot route any traffic until providers are added again.'
        )}
        confirmText={t('Save without providers')}
        destructive
        isLoading={saveMutation.isPending}
        handleConfirm={() => {
          setConfirmClearMembers(false)
          saveMutation.mutate()
        }}
      />
    </Card>
  )
}
