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
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { parseHttpStatusCodeRules } from '@/lib/http-status-code-rules'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

let nextErrorRewriteRuleId = 0

type ErrorRewriteRule = {
  name: string
  status_codes: string
  keywords: string[]
  message: string
  status_code: number
  enabled: boolean
}

type ErrorRewriteRuleForm = {
  id: string
  name: string
  statusCodes: string
  keywordsText: string
  message: string
  statusCode: number
  enabled: boolean
}

type ErrorRewriteSectionProps = {
  defaultValues: {
    'error_rewrite.enabled': boolean
    'error_rewrite.rules': string
  }
}

const DEFAULT_ERROR_REWRITE_RULES: ErrorRewriteRule[] = [
  {
    name: 'quota-limited-429',
    status_codes: '429',
    keywords: ['exceeded your current quota', 'credit balance'],
    message: 'The service is temporarily unavailable. Please try again later.',
    status_code: 503,
    enabled: true,
  },
  {
    name: 'generic-429',
    status_codes: '429',
    keywords: [],
    message: "We're experiencing high demand right now. Please retry in a moment.",
    status_code: 429,
    enabled: true,
  },
  {
    name: 'account-or-auth-unavailable',
    status_codes: '',
    keywords: [
      'no available accounts',
      'auth_unavailable',
      'authentication token',
      'signing in again',
    ],
    message: 'The service is temporarily unavailable. Please try again later.',
    status_code: 503,
    enabled: true,
  },
  {
    name: 'claude-cli-required',
    status_codes: '',
    keywords: ['official Claude CLI', 'only accessible via the official Claude CLI'],
    message:
      'This model requires the official Claude Code CLI. Please use Claude Code and retry.',
    status_code: 403,
    enabled: true,
  },
  {
    name: 'codex-cli-required',
    status_codes: '',
    keywords: ['codex 客户端限制', '请使用 /v1/responses'],
    message:
      'This model requires the official Codex CLI. Please use Codex (/v1/responses) and retry.',
    status_code: 403,
    enabled: true,
  },
  {
    name: 'permission-unavailable',
    status_codes: '403,502',
    keywords: ['access forbidden', 'permission denied'],
    message: 'The service is temporarily unavailable. Please try again later.',
    status_code: 503,
    enabled: true,
  },
  {
    name: 'temporary-upstream-issue',
    status_codes: '500,502,503,504,524',
    keywords: [],
    message: 'The service encountered a temporary issue. Please retry later.',
    status_code: 0,
    enabled: true,
  },
]

function ruleToForm(rule: ErrorRewriteRule): ErrorRewriteRuleForm {
  const keywordsText = Array.isArray(rule.keywords) ? rule.keywords.join('\n') : ''
  const id = `error-rewrite-rule-${nextErrorRewriteRuleId}`
  nextErrorRewriteRuleId += 1
  return {
    id,
    name: rule.name ?? '',
    statusCodes: rule.status_codes ?? '',
    keywordsText,
    message: rule.message ?? '',
    statusCode: Number(rule.status_code) || 0,
    enabled: rule.enabled !== false,
  }
}

function formToRule(rule: ErrorRewriteRuleForm): ErrorRewriteRule {
  const statusCode = Number(rule.statusCode) || 0
  return {
    name: rule.name.trim(),
    status_codes: parseHttpStatusCodeRules(rule.statusCodes).normalized,
    keywords: rule.keywordsText
      .split('\n')
      .map((keyword) => keyword.trim())
      .filter(Boolean),
    message: rule.message.trim(),
    status_code: statusCode,
    enabled: rule.enabled,
  }
}

function parseRules(value: string): ErrorRewriteRuleForm[] {
  try {
    const parsed = JSON.parse(value)
    if (!Array.isArray(parsed)) return DEFAULT_ERROR_REWRITE_RULES.map(ruleToForm)
    return parsed.map((rule) => ruleToForm(rule as ErrorRewriteRule))
  } catch {
    return DEFAULT_ERROR_REWRITE_RULES.map(ruleToForm)
  }
}

