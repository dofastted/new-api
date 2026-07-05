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
import { useEffect, useRef } from 'react'
import { useForm, type Path } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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

type AbuseGuardFormValues = {
  abuse_guard: {
    enabled: boolean
    monitor_only: boolean
    model_scope_patterns: string
    exempt_groups: string
    block_words: string
    disabled_builtin_ids: string
    custom_patterns: string
    pattern_block_score: number
    scan_window_kb: number
    moderation_api_key: string
    moderation_base_url: string
    moderation_model: string
    sample_rate_percent: number
    review_snippet_kb: number
    queue_size: number
    worker_count: number
    category_scores: string
    instant_ban_categories: string
    score_window_hours: number
    ban_threshold: number
    temp_ban_hours: number
    perm_ban_after_temp_bans: number
  }
}

function linesToArray(text: string): string[] {
  return text
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l.length > 0)
}

function normalizeJsonText(value: string, emptyValue: '[]' | '{}'): string {
  const trimmed = value.trim()
  return trimmed === '' ? emptyValue : trimmed
}

function buildFormDefaults(v: AbuseGuardValues): AbuseGuardFormValues {
  return {
    abuse_guard: {
      enabled: v['abuse_guard.enabled'],
      monitor_only: v['abuse_guard.monitor_only'],
      model_scope_patterns: (
        v['abuse_guard.model_scope_patterns'] ?? []
      ).join('\n'),
      exempt_groups: (v['abuse_guard.exempt_groups'] ?? []).join('\n'),
      block_words: (v['abuse_guard.block_words'] ?? []).join('\n'),
      disabled_builtin_ids: (
        v['abuse_guard.disabled_builtin_ids'] ?? []
      ).join('\n'),
      custom_patterns: normalizeJsonText(
        v['abuse_guard.custom_patterns'] ?? '',
        '[]'
      ),
      pattern_block_score: v['abuse_guard.pattern_block_score'],
      scan_window_kb: v['abuse_guard.scan_window_kb'],
      moderation_api_key: v['abuse_guard.moderation_api_key'],
      moderation_base_url: v['abuse_guard.moderation_base_url'],
      moderation_model: v['abuse_guard.moderation_model'],
      sample_rate_percent: v['abuse_guard.sample_rate_percent'],
      review_snippet_kb: v['abuse_guard.review_snippet_kb'],
      queue_size: v['abuse_guard.queue_size'],
      worker_count: v['abuse_guard.worker_count'],
      category_scores: normalizeJsonText(
        v['abuse_guard.category_scores'] ?? '',
        '{}'
      ),
      instant_ban_categories: (
        v['abuse_guard.instant_ban_categories'] ?? []
      ).join('\n'),
      score_window_hours: v['abuse_guard.score_window_hours'],
      ban_threshold: v['abuse_guard.ban_threshold'],
      temp_ban_hours: v['abuse_guard.temp_ban_hours'],
      perm_ban_after_temp_bans: v['abuse_guard.perm_ban_after_temp_bans'],
    },
  }
}

