package model

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"gorm.io/gorm"
)

const (
	ModelPricingModePerToken   = "per-token"
	ModelPricingModePerRequest = "per-request"
	ModelPricingModeTieredExpr = "tiered_expr"
)

type ModelPricingConfig struct {
	Mode                 string   `json:"mode,omitempty"`
	Price                *float64 `json:"price,omitempty"`
	Ratio                *float64 `json:"ratio,omitempty"`
	CompletionRatio      *float64 `json:"completion_ratio,omitempty"`
	CacheRatio           *float64 `json:"cache_ratio,omitempty"`
	CreateCacheRatio     *float64 `json:"create_cache_ratio,omitempty"`
	ImageRatio           *float64 `json:"image_ratio,omitempty"`
	AudioRatio           *float64 `json:"audio_ratio,omitempty"`
	AudioCompletionRatio *float64 `json:"audio_completion_ratio,omitempty"`
	BillingExpr          string   `json:"billing_expr,omitempty"`
}

func ParseModelPricingConfig(raw string) (ModelPricingConfig, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return ModelPricingConfig{}, false, nil
	}

	var cfg ModelPricingConfig
	if err := common.UnmarshalJsonStr(raw, &cfg); err != nil {
		return ModelPricingConfig{}, false, err
	}
	cfg.Mode = strings.TrimSpace(cfg.Mode)
	if cfg.Mode == "" {
		cfg.Mode = inferModelPricingMode(cfg)
	}
	if cfg.Mode == "" || cfg.isEmpty() {
		return ModelPricingConfig{}, false, nil
	}
	if !isSupportedModelPricingMode(cfg.Mode) {
		return ModelPricingConfig{}, false, fmt.Errorf("unsupported model pricing mode: %s", cfg.Mode)
	}
	if err := validateModelPricingNumbers(cfg); err != nil {
		return ModelPricingConfig{}, false, err
	}
	if cfg.Mode == ModelPricingModePerRequest && cfg.Price == nil {
		return ModelPricingConfig{}, false, fmt.Errorf("per-request pricing requires price")
	}
	if cfg.Mode == ModelPricingModeTieredExpr && strings.TrimSpace(cfg.BillingExpr) == "" {
		return ModelPricingConfig{}, false, fmt.Errorf("tiered_expr pricing requires billing_expr")
	}
	return cfg, true, nil
}

func ValidateModelPricingConfig(raw string) error {
	_, _, err := ParseModelPricingConfig(raw)
	return err
}

func inferModelPricingMode(cfg ModelPricingConfig) string {
	if cfg.Price != nil {
		return ModelPricingModePerRequest
	}
	if strings.TrimSpace(cfg.BillingExpr) != "" {
		return ModelPricingModeTieredExpr
	}
	if cfg.Ratio != nil || cfg.CompletionRatio != nil || cfg.CacheRatio != nil || cfg.CreateCacheRatio != nil || cfg.ImageRatio != nil || cfg.AudioRatio != nil || cfg.AudioCompletionRatio != nil {
		return ModelPricingModePerToken
	}
	return ""
}

func isSupportedModelPricingMode(mode string) bool {
	switch mode {
	case ModelPricingModePerToken, ModelPricingModePerRequest, ModelPricingModeTieredExpr:
		return true
	default:
		return false
	}
}

func (cfg ModelPricingConfig) isEmpty() bool {
	return cfg.Price == nil && cfg.Ratio == nil && cfg.CompletionRatio == nil && cfg.CacheRatio == nil && cfg.CreateCacheRatio == nil && cfg.ImageRatio == nil && cfg.AudioRatio == nil && cfg.AudioCompletionRatio == nil && strings.TrimSpace(cfg.BillingExpr) == ""
}

func validateModelPricingNumbers(cfg ModelPricingConfig) error {
	fields := map[string]*float64{
		"price":                  cfg.Price,
		"ratio":                  cfg.Ratio,
		"completion_ratio":       cfg.CompletionRatio,
		"cache_ratio":            cfg.CacheRatio,
		"create_cache_ratio":     cfg.CreateCacheRatio,
		"image_ratio":            cfg.ImageRatio,
		"audio_ratio":            cfg.AudioRatio,
		"audio_completion_ratio": cfg.AudioCompletionRatio,
	}
	for field, value := range fields {
		if value == nil {
			continue
		}
		if math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 {
			return fmt.Errorf("invalid %s", field)
		}
	}
	return nil
}

