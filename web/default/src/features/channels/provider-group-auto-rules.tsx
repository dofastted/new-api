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
import { ArrowDown, ArrowUp, GitBranch, Plus, Save, Trash2 } from 'lucide-react'
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
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
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

export function ProviderGroupAutoRules() {
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
        .map((group) => group.name),
    [groupsQuery.data]
  )

  const [rules, setRules] = useState<AutoRulesState>({
    completions: [],
    responses: [],
    messages: [],
    other: [],
  })
  const [dirty, setDirty] = useState(false)

  useEffect(() => {
    if (dirty) return
    setRules(buildAutoRulesState(rulesQuery.data?.data ?? []))
  }, [rulesQuery.data, dirty])

  const updateRouteType = (routeType: ProviderRouteType, next: string[]) => {
    setDirty(true)
    setRules((current) => ({ ...current, [routeType]: next }))
  }

  const saveMutation = useMutation({
    mutationFn: async () => {
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
      const response = await updateProviderGroupAutoRules(items)
      if (!response.success) {
        throw new Error(
          response.message || t('Failed to save auto routing rules')
        )
      }
    },
    onSuccess: () => {
      toast.success(t('Auto routing rules saved'))
      setDirty(false)
      queryClient.invalidateQueries({
        queryKey: providerGroupQueryKeys.autoRules(),
      })
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

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
        <div className='flex h-full min-h-0 flex-col gap-4'>
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

          <div className='grid min-h-0 flex-1 gap-4 lg:grid-cols-2'>
            {PROVIDER_ROUTE_TYPES.map((routeType) => (
              <AutoRouteTypeCard
                key={routeType}
                routeType={routeType}
                candidates={rules[routeType]}
                availableGroups={candidateGroups}
                onChange={(next) => updateRouteType(routeType, next)}
              />
            ))}
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function AutoRouteTypeCard({
  routeType,
  candidates,
  availableGroups,
  onChange,
}: {
  routeType: ProviderRouteType
  candidates: string[]
  availableGroups: string[]
  onChange: (next: string[]) => void
}) {
  const { t } = useTranslation()
  const [pendingGroup, setPendingGroup] = useState('')

  const selectable = availableGroups.filter(
    (group) => !candidates.includes(group)
  )

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

  const add = () => {
    if (!pendingGroup) return
    onChange([...candidates, pendingGroup])
    setPendingGroup('')
  }

  return (
    <Card className='min-h-0'>
      <CardHeader>
        <CardTitle className='text-base'>{routeType}</CardTitle>
        <CardDescription>{ROUTE_TYPE_LABELS[routeType]}</CardDescription>
      </CardHeader>
      <CardContent className='space-y-3'>
        {candidates.length === 0 ? (
          <p className='text-muted-foreground rounded-lg border border-dashed p-3 text-xs'>
            {t(
              'No candidates. Auto requests for this route type fall back to legacy auto behavior.'
            )}
          </p>
        ) : (
          <ol className='space-y-2'>
            {candidates.map((group, index) => (
              <li
                key={group}
                className='flex items-center justify-between gap-2 rounded-lg border p-2'
              >
                <div className='flex items-center gap-2'>
                  <StatusBadge
                    label={String(index + 1)}
                    variant='info'
                    size='sm'
                    copyable={false}
                  />
                  <span className='text-sm font-medium'>{group}</span>
                </div>
                <div className='flex items-center gap-1'>
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
        <div className='flex items-center gap-2'>
          <NativeSelect
            className='w-full'
            value={pendingGroup}
            disabled={selectable.length === 0}
            onChange={(event) => setPendingGroup(event.target.value)}
          >
            <NativeSelectOption value=''>
              {t('Add candidate provider group...')}
            </NativeSelectOption>
            {selectable.map((group) => (
              <NativeSelectOption key={group} value={group}>
                {group}
              </NativeSelectOption>
            ))}
          </NativeSelect>
          <Button
            variant='outline'
            size='sm'
            onClick={add}
            disabled={!pendingGroup}
          >
            <Plus className='size-4' aria-hidden='true' />
            {t('Add')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
