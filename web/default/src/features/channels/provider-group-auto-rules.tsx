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
import { ArrowDown, ArrowUp, GitBranch, Save, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import { Combobox } from '@/components/ui/combobox'
import {
  PROVIDER_GROUP_STATUS,
  PROVIDER_ROUTE_TYPES,
  getProviderGroupAutoRules,
  getProviderGroups,
  providerGroupQueryKeys,
  updateProviderGroupAutoRules,
  type ProviderGroupAutoRule,
  type ProviderRouteType,
} from '@/features/channels/provider-group-api'
import { FormNavigationGuard } from '@/features/system-settings/components/form-navigation-guard'

const ROUTE_TYPE_LABELS: Record<ProviderRouteType, string> = {
  completions: '/v1/chat/completions',
  responses: '/v1/responses',
  messages: '/v1/messages',
  other: 'other / fallback',
}

type AutoRulesState = Record<ProviderRouteType, string[]>

function buildAutoRulesState(rules: ProviderGroupAutoRule[]): AutoRulesState {
  const state: AutoRulesState = {
    completions: [],
    responses: [],
    messages: [],
    other: [],
  }
  const ordered = [...rules].sort(
    (left, right) => left.sort_order - right.sort_order
  )
  for (const rule of ordered) {
    if (!rule.enabled) continue
    const candidate = rule.candidate_group.trim()
    if (!candidate) continue
    const bucket = state[rule.route_type]
    if (bucket && !bucket.includes(candidate)) {
      bucket.push(candidate)
    }
  }
  return state
}

function serializeAutoRulesState(
  rules: AutoRulesState
): ProviderGroupAutoRule[] {
  const items: ProviderGroupAutoRule[] = []
  for (const routeType of PROVIDER_ROUTE_TYPES) {
    rules[routeType].forEach((candidate, index) => {
      items.push({
        route_type: routeType,
        candidate_group: candidate,
        sort_order: index,
        enabled: true,
      })
    })
  }
  return items
}

function autoRulesEqual(a: AutoRulesState, b: AutoRulesState): boolean {
  return PROVIDER_ROUTE_TYPES.every(
    (routeType) =>
      a[routeType].length === b[routeType].length &&
      a[routeType].every((value, index) => value === b[routeType][index])
  )
}

type AutoRulesEditorProps = {
  /** Compact layout for embedding inside the groups page. */
  compact?: boolean
  /** Hide the outer page chrome when embedded. */
  embedded?: boolean
  onDirtyChange?: (dirty: boolean) => void
}

/**
 * Editable Auto route-type candidate lists.
 * Used both on `/providers/auto` and inline when the Auto group is selected.
 */
export function AutoRulesEditor({
  compact = false,
  embedded = false,
  onDirtyChange,
}: AutoRulesEditorProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const rulesQuery = useQuery({
    queryKey: providerGroupQueryKeys.autoRules(),
    queryFn: getProviderGroupAutoRules,
  })
  const groupsQuery = useQuery({
    queryKey: providerGroupQueryKeys.list(),
    queryFn: getProviderGroups,
  })

  const candidateGroups = useMemo(
    () =>
      (groupsQuery.data?.data ?? [])
        .filter(
          (group) =>
            !group.is_auto && group.status === PROVIDER_GROUP_STATUS.enabled
        )
        .map((group) => ({
          name: group.name,
          label: group.display_name || group.name,
        })),
    [groupsQuery.data]
  )

  const [rules, setRules] = useState<AutoRulesState>({
    completions: [],
    responses: [],
    messages: [],
    other: [],
  })
  const [baseline, setBaseline] = useState<AutoRulesState>({
    completions: [],
    responses: [],
    messages: [],
    other: [],
  })

  useEffect(() => {
    if (rulesQuery.data?.data === undefined) return
    const next = buildAutoRulesState(rulesQuery.data.data)
    setBaseline(next)
    setRules(next)
  }, [rulesQuery.data])

  const dirty = !autoRulesEqual(rules, baseline)

  useEffect(() => {
    onDirtyChange?.(dirty)
  }, [dirty, onDirtyChange])

  const updateRouteType = (routeType: ProviderRouteType, next: string[]) => {
    setRules((current) => ({ ...current, [routeType]: next }))
  }

  const discard = () => {
    setRules(baseline)
  }

  const saveMutation = useMutation({
    mutationFn: async () => {
      const response = await updateProviderGroupAutoRules(
        serializeAutoRulesState(rules)
      )
      if (!response.success) {
        throw new Error(
          response.message || t('Failed to save auto routing rules')
        )
      }
    },
    onSuccess: () => {
      toast.success(t('Auto routing rules saved'))
      setBaseline(rules)
      queryClient.invalidateQueries({
        queryKey: providerGroupQueryKeys.autoRules(),
      })
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const editor = (
    <div className={compact ? 'space-y-3' : 'space-y-4'}>
      {!embedded && (
        <Card className='border-primary/20 bg-primary/5'>
          <CardHeader>
            <CardTitle>{t('Auto provider group routing')}</CardTitle>
            <CardDescription>
              {t(
                'Auto is a provider group an API key can select. When a key uses auto, requests are routed to the candidate provider groups below, in order, based on the request route type. Candidates are configured globally by admins; users cannot customize auto candidates per key.'
              )}
            </CardDescription>
          </CardHeader>
        </Card>
      )}

      {embedded && (
        <p className='text-muted-foreground text-xs'>
          {t(
            'Configure candidate provider groups for each route type. Candidates are tried from top to bottom.'
          )}
        </p>
      )}

      <div
        className={
          compact ? 'space-y-2' : 'grid min-h-0 flex-1 gap-4 lg:grid-cols-2'
        }
      >
        {PROVIDER_ROUTE_TYPES.map((routeType) => (
          <AutoRouteTypeRow
            key={routeType}
            routeType={routeType}
            candidates={rules[routeType]}
            availableGroups={candidateGroups}
            compact={compact}
            onChange={(next) => updateRouteType(routeType, next)}
          />
        ))}
      </div>

      {embedded && (
        <div className='flex flex-wrap items-center justify-end gap-2'>
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
            onClick={() => saveMutation.mutate()}
            disabled={saveMutation.isPending || !dirty}
          >
            <Save className='size-4' aria-hidden='true' />
            {saveMutation.isPending ? t('Saving...') : t('Save changes')}
          </Button>
        </div>
      )}
    </div>
  )

  if (embedded) {
    return (
      <>
        <FormNavigationGuard when={dirty} />
        {editor}
      </>
    )
  }

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>
        {t('Auto routing rules')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button variant='outline' render={<Link to='/providers/groups' />}>
          <GitBranch className='size-4' aria-hidden='true' />
          {t('Provider groups')}
        </Button>
        <Button
          variant='outline'
          onClick={discard}
          disabled={!dirty || saveMutation.isPending}
        >
          {t('Discard')}
        </Button>
        <Button
          onClick={() => saveMutation.mutate()}
          disabled={saveMutation.isPending || !dirty}
        >
          <Save className='size-4' aria-hidden='true' />
          {saveMutation.isPending
            ? t('Saving...')
            : t('Save auto routing rules')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <FormNavigationGuard when={dirty} />
        <div className='flex h-full min-h-0 flex-col gap-4 overflow-y-auto'>
          {editor}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

export function ProviderGroupAutoRules() {
  return <AutoRulesEditor />
}

function AutoRouteTypeRow({
  routeType,
  candidates,
  availableGroups,
  compact,
  onChange,
}: {
  routeType: ProviderRouteType
  candidates: string[]
  availableGroups: Array<{ name: string; label: string }>
  compact: boolean
  onChange: (next: string[]) => void
}) {
  const { t } = useTranslation()

  const selectable = availableGroups.filter(
    (group) => !candidates.includes(group.name)
  )
  const comboboxOptions = selectable.map((group) => ({
    value: group.name,
    label: `${group.label} (${group.name})`,
  }))

  const move = (index: number, direction: -1 | 1) => {
    const target = index + direction
    if (target < 0 || target >= candidates.length) return
    const next = [...candidates]
    const [moved] = next.splice(index, 1)
    next.splice(target, 0, moved)
    onChange(next)
  }

  const remove = (group: string) => {
    onChange(candidates.filter((item) => item !== group))
  }

  const add = (groupName: string | null) => {
    if (!groupName) return
    if (candidates.includes(groupName)) return
    onChange([...candidates, groupName])
  }

  const content = (
    <>
      {candidates.length === 0 ? (
        <p className='text-muted-foreground rounded-lg border border-dashed p-2 text-xs'>
          {t(
            'No candidates. Auto requests for this route type fall back to legacy auto behavior.'
          )}
        </p>
      ) : (
        <ol className='space-y-1.5'>
          {candidates.map((group, index) => (
            <li
              key={group}
              className='flex items-center justify-between gap-2 rounded-lg border px-2 py-1.5'
            >
              <div className='flex min-w-0 items-center gap-2'>
                <StatusBadge
                  label={String(index + 1)}
                  variant='info'
                  size='sm'
                  copyable={false}
                />
                <span className='truncate text-sm font-medium'>{group}</span>
              </div>
              <div className='flex shrink-0 items-center gap-0.5'>
                <Button
                  variant='ghost'
                  size='icon-sm'
                  aria-label={t('Move up')}
                  disabled={index === 0}
                  onClick={() => move(index, -1)}
                >
                  <ArrowUp className='size-4' aria-hidden='true' />
                </Button>
                <Button
                  variant='ghost'
                  size='icon-sm'
                  aria-label={t('Move down')}
                  disabled={index === candidates.length - 1}
                  onClick={() => move(index, 1)}
                >
                  <ArrowDown className='size-4' aria-hidden='true' />
                </Button>
                <Button
                  variant='ghost'
                  size='icon-sm'
                  aria-label={t('Remove')}
                  onClick={() => remove(group)}
                >
                  <Trash2 className='size-4' aria-hidden='true' />
                </Button>
              </div>
            </li>
          ))}
        </ol>
      )}
      <Combobox
        options={comboboxOptions}
        value=''
        onValueChange={add}
        placeholder={t('Add candidate provider group...')}
        searchPlaceholder={t('Search provider groups...')}
        emptyText={t('No provider groups available.')}
        className='w-full'
      />
    </>
  )

  if (compact) {
    return (
      <div className='space-y-2 rounded-lg border p-3'>
        <div className='flex flex-wrap items-baseline justify-between gap-2'>
          <div className='text-sm font-medium'>{routeType}</div>
          <div className='text-muted-foreground text-[11px]'>
            {ROUTE_TYPE_LABELS[routeType]}
          </div>
        </div>
        {content}
      </div>
    )
  }

  return (
    <Card className='min-h-0'>
      <CardHeader>
        <CardTitle className='text-base'>{routeType}</CardTitle>
        <CardDescription>{ROUTE_TYPE_LABELS[routeType]}</CardDescription>
      </CardHeader>
      <CardContent className='space-y-3'>{content}</CardContent>
    </Card>
  )
}
