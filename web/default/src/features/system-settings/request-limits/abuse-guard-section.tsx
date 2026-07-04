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
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import {
  SettingsForm,
  SettingsFormGrid,
  SettingsFormGridItem,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

export type AbuseGuardValues = {
  'abuse_guard.enabled': boolean
  'abuse_guard.monitor_only': boolean
  'abuse_guard.model_scope_patterns': string[]
  'abuse_guard.exempt_groups': string[]
  'abuse_guard.block_words': string[]
  'abuse_guard.disabled_builtin_ids': string[]
  'abuse_guard.custom_patterns': string
  'abuse_guard.pattern_block_score': number
  'abuse_guard.scan_window_kb': number
  'abuse_guard.moderation_api_key': string
  'abuse_guard.moderation_base_url': string
  'abuse_guard.moderation_model': string
  'abuse_guard.sample_rate_percent': number
  'abuse_guard.review_snippet_kb': number
  'abuse_guard.queue_size': number
  'abuse_guard.worker_count': number
  'abuse_guard.category_scores': string
  'abuse_guard.instant_ban_categories': string[]
  'abuse_guard.score_window_hours': number
  'abuse_guard.ban_threshold': number
  'abuse_guard.temp_ban_hours': number
  'abuse_guard.perm_ban_after_temp_bans': number
}

type AbuseGuardSectionProps = {
  defaultValues: AbuseGuardValues
}

const LIST_KEYS = new Set<keyof AbuseGuardValues>([
  'abuse_guard.model_scope_patterns',
  'abuse_guard.exempt_groups',
  'abuse_guard.block_words',
  'abuse_guard.disabled_builtin_ids',
  'abuse_guard.instant_ban_categories',
])

// 表单里列表字段以多行文本编辑,保存时转换为 JSON 数组字符串。
type FormShape = Omit<
  AbuseGuardValues,
  | 'abuse_guard.model_scope_patterns'
  | 'abuse_guard.exempt_groups'
  | 'abuse_guard.block_words'
  | 'abuse_guard.disabled_builtin_ids'
  | 'abuse_guard.instant_ban_categories'
> & {
  'abuse_guard.model_scope_patterns': string
  'abuse_guard.exempt_groups': string
  'abuse_guard.block_words': string
  'abuse_guard.disabled_builtin_ids': string
  'abuse_guard.instant_ban_categories': string
}

function linesToArray(text: string): string[] {
  return text
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l.length > 0)
}

function toFormShape(v: AbuseGuardValues): FormShape {
  return {
    ...v,
    'abuse_guard.model_scope_patterns': (
      v['abuse_guard.model_scope_patterns'] ?? []
    ).join('\n'),
    'abuse_guard.exempt_groups': (v['abuse_guard.exempt_groups'] ?? []).join(
      '\n'
    ),
    'abuse_guard.block_words': (v['abuse_guard.block_words'] ?? []).join('\n'),
    'abuse_guard.disabled_builtin_ids': (
      v['abuse_guard.disabled_builtin_ids'] ?? []
    ).join('\n'),
    'abuse_guard.instant_ban_categories': (
      v['abuse_guard.instant_ban_categories'] ?? []
    ).join('\n'),
  }
}

