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
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import * as z from "zod";

import { ConfirmDialog } from "@/components/confirm-dialog";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

import {
  calibrateModelPricing,
  getModelPricing,
  saveModelPricing,
} from "../api";
import { SettingsPageTitleStatusPortal } from "../components/settings-page-context";
import { SettingsSection } from "../components/settings-section";
import { useUpdateOption } from "../hooks/use-update-option";
import type { ModelPricingBatchRequest } from "../types";
import { GroupRatioForm } from "./group-ratio-form";
import {
  buildModelPricingFormValues,
  buildModelPricingSourceMap,
  buildModelSnapshots,
  getSnapshotSignature,
  snapshotToModelPricingConfig,
} from "./model-pricing-snapshots";
import { ModelRatioForm } from "./model-ratio-form";
import { ToolPriceSettings } from "./tool-price-settings";
import { UpstreamRatioSync } from "./upstream-ratio-sync";
import {
  formatJsonForTextarea,
  type JsonValidationError,
  normalizeJsonString,
  validateJsonString,
} from "./utils";

type Translate = (key: string, options?: Record<string, unknown>) => string;

function formatJsonValidationError(
  t: Translate,
  error?: JsonValidationError,
  fallback = "Invalid JSON",
) {
  if (!error) return t(fallback);

  if (error.type === "required") return t("Value is required");
  if (error.type === "structure") {
    return t(
      fallback === "Invalid JSON" ? "JSON structure is invalid" : fallback,
    );
  }

  let detail = t("JSON is invalid. Please check the syntax.");
  if (error.line && error.column) {
    detail = t("JSON is invalid at line {{line}}, column {{column}}.", {
      line: error.line,
      column: error.column,
    });
  } else if (error.position !== undefined) {
    detail = t("JSON is invalid at position {{position}}.", {
      position: error.position,
    });
  }

  const parts = [detail];

  if (error.missingCommaLine) {
    parts.push(
      t("Check line {{line}} for a missing comma.", {
        line: error.missingCommaLine,
      }),
    );
  }

  return parts.join(" ");
}

function createJsonStringField(
  t: Translate,
  options?: Parameters<typeof validateJsonString>[1],
) {
  return z.string().superRefine((value, ctx) => {
    const result = validateJsonString(value, options);
    if (!result.valid) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: formatJsonValidationError(t, result.error, result.message),
      });
    }
  });
}

