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
import { useQuery } from '@tanstack/react-query'
import { Copy, ExternalLink, Info } from 'lucide-react'
import { useState, useEffect, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { ComboboxInput } from '@/components/ui/combobox-input'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { useStatus } from '@/hooks/use-status'
import { getUserModels } from '@/lib/api'

const APP_CONFIGS = {
  claude: {
    label: 'Claude',
    defaultName: 'My Claude',
    modelFields: [
      { key: 'model', labelKey: 'Primary Model', required: true },
      { key: 'haikuModel', labelKey: 'Haiku Model', required: false },
      { key: 'sonnetModel', labelKey: 'Sonnet Model', required: false },
      { key: 'opusModel', labelKey: 'Opus Model', required: false },
    ],
  },
  codex: {
    label: 'Codex',
    defaultName: 'My Codex',
    modelFields: [{ key: 'model', labelKey: 'Primary Model', required: true }],
  },
  gemini: {
    label: 'Gemini',
    defaultName: 'My Gemini',
    modelFields: [{ key: 'model', labelKey: 'Primary Model', required: true }],
  },
} as const

type AppType = keyof typeof APP_CONFIGS

const APP_MODEL_MATCHERS: Record<AppType, string[]> = {
  claude: ['claude'],
  codex: ['codex', 'gpt', 'o1', 'o3', 'o4'],
  gemini: ['gemini'],
}

const APP_MODEL_PRIORITIES: Record<AppType, string[]> = {
  claude: ['sonnet', 'claude', 'opus', 'haiku'],
  codex: [
    'gpt-5-codex',
    'codex',
    'gpt-5',
    'o4-mini',
    'o3',
    'gpt-4.1',
    'gpt-4o',
    'gpt',
  ],
  gemini: ['gemini-2.5-pro', 'gemini-2.5-flash', 'gemini'],
}

function getStatusString(status: unknown, key: string): string {
  if (!status || typeof status !== 'object') return ''
  const record = status as Record<string, unknown>
  const value = record[key]
  if (typeof value === 'string') return value.trim()
  const data = record.data
  if (data && typeof data === 'object') {
    const nestedValue = (data as Record<string, unknown>)[key]
    if (typeof nestedValue === 'string') return nestedValue.trim()
  }
  return ''
}

function getStoredStatusString(key: string): string {
  try {
    const raw = localStorage.getItem('status')
    if (!raw) return ''
    return getStatusString(JSON.parse(raw), key)
  } catch {
    return ''
  }
}

function getServerAddress(status: unknown): string {
  const fromStatus =
    getStatusString(status, 'server_address') ||
    getStatusString(status, 'serverAddress')
  const fromStorage =
    getStoredStatusString('server_address') ||
    getStoredStatusString('serverAddress')
  const serverAddress = fromStatus || fromStorage || window.location.origin
  return serverAddress.replace(/\/+$/, '')
}

function getSiteName(status: unknown): string {
  const fromStatus =
    getStatusString(status, 'system_name') ||
    getStatusString(status, 'systemName')
  const fromStorage =
    getStoredStatusString('system_name') || getStoredStatusString('systemName')
  const siteName = fromStatus || fromStorage || document.title || 'fkcodex'
  return siteName.trim().toLowerCase() || 'fkcodex'
}

function getEndpoint(app: AppType, serverAddress: string): string {
  return app === 'codex' ? `${serverAddress}/v1` : serverAddress
}

function normalizeModels(models: string[]): string[] {
  const seen = new Set<string>()
  const normalized: string[] = []
  for (const model of models) {
    const value = model.trim()
    if (!value || seen.has(value)) continue
    seen.add(value)
    normalized.push(value)
  }
  return normalized
}

function modelMatches(model: string, terms: string[]): boolean {
  const normalized = model.toLowerCase()
  return terms.some((term) => normalized.includes(term))
}

function getAppModelCandidates(app: AppType, models: string[]): string[] {
  const normalized = normalizeModels(models)
  const matched = normalized.filter((model) =>
    modelMatches(model, APP_MODEL_MATCHERS[app])
  )
  return matched.length > 0 ? matched : normalized
}

function pickPreferredModel(models: string[], terms: string[]): string {
  for (const term of terms) {
    const found = models.find((model) => model.toLowerCase().includes(term))
    if (found) return found
  }
  return models[0] ?? ''
}

function getDefaultModels(
  app: AppType,
  models: string[]
): Record<string, string> {
  const candidates = getAppModelCandidates(app, models)
  if (candidates.length === 0) return {}
  if (app !== 'claude') {
    return { model: pickPreferredModel(candidates, APP_MODEL_PRIORITIES[app]) }
  }
  const sonnetModel = pickPreferredModel(
    candidates.filter((model) => modelMatches(model, ['sonnet'])),
    ['sonnet']
  )
  const opusModel = pickPreferredModel(
    candidates.filter((model) => modelMatches(model, ['opus'])),
    ['opus']
  )
  const haikuModel = pickPreferredModel(
    candidates.filter((model) => modelMatches(model, ['haiku'])),
    ['haiku']
  )
  return {
    model:
      sonnetModel ||
      opusModel ||
      haikuModel ||
      pickPreferredModel(candidates, APP_MODEL_PRIORITIES.claude),
    haikuModel,
    sonnetModel,
    opusModel,
  }
}

function getFieldModels(
  app: AppType,
  fieldKey: string,
  models: string[]
): string[] {
  const candidates = getAppModelCandidates(app, models)
  if (app !== 'claude') return candidates
  const fieldMatchers: Record<string, string[]> = {
    haikuModel: ['haiku'],
    opusModel: ['opus'],
    sonnetModel: ['sonnet'],
  }
  const terms = fieldMatchers[fieldKey]
  if (!terms) return candidates
  const matched = candidates.filter((model) => modelMatches(model, terms))
  return matched.length > 0 ? matched : candidates
}

function toModelOptions(models: string[]) {
  return models.map((model) => ({ value: model, label: model }))
}

const BALANCE_QUERY_AUTO_INTERVAL_MINUTES = 30

const CCSWITCH_BALANCE_USAGE_SCRIPT = `({
  request: {
    url: '{{baseUrl}}/api/usage/balance',
    method: 'GET',
    headers: {
      Authorization: 'Bearer {{apiKey}}',
    },
  },
  extractor: function (response) {
    if (!response || response.success === false) {
      throw new Error((response && response.message) || 'Balance query failed')
    }
    var data = response.data || response
    var display = data.display || {}
    var expiresAt = Number(data.expires_at || 0)
    var now = Math.floor(Date.now() / 1000)
    var expired = expiresAt > 0 && expiresAt < now
    var unlimited = data.unlimited_quota === true
    return {
      planName: data.token_name || 'Account',
      total: Number(display.total || 0),
      used: Number(display.used || 0),
      remaining: Number(display.remaining || 0),
      unit: display.unit || '',
      isValid: !expired,
      invalidMessage: expired ? 'API key expired' : undefined,
      extra: unlimited ? 'Unlimited quota' : data.scope === 'account' ? 'Account balance' : 'API key balance',
    }
  },
})`

function encodeBase64Utf8(value: string): string {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return window.btoa(binary)
}

function buildCCSwitchURL(params: {
  app: AppType
  name: string
  models: Record<string, string>
  apiKey: string
  serverAddress: string
}): string {
  const endpoint = getEndpoint(params.app, params.serverAddress)
  const searchParams = new URLSearchParams()
  searchParams.set('resource', 'provider')
  searchParams.set('app', params.app)
  searchParams.set('name', params.name)
  searchParams.set('endpoint', endpoint)
  searchParams.set('apiKey', params.apiKey)
  for (const [k, v] of Object.entries(params.models)) {
    if (v) searchParams.set(k, v)
  }
  searchParams.set('homepage', params.serverAddress)
  searchParams.set(
    'notes',
    'Imported from API gateway with automatic balance query.'
  )
  searchParams.set('enabled', 'true')
  searchParams.set('usageEnabled', 'true')
  searchParams.set(
    'usageScript',
    encodeBase64Utf8(CCSWITCH_BALANCE_USAGE_SCRIPT)
  )
  searchParams.set('usageBaseUrl', params.serverAddress)
  searchParams.set(
    'usageAutoInterval',
    String(BALANCE_QUERY_AUTO_INTERVAL_MINUTES)
  )
  return `ccswitch://v1/import?${searchParams.toString()}`
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  tokenKey: string
}

export function CCSwitchDialog(props: Props) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const [app, setApp] = useState<AppType>('claude')
  const [name, setName] = useState<string>('fkcodex')
  const [models, setModels] = useState<Record<string, string>>({})
  const wasOpenRef = useRef(false)
  const pendingResetAppRef = useRef<AppType | null>(null)

  const siteName = useMemo(() => getSiteName(status), [status])
  const serverAddress = useMemo(() => getServerAddress(status), [status])
  const endpoint = useMemo(
    () => getEndpoint(app, serverAddress),
    [app, serverAddress]
  )

  const { data: modelsData } = useQuery({
    queryKey: ['user-models-ccswitch'],
    queryFn: getUserModels,
    enabled: props.open,
    staleTime: 5 * 60 * 1000,
  })

  const availableModels = useMemo(
    () => modelsData?.data ?? [],
    [modelsData?.data]
  )
  const currentConfig = APP_CONFIGS[app]
  const modelOptionsByField = useMemo(() => {
    const options: Record<string, { value: string; label: string }[]> = {}
    for (const field of currentConfig.modelFields) {
      options[field.key] = toModelOptions(
        getFieldModels(app, field.key, availableModels)
      )
    }
    return options
  }, [app, availableModels, currentConfig.modelFields])

  useEffect(() => {
    if (!props.open) {
      wasOpenRef.current = false
      pendingResetAppRef.current = null
      return
    }
    if (wasOpenRef.current) return
    wasOpenRef.current = true
    pendingResetAppRef.current = 'claude'

    // eslint-disable-next-line react-hooks/set-state-in-effect
    setModels({})

    setApp('claude')

    setName(siteName)
  }, [props.open, siteName])

  useEffect(() => {
    if (!props.open) return
    const pendingApp = pendingResetAppRef.current
    if (pendingApp && app !== pendingApp) return
    const defaults = getDefaultModels(app, availableModels)
    if (Object.keys(defaults).length === 0) return
    setModels((prev) => {
      let changed = false
      const next = { ...prev }
      for (const field of APP_CONFIGS[app].modelFields) {
        const value = defaults[field.key]
        if (!value || next[field.key]) continue
        next[field.key] = value
        changed = true
      }
      return changed ? next : prev
    })
    pendingResetAppRef.current = null
  }, [app, availableModels, props.open])

  const handleAppChange = (val: string) => {
    const appVal = val as AppType
    pendingResetAppRef.current = null
    setApp(appVal)
    setName(siteName)
    setModels(getDefaultModels(appVal, availableModels))
  }

  const buildImportURL = () => {
    if (!models.model) {
      toast.warning(t('Please select a primary model'))
      return null
    }
    const key = props.tokenKey.startsWith('sk-')
      ? props.tokenKey
      : `sk-${props.tokenKey}`
    return buildCCSwitchURL({
      apiKey: key,
      app,
      models,
      name: name.trim() || siteName,
      serverAddress,
    })
  }

  const handleCopyLink = async () => {
    const url = buildImportURL()
    if (!url) return

    try {
      await navigator.clipboard.writeText(url)
      toast.success(t('CC Switch import link copied'))
    } catch {
      toast.error(t('Failed to copy import link'))
    }
  }

  const handleSubmit = () => {
    const url = buildImportURL()
    if (!url) return

    window.open(url, '_blank')
    props.onOpenChange(false)
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Import to CC Switch')}
      contentClassName='sm:max-w-2xl'
      contentHeight='auto'
      bodyClassName={
        currentConfig.modelFields.length === 1 ? 'space-y-4 pb-52' : 'space-y-4'
      }
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button variant='outline' onClick={handleCopyLink}>
            <Copy className='size-4' />
            {t('Copy import link')}
          </Button>
          <Button onClick={handleSubmit}>
            <ExternalLink className='size-4' />
            {t('Open CC Switch')}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        <div className='bg-muted/40 text-muted-foreground flex gap-2 rounded-xl border p-3 text-xs leading-5'>
          <Info className='text-primary mt-0.5 size-4 shrink-0' />
          <div className='space-y-1'>
            <p>
              {t(
                'This deep link imports the selected API key into ccswitch, writes Claude Code or Codex settings, and enables automatic balance viewing.'
              )}
            </p>
            <p>
              {t(
                'If your browser blocks the external link, copy the import link and open it after ccswitch is running.'
              )}
            </p>
          </div>
        </div>
        <div className='space-y-2'>
          <Label>{t('Application')}</Label>
          <RadioGroup
            value={app}
            onValueChange={handleAppChange}
            className='flex gap-4'
          >
            {(
              Object.entries(APP_CONFIGS) as [
                AppType,
                (typeof APP_CONFIGS)[AppType],
              ][]
            ).map(([key, cfg]) => (
              <div key={key} className='flex items-center gap-2'>
                <RadioGroupItem value={key} id={`app-${key}`} />
                <Label htmlFor={`app-${key}`} className='cursor-pointer'>
                  {cfg.label}
                </Label>
              </div>
            ))}
          </RadioGroup>
        </div>

        <div className='grid gap-4 sm:grid-cols-2'>
          <div className='space-y-2'>
            <Label>{t('Name')}</Label>
            <ComboboxInput
              options={[]}
              value={name}
              onValueChange={setName}
              placeholder={siteName}
              emptyText=''
              allowCustomValue
            />
          </div>

          <div className='space-y-2'>
            <Label>{t('Base URL')}</Label>
            <Input value={endpoint} readOnly className='font-mono text-xs' />
          </div>
        </div>

        {currentConfig.modelFields.map((field) => (
          <div key={field.key} className='space-y-2'>
            <Label>
              {t(field.labelKey)}
              {field.required && (
                <span className='text-destructive ml-0.5'>*</span>
              )}
            </Label>
            <ComboboxInput
              options={modelOptionsByField[field.key] ?? []}
              value={models[field.key] || ''}
              onValueChange={(v) =>
                setModels((prev) => ({ ...prev, [field.key]: v }))
              }
              placeholder={t('Select or enter model name')}
              emptyText={t('No models found')}
              allowCustomValue
            />
          </div>
        ))}
      </div>
    </Dialog>
  )
}
