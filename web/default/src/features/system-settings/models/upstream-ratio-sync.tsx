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
import { CheckSquare, RefreshCcw } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'

import {
  fetchUpstreamRatios,
  getUpstreamChannels,
  saveModelPricing,
} from '../api'
import type {
  DifferencesMap,
  ModelPricingBatchRequest,
  ModelPricingConfig,
  RatioType,
  UpstreamChannel,
  UpstreamConfig,
} from '../types'
import { ChannelSelectorDialog } from './channel-selector-dialog'
import {
  ConflictConfirmDialog,
  type ConflictItem,
} from './conflict-confirm-dialog'
import {
  CLAUDE_OFFICIAL_CHANNEL_ENDPOINT,
  CLAUDE_OFFICIAL_CHANNEL_ID,
  DEEPSEEK_OFFICIAL_CHANNEL_ENDPOINT,
  DEEPSEEK_OFFICIAL_CHANNEL_ID,
  DEFAULT_ENDPOINT,
  GEMINI_OFFICIAL_CHANNEL_ENDPOINT,
  GEMINI_OFFICIAL_CHANNEL_ID,
  GLM_OFFICIAL_CHANNEL_ENDPOINT,
  GLM_OFFICIAL_CHANNEL_ID,
  OPENAI_OFFICIAL_CHANNEL_ENDPOINT,
  OPENAI_OFFICIAL_CHANNEL_ID,
  XAI_OFFICIAL_CHANNEL_ENDPOINT,
  XAI_OFFICIAL_CHANNEL_ID,
} from './constants'
import {
  RATIO_SYNC_FIELDS,
  getPreferredSyncField,
  type ResolutionsMap,
} from './upstream-ratio-sync-helpers'
import { UpstreamRatioSyncTable } from './upstream-ratio-sync-table'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type UpstreamRatioSyncProps = {
  modelRatios: {
    ModelPrice: string
    ModelRatio: string
    CompletionRatio: string
    CacheRatio: string
    CreateCacheRatio: string
    ImageRatio: string
    AudioRatio: string
    AudioCompletionRatio: string
    'billing_setting.billing_mode': string
    'billing_setting.billing_expr': string
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// Synthesized official presets always carry stable negative IDs assigned by
// `controller/ratio_sync.go`; matching by ID alone is sufficient and avoids
// fragile name/base_url comparisons.
function getDefaultEndpointForChannel(channel: UpstreamChannel): string {
  if (channel.id === OPENAI_OFFICIAL_CHANNEL_ID) {
    return OPENAI_OFFICIAL_CHANNEL_ENDPOINT
  }
  if (channel.id === CLAUDE_OFFICIAL_CHANNEL_ID) {
    return CLAUDE_OFFICIAL_CHANNEL_ENDPOINT
  }
  if (channel.id === XAI_OFFICIAL_CHANNEL_ID) {
    return XAI_OFFICIAL_CHANNEL_ENDPOINT
  }
  if (channel.id === GEMINI_OFFICIAL_CHANNEL_ID) {
    return GEMINI_OFFICIAL_CHANNEL_ENDPOINT
  }
  if (channel.id === GLM_OFFICIAL_CHANNEL_ID) {
    return GLM_OFFICIAL_CHANNEL_ENDPOINT
  }
  if (channel.id === DEEPSEEK_OFFICIAL_CHANNEL_ID) {
    return DEEPSEEK_OFFICIAL_CHANNEL_ENDPOINT
  }
  return DEFAULT_ENDPOINT
}

function getBillingCategory(ratioType: string): 'price' | 'ratio' | 'tiered' {
  if (ratioType === 'model_price') {
    return 'price'
  }
  if (ratioType === 'billing_mode' || ratioType === 'billing_expr') {
    return 'tiered'
  }
  return 'ratio'
}

function parseJsonRecord<T>(raw: string | undefined | null): Record<string, T> {
  try {
    return JSON.parse(raw || '{}') as Record<string, T>
  } catch {
    return {}
  }
}

function deleteResolutionField(
  res: ResolutionsMap,
  model: string,
  ratioType: string
): ResolutionsMap {
  if (!res[model]) return res
  const newModelRes = { ...res[model] }
  delete newModelRes[ratioType]
  if (ratioType === 'billing_expr') delete newModelRes['billing_mode']
  if (ratioType === 'billing_mode') delete newModelRes['billing_expr']
  const next = { ...res }
  if (Object.keys(newModelRes).length === 0) {
    delete next[model]
  } else {
    next[model] = newModelRes
  }
  return next
}

function buildPricingConfigPatch(
  ratios: Record<string, number | string>
): ModelPricingConfig {
  if (ratios.model_price !== undefined) {
    return { mode: 'per-request', price: Number(ratios.model_price) }
  }
  if (
    ratios.billing_mode === 'tiered_expr' ||
    ratios.billing_expr !== undefined
  ) {
    return {
      mode: 'tiered_expr',
      billing_expr: String(ratios.billing_expr ?? ''),
    }
  }
  return {
    mode: 'per-token',
    ratio:
      ratios.model_ratio === undefined ? undefined : Number(ratios.model_ratio),
    completion_ratio:
      ratios.completion_ratio === undefined
        ? undefined
        : Number(ratios.completion_ratio),
    cache_ratio:
      ratios.cache_ratio === undefined ? undefined : Number(ratios.cache_ratio),
    create_cache_ratio:
      ratios.create_cache_ratio === undefined
        ? undefined
        : Number(ratios.create_cache_ratio),
    image_ratio:
      ratios.image_ratio === undefined ? undefined : Number(ratios.image_ratio),
    audio_ratio:
      ratios.audio_ratio === undefined ? undefined : Number(ratios.audio_ratio),
    audio_completion_ratio:
      ratios.audio_completion_ratio === undefined
        ? undefined
        : Number(ratios.audio_completion_ratio),
  }
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function UpstreamRatioSync({ modelRatios }: UpstreamRatioSyncProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [channelDialogOpen, setChannelDialogOpen] = useState(false)
  const [conflictDialogOpen, setConflictDialogOpen] = useState(false)
  const [selectedChannelIds, setSelectedChannelIds] = useState<number[]>([])
  const [channelEndpoints, setChannelEndpoints] = useState<
    Record<number, string>
  >({})
  const [differences, setDifferences] = useState<DifferencesMap>({})
  const [resolutions, setResolutions] = useState<ResolutionsMap>({})
  const [conflictItems, setConflictItems] = useState<ConflictItem[]>([])
  const [confirmLoading, setConfirmLoading] = useState(false)

  const { data: channelsData } = useQuery({
    queryKey: ['upstream-channels'],
    queryFn: getUpstreamChannels,
    enabled: channelDialogOpen,
  })

  // Memoize the channels list so the effect below only re-runs when the query
  // data actually changes, instead of on every render (the `|| []` fallback
  // would otherwise produce a new array reference each render).
  const channels = useMemo(() => channelsData?.data ?? [], [channelsData?.data])

  useEffect(() => {
    if (channels.length === 0) return
    setChannelEndpoints((prev) => {
      let mutated = false
      const next = { ...prev }
      for (const channel of channels) {
        if (!next[channel.id]) {
          next[channel.id] = getDefaultEndpointForChannel(channel)
          mutated = true
        }
      }
      return mutated ? next : prev
    })
  }, [channels])

  const fetchMutation = useMutation({
    mutationFn: fetchUpstreamRatios,
    onSuccess: (data) => {
      if (!data.success) {
        toast.error(data.message || t('Failed to fetch upstream prices'))
        return
      }

      const { differences: diffs, test_results } = data.data

      const errorResults = test_results.filter((r) => r.status === 'error')
      if (errorResults.length > 0) {
        const errorMsg = errorResults
          .map((r) => `${r.name}: ${r.error}`)
          .join(', ')
        toast.warning(t('Some channels failed: {{errorMsg}}', { errorMsg }))
      }

      setDifferences(diffs)
      setResolutions({})

      if (Object.keys(diffs).length === 0) {
        toast.success(t('No price differences found'))
      } else {
        toast.success(t('Upstream prices fetched successfully'))
      }
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to fetch upstream prices'))
    },
  })

  const { mutate: syncMutate, isPending: isSyncPending } = useMutation({
    mutationFn: async (request: ModelPricingBatchRequest) => {
      const response = await saveModelPricing(request)
      if (!response.success) {
        throw new Error(response.message || t('Failed to sync prices'))
      }
      return response
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['model-pricing'] })
      queryClient.invalidateQueries({ queryKey: ['system-options'] })

      setDifferences((prevDiffs) => {
        const newDiffs = { ...prevDiffs }
        Object.entries(resolutions).forEach(([model, ratios]) => {
          Object.keys(ratios).forEach((ratioType) => {
            if (newDiffs[model]?.[ratioType as RatioType]) {
              delete newDiffs[model][ratioType as RatioType]
              if (Object.keys(newDiffs[model]).length === 0) {
                delete newDiffs[model]
              }
            }
          })
        })
        return newDiffs
      })

      setResolutions({})
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to sync prices'))
    },
  })

  const handleOpenChannelDialog = () => {
    setChannelDialogOpen(true)
  }

  const handleConfirmChannelSelection = (selectedIds: number[]) => {
    const selectedChannels = channels.filter((ch) =>
      selectedIds.includes(ch.id)
    )

    if (selectedChannels.length === 0) {
      toast.warning(t('Please select at least one channel'))
      return
    }

    const upstreams: UpstreamConfig[] = selectedChannels.map((ch) => ({
      id: ch.id,
      name: ch.name,
      base_url: ch.base_url,
      endpoint: channelEndpoints[ch.id] || DEFAULT_ENDPOINT,
    }))

    fetchMutation.mutate({ upstreams, timeout: 10 })
  }

  const handleSelectValue = useCallback(
    (
      model: string,
      ratioType: RatioType,
      value: number | string,
      sourceName: string
    ) => {
      const modelDiffs = differences[model]

      // Prefer billing_expr over individual ratio fields when available
      const preferredType = sourceName
        ? getPreferredSyncField(modelDiffs || {}, ratioType, sourceName)
        : ratioType
      const preferredValue =
        preferredType === ratioType
          ? value
          : (modelDiffs?.[preferredType]?.upstreams?.[sourceName] ?? value)

      const finalType = preferredType
      const finalValue = preferredValue as number | string
      const category = getBillingCategory(finalType)

      setResolutions((prev) => {
        const newModelRes = { ...prev[model] }

        // Clear conflicting categories
        Object.keys(newModelRes).forEach((rt) => {
          if (
            category !== 'tiered' &&
            getBillingCategory(rt) !== 'tiered' &&
            getBillingCategory(rt) !== category
          ) {
            delete newModelRes[rt]
          }
        })

        newModelRes[finalType] = finalValue

        // When selecting a tiered field, auto-populate paired fields from the same source
        if (category === 'tiered' && sourceName && modelDiffs) {
          const modeVal = modelDiffs.billing_mode?.upstreams?.[sourceName]
          const exprVal = modelDiffs.billing_expr?.upstreams?.[sourceName]
          if (modeVal !== undefined && modeVal !== null && modeVal !== 'same') {
            newModelRes['billing_mode'] = modeVal
          } else if (finalType === 'billing_expr') {
            newModelRes['billing_mode'] = 'tiered_expr'
          }
          if (exprVal !== undefined && exprVal !== null && exprVal !== 'same') {
            newModelRes['billing_expr'] = exprVal
          }
        }

        return { ...prev, [model]: newModelRes }
      })
    },
    [differences]
  )

  const handleUnselectValue = useCallback(
    (model: string, ratioType: RatioType) => {
      setResolutions((prev) => deleteResolutionField(prev, model, ratioType))
    },
    []
  )

  const parsedRatios = useMemo(() => {
    return {
      ModelRatio: parseJsonRecord<number>(modelRatios.ModelRatio),
      CompletionRatio: parseJsonRecord<number>(modelRatios.CompletionRatio),
      CacheRatio: parseJsonRecord<number>(modelRatios.CacheRatio),
      CreateCacheRatio: parseJsonRecord<number>(modelRatios.CreateCacheRatio),
      ImageRatio: parseJsonRecord<number>(modelRatios.ImageRatio),
      AudioRatio: parseJsonRecord<number>(modelRatios.AudioRatio),
      AudioCompletionRatio: parseJsonRecord<number>(
        modelRatios.AudioCompletionRatio
      ),
      ModelPrice: parseJsonRecord<number>(modelRatios.ModelPrice),
      'billing_setting.billing_mode': parseJsonRecord<string>(
        modelRatios['billing_setting.billing_mode']
      ),
      'billing_setting.billing_expr': parseJsonRecord<string>(
        modelRatios['billing_setting.billing_expr']
      ),
    }
  }, [modelRatios])

  type ParsedRatios = typeof parsedRatios

  const getLocalBillingCategory = (
    model: string,
    currentRatios: ParsedRatios
  ): 'price' | 'ratio' | null => {
    if (currentRatios.ModelPrice[model] !== undefined) {
      return 'price'
    }
    if (
      currentRatios.ModelRatio[model] !== undefined ||
      currentRatios.CompletionRatio[model] !== undefined ||
      currentRatios.CacheRatio[model] !== undefined ||
      currentRatios.CreateCacheRatio[model] !== undefined ||
      currentRatios.ImageRatio[model] !== undefined ||
      currentRatios.AudioRatio[model] !== undefined ||
      currentRatios.AudioCompletionRatio[model] !== undefined
    ) {
      return 'ratio'
    }
    return null
  }

  const performSync = useCallback(
    async (currentRatios: ParsedRatios): Promise<boolean> => {
      const upserts = Object.entries(resolutions).map(([modelName, ratios]) => {
        let currentConfig: ModelPricingConfig
        if (
          currentRatios['billing_setting.billing_mode'][modelName] ===
          'tiered_expr'
        ) {
          currentConfig = {
            mode: 'tiered_expr',
            billing_expr:
              currentRatios['billing_setting.billing_expr'][modelName] ?? '',
          }
        } else if (currentRatios.ModelPrice[modelName] !== undefined) {
          currentConfig = {
            mode: 'per-request',
            price: currentRatios.ModelPrice[modelName],
          }
        } else {
          currentConfig = {
            mode: 'per-token',
            ratio: currentRatios.ModelRatio[modelName],
            completion_ratio: currentRatios.CompletionRatio[modelName],
            cache_ratio: currentRatios.CacheRatio[modelName],
            create_cache_ratio: currentRatios.CreateCacheRatio[modelName],
            image_ratio: currentRatios.ImageRatio[modelName],
            audio_ratio: currentRatios.AudioRatio[modelName],
            audio_completion_ratio:
              currentRatios.AudioCompletionRatio[modelName],
          }
        }

        const patch = buildPricingConfigPatch(ratios)
        const config =
          patch.mode !== currentConfig.mode
            ? patch
            : ({
                ...currentConfig,
                ...Object.fromEntries(
                  Object.entries(patch).filter(
                    ([, value]) => value !== undefined
                  )
                ),
              } as ModelPricingConfig)
        return { model_name: modelName, config }
      })

      return new Promise<boolean>((resolve) => {
        syncMutate(
          { upserts, restore: [] },
          {
            onSuccess: () => resolve(true),
            onError: () => resolve(false),
          }
        )
      })
    },
    [resolutions, syncMutate]
  )

  const findSourceChannel = (
    model: string,
    ratioType: RatioType,
    value: number | string
  ): string => {
    const upMap = differences[model]?.[ratioType]?.upstreams
    if (!upMap) return 'Unknown'
    const entry = Object.entries(upMap).find(([, v]) => v === value)
    return entry ? entry[0] : 'Unknown'
  }

  const handleApplySync = () => {
    const currentRatios = parsedRatios
    const conflicts: ConflictItem[] = []

    const fixedPriceLabel = t('Fixed price')
    const modelRatioLabel = t('Model ratio')
    const completionRatioLabel = t('Completion ratio')

    Object.entries(resolutions).forEach(([model, ratios]) => {
      const localCat = getLocalBillingCategory(model, currentRatios)
      const selectedTypes = Object.keys(ratios)
      let newCat: 'price' | 'ratio' | 'tiered'
      if ('model_price' in ratios) {
        newCat = 'price'
      } else if (RATIO_SYNC_FIELDS.some((rt) => selectedTypes.includes(rt))) {
        newCat = 'ratio'
      } else {
        newCat = 'tiered'
      }

      if (localCat && newCat !== 'tiered' && localCat !== newCat) {
        const currentDesc =
          localCat === 'price'
            ? `${fixedPriceLabel}: ${currentRatios.ModelPrice[model]}`
            : `${modelRatioLabel}: ${currentRatios.ModelRatio[model] ?? '-'}\n${completionRatioLabel}: ${currentRatios.CompletionRatio[model] ?? '-'}`

        const newDesc =
          newCat === 'price'
            ? `${fixedPriceLabel}: ${ratios.model_price}`
            : `${modelRatioLabel}: ${ratios.model_ratio ?? '-'}\n${completionRatioLabel}: ${ratios.completion_ratio ?? '-'}`

        const channelNames = selectedTypes
          .map((rt) => findSourceChannel(model, rt as RatioType, ratios[rt]))
          .filter((v, idx, arr) => arr.indexOf(v) === idx)
          .join(', ')

        conflicts.push({
          channel: channelNames,
          model,
          current: currentDesc,
          newVal: newDesc,
        })
      }
    })

    if (conflicts.length > 0) {
      setConflictItems(conflicts)
      setConflictDialogOpen(true)
      return
    }

    toast.info(t('Syncing prices, please wait...'))
    performSync(currentRatios)
  }

  const handleConfirmConflict = async () => {
    setConfirmLoading(true)
    try {
      const success = await performSync(parsedRatios)
      if (success) {
        setConflictDialogOpen(false)
      }
    } finally {
      setConfirmLoading(false)
    }
  }

  const hasSelections = Object.keys(resolutions).length > 0
  const isLoading = fetchMutation.isPending || isSyncPending || confirmLoading

  return (
    <div className='space-y-4'>
      <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
        <div className='flex flex-col gap-2 sm:flex-row'>
          <Button onClick={handleOpenChannelDialog} disabled={isLoading}>
            <RefreshCcw className='mr-2 h-4 w-4' />
            {t('Select Sync Channels')}
          </Button>
          <Button
            variant='secondary'
            onClick={handleApplySync}
            disabled={!hasSelections || isLoading}
          >
            {(isSyncPending || confirmLoading) && (
              <span className='mr-2 h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent' />
            )}
            <CheckSquare className='mr-2 h-4 w-4' />
            {t('Apply Sync')}
          </Button>
        </div>
      </div>

      <UpstreamRatioSyncTable
        differences={differences}
        resolutions={resolutions}
        isDisabled={isLoading}
        isSyncing={fetchMutation.isPending}
        onSelectValue={handleSelectValue}
        onUnselectValue={handleUnselectValue}
      />

      <ChannelSelectorDialog
        open={channelDialogOpen}
        onOpenChange={setChannelDialogOpen}
        channels={channels}
        selectedChannelIds={selectedChannelIds}
        onSelectedChannelIdsChange={setSelectedChannelIds}
        channelEndpoints={channelEndpoints}
        onChannelEndpointsChange={setChannelEndpoints}
        onConfirm={handleConfirmChannelSelection}
      />

      <ConflictConfirmDialog
        open={conflictDialogOpen}
        onOpenChange={setConflictDialogOpen}
        conflicts={conflictItems}
        onConfirm={handleConfirmConflict}
        isLoading={confirmLoading}
      />
    </div>
  )
}
