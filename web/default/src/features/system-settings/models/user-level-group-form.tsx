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
import { Plus, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import { SettingsPageActionsPortal } from '../components/settings-page-context'
import { safeJsonParse } from '../utils/json-parser'

const BUILTIN_GROUPS = ['default', 'vip', 'premium'] as const
const BUILTIN_DEFAULT_RPM: Record<string, number> = {
  default: 15,
  vip: 25,
  premium: 50,
}

function isBuiltinGroup(name: string): boolean {
  return (BUILTIN_GROUPS as readonly string[]).includes(name)
}

type UserLevelGroupRow = {
  _id: string
  name: string
  /** Billing ratio applied to API call cost for this user level. */
  ratio: number
  /** Successful requests per minute cap. */
  rpm: number
  /** Recharge / top-up price multiplier. */
  topupRatio: number
}

type UserLevelGroupFormValues = {
  GroupRatio: string
  TopupGroupRatio: string
  ModelRequestRateLimitGroup: string
}

type UserLevelGroupFormProps = {
  groupRatio: string
  topupGroupRatio: string
  rateLimitGroup: string
  onSave: (values: UserLevelGroupFormValues) => Promise<void>
  isSaving: boolean
}

let rowIdCounter = 0
function createRowId() {
  rowIdCounter += 1
  return `ulg_${rowIdCounter}`
}

function toFiniteNumber(value: unknown, fallback: number): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : fallback
}

function buildRows(
  groupRatio: string,
  topupGroupRatio: string,
  rateLimitGroup: string
): UserLevelGroupRow[] {
  const ratioMap = safeJsonParse<Record<string, number>>(groupRatio, {
    fallback: {},
    context: 'group ratios',
  })
  const topupMap = safeJsonParse<Record<string, number>>(topupGroupRatio, {
    fallback: {},
    context: 'top-up group ratios',
  })
  const rateLimitMap = safeJsonParse<Record<string, [number, number]>>(
    rateLimitGroup,
    {
      fallback: {},
      context: 'group rate limits',
    }
  )

  const names = new Set<string>([
    ...BUILTIN_GROUPS,
    ...Object.keys(ratioMap),
    ...Object.keys(topupMap),
    ...Object.keys(rateLimitMap),
  ])

  return [...names].map((name) => {
    const limit = rateLimitMap[name]
    const rpmDefault = BUILTIN_DEFAULT_RPM[name] ?? 0
    return {
      _id: createRowId(),
      name,
      ratio: toFiniteNumber(ratioMap[name], 1),
      rpm: Array.isArray(limit)
        ? toFiniteNumber(limit[1], rpmDefault)
        : rpmDefault,
      topupRatio: toFiniteNumber(topupMap[name], 1),
    }
  })
}

function serializeRows(
  rows: UserLevelGroupRow[],
  previousRateLimitGroup: string
): UserLevelGroupFormValues {
  const previousLimits = safeJsonParse<Record<string, [number, number]>>(
    previousRateLimitGroup,
    { fallback: {}, silent: true }
  )

  const groupRatio: Record<string, number> = {}
  const topupGroupRatio: Record<string, number> = {}
  const rateLimitGroup: Record<string, [number, number]> = {}

  for (const row of rows) {
    const name = row.name.trim()
    if (!name) continue
    groupRatio[name] = toFiniteNumber(row.ratio, 1)
    topupGroupRatio[name] = toFiniteNumber(row.topupRatio, 1)
    // Preserve the existing total-count slot (failures included); only the
    // success-count slot drives the user-facing requests-per-minute cap.
    const previousTotal = Array.isArray(previousLimits[name])
      ? toFiniteNumber(previousLimits[name][0], 0)
      : 0
    rateLimitGroup[name] = [previousTotal, Math.max(1, Math.round(row.rpm))]
  }

  return {
    GroupRatio: JSON.stringify(groupRatio, null, 2),
    TopupGroupRatio: JSON.stringify(topupGroupRatio, null, 2),
    ModelRequestRateLimitGroup: JSON.stringify(rateLimitGroup, null, 2),
  }
}

function rowsSignature(rows: UserLevelGroupRow[]): string {
  return JSON.stringify(
    rows
      .map((row) => ({
        name: row.name.trim(),
        ratio: toFiniteNumber(row.ratio, 1),
        rpm: Math.max(1, Math.round(toFiniteNumber(row.rpm, 0))),
        topupRatio: toFiniteNumber(row.topupRatio, 1),
      }))
      .filter((row) => row.name)
      .sort((a, b) => a.name.localeCompare(b.name))
  )
}

function sourceSignature(
  groupRatio: string,
  topupGroupRatio: string,
  rateLimitGroup: string
): string {
  return rowsSignature(buildRows(groupRatio, topupGroupRatio, rateLimitGroup))
}

