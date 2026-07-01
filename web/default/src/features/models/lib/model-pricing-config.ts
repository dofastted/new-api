export type ModelPricingConfigMode = 'per-token' | 'per-request' | 'tiered_expr'

export type ModelPricingConfigDraft = {
  price?: string
  ratio?: string
  cacheRatio?: string
  createCacheRatio?: string
  completionRatio?: string
  imageRatio?: string
  audioRatio?: string
  audioCompletionRatio?: string
  billingExpr?: string
}

export type ParsedModelPricingConfig = {
  mode: ModelPricingConfigMode
  draft: ModelPricingConfigDraft
}

function getConfigValue(value: object, key: string): unknown {
  return Reflect.get(value, key)
}

function asConfigObject(value: unknown): object | null {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    return null
  }
  return value
}

function numberString(value: unknown): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return ''
  }
  return String(value)
}

function parseMode(value: unknown): ModelPricingConfigMode {
  if (value === 'per-request' || value === 'tiered_expr') {
    return value
  }
  return 'per-token'
}

export function parseModelPricingConfig(
  raw?: string
): ParsedModelPricingConfig {
  if (!raw?.trim()) {
    return { mode: 'per-token', draft: {} }
  }

  try {
    const config = asConfigObject(JSON.parse(raw))
    if (!config) {
      return { mode: 'per-token', draft: {} }
    }
    return {
      mode: parseMode(getConfigValue(config, 'mode')),
      draft: {
        price: numberString(getConfigValue(config, 'price')),
        ratio: numberString(getConfigValue(config, 'ratio')),
        cacheRatio: numberString(getConfigValue(config, 'cache_ratio')),
        createCacheRatio: numberString(
          getConfigValue(config, 'create_cache_ratio')
        ),
        completionRatio: numberString(
          getConfigValue(config, 'completion_ratio')
        ),
        imageRatio: numberString(getConfigValue(config, 'image_ratio')),
        audioRatio: numberString(getConfigValue(config, 'audio_ratio')),
        audioCompletionRatio: numberString(
          getConfigValue(config, 'audio_completion_ratio')
        ),
        billingExpr:
          typeof getConfigValue(config, 'billing_expr') === 'string'
            ? String(getConfigValue(config, 'billing_expr'))
            : '',
      },
    }
  } catch {
    return { mode: 'per-token', draft: {} }
  }
}

function parseDraftNumber(value?: string): number | undefined {
  if (!value) {
    return undefined
  }
  const parsed = Number.parseFloat(value)
  return Number.isFinite(parsed) ? parsed : undefined
}

function assignNumber(
  target: Record<string, number | string>,
  key: string,
  value?: string
): boolean {
  const parsed = parseDraftNumber(value)
  if (parsed === undefined) {
    return false
  }
  target[key] = parsed
  return true
}

export function buildModelPricingConfig(
  mode: ModelPricingConfigMode,
  draft: ModelPricingConfigDraft
): string {
  const config: Record<string, number | string> = { mode }
  let hasPricing = false

  if (mode === 'per-request') {
    hasPricing = assignNumber(config, 'price', draft.price)
    return hasPricing ? JSON.stringify(config) : ''
  }

  if (mode === 'tiered_expr') {
    const expr = draft.billingExpr?.trim()
    if (!expr) {
      return ''
    }
    config.billing_expr = expr
    return JSON.stringify(config)
  }

  hasPricing = assignNumber(config, 'ratio', draft.ratio) || hasPricing
  hasPricing =
    assignNumber(config, 'cache_ratio', draft.cacheRatio) || hasPricing
  hasPricing =
    assignNumber(config, 'create_cache_ratio', draft.createCacheRatio) ||
    hasPricing
  hasPricing =
    assignNumber(config, 'completion_ratio', draft.completionRatio) ||
    hasPricing
  hasPricing =
    assignNumber(config, 'image_ratio', draft.imageRatio) || hasPricing
  hasPricing =
    assignNumber(config, 'audio_ratio', draft.audioRatio) || hasPricing
  hasPricing =
    assignNumber(
      config,
      'audio_completion_ratio',
      draft.audioCompletionRatio
    ) || hasPricing

  return hasPricing ? JSON.stringify(config) : ''
}
