package ratio_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/types"
)

type ModelMetadataPricingValues struct {
	ModelPrice           *float64
	ModelRatio           *float64
	CompletionRatio      *float64
	CacheRatio           *float64
	CreateCacheRatio     *float64
	ImageRatio           *float64
	AudioRatio           *float64
	AudioCompletionRatio *float64
	BillingMode          string
	BillingExpr          string
}

var (
	metadataModelPriceMap           = types.NewRWMap[string, float64]()
	metadataModelRatioMap           = types.NewRWMap[string, float64]()
	metadataCompletionRatioMap      = types.NewRWMap[string, float64]()
	metadataCacheRatioMap           = types.NewRWMap[string, float64]()
	metadataCreateCacheRatioMap     = types.NewRWMap[string, float64]()
	metadataImageRatioMap           = types.NewRWMap[string, float64]()
	metadataAudioRatioMap           = types.NewRWMap[string, float64]()
	metadataAudioCompletionRatioMap = types.NewRWMap[string, float64]()
	metadataBillingModeMap          = types.NewRWMap[string, string]()
	metadataBillingExprMap          = types.NewRWMap[string, string]()
)

func ReplaceModelMetadataPricing(entries map[string]ModelMetadataPricingValues) {
	metadataModelPriceMap.Clear()
	metadataModelRatioMap.Clear()
	metadataCompletionRatioMap.Clear()
	metadataCacheRatioMap.Clear()
	metadataCreateCacheRatioMap.Clear()
	metadataImageRatioMap.Clear()
	metadataAudioRatioMap.Clear()
	metadataAudioCompletionRatioMap.Clear()
	metadataBillingModeMap.Clear()
	metadataBillingExprMap.Clear()

	for model, values := range entries {
		for _, alias := range PricingAliases(model) {
			setModelMetadataPricingValues(alias, values)
		}
	}
	for model, values := range entries {
		setModelMetadataPricingValues(model, values)
	}
	InvalidateExposedDataCache()
}

func setModelMetadataPricingValues(model string, values ModelMetadataPricingValues) {
	if values.ModelPrice != nil {
		metadataModelPriceMap.Set(model, *values.ModelPrice)
	}
	if values.ModelRatio != nil {
		metadataModelRatioMap.Set(model, *values.ModelRatio)
	}
	if values.CompletionRatio != nil {
		metadataCompletionRatioMap.Set(model, *values.CompletionRatio)
	}
	if values.CacheRatio != nil {
		metadataCacheRatioMap.Set(model, *values.CacheRatio)
	}
	if values.CreateCacheRatio != nil {
		metadataCreateCacheRatioMap.Set(model, *values.CreateCacheRatio)
	}
	if values.ImageRatio != nil {
		metadataImageRatioMap.Set(model, *values.ImageRatio)
	}
	if values.AudioRatio != nil {
		metadataAudioRatioMap.Set(model, *values.AudioRatio)
	}
	if values.AudioCompletionRatio != nil {
		metadataAudioCompletionRatioMap.Set(model, *values.AudioCompletionRatio)
	}
	if strings.TrimSpace(values.BillingMode) != "" {
		metadataBillingModeMap.Set(model, strings.TrimSpace(values.BillingMode))
	}
	if strings.TrimSpace(values.BillingExpr) != "" {
		metadataBillingExprMap.Set(model, values.BillingExpr)
	}
}

func modelMetadataPricingNames(name string) []string {
	formatted := FormatMatchingModelName(name)
	names := []string{formatted}
	if strings.HasSuffix(formatted, CompactModelSuffix) {
		names = append(names, strings.TrimSuffix(formatted, CompactModelSuffix))
	}
	return names
}

func GetMetadataModelPrice(name string) (float64, bool) {
	for _, candidate := range modelMetadataPricingNames(name) {
		if price, ok := metadataModelPriceMap.Get(candidate); ok {
			return price, true
		}
	}
	return 0, false
}

func GetMetadataModelRatio(name string) (float64, bool) {
	for _, candidate := range modelMetadataPricingNames(name) {
		if ratio, ok := metadataModelRatioMap.Get(candidate); ok {
			return ratio, true
		}
	}
	return 0, false
}

func GetMetadataCompletionRatio(name string) (float64, bool) {
	for _, candidate := range modelMetadataPricingNames(name) {
		if ratio, ok := metadataCompletionRatioMap.Get(candidate); ok {
			return ratio, true
		}
	}
	return 0, false
}

func GetMetadataCacheRatio(name string) (float64, bool) {
	for _, candidate := range modelMetadataPricingNames(name) {
		if ratio, ok := metadataCacheRatioMap.Get(candidate); ok {
			return ratio, true
		}
	}
	return 0, false
}

func GetMetadataCreateCacheRatio(name string) (float64, bool) {
	for _, candidate := range modelMetadataPricingNames(name) {
		if ratio, ok := metadataCreateCacheRatioMap.Get(candidate); ok {
			return ratio, true
		}
	}
	return 0, false
}

func GetMetadataImageRatio(name string) (float64, bool) {
	for _, candidate := range modelMetadataPricingNames(name) {
		if ratio, ok := metadataImageRatioMap.Get(candidate); ok {
			return ratio, true
		}
	}
	return 0, false
}

func GetMetadataAudioRatio(name string) (float64, bool) {
	for _, candidate := range modelMetadataPricingNames(name) {
		if ratio, ok := metadataAudioRatioMap.Get(candidate); ok {
			return ratio, true
		}
	}
	return 0, false
}

func GetMetadataAudioCompletionRatio(name string) (float64, bool) {
	for _, candidate := range modelMetadataPricingNames(name) {
		if ratio, ok := metadataAudioCompletionRatioMap.Get(candidate); ok {
			return ratio, true
		}
	}
	return 0, false
}

func GetMetadataBillingMode(name string) string {
	for _, candidate := range modelMetadataPricingNames(name) {
		if mode, ok := metadataBillingModeMap.Get(candidate); ok {
			return mode
		}
	}
	return ""
}

func GetMetadataBillingExpr(name string) (string, bool) {
	for _, candidate := range modelMetadataPricingNames(name) {
		if expr, ok := metadataBillingExprMap.Get(candidate); ok {
			return expr, true
		}
	}
	return "", false
}

func GetMetadataModelPriceCopy() map[string]float64 {
	return metadataModelPriceMap.ReadAll()
}

func GetMetadataModelRatioCopy() map[string]float64 {
	return metadataModelRatioMap.ReadAll()
}

func GetMetadataCompletionRatioCopy() map[string]float64 {
	return metadataCompletionRatioMap.ReadAll()
}

func GetMetadataCacheRatioCopy() map[string]float64 {
	return metadataCacheRatioMap.ReadAll()
}

func GetMetadataCreateCacheRatioCopy() map[string]float64 {
	return metadataCreateCacheRatioMap.ReadAll()
}