export function UserLevelGroupForm({
  groupRatio,
  topupGroupRatio,
  rateLimitGroup,
  onSave,
  isSaving,
}: UserLevelGroupFormProps) {
  const { t } = useTranslation()
  const [rows, setRows] = useState<UserLevelGroupRow[]>(() =>
    buildRows(groupRatio, topupGroupRatio, rateLimitGroup)
  )

  useEffect(() => {
    const incoming = sourceSignature(
      groupRatio,
      topupGroupRatio,
      rateLimitGroup
    )
    setRows((current) => {
      if (rowsSignature(current) === incoming) {
        return current
      }
      return buildRows(groupRatio, topupGroupRatio, rateLimitGroup)
    })
  }, [groupRatio, topupGroupRatio, rateLimitGroup])

  const updateRow = useCallback(
    (
      id: string,
      field: Exclude<keyof UserLevelGroupRow, '_id'>,
      value: string | number
    ) => {
      setRows((current) =>
        current.map((row) =>
          row._id === id ? { ...row, [field]: value } : row
        )
      )
    },
    []
  )

  const addRow = useCallback(() => {
    setRows((current) => {
      const existing = new Set(current.map((row) => row.name))
      let index = 1
      let name = `group_${index}`
      while (existing.has(name)) {
        index += 1
        name = `group_${index}`
      }
      return [
        ...current,
        {
          _id: createRowId(),
          name,
          ratio: 1,
          rpm: 0,
          topupRatio: 1,
        },
      ]
    })
  }, [])

  const removeRow = useCallback((id: string) => {
    setRows((current) => current.filter((row) => row._id !== id))
  }, [])

  const duplicateNames = useMemo(() => {
    const counts = new Map<string, number>()
    for (const row of rows) {
      const name = row.name.trim()
      if (!name) continue
      counts.set(name, (counts.get(name) ?? 0) + 1)
    }
    return [...counts.entries()]
      .filter(([, count]) => count > 1)
      .map(([name]) => name)
  }, [rows])

  const handleSave = useCallback(() => {
    return onSave(serializeRows(rows, rateLimitGroup))
  }, [onSave, rateLimitGroup, rows])

  return (
    <div className='space-y-4'>
      <SettingsPageActionsPortal>
        <Button
          type='button'
          size='sm'
          onClick={handleSave}
          disabled={isSaving || duplicateNames.length > 0}
        >
          {isSaving ? t('Saving...') : t('Save user level groups')}
        </Button>
      </SettingsPageActionsPortal>

      <div className='flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
        <p className='text-muted-foreground text-sm leading-6'>
          {t(
            'User level groups represent account tiers. Configure the billing ratio, requests-per-minute limit, and recharge multiplier for each tier.'
          )}
        </p>
        <Button onClick={addRow} size='sm' className='sm:self-start'>
          <Plus className='mr-2 h-4 w-4' />
          {t('Add user level group')}
        </Button>
      </div>

      <StaticDataTable
        data={rows}
        getRowKey={(row) => row._id}
        emptyClassName='text-muted-foreground h-20 text-sm'
        emptyContent={t(
          'No user level groups yet. Add a group to get started.'
        )}
        columns={[
          {
            id: 'name',
            header: t('User level name'),
            className: 'min-w-48',
            cell: (row) =>
              isBuiltinGroup(row.name) ? (
                <div className='flex items-center gap-2'>
                  <span className='font-medium'>{row.name}</span>
                  <StatusBadge variant='info' copyable={false}>
                    {t('Built-in')}
                  </StatusBadge>
                </div>
              ) : (
                <Input
                  value={row.name}
                  onChange={(event) =>
                    updateRow(row._id, 'name', event.target.value)
                  }
                  aria-invalid={duplicateNames.includes(row.name.trim())}
                  aria-label={t('User level name')}
                />
              ),
          },
          {
            id: 'ratio',
            header: t('Billing ratio'),
            className: 'w-28',
            cell: (row) => (
              <Input
                type='number'
                min={0}
                step={0.1}
                value={String(row.ratio)}
                onChange={(event) =>
                  updateRow(
                    row._id,
                    'ratio',
                    toFiniteNumber(event.target.value, row.ratio)
                  )
                }
                aria-label={t('Billing ratio')}
              />
            ),
          },
          {
            id: 'rpm',
            header: t('RPM limit'),
            className: 'w-28',
            cell: (row) => (
              <Input
                type='number'
                min={1}
                step={1}
                value={String(row.rpm)}
                onChange={(event) =>
                  updateRow(
                    row._id,
                    'rpm',
                    toFiniteNumber(event.target.value, row.rpm)
                  )
                }
                aria-label={t('RPM limit')}
              />
            ),
          },
          {
            id: 'topup',
            header: t('Recharge multiplier'),
            className: 'w-32',
            cell: (row) => (
              <Input
                type='number'
                min={0}
                step={0.1}
                value={String(row.topupRatio)}
                onChange={(event) =>
                  updateRow(
                    row._id,
                    'topupRatio',
                    toFiniteNumber(event.target.value, row.topupRatio)
                  )
                }
                aria-label={t('Recharge multiplier')}
              />
            ),
          },
          {
            id: 'actions',
            header: t('Actions'),
            className: 'text-right',
            cellClassName: 'text-right',
            cell: (row) => (
              <Button
                variant='ghost'
                size='sm'
                disabled={isBuiltinGroup(row.name)}
                onClick={() => removeRow(row._id)}
                aria-label={t('Delete')}
              >
                <Trash2 className='h-4 w-4' />
              </Button>
            ),
          },
        ]}
      />

      {duplicateNames.length > 0 && (
        <p className='text-destructive text-sm'>
          {t('Duplicate group names: {{names}}', {
            names: duplicateNames.join(', '),
          })}
        </p>
      )}

      <p className='text-muted-foreground text-xs leading-5'>
        {t(
          'Built-in groups (default, vip, premium) cannot be renamed or removed. RPM is the maximum number of successful requests allowed per minute.'
        )}
      </p>
    </div>
  )
}