func (cfg ModelPricingConfig) metadataValues() (ratio_setting.ModelMetadataPricingValues, bool) {
	if cfg.Mode == ModelPricingModePerRequest {
		return ratio_setting.ModelMetadataPricingValues{ModelPrice: cfg.Price}, cfg.Price != nil
	}
	values := ratio_setting.ModelMetadataPricingValues{
		ModelRatio:           cfg.Ratio,
		CompletionRatio:      cfg.CompletionRatio,
		CacheRatio:           cfg.CacheRatio,
		CreateCacheRatio:     cfg.CreateCacheRatio,
		ImageRatio:           cfg.ImageRatio,
		AudioRatio:           cfg.AudioRatio,
		AudioCompletionRatio: cfg.AudioCompletionRatio,
	}
	if cfg.Mode == ModelPricingModeTieredExpr {
		values.BillingMode = ModelPricingModeTieredExpr
		values.BillingExpr = cfg.BillingExpr
	}
	return values, !cfg.isEmpty()
}

func MigrateModelPricingOverridesFromLegacy() error {
	completed, err := modelPricingOverrideMigrationCompleted()
	if err != nil || completed {
		return err
	}

	officialRows, err := GetActiveOfficialPricingRows()
	if err != nil {
		return err
	}
	hasOfficialSnapshot := len(officialRows) > 0
	officialByName := make(map[string]ModelPricingConfig, len(officialRows))
	for _, row := range officialRows {
		officialByName[row.ModelName] = ModelPricingConfigFromOfficialPrice(row)
	}

	var legacy legacyModelPricingOptions
	if hasOfficialSnapshot {
		legacy, err = loadLegacyModelPricingOptions()
		if err != nil {
			return err
		}
	}
	var manualModels []Model
	if err := DB.Where("sync_official = ?", 0).Find(&manualModels).Error; err != nil {
		return err
	}
	existingRows, err := GetAllModelPricingOverrides()
	if err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(existingRows))
	for _, item := range existingRows {
		existing[item.ModelName] = struct{}{}
	}

	migrated := make([]string, 0)
	err = DB.Transaction(func(tx *gorm.DB) error {
		for _, item := range manualModels {
			if _, ok := existing[item.ModelName]; ok {
				continue
			}
			cfg, ok, err := ParseModelPricingConfig(item.PricingConfig)
			if err != nil {
				return fmt.Errorf("parse manual model pricing for %s: %w", item.ModelName, err)
			}
			if !ok {
				continue
			}
			cfg = MergeModelPricingConfig(officialByName[item.ModelName], cfg)
			if err := UpsertModelPricingOverrideTx(tx, item.ModelName, cfg, ModelPricingOverrideOriginModelMetadata); err != nil {
				return err
			}
			existing[item.ModelName] = struct{}{}
			migrated = append(migrated, item.ModelName)
		}

		if hasOfficialSnapshot {
			for _, modelName := range legacy.modelNames() {
				if _, ok := existing[modelName]; ok {
					continue
				}
				legacyCfg, ok := legacy.configForModel(modelName)
				if !ok {
					continue
				}
				officialCfg, hasOfficial := officialByName[modelName]
				if hasOfficial && pricingConfigMatchesDefinedFields(legacyCfg, officialCfg) {
					continue
				}
				legacyCfg = MergeModelPricingConfig(officialCfg, legacyCfg)
				if err := UpsertModelPricingOverrideTx(tx, modelName, legacyCfg, ModelPricingOverrideOriginLegacyOption); err != nil {
					return err
				}
				existing[modelName] = struct{}{}
				migrated = append(migrated, modelName)
			}
			if err := markModelPricingOverrideMigrationCompletedTx(tx); err != nil {
				return err
			}
		}
		if hasOfficialSnapshot || len(migrated) > 0 {
			_, err = BumpPricingRuntimeRevisionTx(tx)
		}
		return err
	})
	if err != nil {
		return err
	}
	if len(migrated) > 0 {
		common.SysLog(fmt.Sprintf("migrated manual model pricing overrides: count=%d models=%s", len(migrated), strings.Join(migrated, ",")))
	}
	return nil
}

func ModelPricingConfigFromOfficialPrice(row OfficialModelPrice) ModelPricingConfig {
	ratio := row.ModelRatio
	completionRatio := row.CompletionRatio
	return ModelPricingConfig{
		Mode:             ModelPricingModePerToken,
		Ratio:            &ratio,
		CompletionRatio:  &completionRatio,
		CacheRatio:       cloneFloatPointer(row.CacheRatio),
		CreateCacheRatio: cloneFloatPointer(row.CreateCacheRatio),
	}
}

