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
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

import type { ModelPricingConfig, ModelPricingView } from "../types";

type OfficialPricingConflictReviewProps = {
  conflicts: ModelPricingView[];
  onKeepManual: (view: ModelPricingView) => Promise<void>;
  onUseOfficial: (view: ModelPricingView) => Promise<void>;
};

type PricingConfigField = keyof ModelPricingConfig;

const pricingFields: Array<{ key: PricingConfigField; label: string }> = [
  { key: "mode", label: "Mode" },
  { key: "price", label: "Fixed price" },
  { key: "ratio", label: "Input ratio" },
  { key: "completion_ratio", label: "Completion ratio" },
  { key: "cache_ratio", label: "Cache ratio" },
  { key: "create_cache_ratio", label: "Create cache ratio" },
  { key: "image_ratio", label: "Image ratio" },
  { key: "audio_ratio", label: "Audio ratio" },
  { key: "audio_completion_ratio", label: "Audio completion ratio" },
  { key: "billing_expr", label: "Billing expression" },
];

const pricingValue = (config: ModelPricingConfig, key: PricingConfigField) => {
  const value = config[key];
  if (value === undefined || value === "") return null;
  return String(value);
};

export function OfficialPricingConflictReview({
  conflicts,
  onKeepManual,
  onUseOfficial,
}: OfficialPricingConflictReviewProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [selectedModel, setSelectedModel] = useState("");
  const [submitting, setSubmitting] = useState<"manual" | "official" | null>(
    null,
  );

  const selected =
    conflicts.find((view) => view.model_name === selectedModel) ?? conflicts[0];

  useEffect(() => {
    if (conflicts.length === 0) {
      setOpen(false);
      setSelectedModel("");
      return;
    }
    if (!conflicts.some((view) => view.model_name === selectedModel)) {
      setSelectedModel(conflicts[0].model_name);
    }
  }, [conflicts, selectedModel]);

  const differences = useMemo(() => {
    const manualConfig = selected?.manual_config;
    const officialConfig = selected?.official_config;
    if (!manualConfig || !officialConfig) return [];
    return pricingFields.flatMap((field) => {
      const manual = pricingValue(manualConfig, field.key);
      const official = pricingValue(officialConfig, field.key);
      if (manual === official) return [];
      return [{ ...field, manual, official }];
    });
  }, [selected]);

  if (conflicts.length === 0 || !selected) return null;

  const runAction = async (action: "manual" | "official") => {
    setSubmitting(action);
    try {
      if (action === "manual") {
        await onKeepManual(selected);
      } else {
        await onUseOfficial(selected);
      }
      setOpen(false);
    } catch {
      // The mutation callback reports the error; keep the dialog open for retry.
    } finally {
      setSubmitting(null);
    }
  };

  return (
    <>
      <Alert variant="destructive">
        <AlertTitle>{t("Official price changes need review")}</AlertTitle>
        <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
          <span>
            {t(
              "{{count}} manual model prices differ from the latest official prices.",
              { count: conflicts.length },
            )}
          </span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setOpen(true)}
          >
            {t("Review price changes")}
          </Button>
        </AlertDescription>
      </Alert>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-4xl">
          <DialogHeader>
            <DialogTitle>{t("Review official price change")}</DialogTitle>
            <DialogDescription>
              {t("Manual price remains active until you choose.")}
            </DialogDescription>
          </DialogHeader>

          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="font-mono text-sm font-medium">
              {selected.model_name}
            </div>
            {conflicts.length > 1 && (
              <select
                className="border-input bg-background rounded-md border px-2 py-1 text-sm"
                value={selected.model_name}
                onChange={(event) => setSelectedModel(event.target.value)}
                aria-label={t("Select model price conflict")}
              >
                {conflicts.map((view) => (
                  <option key={view.model_name} value={view.model_name}>
                    {view.model_name}
                  </option>
                ))}
              </select>
            )}
          </div>

          <div className="overflow-x-auto rounded-lg border">
            <table className="w-full text-left text-sm">
              <thead className="bg-muted/50">
                <tr>
                  <th className="px-3 py-2 font-medium">
                    {t("Pricing field")}
                  </th>
                  <th className="px-3 py-2 font-medium">{t("Manual value")}</th>
                  <th className="px-3 py-2 font-medium">
                    {t("Official value")}
                  </th>
                </tr>
              </thead>
              <tbody>
                {differences.map((difference) => (
                  <tr key={difference.key} className="border-t align-top">
                    <td className="px-3 py-2">{t(difference.label)}</td>
                    <td className="px-3 py-2 font-mono whitespace-pre-wrap">
                      {difference.manual ?? t("No value")}
                    </td>
                    <td className="px-3 py-2 font-mono whitespace-pre-wrap">
                      {difference.official ?? t("No value")}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="grid gap-3 md:grid-cols-2">
            <div>
              <div className="mb-1 text-sm font-medium">
                {t("Complete manual configuration")}
              </div>
              <pre className="bg-muted max-h-48 overflow-auto rounded-lg p-3 text-xs">
                {JSON.stringify(selected.manual_config, null, 2)}
              </pre>
            </div>
            <div>
              <div className="mb-1 text-sm font-medium">
                {t("Complete official configuration")}
              </div>
              <pre className="bg-muted max-h-48 overflow-auto rounded-lg p-3 text-xs">
                {JSON.stringify(selected.official_config, null, 2)}
              </pre>
            </div>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={submitting !== null}
              onClick={() => runAction("manual")}
            >
              {submitting === "manual"
                ? t("Keeping manual price...")
                : t("Keep manual price")}
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={submitting !== null}
              onClick={() => runAction("official")}
            >
              {submitting === "official"
                ? t("Switching to official price...")
                : t("Use official price")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
