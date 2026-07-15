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
import {
  combineBillingExpr,
  splitBillingExprAndRequestRules,
} from "@/features/pricing/lib/billing-expr";

import type { ModelPricingConfig, ModelPricingView } from "../types";
import { safeJsonParse } from "../utils/json-parser";
import { formatPricingNumber } from "./pricing-format";

export type ModelPricingSnapshotInput = {
  modelPrice: string;
  modelRatio: string;
  cacheRatio: string;
  createCacheRatio: string;
  completionRatio: string;
  imageRatio: string;
  audioRatio: string;
  audioCompletionRatio: string;
  billingMode: string;
  billingExpr: string;
};

export type ModelPricingSnapshot = {
  name: string;
  price?: string;
  ratio?: string;
  cacheRatio?: string;
  createCacheRatio?: string;
  completionRatio?: string;
  imageRatio?: string;
  audioRatio?: string;
  audioCompletionRatio?: string;
  billingMode?: string;
  billingExpr?: string;
  requestRuleExpr?: string;
  hasConflict: boolean;
};

export type ModelRow = ModelPricingSnapshot & {
  saved?: ModelPricingSnapshot;
  draft?: ModelPricingSnapshot;
  isDraftChanged: boolean;
  isDraftDeleted: boolean;
  isDraftNew: boolean;
  pricingSource?: ModelPricingView["authority"];
  isOfficialStale?: boolean;
  officialLastConfirmedAt?: number;
  hasOfficialPricing?: boolean;
  pricingAuthorityModelName?: string;
};

export type ModelPricingSourceInfo = {
  authority: ModelPricingView["authority"];
  stale: boolean;
  lastConfirmedAt: number;
  hasOfficial: boolean;
  authorityModelName: string;
};

export type ModelPricingFormValues = {
  ModelPrice: string;
  ModelRatio: string;
  CacheRatio: string;
  CreateCacheRatio: string;
  CompletionRatio: string;
  ImageRatio: string;
  AudioRatio: string;
  AudioCompletionRatio: string;
  ExposeRatioEnabled: boolean;
  BillingMode: string;
  BillingExpr: string;
};

const formatPricingMap = (value: Record<string, number | string>) =>
  JSON.stringify(value, null, 2);

export const buildModelPricingFormValues = (
  views: ModelPricingView[],
  exposeRatioEnabled: boolean,
): ModelPricingFormValues => {
  const modelPrice: Record<string, number> = {};
  const modelRatio: Record<string, number> = {};
  const cacheRatio: Record<string, number> = {};
  const createCacheRatio: Record<string, number> = {};
  const completionRatio: Record<string, number> = {};
  const imageRatio: Record<string, number> = {};
  const audioRatio: Record<string, number> = {};
  const audioCompletionRatio: Record<string, number> = {};
  const billingMode: Record<string, string> = {};
  const billingExpr: Record<string, string> = {};

  for (const view of views) {
    const name = view.model_name;
    const config = view.effective_config;
    if (config.price !== undefined) modelPrice[name] = config.price;
    if (config.ratio !== undefined) modelRatio[name] = config.ratio;
    if (config.cache_ratio !== undefined) cacheRatio[name] = config.cache_ratio;
    if (config.create_cache_ratio !== undefined) {
      createCacheRatio[name] = config.create_cache_ratio;
    }
    if (config.completion_ratio !== undefined) {
      completionRatio[name] = config.completion_ratio;
    }
    if (config.image_ratio !== undefined) imageRatio[name] = config.image_ratio;
    if (config.audio_ratio !== undefined) audioRatio[name] = config.audio_ratio;
    if (config.audio_completion_ratio !== undefined) {
      audioCompletionRatio[name] = config.audio_completion_ratio;
    }
    if (config.mode === "tiered_expr") {
      billingMode[name] = config.mode;
      billingExpr[name] = config.billing_expr ?? "";
    }
  }

  return {
    ModelPrice: formatPricingMap(modelPrice),
    ModelRatio: formatPricingMap(modelRatio),
    CacheRatio: formatPricingMap(cacheRatio),
    CreateCacheRatio: formatPricingMap(createCacheRatio),
    CompletionRatio: formatPricingMap(completionRatio),
    ImageRatio: formatPricingMap(imageRatio),
    AudioRatio: formatPricingMap(audioRatio),
    AudioCompletionRatio: formatPricingMap(audioCompletionRatio),
    ExposeRatioEnabled: exposeRatioEnabled,
    BillingMode: formatPricingMap(billingMode),
    BillingExpr: formatPricingMap(billingExpr),
  };
};