func MergeModelPricingConfig(base ModelPricingConfig, patch ModelPricingConfig) ModelPricingConfig {
	if patch.Mode == ModelPricingModePerRequest {
		return patch
	}
	if patch.Mode != "" && patch.Mode != base.Mode {
		base = ModelPricingConfig{Mode: patch.Mode}
	}
	if patch.Price != nil {
		base.Price = cloneFloatPointer(patch.Price)
	}
	if patch.Ratio != nil {
		base.Ratio = cloneFloatPointer(patch.Ratio)
	}
	if patch.CompletionRatio != nil {
		base.CompletionRatio = cloneFloatPointer(patch.CompletionRatio)
	}
	if patch.CacheRatio != nil {
		base.CacheRatio = cloneFloatPointer(patch.CacheRatio)
	}
	if patch.CreateCacheRatio != nil {
		base.CreateCacheRatio = cloneFloatPointer(patch.CreateCacheRatio)
	}
	if patch.ImageRatio != nil {
		base.ImageRatio = cloneFloatPointer(patch.ImageRatio)
	}
	if patch.AudioRatio != nil {
		base.AudioRatio = cloneFloatPointer(patch.AudioRatio)
	}
	if patch.AudioCompletionRatio != nil {
		base.AudioCompletionRatio = cloneFloatPointer(patch.AudioCompletionRatio)
	}
	if strings.TrimSpace(patch.BillingExpr) != "" {
		base.BillingExpr = patch.BillingExpr
	}
	return base
}

func pricingConfigMatchesDefinedFields(expected ModelPricingConfig, actual ModelPricingConfig) bool {
	if expected.Mode == ModelPricingModePerRequest || expected.Mode == ModelPricingModeTieredExpr {
		return false
	}
	return equalOptionalFloat(expected.Ratio, actual.Ratio) &&
		equalOptionalFloat(expected.CompletionRatio, actual.CompletionRatio) &&
		equalOptionalFloat(expected.CacheRatio, actual.CacheRatio) &&
		equalOptionalFloat(expected.CreateCacheRatio, actual.CreateCacheRatio) &&
		equalOptionalFloat(expected.ImageRatio, actual.ImageRatio) &&
		equalOptionalFloat(expected.AudioRatio, actual.AudioRatio) &&
		equalOptionalFloat(expected.AudioCompletionRatio, actual.AudioCompletionRatio)
}

func equalOptionalFloat(expected *float64, actual *float64) bool {
	return expected == nil || actual != nil && *expected == *actual
}

func cloneFloatPointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

type legacyModelPricingOptions struct {
	price                map[string]float64
	ratio                map[string]float64
	completionRatio      map[string]float64
	cacheRatio           map[string]float64
	createCacheRatio     map[string]float64
	imageRatio           map[string]float64
	audioRatio           map[string]float64
	audioCompletionRatio map[string]float64
	billingMode          map[string]string
	billingExpr          map[string]string
}

func (legacy legacyModelPricingOptions) empty() bool {
	return len(legacy.price) == 0 && len(legacy.ratio) == 0 && len(legacy.completionRatio) == 0 && len(legacy.cacheRatio) == 0 && len(legacy.createCacheRatio) == 0 && len(legacy.imageRatio) == 0 && len(legacy.audioRatio) == 0 && len(legacy.audioCompletionRatio) == 0 && len(legacy.billingMode) == 0 && len(legacy.billingExpr) == 0
}

func (legacy legacyModelPricingOptions) modelNames() []string {
	names := make(map[string]struct{})
	for _, values := range []map[string]float64{
		legacy.price,
		legacy.ratio,
		legacy.completionRatio,
		legacy.cacheRatio,
		legacy.createCacheRatio,
		legacy.imageRatio,
		legacy.audioRatio,
		legacy.audioCompletionRatio,
	} {
		for modelName := range values {
			names[modelName] = struct{}{}
		}
	}
	for modelName := range legacy.billingMode {
		names[modelName] = struct{}{}
	}
	for modelName := range legacy.billingExpr {
		names[modelName] = struct{}{}
	}
	result := make([]string, 0, len(names))
	for modelName := range names {
		result = append(result, modelName)
	}
	sort.Strings(result)
	return result
}