export function AbuseGuardSection({ defaultValues }: AbuseGuardSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const initial = toFormShape(defaultValues)
  const form = useForm<FormShape>({ defaultValues: initial })

  useEffect(() => {
    form.reset(toFormShape(defaultValues))
  }, [defaultValues, form])

  const onSubmit = async (values: FormShape) => {
    const entries = Object.entries(values) as Array<
      [keyof AbuseGuardValues, unknown]
    >
    for (const [key, value] of entries) {
      let payload: string | number | boolean
      if (LIST_KEYS.has(key)) {
        payload = JSON.stringify(linesToArray(String(value ?? '')))
      } else if (typeof value === 'boolean') {
        payload = value
      } else if (typeof value === 'number') {
        payload = value
      } else {
        payload = String(value ?? '')
      }
      await updateOption.mutateAsync({ key, value: payload })
    }
  }

  const numberField = (
    name: keyof FormShape,
    label: string,
    description?: string
  ) => (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <SettingsFormGridItem>
          <FormItem>
            <FormLabel>{t(label)}</FormLabel>
            <FormControl>
              <Input
                type='number'
                value={field.value as number}
                onChange={(e) => field.onChange(Number(e.target.value))}
              />
            </FormControl>
            {description ? (
              <FormDescription>{t(description)}</FormDescription>
            ) : null}
            <FormMessage />
          </FormItem>
        </SettingsFormGridItem>
      )}
    />
  )

  const textField = (
    name: keyof FormShape,
    label: string,
    description?: string
  ) => (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <SettingsFormGridItem>
          <FormItem>
            <FormLabel>{t(label)}</FormLabel>
            <FormControl>
              <Input {...field} value={field.value as string} />
            </FormControl>
            {description ? (
              <FormDescription>{t(description)}</FormDescription>
            ) : null}
            <FormMessage />
          </FormItem>
        </SettingsFormGridItem>
      )}
    />
  )

  const listField = (
    name: keyof FormShape,
    label: string,
    description: string,
    placeholder: string
  ) => (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t(label)}</FormLabel>
          <FormControl>
            <Textarea
              rows={6}
              placeholder={t(placeholder)}
              {...field}
              value={field.value as string}
            />
          </FormControl>
          <FormDescription>{t(description)}</FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )

  return (
    <SettingsSection title={t('Abuse Guard')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save abuse guard settings'
          />

          <div className='space-y-4'>
            <FormField
              control={form.control}
              name='abuse_guard.enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Enable abuse guard')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Master switch. When off, request behavior is unchanged.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
            <FormField
              control={form.control}
              name='abuse_guard.monitor_only'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Monitor only')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Record violations without blocking or banning. Use to evaluate false positives before enforcing.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
          </div>

          {listField(
            'abuse_guard.model_scope_patterns',
            'Inspected model patterns',
            'Only requests whose model name matches one of these prefixes are inspected. Use a trailing * for prefix match. One per line.',
            'claude*\ngpt*\no1*'
          )}
          {listField(
            'abuse_guard.exempt_groups',
            'Exempt groups',
            'Users in these groups skip all detection and review. One group per line. Admin roles are always exempt.',
            'vip'
          )}
          {listField(
            'abuse_guard.block_words',
            'Hard block words',
            'Requests containing any of these words are blocked immediately. One per line.',
            'one keyword per line'
          )}

          <SettingsFormGrid>
            {numberField(
              'abuse_guard.pattern_block_score',
              'Pattern block score',
              'Combined jailbreak-pattern weight at which a request is blocked.'
            )}
            {numberField(
              'abuse_guard.scan_window_kb',
              'Scan window (KB)',
              'Head/tail window per side used to bound scan time on long prompts.'
            )}
          </SettingsFormGrid>

          {listField(
            'abuse_guard.disabled_builtin_ids',
            'Disabled builtin pattern IDs',
            'Builtin jailbreak pattern IDs to disable. One per line.',
            'per_jailbreak'
          )}
          {listField(
            'abuse_guard.custom_patterns',
            'Custom patterns (JSON)',
            'Advanced. JSON array of {id, kind: keyword|regex, pattern, weight}.',
            '[{"id":"c1","kind":"regex","pattern":"...","weight":4}]'
          )}

          <SettingsFormGrid>
            {textField(
              'abuse_guard.moderation_api_key',
              'Moderation API key',
              'OpenAI-compatible key used for async semantic review. Leave blank to disable review.'
            )}
            {textField(
              'abuse_guard.moderation_base_url',
              'Moderation base URL'
            )}
            {textField('abuse_guard.moderation_model', 'Moderation model')}
            {numberField(
              'abuse_guard.sample_rate_percent',
              'Sample rate (%)',
              'Percentage of non-first requests sent for async review.'
            )}
            {numberField(
              'abuse_guard.review_snippet_kb',
              'Review snippet (KB)'
            )}
            {numberField('abuse_guard.queue_size', 'Review queue size')}
            {numberField('abuse_guard.worker_count', 'Review worker count')}
          </SettingsFormGrid>

          {listField(
            'abuse_guard.instant_ban_categories',
            'Instant-ban categories',
            'Moderation categories that trigger a temporary ban on a single hit. One per line.',
            'sexual/minors'
          )}
          {listField(
            'abuse_guard.category_scores',
            'Category scores (JSON)',
            'Advanced. JSON object mapping moderation category to score, e.g. {"violence":2}.',
            '{"violence":2}'
          )}

          <SettingsFormGrid>
            {numberField(
              'abuse_guard.score_window_hours',
              'Score window (hours)'
            )}
            {numberField(
              'abuse_guard.ban_threshold',
              'Ban threshold',
              'Accumulated score within the window that triggers a temporary ban.'
            )}
            {numberField(
              'abuse_guard.temp_ban_hours',
              'Temporary ban duration (hours)'
            )}
            {numberField(
              'abuse_guard.perm_ban_after_temp_bans',
              'Permanent ban after N temp bans'
            )}
          </SettingsFormGrid>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