export const buildModelPricingSourceMap = (views: ModelPricingView[]) =>
  Object.fromEntries(
    views.map((view) => [
      view.model_name,
      {
        authority: view.authority,
        stale: view.official_stale,
        lastConfirmedAt: view.official_last_confirmed_at,
        hasOfficial: Boolean(view.official_config),
        authorityModelName: view.authority_model_name,
      },
    ]),
  ) as Record<string, ModelPricingSourceInfo>;

const optionalNumber = (value?: string) => {
  if (!hasPricingValue(value)) return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
};

export const snapshotToModelPricingConfig = (
  snapshot: ModelPricingSnapshot,
): ModelPricingConfig => {
  if (snapshot.billingMode === "per-request") {
    return {
      mode: "per-request",
      price: optionalNumber(snapshot.price),
    };
  }
  if (snapshot.billingMode === "tiered_expr") {
    return {
      mode: "tiered_expr",
      billing_expr: combineBillingExpr(
        snapshot.billingExpr ?? "",
        snapshot.requestRuleExpr ?? "",
      ),
    };
  }
  return {
    mode: "per-token",
    ratio: optionalNumber(snapshot.ratio),
    completion_ratio: optionalNumber(snapshot.completionRatio),
    cache_ratio: optionalNumber(snapshot.cacheRatio),
    create_cache_ratio: optionalNumber(snapshot.createCacheRatio),
    image_ratio: optionalNumber(snapshot.imageRatio),
    audio_ratio: optionalNumber(snapshot.audioRatio),
    audio_completion_ratio: optionalNumber(snapshot.audioCompletionRatio),
  };
};

export const hasPricingValue = (value?: string) =>
  value !== undefined && value !== "";

const toNumberOrNull = (value?: string) => {
  if (!hasPricingValue(value)) return null;
  const num = Number(value);
  return Number.isFinite(num) ? num : null;
};

const ratioToPrice = (ratio?: string, denominator?: string) => {
  const ratioNumber = toNumberOrNull(ratio);
  const denominatorNumber = denominator ? toNumberOrNull(denominator) : 2;
  if (ratioNumber === null || denominatorNumber === null) return "";
  return formatPricingNumber(ratioNumber * denominatorNumber);
};

export const getModeLabel = (mode?: string) => {
  if (mode === "per-request") return "Per-request";
  if (mode === "tiered_expr") return "Expression";
  return "Per-token";
};

export const getModeVariant = (
  mode?: string,
): "warning" | "info" | "success" => {
  if (mode === "per-request") return "warning";
  if (mode === "tiered_expr") return "info";
  return "success";
};

