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
import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { UserLevelGroupForm } from '../models/user-level-group-form'
import { normalizeJsonString } from '../models/utils'

type UserLevelGroupSectionProps = {
  groupRatio: string
  topupGroupRatio: string
  rateLimitGroup: string
}

const OPTION_KEYS = {
  GroupRatio: 'GroupRatio',
  TopupGroupRatio: 'TopupGroupRatio',
  ModelRequestRateLimitGroup: 'ModelRequestRateLimitGroup',
} as const

export function UserLevelGroupSection({
  groupRatio,
  topupGroupRatio,
  rateLimitGroup,
}: UserLevelGroupSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const handleSave = useCallback(
    async (values: {
      GroupRatio: string
      TopupGroupRatio: string
      ModelRequestRateLimitGroup: string
    }) => {
      const next = {
        GroupRatio: normalizeJsonString(values.GroupRatio),
        TopupGroupRatio: normalizeJsonString(values.TopupGroupRatio),
        ModelRequestRateLimitGroup: normalizeJsonString(
          values.ModelRequestRateLimitGroup
        ),
      }
      const current = {
        GroupRatio: normalizeJsonString(groupRatio),
        TopupGroupRatio: normalizeJsonString(topupGroupRatio),
        ModelRequestRateLimitGroup: normalizeJsonString(rateLimitGroup),
      }

      const changed = (Object.keys(next) as Array<keyof typeof next>).filter(
        (key) => next[key] !== current[key]
      )

      for (const key of changed) {
        await updateOption.mutateAsync({
          key: OPTION_KEYS[key],
          value: next[key],
        })
      }
    },
    [groupRatio, rateLimitGroup, topupGroupRatio, updateOption]
  )

  return (
    <SettingsSection title={t('User Level Groups')}>
      <UserLevelGroupForm
        groupRatio={groupRatio}
        topupGroupRatio={topupGroupRatio}
        rateLimitGroup={rateLimitGroup}
        onSave={handleSave}
        isSaving={updateOption.isPending}
      />
    </SettingsSection>
  )
}