func (legacy legacyModelPricingOptions) configForModel(modelName string) (ModelPricingConfig, bool) {
	if price, ok := legacy.price[modelName]; ok {
		return ModelPricingConfig{
			Mode:  ModelPricingModePerRequest,
			Price: &price,
		}, true
	}

	cfg := ModelPricingConfig{Mode: ModelPricingModePerToken}
	cfg.Ratio = floatPointerFromMap(legacy.ratio, modelName)
	cfg.CompletionRatio = floatPointerFromMap(legacy.completionRatio, modelName)
	cfg.CacheRatio = floatPointerFromMap(legacy.cacheRatio, modelName)
	cfg.CreateCacheRatio = floatPointerFromMap(legacy.createCacheRatio, modelName)
	cfg.ImageRatio = floatPointerFromMap(legacy.imageRatio, modelName)
	cfg.AudioRatio = floatPointerFromMap(legacy.audioRatio, modelName)
	cfg.AudioCompletionRatio = floatPointerFromMap(legacy.audioCompletionRatio, modelName)
	mode := strings.TrimSpace(legacy.billingMode[modelName])
	expr := legacy.billingExpr[modelName]
	if mode == ModelPricingModeTieredExpr && strings.TrimSpace(expr) != "" {
		cfg.Mode = ModelPricingModeTieredExpr
		cfg.BillingExpr = expr
	}
	if cfg.isEmpty() {
		return ModelPricingConfig{}, false
	}
	return cfg, true
}

func floatPointerFromMap(values map[string]float64, key string) *float64 {
	value, ok := values[key]
	if !ok {
		return nil
	}
	return &value
}

func loadLegacyModelPricingOptions() (legacyModelPricingOptions, error) {
	keys := []string{
		"ModelPrice",
		"ModelRatio",
		"CompletionRatio",
		"CacheRatio",
		"CreateCacheRatio",
		"ImageRatio",
		"AudioRatio",
		"AudioCompletionRatio",
	}
	var options []Option
	if err := DB.Where("key IN ?", keys).Find(&options).Error; err != nil {
		return legacyModelPricingOptions{}, err
	}
	values := make(map[string]string, len(options))
	for _, option := range options {
		values[option.Key] = option.Value
	}
	return legacyModelPricingOptions{
		price:                parseLegacyFloatMap(values["ModelPrice"]),
		ratio:                parseLegacyFloatMap(values["ModelRatio"]),
		completionRatio:      parseLegacyFloatMap(values["CompletionRatio"]),
		cacheRatio:           parseLegacyFloatMap(values["CacheRatio"]),
		createCacheRatio:     parseLegacyFloatMap(values["CreateCacheRatio"]),
		imageRatio:           parseLegacyFloatMap(values["ImageRatio"]),
		audioRatio:           parseLegacyFloatMap(values["AudioRatio"]),
		audioCompletionRatio: parseLegacyFloatMap(values["AudioCompletionRatio"]),
		billingMode:          billing_setting.GetBillingModeCopy(),
		billingExpr:          billing_setting.GetBillingExprCopy(),
	}, nil
}

func GetLegacyModelPricingConfigs() (map[string]ModelPricingConfig, error) {
	legacy := legacyModelPricingOptions{
		price:                ratio_setting.GetModelPriceCopy(),
		ratio:                ratio_setting.GetModelRatioCopy(),
		completionRatio:      ratio_setting.GetCompletionRatioCopy(),
		cacheRatio:           ratio_setting.GetCacheRatioCopy(),
		createCacheRatio:     ratio_setting.GetCreateCacheRatioCopy(),
		imageRatio:           ratio_setting.GetImageRatioCopy(),
		audioRatio:           ratio_setting.GetAudioRatioCopy(),
		audioCompletionRatio: ratio_setting.GetAudioCompletionRatioCopy(),
		billingMode:          billing_setting.GetBillingModeCopy(),
		billingExpr:          billing_setting.GetBillingExprCopy(),
	}
	configs := make(map[string]ModelPricingConfig)
	for _, modelName := range legacy.modelNames() {
		if cfg, ok := legacy.configForModel(modelName); ok {
			configs[modelName] = cfg
		}
	}
	return configs, nil
}

func ValidateModelPricingConfigValue(cfg ModelPricingConfig) error {
	payload, err := common.Marshal(cfg)
	if err != nil {
		return err
	}
	_, ok, err := ParseModelPricingConfig(string(payload))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("model pricing config is empty")
	}
	return nil
}

func parseLegacyFloatMap(raw string) map[string]float64 {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	values := make(map[string]float64)
	if err := common.UnmarshalJsonStr(raw, &values); err != nil {
		return nil
	}
	return values
}