function normalizeFormValues(values: AbuseGuardFormValues): AbuseGuardValues {
  return {
    'abuse_guard.enabled': values.abuse_guard.enabled,
    'abuse_guard.monitor_only': values.abuse_guard.monitor_only,
    'abuse_guard.model_scope_patterns': linesToArray(
      values.abuse_guard.model_scope_patterns
    ),
    'abuse_guard.exempt_groups': linesToArray(values.abuse_guard.exempt_groups),
    'abuse_guard.block_words': linesToArray(values.abuse_guard.block_words),
    'abuse_guard.disabled_builtin_ids': linesToArray(
      values.abuse_guard.disabled_builtin_ids
    ),
    'abuse_guard.custom_patterns': normalizeJsonText(
      values.abuse_guard.custom_patterns,
      '[]'
    ),
    'abuse_guard.pattern_block_score':
      values.abuse_guard.pattern_block_score,
    'abuse_guard.scan_window_kb': values.abuse_guard.scan_window_kb,
    'abuse_guard.moderation_api_key':
      values.abuse_guard.moderation_api_key,
    'abuse_guard.moderation_base_url':
      values.abuse_guard.moderation_base_url,
    'abuse_guard.moderation_model': values.abuse_guard.moderation_model,
    'abuse_guard.sample_rate_percent':
      values.abuse_guard.sample_rate_percent,
    'abuse_guard.review_snippet_kb': values.abuse_guard.review_snippet_kb,
    'abuse_guard.queue_size': values.abuse_guard.queue_size,
    'abuse_guard.worker_count': values.abuse_guard.worker_count,
    'abuse_guard.category_scores': normalizeJsonText(
      values.abuse_guard.category_scores,
      '{}'
    ),
    'abuse_guard.instant_ban_categories': linesToArray(
      values.abuse_guard.instant_ban_categories
    ),
    'abuse_guard.score_window_hours': values.abuse_guard.score_window_hours,
    'abuse_guard.ban_threshold': values.abuse_guard.ban_threshold,
    'abuse_guard.temp_ban_hours': values.abuse_guard.temp_ban_hours,
    'abuse_guard.perm_ban_after_temp_bans':
      values.abuse_guard.perm_ban_after_temp_bans,
  }
}

function isEqual(a: unknown, b: unknown): boolean {
  if (Array.isArray(a) && Array.isArray(b)) {
    return JSON.stringify(a) === JSON.stringify(b)
  }
  return a === b
}

