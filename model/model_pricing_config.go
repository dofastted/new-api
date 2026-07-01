package model

import (
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
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

func LoadModelPricingConfigsIntoRuntime() error {
	var models []Model
	if err := DB.Select("model_name", "pricing_config").Find(&models).Error; err != nil {
		return err
	}

	entries := make(map[string]ratio_setting.ModelMetadataPricingValues)
	for _, item := range models {
		cfg, ok, err := ParseModelPricingConfig(item.PricingConfig)
		if err != nil {
			return fmt.Errorf("parse pricing_config for %s: %w", item.ModelName, err)
		}
		if !ok {
			continue
		}
		values, hasValues := cfg.metadataValues()
		if hasValues {
			entries[item.ModelName] = values
		}
	}
	ratio_setting.ReplaceModelMetadataPricing(entries)
	return nil
}

func MigrateModelPricingConfigFromLegacyOptions() error {
	legacy, err := loadLegacyModelPricingOptions()
	if err != nil {
		return err
	}
	if legacy.empty() {
		return nil
	}

	var models []Model
	if err := DB.Where("(pricing_config = ? OR pricing_config IS NULL) AND sync_official = ?", "", 0).Find(&models).Error; err != nil {
		return err
	}
	for _, item := range models {
		cfg, ok := legacy.configForModel(item.ModelName)
		if !ok {
			continue
		}
		payload, err := common.Marshal(cfg)
		if err != nil {
			return err
		}
		if err := DB.Model(&Model{}).Where("id = ? AND (pricing_config = ? OR pricing_config IS NULL)", item.Id, "").Update("pricing_config", string(payload)).Error; err != nil {
			return err
		}
	}
	return nil
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

func (legacy legacyModelPricingOptions) configForModel(modelName string) (ModelPricingConfig, bool) {
	if price, ok := legacy.price[modelName]; ok {
		return ModelPricingConfig{
			Mode:  ModelPricingModePerRequest,
			Price: &price,
		}, true
	}

	mode := strings.TrimSpace(legacy.billingMode[modelName])
	expr := legacy.billingExpr[modelName]
	if mode == ModelPricingModeTieredExpr && strings.TrimSpace(expr) != "" {
		return ModelPricingConfig{
			Mode:        ModelPricingModeTieredExpr,
			BillingExpr: expr,
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
