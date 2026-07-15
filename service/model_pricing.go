package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"gorm.io/gorm"
)

const maxModelPricingMutations = 1000

type ModelPricingView struct {
	ModelName               string                    `json:"model_name"`
	Authority               model.AuthorityLevel      `json:"authority"`
	AuthorityModelName      string                    `json:"authority_model_name"`
	EffectiveConfig         model.ModelPricingConfig  `json:"effective_config"`
	ManualConfig            *model.ModelPricingConfig `json:"manual_config,omitempty"`
	ManualOrigin            string                    `json:"manual_origin,omitempty"`
	OfficialConfig          *model.ModelPricingConfig `json:"official_config,omitempty"`
	OfficialConfigHash      string                    `json:"official_config_hash,omitempty"`
	OfficialStale           bool                      `json:"official_stale"`
	OfficialLastConfirmedAt int64                     `json:"official_last_confirmed_at"`
	PricingConflict         bool                      `json:"pricing_conflict"`
}

type ModelPricingMutation struct {
	ModelName string                   `json:"model_name"`
	Config    model.ModelPricingConfig `json:"config"`
}

type ModelPricingAcknowledgement struct {
	ModelName          string `json:"model_name"`
	OfficialConfigHash string `json:"official_config_hash"`
}

type ModelPricingBatchRequest struct {
	Upserts     []ModelPricingMutation        `json:"upserts"`
	Restore     []string                      `json:"restore"`
	Acknowledge []ModelPricingAcknowledgement `json:"acknowledge"`
}

type ModelPricingBatchResult struct {
	Updated      int   `json:"updated"`
	Restored     int   `json:"restored"`
	Acknowledged int   `json:"acknowledged"`
	Revision     int64 `json:"revision"`
}

func expandPricingAliases[T any](values map[string]T, names map[string]struct{}) {
	type namedValue struct {
		name  string
		value T
	}
	originals := make([]namedValue, 0, len(values))
	for name, value := range values {
		originals = append(originals, namedValue{name: name, value: value})
	}
	for _, item := range originals {
		for _, alias := range ratio_setting.PricingAliases(item.name) {
			if _, exists := values[alias]; exists {
				continue
			}
			values[alias] = item.value
			names[alias] = struct{}{}
		}
	}
}

func lookupPricingValue[T any](name string, values map[string]T) (T, bool) {
	for _, candidate := range ratio_setting.PricingLookupNames(name) {
		if value, ok := values[candidate]; ok {
			return value, true
		}
	}
	var zero T
	return zero, false
}

func ListModelPricing(_ context.Context) ([]ModelPricingView, error) {
	overrides, err := model.GetAllModelPricingOverrides()
	if err != nil {
		return nil, err
	}
	officialRows, err := model.GetActiveOfficialPricingRows()
	if err != nil {
		return nil, err
	}
	fallbackConfigs, err := model.GetLegacyModelPricingConfigs()
	if err != nil {
		return nil, err
	}

	overrideByName := make(map[string]model.ModelPricingOverride, len(overrides))
	names := make(map[string]struct{}, len(overrides)+len(officialRows)+len(fallbackConfigs))
	for _, item := range overrides {
		overrideByName[item.ModelName] = item
		names[item.ModelName] = struct{}{}
	}
	officialByName := make(map[string]model.OfficialModelPrice, len(officialRows))
	for _, row := range officialRows {
		officialByName[row.ModelName] = row
		names[row.ModelName] = struct{}{}
	}
	for modelName := range fallbackConfigs {
		names[modelName] = struct{}{}
	}
	expandPricingAliases(overrideByName, names)
	expandPricingAliases(officialByName, names)

	modelNames := make([]string, 0, len(names))
	for modelName := range names {
		modelNames = append(modelNames, modelName)
	}
	sort.Strings(modelNames)

	views := make([]ModelPricingView, 0, len(modelNames))
	for _, modelName := range modelNames {
		view := ModelPricingView{ModelName: modelName, Authority: model.AuthorityLevelFallback, AuthorityModelName: modelName}
		if row, ok := lookupPricingValue(modelName, officialByName); ok {
			cfg := model.ModelPricingConfigFromOfficialPrice(row)
			officialHash, err := model.ModelPricingConfigHash(cfg)
			if err != nil {
				return nil, fmt.Errorf("hash official pricing for %s: %w", row.ModelName, err)
			}
			view.Authority = model.AuthorityLevelOfficial
			view.AuthorityModelName = row.ModelName
			view.EffectiveConfig = cfg
			view.OfficialConfig = &cfg
			view.OfficialConfigHash = officialHash
			view.OfficialStale = row.Stale
			view.OfficialLastConfirmedAt = row.LastConfirmedAt
		} else if cfg, ok := lookupPricingValue(modelName, fallbackConfigs); ok {
			view.EffectiveConfig = cfg
		}
		if item, ok := lookupPricingValue(modelName, overrideByName); ok {
			cfg, valid, err := model.ParseModelPricingConfig(item.PricingConfig)
			if err != nil {
				return nil, fmt.Errorf("parse manual pricing override for %s: %w", modelName, err)
			}
			if valid {
				view.Authority = model.AuthorityLevelManual
				view.EffectiveConfig = cfg
				view.AuthorityModelName = item.ModelName
				view.ManualConfig = &cfg
				view.ManualOrigin = item.Origin
				view.PricingConflict = item.ModelName == modelName &&
					view.OfficialConfig != nil &&
					!model.ModelPricingConfigsEqual(cfg, *view.OfficialConfig) &&
					item.ReviewedOfficialHash != view.OfficialConfigHash
			}
		}
		views = append(views, view)
	}
	return views, nil
}

