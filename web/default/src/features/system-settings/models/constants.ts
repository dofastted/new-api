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
export const DEFAULT_ENDPOINT = '/api/pricing'

// ---------------------------------------------------------------------------
// Built-in official pricing presets
//
// The backend (`controller/ratio_sync.go`) only exposes official provider
// pricing sources here. The *_NAME values mirror backend wire identifiers and
// must not be translated.
// ---------------------------------------------------------------------------

export const OPENAI_OFFICIAL_CHANNEL_ID = -102
export const OPENAI_OFFICIAL_CHANNEL_NAME = 'OpenAI 官方价格'
export const OPENAI_OFFICIAL_CHANNEL_BASE_URL = 'https://developers.openai.com'
export const OPENAI_OFFICIAL_CHANNEL_ENDPOINT =
  'https://developers.openai.com/api/docs/pricing.md'

export const CLAUDE_OFFICIAL_CHANNEL_ID = -103
export const CLAUDE_OFFICIAL_CHANNEL_NAME = 'Claude 官方价格'
export const CLAUDE_OFFICIAL_CHANNEL_BASE_URL = 'https://platform.claude.com'
export const CLAUDE_OFFICIAL_CHANNEL_ENDPOINT =
  'https://platform.claude.com/docs/en/about-claude/pricing.md'

export const GEMINI_OFFICIAL_CHANNEL_ID = -104
export const GEMINI_OFFICIAL_CHANNEL_NAME = 'Gemini 官方价格'
export const GEMINI_OFFICIAL_CHANNEL_BASE_URL = 'https://ai.google.dev'
export const GEMINI_OFFICIAL_CHANNEL_ENDPOINT =
  'https://ai.google.dev/gemini-api/docs/pricing'

export const GLM_OFFICIAL_CHANNEL_ID = -105
export const GLM_OFFICIAL_CHANNEL_NAME = 'GLM 官方价格'
export const GLM_OFFICIAL_CHANNEL_BASE_URL = 'https://docs.bigmodel.cn'
export const GLM_OFFICIAL_CHANNEL_ENDPOINT =
  'https://docs.bigmodel.cn/cn/guide/models/text/glm-4.5'

export const XAI_OFFICIAL_CHANNEL_ID = -106
export const XAI_OFFICIAL_CHANNEL_NAME = 'xAI 官方价格'
export const XAI_OFFICIAL_CHANNEL_BASE_URL = 'https://docs.x.ai'
export const XAI_OFFICIAL_CHANNEL_ENDPOINT =
  'https://docs.x.ai/developers/models.md'

export const DEEPSEEK_OFFICIAL_CHANNEL_ID = -107
export const DEEPSEEK_OFFICIAL_CHANNEL_NAME = 'DeepSeek 官方价格'
export const DEEPSEEK_OFFICIAL_CHANNEL_BASE_URL =
  'https://api-docs.deepseek.com'
export const DEEPSEEK_OFFICIAL_CHANNEL_ENDPOINT =
  'https://api-docs.deepseek.com/quick_start/pricing'

export const ENDPOINT_OPTIONS = [
  { label: 'pricing', value: DEFAULT_ENDPOINT },
  { label: 'ratio_config', value: '/api/ratio_config' },
  { label: 'custom', value: 'custom' },
] as const

// Labels reuse the existing sentence-case i18n keys defined for form fields
// (e.g. `Model ratio`, `Audio completion ratio`). Do NOT switch to Title Case
// here without updating the i18n catalog; otherwise we end up with two keys per
// ratio type that only differ in capitalization.
export const RATIO_TYPE_OPTIONS = [
  { label: 'Model ratio', value: 'model_ratio' },
  { label: 'Completion ratio', value: 'completion_ratio' },
  { label: 'Cache ratio', value: 'cache_ratio' },
  { label: 'Create cache ratio', value: 'create_cache_ratio' },
  { label: 'Image ratio', value: 'image_ratio' },
  { label: 'Audio ratio', value: 'audio_ratio' },
  { label: 'Audio completion ratio', value: 'audio_completion_ratio' },
  { label: 'Fixed price', value: 'model_price' },
  { label: 'Expression billing', value: 'billing_expr' },
] as const

export const CHANNEL_STATUS_CONFIG = {
  1: { label: 'Enabled', variant: 'success' as const },
  2: { label: 'Disabled', variant: 'danger' as const },
  3: { label: 'Auto-Disabled', variant: 'warning' as const },
} as const
