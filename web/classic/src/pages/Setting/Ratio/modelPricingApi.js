/*
Copyright (C) 2025 QuantumNous

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

import { API } from '../../../helpers';

const formatMap = (value) => JSON.stringify(value, null, 2);

export async function fetchModelPricing() {
  const response = await API.get('/api/model-pricing/');
  if (!response?.data?.success) {
    throw new Error(response?.data?.message || 'Failed to load model pricing');
  }
  return response.data.data || [];
}

export async function saveModelPricingBatch(request) {
  const response = await API.put('/api/model-pricing/', request);
  if (!response?.data?.success) {
    throw new Error(response?.data?.message || 'Failed to save model pricing');
  }
  return response.data.data;
}

export async function calibrateModelPricing() {
  const response = await API.post('/api/model-pricing/calibrate');
  if (!response?.data?.success) {
    throw new Error(
      response?.data?.message || 'Failed to calibrate model pricing',
    );
  }
  return response.data.data;
}

export function buildPricingViewMap(views) {
  return Object.fromEntries(views.map((view) => [view.model_name, view]));
}

export function buildCanonicalPricingOptions(views, baseOptions = {}) {
  const modelPrice = {};
  const modelRatio = {};
  const completionRatio = {};
  const cacheRatio = {};
  const createCacheRatio = {};
  const imageRatio = {};
  const audioRatio = {};
  const audioCompletionRatio = {};
  const billingMode = {};
  const billingExpr = {};

  for (const view of views) {
    const name = view.model_name;
    const config = view.effective_config || {};
    if (config.price !== undefined) modelPrice[name] = config.price;
    if (config.ratio !== undefined) modelRatio[name] = config.ratio;
    if (config.completion_ratio !== undefined) {
      completionRatio[name] = config.completion_ratio;
    }
    if (config.cache_ratio !== undefined) cacheRatio[name] = config.cache_ratio;
    if (config.create_cache_ratio !== undefined) {
      createCacheRatio[name] = config.create_cache_ratio;
    }
    if (config.image_ratio !== undefined) imageRatio[name] = config.image_ratio;
    if (config.audio_ratio !== undefined) audioRatio[name] = config.audio_ratio;
    if (config.audio_completion_ratio !== undefined) {
      audioCompletionRatio[name] = config.audio_completion_ratio;
    }
    if (config.mode === 'tiered_expr') {
      billingMode[name] = config.mode;
      billingExpr[name] = config.billing_expr || '';
    }
  }

  return {
    ...baseOptions,
    ModelPrice: formatMap(modelPrice),
    ModelRatio: formatMap(modelRatio),
    CompletionRatio: formatMap(completionRatio),
    CacheRatio: formatMap(cacheRatio),
    CreateCacheRatio: formatMap(createCacheRatio),
    ImageRatio: formatMap(imageRatio),
    AudioRatio: formatMap(audioRatio),
    AudioCompletionRatio: formatMap(audioCompletionRatio),
    'billing_setting.billing_mode': formatMap(billingMode),
    'billing_setting.billing_expr': formatMap(billingExpr),
  };
}

const optionalValue = (value) => (value === null ? undefined : value);

export function normalizePricingConfig(config = {}) {
  return {
    mode: config.mode || 'per-token',
    price: optionalValue(config.price),
    ratio: optionalValue(config.ratio),
    completion_ratio: optionalValue(config.completion_ratio),
    cache_ratio: optionalValue(config.cache_ratio),
    create_cache_ratio: optionalValue(config.create_cache_ratio),
    image_ratio: optionalValue(config.image_ratio),
    audio_ratio: optionalValue(config.audio_ratio),
    audio_completion_ratio: optionalValue(config.audio_completion_ratio),
    billing_expr: config.billing_expr || undefined,
  };
}

export function pricingConfigSignature(config) {
  return JSON.stringify(normalizePricingConfig(config));
}