type ModelFormValues = {
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

type GroupFormValues = {
  GroupRatio: string;
  TopupGroupRatio: string;
  UserUsableGroups: string;
  GroupGroupRatio: string;
  AutoGroups: string;
  DefaultUseAutoGroup: boolean;
  GroupSpecialUsableGroup: string;
};

const createModelSchema = (t: Translate) =>
  z.object({
    ModelPrice: createJsonStringField(t),
    ModelRatio: createJsonStringField(t),
    CacheRatio: createJsonStringField(t),
    CreateCacheRatio: createJsonStringField(t),
    CompletionRatio: createJsonStringField(t),
    ImageRatio: createJsonStringField(t),
    AudioRatio: createJsonStringField(t),
    AudioCompletionRatio: createJsonStringField(t),
    ExposeRatioEnabled: z.boolean(),
    BillingMode: createJsonStringField(t),
    BillingExpr: createJsonStringField(t),
  });

const createGroupSchema = (t: Translate) =>
  z.object({
    GroupRatio: createJsonStringField(t),
    TopupGroupRatio: createJsonStringField(t),
    UserUsableGroups: createJsonStringField(t),
    GroupGroupRatio: createJsonStringField(t),
    AutoGroups: createJsonStringField(t, {
      predicate: (parsed) =>
        Array.isArray(parsed) &&
        parsed.every((item) => typeof item === "string"),
      predicateMessage: "Expected a JSON array of group identifiers",
    }),
    DefaultUseAutoGroup: z.boolean(),
    GroupSpecialUsableGroup: createJsonStringField(t),
  });

type RatioTabId = "models" | "groups" | "tool-prices" | "upstream-sync";

const normalizeModelFormValues = (
  values: ModelFormValues,
): ModelFormValues => ({
  ModelPrice: normalizeJsonString(values.ModelPrice),
  ModelRatio: normalizeJsonString(values.ModelRatio),
  CacheRatio: normalizeJsonString(values.CacheRatio),
  CreateCacheRatio: normalizeJsonString(values.CreateCacheRatio),
  CompletionRatio: normalizeJsonString(values.CompletionRatio),
  ImageRatio: normalizeJsonString(values.ImageRatio),
  AudioRatio: normalizeJsonString(values.AudioRatio),
  AudioCompletionRatio: normalizeJsonString(values.AudioCompletionRatio),
  ExposeRatioEnabled: values.ExposeRatioEnabled,
  BillingMode: normalizeJsonString(values.BillingMode),
  BillingExpr: normalizeJsonString(values.BillingExpr),
});

const buildSnapshotsFromFormValues = (values: ModelFormValues) =>
  buildModelSnapshots({
    modelPrice: values.ModelPrice,
    modelRatio: values.ModelRatio,
    cacheRatio: values.CacheRatio,
    createCacheRatio: values.CreateCacheRatio,
    completionRatio: values.CompletionRatio,
    imageRatio: values.ImageRatio,
    audioRatio: values.AudioRatio,
    audioCompletionRatio: values.AudioCompletionRatio,
    billingMode: values.BillingMode,
    billingExpr: values.BillingExpr,
  });

type RatioSettingsCardProps = {
  modelDefaults: ModelFormValues;
  groupDefaults: GroupFormValues;
  toolPricesDefault: string;
  titleKey?: string;
  visibleTabs?: RatioTabId[];
};

export function RatioSettingsCard({
  modelDefaults,
  groupDefaults,
  toolPricesDefault,
  titleKey = "Pricing Ratios",
  visibleTabs = ["models", "groups", "tool-prices", "upstream-sync"],
}: RatioSettingsCardProps) {
  const { t } = useTranslation();
  const updateOption = useUpdateOption();
  const queryClient = useQueryClient();
  const [confirmOpen, setConfirmOpen] = useState(false);

  const pricingQuery = useQuery({
    queryKey: ["model-pricing"],
    queryFn: getModelPricing,
  });
  const pricingViews = pricingQuery.data?.success
    ? pricingQuery.data.data
    : undefined;
  const canonicalModelDefaults = useMemo(
    () =>
      pricingViews
        ? buildModelPricingFormValues(
            pricingViews,
            modelDefaults.ExposeRatioEnabled,
          )
        : modelDefaults,
    [modelDefaults, pricingViews],
  );
  const pricingSources = useMemo(
    () => buildModelPricingSourceMap(pricingViews ?? []),
    [pricingViews],
  );
  const pricingMutation = useMutation({
    mutationFn: async (request: ModelPricingBatchRequest) => {
      const response = await saveModelPricing(request);
      if (!response.success) {
        throw new Error(response.message || t("Failed to save model prices"));
      }
      return response;
    },
    onError: (error: Error) => {
      toast.error(error.message || t("Failed to save model prices"));
    },
  });
  const resetMutation = useMutation({
    mutationFn: calibrateModelPricing,
    onSuccess: (data) => {
      if (data.success) {
        toast.success(t("Model price calibration task started"));
        queryClient.invalidateQueries({ queryKey: ["model-pricing"] });
        setConfirmOpen(false);
      } else {
        toast.error(data.message || t("Failed to calibrate model prices"));
      }
    },
    onError: (error: Error) => {
      toast.error(error.message || t("Failed to calibrate model prices"));
    },
  });

  const modelNormalizedDefaults = useRef(
    normalizeModelFormValues(modelDefaults),
  );
  const [savedModelValues, setSavedModelValues] = useState(
    modelNormalizedDefaults.current,
  );

  const groupNormalizedDefaults = useRef({
    GroupRatio: normalizeJsonString(groupDefaults.GroupRatio),
    TopupGroupRatio: normalizeJsonString(groupDefaults.TopupGroupRatio),
    UserUsableGroups: normalizeJsonString(groupDefaults.UserUsableGroups),
    GroupGroupRatio: normalizeJsonString(groupDefaults.GroupGroupRatio),
    AutoGroups: normalizeJsonString(groupDefaults.AutoGroups),
    DefaultUseAutoGroup: groupDefaults.DefaultUseAutoGroup,
    GroupSpecialUsableGroup: normalizeJsonString(
      groupDefaults.GroupSpecialUsableGroup,
    ),
  });
  const modelSchema = useMemo(() => createModelSchema(t), [t]);
  const groupSchema = useMemo(() => createGroupSchema(t), [t]);

  const modelForm = useForm<ModelFormValues>({
    resolver: zodResolver(modelSchema),
    mode: "onChange",
    defaultValues: {
      ...modelDefaults,
      ModelPrice: formatJsonForTextarea(modelDefaults.ModelPrice),
      ModelRatio: formatJsonForTextarea(modelDefaults.ModelRatio),
      CacheRatio: formatJsonForTextarea(modelDefaults.CacheRatio),
      CreateCacheRatio: formatJsonForTextarea(modelDefaults.CreateCacheRatio),
      CompletionRatio: formatJsonForTextarea(modelDefaults.CompletionRatio),
      ImageRatio: formatJsonForTextarea(modelDefaults.ImageRatio),
      AudioRatio: formatJsonForTextarea(modelDefaults.AudioRatio),
      AudioCompletionRatio: formatJsonForTextarea(
        modelDefaults.AudioCompletionRatio,
      ),
      BillingMode: formatJsonForTextarea(modelDefaults.BillingMode),
      BillingExpr: formatJsonForTextarea(modelDefaults.BillingExpr),
    },
  });

  const groupForm = useForm<GroupFormValues>({
    resolver: zodResolver(groupSchema),
    mode: "onChange",
    defaultValues: {
      ...groupDefaults,
      GroupRatio: formatJsonForTextarea(groupDefaults.GroupRatio),
      TopupGroupRatio: formatJsonForTextarea(groupDefaults.TopupGroupRatio),
      UserUsableGroups: formatJsonForTextarea(groupDefaults.UserUsableGroups),
      GroupGroupRatio: formatJsonForTextarea(groupDefaults.GroupGroupRatio),
      AutoGroups: formatJsonForTextarea(groupDefaults.AutoGroups),
      GroupSpecialUsableGroup: formatJsonForTextarea(
        groupDefaults.GroupSpecialUsableGroup,
      ),
    },
  });

  useEffect(() => {
    const normalized = normalizeModelFormValues(canonicalModelDefaults);
    modelNormalizedDefaults.current = normalized;
    setSavedModelValues(normalized);

    modelForm.reset({
      ...canonicalModelDefaults,
      ModelPrice: formatJsonForTextarea(canonicalModelDefaults.ModelPrice),
      ModelRatio: formatJsonForTextarea(canonicalModelDefaults.ModelRatio),
      CacheRatio: formatJsonForTextarea(canonicalModelDefaults.CacheRatio),
      CreateCacheRatio: formatJsonForTextarea(
        canonicalModelDefaults.CreateCacheRatio,
      ),
      CompletionRatio: formatJsonForTextarea(
        canonicalModelDefaults.CompletionRatio,
      ),
      ImageRatio: formatJsonForTextarea(canonicalModelDefaults.ImageRatio),
      AudioRatio: formatJsonForTextarea(canonicalModelDefaults.AudioRatio),
      AudioCompletionRatio: formatJsonForTextarea(
        canonicalModelDefaults.AudioCompletionRatio,
      ),
      BillingMode: formatJsonForTextarea(canonicalModelDefaults.BillingMode),
      BillingExpr: formatJsonForTextarea(canonicalModelDefaults.BillingExpr),
    });
  }, [canonicalModelDefaults, modelForm]);

  useEffect(() => {
    groupNormalizedDefaults.current = {
      GroupRatio: normalizeJsonString(groupDefaults.GroupRatio),
      TopupGroupRatio: normalizeJsonString(groupDefaults.TopupGroupRatio),
      UserUsableGroups: normalizeJsonString(groupDefaults.UserUsableGroups),
      GroupGroupRatio: normalizeJsonString(groupDefaults.GroupGroupRatio),
      AutoGroups: normalizeJsonString(groupDefaults.AutoGroups),
      DefaultUseAutoGroup: groupDefaults.DefaultUseAutoGroup,
      GroupSpecialUsableGroup: normalizeJsonString(
        groupDefaults.GroupSpecialUsableGroup,
      ),
    };

    groupForm.reset({
      ...groupDefaults,
      GroupRatio: formatJsonForTextarea(groupDefaults.GroupRatio),
      TopupGroupRatio: formatJsonForTextarea(groupDefaults.TopupGroupRatio),
      UserUsableGroups: formatJsonForTextarea(groupDefaults.UserUsableGroups),
      GroupGroupRatio: formatJsonForTextarea(groupDefaults.GroupGroupRatio),
      AutoGroups: formatJsonForTextarea(groupDefaults.AutoGroups),
      GroupSpecialUsableGroup: formatJsonForTextarea(
        groupDefaults.GroupSpecialUsableGroup,
      ),
    });
  }, [groupDefaults, groupForm]);

  const saveModelRatios = useCallback(
    async (values: ModelFormValues) => {
      const normalized = normalizeModelFormValues(values);
      const savedByName = new Map(
        buildSnapshotsFromFormValues(modelNormalizedDefaults.current).map(
          (snapshot) => [snapshot.name, snapshot],
        ),
      );
      const nextByName = new Map(
        buildSnapshotsFromFormValues(normalized).map((snapshot) => [
          snapshot.name,
          snapshot,
        ]),
      );
      const modelNames = new Set([...savedByName.keys(), ...nextByName.keys()]);
      const upserts: ModelPricingBatchRequest["upserts"] = [];
      const restore: string[] = [];

      for (const modelName of modelNames) {
        const saved = savedByName.get(modelName);
        const next = nextByName.get(modelName);
        if (!next) {
          if (pricingSources[modelName]?.authority === "manual") {
            restore.push(modelName);
          }
          continue;
        }
        if (getSnapshotSignature(saved) === getSnapshotSignature(next)) {
          continue;
        }
        upserts.push({
          model_name: modelName,
          config: snapshotToModelPricingConfig(next),
        });
      }

      const exposeChanged =
        normalized.ExposeRatioEnabled !==
        modelNormalizedDefaults.current.ExposeRatioEnabled;
      if (upserts.length === 0 && restore.length === 0 && !exposeChanged) {
        toast.info(t("No model price changes to save"));
        return;
      }

      try {
        if (upserts.length > 0 || restore.length > 0) {
          await pricingMutation.mutateAsync({ upserts, restore });
        }
        if (exposeChanged) {
          await updateOption.mutateAsync({
            key: "ExposeRatioEnabled",
            value: normalized.ExposeRatioEnabled,
          });
        }

        modelNormalizedDefaults.current = normalized;
        setSavedModelValues(normalized);
        await queryClient.invalidateQueries({ queryKey: ["model-pricing"] });
        toast.success(t("Setting updated successfully"));
      } catch {
        return;
      }
    },
    [pricingMutation, pricingSources, queryClient, t, updateOption],
  );

  const saveGroupRatios = useCallback(
    async (values: GroupFormValues) => {
      const normalized = {
        GroupRatio: normalizeJsonString(values.GroupRatio),
        TopupGroupRatio: normalizeJsonString(values.TopupGroupRatio),
        UserUsableGroups: normalizeJsonString(values.UserUsableGroups),
        GroupGroupRatio: normalizeJsonString(values.GroupGroupRatio),
        AutoGroups: normalizeJsonString(values.AutoGroups),
        DefaultUseAutoGroup: values.DefaultUseAutoGroup,
        GroupSpecialUsableGroup: normalizeJsonString(
          values.GroupSpecialUsableGroup,
        ),
      };

      // Map form field names to API keys (most are 1:1, except GroupSpecialUsableGroup)
      const apiKeyMap: Record<string, string> = {
        GroupSpecialUsableGroup:
          "group_ratio_setting.group_special_usable_group",
      };

      const updates = (
        Object.keys(normalized) as Array<keyof typeof normalized>
      ).filter(
        (key) => normalized[key] !== groupNormalizedDefaults.current[key],
      );

      for (const key of updates) {
        const apiKey = apiKeyMap[key] || key;
        await updateOption.mutateAsync({ key: apiKey, value: normalized[key] });
      }
    },
    [updateOption],
  );

  const handleResetRatios = useCallback(() => {
    setConfirmOpen(true);
  }, []);

  const { mutate: resetMutate } = resetMutation;
  const handleConfirmReset = useCallback(() => {
    resetMutate();
  }, [resetMutate]);

  const tabLabels: Record<RatioTabId, string> = {
    models: "Model prices",
    groups: "Group ratios",
    "tool-prices": "Tool prices",
    "upstream-sync": "Upstream price sync",
  };
  const tabsGridClass =
    {
      1: "grid-cols-1",
      2: "grid-cols-2",
      3: "grid-cols-3",
      4: "grid-cols-4",
    }[visibleTabs.length] ?? "grid-cols-4";
  const defaultTab = visibleTabs[0] ?? "models";

  const renderTabContent = (tab: RatioTabId) => {
    if (tab === "models" || tab === "upstream-sync") {
      if (pricingQuery.isLoading) {
        return (
          <div className="text-muted-foreground text-sm">{t("Loading...")}</div>
        );
      }
      if (!pricingViews) {
        return (
          <div className="flex items-center gap-3">
            <span className="text-destructive text-sm">
              {t("Failed to load")}
            </span>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={pricingQuery.isFetching}
              onClick={() => pricingQuery.refetch()}
            >
              {t("Retry")}
            </Button>
          </div>
        );
      }
    }
    if (tab === "models") {
      return (
        <ModelRatioForm
          form={modelForm}
          savedValues={savedModelValues}
          pricingSources={pricingSources}
          onSave={saveModelRatios}
          onReset={handleResetRatios}
          isSaving={updateOption.isPending || pricingMutation.isPending}
          isResetting={resetMutation.isPending}
        />
      );
    }
    if (tab === "groups") {
      return (
        <GroupRatioForm
          form={groupForm}
          onSave={saveGroupRatios}
          isSaving={updateOption.isPending}
        />
      );
    }
    if (tab === "tool-prices") {
      return <ToolPriceSettings defaultValue={toolPricesDefault} />;
    }
    return (
      <UpstreamRatioSync
        modelRatios={{
          ModelPrice: savedModelValues.ModelPrice,
          ModelRatio: savedModelValues.ModelRatio,
          CompletionRatio: savedModelValues.CompletionRatio,
          CacheRatio: savedModelValues.CacheRatio,
          CreateCacheRatio: savedModelValues.CreateCacheRatio,
          ImageRatio: savedModelValues.ImageRatio,
          AudioRatio: savedModelValues.AudioRatio,
          AudioCompletionRatio: savedModelValues.AudioCompletionRatio,
          "billing_setting.billing_mode": savedModelValues.BillingMode,
          "billing_setting.billing_expr": savedModelValues.BillingExpr,
        }}
      />
    );
  };

  const renderTabSwitcher = () => (
    <TabsList className={`grid w-fit max-w-full ${tabsGridClass}`}>
      {visibleTabs.map((tab) => (
        <TabsTrigger key={tab} value={tab}>
          {t(tabLabels[tab])}
        </TabsTrigger>
      ))}
    </TabsList>
  );

  return (
    <>
      {visibleTabs.length === 1 ? (
        <SettingsSection title={t(titleKey)}>
          {renderTabContent(defaultTab)}
        </SettingsSection>
      ) : (
        <Tabs defaultValue={defaultTab} className="space-y-6">
          <SettingsPageTitleStatusPortal>
            {renderTabSwitcher()}
          </SettingsPageTitleStatusPortal>

          <SettingsSection title={t(titleKey)}>
            {visibleTabs.map((tab) => (
              <TabsContent key={tab} value={tab}>
                {renderTabContent(tab)}
              </TabsContent>
            ))}
          </SettingsSection>
        </Tabs>
      )}

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t("Calibrate model prices?")}
        desc={t(
          "Refresh official prices and reset fallback defaults. Manual prices are preserved.",
        )}
        destructive
        isLoading={resetMutation.isPending}
        handleConfirm={handleConfirmReset}
        confirmText={t("Calibrate")}
      />
    </>
  );
}