export function AbuseGuardSection({ defaultValues }: AbuseGuardSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const baselineRef = useRef<AbuseGuardValues>(defaultValues)
  const form = useForm<AbuseGuardFormValues>({
    defaultValues: buildFormDefaults(defaultValues),
  })

  useEffect(() => {
    baselineRef.current = defaultValues
    form.reset(buildFormDefaults(defaultValues))
  }, [defaultValues, form])

  const onSubmit = async (values: AbuseGuardFormValues) => {
    const normalized = normalizeFormValues(values)
    const updates = (
      Object.keys(normalized) as Array<keyof AbuseGuardValues>
    ).filter((key) => !isEqual(normalized[key], baselineRef.current[key]))

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const key of updates) {
      const value = normalized[key]
      await updateOption.mutateAsync({
        key,
        value: Array.isArray(value) ? JSON.stringify(value) : value,
      })
    }

    baselineRef.current = normalized
    form.reset(buildFormDefaults(normalized))
  }

  const numberField = (
    name: Path<AbuseGuardFormValues>,
    labelText: string,
    descriptionText?: string
  ) => (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <SettingsFormGridItem>
          <FormItem>
            <FormLabel>{labelText}</FormLabel>
            <FormControl>
              <Input
                type='number'
                value={field.value as number}
                onChange={(e) => field.onChange(Number(e.target.value))}
              />
            </FormControl>
            {descriptionText ? (
              <FormDescription>{descriptionText}</FormDescription>
            ) : null}
            <FormMessage />
          </FormItem>
        </SettingsFormGridItem>
      )}
    />
  )

  const textField = (
    name: Path<AbuseGuardFormValues>,
    labelText: string,
    descriptionText?: string
  ) => (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <SettingsFormGridItem>
          <FormItem>
            <FormLabel>{labelText}</FormLabel>
            <FormControl>
              <Input {...field} value={field.value as string} />
            </FormControl>
            {descriptionText ? (
              <FormDescription>{descriptionText}</FormDescription>
            ) : null}
            <FormMessage />
          </FormItem>
        </SettingsFormGridItem>
      )}
    />
  )

  const textareaField = (
    name: Path<AbuseGuardFormValues>,
    labelText: string,
    descriptionText: string,
    placeholderText: string
  ) => (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{labelText}</FormLabel>
          <FormControl>
            <Textarea
              rows={6}
              placeholder={placeholderText}
              {...field}
              value={field.value as string}
            />
          </FormControl>
          <FormDescription>{descriptionText}</FormDescription>
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

          {textareaField(
            'abuse_guard.model_scope_patterns',
            t('Inspected model patterns'),
            t(
              'Only requests whose model name matches one of these prefixes are inspected. Use a trailing * for prefix match. One per line.'
            ),
            'claude*\ngpt*\no1*'
          )}
          {textareaField(
            'abuse_guard.exempt_groups',
            t('Exempt groups'),
            t(
              'Users in these groups skip all detection and review. One group per line. Admin roles are always exempt.'
            ),
            'vip'
          )}
          {textareaField(
            'abuse_guard.block_words',
            t('Hard block words'),
            t(
              'Requests containing any of these words are blocked immediately. One per line.'
            ),
            t('one keyword per line')
          )}

          <SettingsFormGrid>
            {numberField(
              'abuse_guard.pattern_block_score',
              t('Pattern block score'),
              t(
                'Combined jailbreak-pattern weight at which a request is blocked.'
              )
            )}
            {numberField(
              'abuse_guard.scan_window_kb',
              t('Scan window (KB)'),
              t(
                'Head/tail window per side used to bound scan time on long prompts.'
              )
            )}
          </SettingsFormGrid>

          {textareaField(
            'abuse_guard.disabled_builtin_ids',
            t('Disabled builtin pattern IDs'),
            t('Builtin jailbreak pattern IDs to disable. One per line.'),
            'per_jailbreak'
          )}
          {textareaField(
            'abuse_guard.custom_patterns',
            t('Custom patterns (JSON)'),
            t(
              'Advanced. JSON array of {id, kind: keyword|regex, pattern, weight}.'
            ),
            '[{"id":"c1","kind":"regex","pattern":"...","weight":4}]'
          )}

          <SettingsFormGrid>
            {textField(
              'abuse_guard.moderation_api_key',
              t('Moderation API key'),
              t(
                'OpenAI-compatible key used for async semantic review. Leave blank to disable review.'
              )
            )}
            {textField(
              'abuse_guard.moderation_base_url',
              t('Moderation base URL')
            )}
            {textField('abuse_guard.moderation_model', t('Moderation model'))}
            {numberField(
              'abuse_guard.sample_rate_percent',
              t('Sample rate (%)'),
              t('Percentage of non-first requests sent for async review.')
            )}
            {numberField(
              'abuse_guard.review_snippet_kb',
              t('Review snippet (KB)')
            )}
            {numberField('abuse_guard.queue_size', t('Review queue size'))}
            {numberField('abuse_guard.worker_count', t('Review worker count'))}
          </SettingsFormGrid>

          {textareaField(
            'abuse_guard.instant_ban_categories',
            t('Instant-ban categories'),
            t(
              'Moderation categories that trigger a temporary ban on a single hit. One per line.'
            ),
            'sexual/minors'
          )}
          {textareaField(
            'abuse_guard.category_scores',
            t('Category scores (JSON)'),
            t(
              'Advanced. JSON object mapping moderation category to score, e.g. {"violence":2}.'
            ),
            '{"violence":2}'
          )}

          <SettingsFormGrid>
            {numberField(
              'abuse_guard.score_window_hours',
              t('Score window (hours)')
            )}
            {numberField(
              'abuse_guard.ban_threshold',
              t('Ban threshold'),
              t(
                'Accumulated score within the window that triggers a temporary ban.'
              )
            )}
            {numberField(
              'abuse_guard.temp_ban_hours',
              t('Temporary ban duration (hours)')
            )}
            {numberField(
              'abuse_guard.perm_ban_after_temp_bans',
              t('Permanent ban after N temp bans')
            )}
          </SettingsFormGrid>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