const getExpressionSummary = (
  row: ModelPricingSnapshot,
  t: (key: string) => string,
) => {
  const tierCount = (row.billingExpr?.match(/tier\(/g) || []).length;
  if (tierCount > 0) {
    return `${t("Tiered pricing")} · ${tierCount} ${t("tiers")}`;
  }
  return t("Expression pricing");
};

export const getPriceSummary = (
  row: ModelPricingSnapshot,
  t: (key: string) => string,
) => {
  if (row.billingMode === "tiered_expr") {
    return getExpressionSummary(row, t);
  }
  if (row.billingMode === "per-request") {
    return row.price ? `$${row.price} / ${t("request")}` : t("Unset price");
  }

  const inputPrice = ratioToPrice(row.ratio);
  if (!inputPrice) return t("Unset price");

  const extraCount = [
    row.completionRatio,
    row.cacheRatio,
    row.createCacheRatio,
    row.imageRatio,
    row.audioRatio,
    row.audioCompletionRatio,
  ].filter(hasPricingValue).length;

  return extraCount > 0
    ? `${t("Input")} $${inputPrice} · ${extraCount} ${t("extras")}`
    : `${t("Input")} $${inputPrice}`;
};

export const getPriceDetail = (
  row: ModelPricingSnapshot,
  t: (key: string) => string,
) => {
  if (row.billingMode === "tiered_expr") {
    return row.requestRuleExpr
      ? t("Includes request rules")
      : t("Expression based");
  }
  if (row.billingMode === "per-request") {
    return t("Fixed request price");
  }

  const inputPrice = ratioToPrice(row.ratio);
  if (!inputPrice) return t("No base input price");

  const details = [
    row.completionRatio &&
      `${t("Output")} $${ratioToPrice(row.completionRatio, inputPrice)}`,
    row.cacheRatio &&
      `${t("Cache")} $${ratioToPrice(row.cacheRatio, inputPrice)}`,
    row.createCacheRatio &&
      `${t("Cache write")} $${ratioToPrice(row.createCacheRatio, inputPrice)}`,
  ]
    .filter(Boolean)
    .slice(0, 2);

  return details.length > 0 ? details.join(" · ") : t("Base input price only");
};

export const buildModelSnapshots = ({
  modelPrice,
  modelRatio,
  cacheRatio,
  createCacheRatio,
  completionRatio,
  imageRatio,
  audioRatio,
  audioCompletionRatio,
  billingMode,
  billingExpr,
}: ModelPricingSnapshotInput): ModelPricingSnapshot[] => {
  const priceMap = safeJsonParse<Record<string, number>>(modelPrice, {
    fallback: {},
    context: "model prices",
  });
  const ratioMap = safeJsonParse<Record<string, number>>(modelRatio, {
    fallback: {},
    context: "model ratios",
  });
  const cacheMap = safeJsonParse<Record<string, number>>(cacheRatio, {
    fallback: {},
    context: "cache ratios",
  });
  const createCacheMap = safeJsonParse<Record<string, number>>(
    createCacheRatio,
    { fallback: {}, context: "create cache ratios" },
  );
  const completionMap = safeJsonParse<Record<string, number>>(completionRatio, {
    fallback: {},
    context: "completion ratios",
  });
  const imageMap = safeJsonParse<Record<string, number>>(imageRatio, {
    fallback: {},
    context: "image ratios",
  });
  const audioMap = safeJsonParse<Record<string, number>>(audioRatio, {
    fallback: {},
    context: "audio ratios",
  });
  const audioCompletionMap = safeJsonParse<Record<string, number>>(
    audioCompletionRatio,
    { fallback: {}, context: "audio completion ratios" },
  );
  const billingModeMap = safeJsonParse<Record<string, string>>(billingMode, {
    fallback: {},
    context: "billing mode",
  });
  const billingExprMap = safeJsonParse<Record<string, string>>(billingExpr, {
    fallback: {},
    context: "billing expression",
  });

  const modelNames = new Set([
    ...Object.keys(priceMap),
    ...Object.keys(ratioMap),
    ...Object.keys(cacheMap),
    ...Object.keys(createCacheMap),
    ...Object.keys(completionMap),
    ...Object.keys(imageMap),
    ...Object.keys(audioMap),
    ...Object.keys(audioCompletionMap),
    ...Object.keys(billingModeMap),
    ...Object.keys(billingExprMap),
  ]);

  return [...modelNames].map((name) => {
    const price = priceMap[name]?.toString() || "";
    const ratio = ratioMap[name]?.toString() || "";
    const cache = cacheMap[name]?.toString() || "";
    const createCache = createCacheMap[name]?.toString() || "";
    const completion = completionMap[name]?.toString() || "";
    const image = imageMap[name]?.toString() || "";
    const audio = audioMap[name]?.toString() || "";
    const audioCompletion = audioCompletionMap[name]?.toString() || "";

    const modeForModel = billingModeMap[name];
    if (modeForModel === "tiered_expr") {
      const fullExpr = billingExprMap[name] || "";
      const { billingExpr: pureExpr, requestRuleExpr } =
        splitBillingExprAndRequestRules(fullExpr);
      return {
        name,
        billingMode: "tiered_expr",
        billingExpr: pureExpr,
        requestRuleExpr,
        price,
        ratio,
        cacheRatio: cache,
        createCacheRatio: createCache,
        completionRatio: completion,
        imageRatio: image,
        audioRatio: audio,
        audioCompletionRatio: audioCompletion,
        hasConflict: false,
      };
    }

    return {
      name,
      price,
      ratio,
      cacheRatio: cache,
      createCacheRatio: createCache,
      completionRatio: completion,
      imageRatio: image,
      audioRatio: audio,
      audioCompletionRatio: audioCompletion,
      billingMode: price !== "" ? "per-request" : "per-token",
      hasConflict:
        price !== "" &&
        (ratio !== "" ||
          completion !== "" ||
          cache !== "" ||
          createCache !== "" ||
          image !== "" ||
          audio !== "" ||
          audioCompletion !== ""),
    };
  });
};

export const getSnapshotSignature = (snapshot?: ModelPricingSnapshot) => {
  if (!snapshot) return "";
  return JSON.stringify({
    price: snapshot.price || "",
    ratio: snapshot.ratio || "",
    cacheRatio: snapshot.cacheRatio || "",
    createCacheRatio: snapshot.createCacheRatio || "",
    completionRatio: snapshot.completionRatio || "",
    imageRatio: snapshot.imageRatio || "",
    audioRatio: snapshot.audioRatio || "",
    audioCompletionRatio: snapshot.audioCompletionRatio || "",
    billingMode: snapshot.billingMode || "per-token",
    billingExpr: snapshot.billingExpr || "",
    requestRuleExpr: snapshot.requestRuleExpr || "",
  });
};