function serializeRules(rules: ErrorRewriteRuleForm[]): string {
  const normalized = rules.map(formToRule)
  return JSON.stringify(normalized)
}

export function ErrorRewriteSection(props: ErrorRewriteSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [enabled, setEnabled] = useState(
    props.defaultValues['error_rewrite.enabled']
  )
  const [rules, setRules] = useState<ErrorRewriteRuleForm[]>(() =>
    parseRules(props.defaultValues['error_rewrite.rules'])
  )
  const baselineRef = useRef({
    enabled: props.defaultValues['error_rewrite.enabled'],
    rules: serializeRules(parseRules(props.defaultValues['error_rewrite.rules'])),
  })

  useEffect(() => {
    const nextRules = parseRules(props.defaultValues['error_rewrite.rules'])
    setEnabled(props.defaultValues['error_rewrite.enabled'])
    setRules(nextRules)
    baselineRef.current = {
      enabled: props.defaultValues['error_rewrite.enabled'],
      rules: serializeRules(nextRules),
    }
  }, [props.defaultValues])

  const validationMessage = useMemo(() => {
    for (const [index, rule] of rules.entries()) {
      const displayName = rule.name.trim() || `#${index + 1}`
      if (!rule.name.trim()) return t('Rule name is required')
      if (!rule.message.trim()) return t('Rewrite message is required')
      const parsed = parseHttpStatusCodeRules(rule.statusCodes)
      if (!parsed.ok) {
        return t('Invalid status code rules in rule {{name}}: {{tokens}}', {
          name: displayName,
          tokens: parsed.invalidTokens.join(', '),
        })
      }
      const statusCode = Number(rule.statusCode) || 0
      if (statusCode !== 0 && (statusCode < 100 || statusCode > 599)) {
        return t('Override status code must be 0 or a valid HTTP status code')
      }
    }
    return ''
  }, [rules, t])

  const normalizedRules = useMemo(() => serializeRules(rules), [rules])
  const hasChanges =
    enabled !== baselineRef.current.enabled ||
    normalizedRules !== baselineRef.current.rules

  const updateRule = (
    index: number,
    patch: Partial<ErrorRewriteRuleForm>
  ): void => {
    setRules((current) =>
      current.map((rule, ruleIndex) =>
        ruleIndex === index ? { ...rule, ...patch } : rule
      )
    )
  }

  const handleAddRule = (): void => {
    const id = `error-rewrite-rule-${nextErrorRewriteRuleId}`
    nextErrorRewriteRuleId += 1
    setRules((current) => [
      ...current,
      {
        id,
        name: 'custom-rule',
        statusCodes: '',
        keywordsText: '',
        message: "We're experiencing high demand right now. Please retry in a moment.",
        statusCode: 0,
        enabled: true,
      },
    ])
  }

  const handleRemoveRule = (index: number): void => {
    setRules((current) => current.filter((_, ruleIndex) => ruleIndex !== index))
  }

  const handleRestoreDefaults = (): void => {
    setEnabled(true)
    setRules(DEFAULT_ERROR_REWRITE_RULES.map(ruleToForm))
  }

  const handleSave = async (): Promise<void> => {
    if (validationMessage) {
      toast.error(validationMessage)
      return
    }
    if (!hasChanges) {
      toast.info(t('No changes to save'))
      return
    }
    if (enabled !== baselineRef.current.enabled) {
      await updateOption.mutateAsync({
        key: 'error_rewrite.enabled',
        value: enabled,
      })
    }
    if (normalizedRules !== baselineRef.current.rules) {
      await updateOption.mutateAsync({
        key: 'error_rewrite.rules',
        value: normalizedRules,
      })
    }
    baselineRef.current = { enabled, rules: normalizedRules }
  }

  return (
    <SettingsSection title={t('Error Rewrite')}>
      <SettingsForm
        onSubmit={(event) => {
          event.preventDefault()
          void handleSave()
        }}
      >
        <SettingsPageFormActions
          onSave={() => void handleSave()}
          onReset={handleRestoreDefaults}
          isSaving={updateOption.isPending}
          isSaveDisabled={Boolean(validationMessage)}
          isResetDisabled={updateOption.isPending}
          saveLabel='Save error rewrite rules'
          resetLabel='Restore defaults'
        />

        <SettingsSwitchItem>
          <SettingsSwitchContent>
            <Label>{t('Enable user-facing error rewrite')}</Label>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Rewrite upstream account, quota, rate limit, and temporary failure messages before they are returned to users.'
              )}
            </p>
          </SettingsSwitchContent>
          <Switch checked={enabled} onCheckedChange={setEnabled} />
        </SettingsSwitchItem>

        {validationMessage && (
          <p className='text-destructive text-sm'>{validationMessage}</p>
        )}

        <div className='space-y-4'>
          <div className='flex items-center justify-between gap-3'>
            <div>
              <h4 className='text-sm font-medium'>{t('Rewrite rules')}</h4>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Rules are evaluated from top to bottom. The first enabled match rewrites the visible message.'
                )}
              </p>
            </div>
            <Button type='button' variant='outline' size='sm' onClick={handleAddRule}>
              <Plus data-icon='inline-start' />
              {t('Add rule')}
            </Button>
          </div>

          {rules.map((rule, index) => (
            <div
              key={rule.id}
              className='border-border/60 space-y-4 rounded-lg border p-4'
            >
              <div className='flex items-center justify-between gap-3'>
                <div className='flex items-center gap-2'>
                  <Switch
                    checked={rule.enabled}
                    onCheckedChange={(checked) =>
                      updateRule(index, { enabled: checked })
                    }
                  />
                  <span className='text-sm font-medium'>
                    {rule.name || t('Unnamed rule')}
                  </span>
                </div>
                <Button
                  type='button'
                  variant='ghost'
                  size='sm'
                  onClick={() => handleRemoveRule(index)}
                >
                  <Trash2 data-icon='inline-start' />
                  {t('Remove')}
                </Button>
              </div>

              <div className='grid gap-4 lg:grid-cols-3'>
                <div className='space-y-2'>
                  <Label>{t('Rule name')}</Label>
                  <Input
                    value={rule.name}
                    onChange={(event) =>
                      updateRule(index, { name: event.target.value })
                    }
                  />
                </div>
                <div className='space-y-2'>
                  <Label>{t('Status codes')}</Label>
                  <Input
                    placeholder={t('e.g. 429, 500-599')}
                    value={rule.statusCodes}
                    onChange={(event) =>
                      updateRule(index, { statusCodes: event.target.value })
                    }
                  />
                </div>
                <div className='space-y-2'>
                  <Label>{t('Override status code')}</Label>
                  <Input
                    type='number'
                    min={0}
                    max={599}
                    step={1}
                    value={rule.statusCode}
                    onChange={(event) =>
                      updateRule(index, {
                        statusCode: Number.parseInt(event.target.value) || 0,
                      })
                    }
                  />
                </div>
              </div>

              <div className='grid gap-4 lg:grid-cols-2'>
                <div className='space-y-2'>
                  <Label>{t('Keywords')}</Label>
                  <Textarea
                    rows={4}
                    placeholder={t('one keyword per line')}
                    value={rule.keywordsText}
                    onChange={(event) =>
                      updateRule(index, { keywordsText: event.target.value })
                    }
                  />
                </div>
                <div className='space-y-2'>
                  <Label>{t('User-facing message')}</Label>
                  <Textarea
                    rows={4}
                    value={rule.message}
                    onChange={(event) =>
                      updateRule(index, { message: event.target.value })
                    }
                  />
                </div>
              </div>
            </div>
          ))}
        </div>
      </SettingsForm>
    </SettingsSection>
  )
}