func SaveModelPricingBatch(_ context.Context, request ModelPricingBatchRequest) (*ModelPricingBatchResult, error) {
	var result *ModelPricingBatchResult
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = SaveModelPricingBatchTx(tx, request)
		return err
	})
	if err != nil {
		return nil, err
	}
	if result.Updated == 0 && result.Restored == 0 {
		return result, nil
	}
	if err := RefreshModelPricingRuntime(); err != nil {
		return result, fmt.Errorf("model pricing was committed, but runtime refresh failed; automatic retry is pending: %w", err)
	}
	return result, nil
}

func SaveModelPricingBatchTx(tx *gorm.DB, request ModelPricingBatchRequest) (*ModelPricingBatchResult, error) {
	if len(request.Upserts)+len(request.Restore)+len(request.Acknowledge) > maxModelPricingMutations {
		return nil, fmt.Errorf("too many model pricing mutations")
	}

	operationNames := make(map[string]struct{}, len(request.Upserts)+len(request.Restore)+len(request.Acknowledge))
	for i := range request.Upserts {
		request.Upserts[i].ModelName = strings.TrimSpace(request.Upserts[i].ModelName)
		if err := addPricingOperationName(operationNames, request.Upserts[i].ModelName); err != nil {
			return nil, err
		}
	}
	request.Restore = normalizePricingModelNames(request.Restore)
	for _, modelName := range request.Restore {
		if err := addPricingOperationName(operationNames, modelName); err != nil {
			return nil, err
		}
	}
	for i := range request.Acknowledge {
		request.Acknowledge[i].ModelName = strings.TrimSpace(request.Acknowledge[i].ModelName)
		request.Acknowledge[i].OfficialConfigHash = strings.TrimSpace(request.Acknowledge[i].OfficialConfigHash)
		if request.Acknowledge[i].OfficialConfigHash == "" {
			return nil, fmt.Errorf("official pricing hash is required for %s", request.Acknowledge[i].ModelName)
		}
		if err := addPricingOperationName(operationNames, request.Acknowledge[i].ModelName); err != nil {
			return nil, err
		}
	}

	officialByName := make(map[string]model.ModelPricingConfig)
	if len(request.Upserts) > 0 || len(request.Acknowledge) > 0 {
		var officialRows []model.OfficialModelPrice
		if err := tx.Where("active = ?", true).Find(&officialRows).Error; err != nil {
			return nil, err
		}
		names := make(map[string]struct{}, len(officialRows))
		for _, row := range officialRows {
			officialByName[row.ModelName] = model.ModelPricingConfigFromOfficialPrice(row)
			names[row.ModelName] = struct{}{}
		}
		expandPricingAliases(officialByName, names)
	}

	result := &ModelPricingBatchResult{}
	for _, mutation := range request.Upserts {
		cfg := mutation.Config
		if err := model.ValidateModelPricingConfigValue(cfg); err != nil {
			return nil, fmt.Errorf("invalid pricing for %s: %w", mutation.ModelName, err)
		}
		if err := model.UpsertModelPricingOverrideTx(tx, mutation.ModelName, cfg, model.ModelPricingOverrideOriginAdmin); err != nil {
			return nil, err
		}
		if officialCfg, ok := lookupPricingValue(mutation.ModelName, officialByName); ok {
			officialHash, err := model.ModelPricingConfigHash(officialCfg)
			if err != nil {
				return nil, err
			}
			if err := model.ReviewModelPricingOverrideTx(tx, mutation.ModelName, officialHash); err != nil {
				return nil, err
			}
		}
		result.Updated++
	}

	for _, acknowledgement := range request.Acknowledge {
		var override model.ModelPricingOverride
		if err := tx.Where("model_name = ?", acknowledgement.ModelName).First(&override).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, fmt.Errorf("manual pricing override not found for %s", acknowledgement.ModelName)
			}
			return nil, err
		}
		officialCfg, ok := lookupPricingValue(acknowledgement.ModelName, officialByName)
		if !ok {
			return nil, fmt.Errorf("official pricing not found for %s", acknowledgement.ModelName)
		}
		currentHash, err := model.ModelPricingConfigHash(officialCfg)
		if err != nil {
			return nil, err
		}
		if currentHash != acknowledgement.OfficialConfigHash {
			return nil, fmt.Errorf("official pricing changed for %s; refresh and review the latest price", acknowledgement.ModelName)
		}
		if override.ReviewedOfficialHash == currentHash {
			continue
		}
		if err := model.ReviewModelPricingOverrideTx(tx, acknowledgement.ModelName, currentHash); err != nil {
			return nil, err
		}
		result.Acknowledged++
	}

	restored, err := model.DeleteModelPricingOverridesTx(tx, request.Restore)
	if err != nil {
		return nil, err
	}
	result.Restored = int(restored)
	if result.Updated == 0 && result.Restored == 0 {
		return result, nil
	}
	revision, err := model.BumpPricingRuntimeRevisionTx(tx)
	if err != nil {
		return nil, err
	}
	result.Revision = revision
	return result, nil
}

func addPricingOperationName(names map[string]struct{}, modelName string) error {
	if modelName == "" {
		return fmt.Errorf("model name is required")
	}
	if _, exists := names[modelName]; exists {
		return fmt.Errorf("duplicate model pricing operation: %s", modelName)
	}
	names[modelName] = struct{}{}
	return nil
}

func RefreshModelPricingRuntime() error {
	return model.RefreshPricingRuntime()
}

func ResetFallbackModelPricing() error {
	return RefreshModelPricingRuntime()
}

func normalizePricingModelNames(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
