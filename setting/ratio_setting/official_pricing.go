package ratio_setting

import (
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/types"
)

type OfficialPricingValues struct {
	ModelRatio       float64
	CompletionRatio  float64
	CacheRatio       *float64
	CreateCacheRatio *float64
}

var (
	officialPricingAuthoritative atomic.Bool
	officialModelRatioMap        = types.NewRWMap[string, float64]()
	officialCompletionRatioMap   = types.NewRWMap[string, float64]()
	officialCacheRatioMap        = types.NewRWMap[string, float64]()
	officialCreateCacheRatioMap  = types.NewRWMap[string, float64]()
)

func ReplaceOfficialPricing(entries map[string]OfficialPricingValues, authoritative bool) {
	officialModelRatioMap.Clear()
	officialCompletionRatioMap.Clear()
	officialCacheRatioMap.Clear()
	officialCreateCacheRatioMap.Clear()
	for model, values := range entries {
		setOfficialPricingValues(model, values)
		for _, alias := range officialPricingAliases(model) {
			setOfficialPricingValues(alias, values)
		}
	}
	officialPricingAuthoritative.Store(authoritative)
	InvalidateExposedDataCache()
}

func setOfficialPricingValues(model string, values OfficialPricingValues) {
	officialModelRatioMap.Set(model, values.ModelRatio)
	officialCompletionRatioMap.Set(model, values.CompletionRatio)
	if values.CacheRatio != nil {
		officialCacheRatioMap.Set(model, *values.CacheRatio)
	}
	if values.CreateCacheRatio != nil {
		officialCreateCacheRatioMap.Set(model, *values.CreateCacheRatio)
	}
}

func officialPricingAliases(model string) []string {
	switch {
	case strings.HasPrefix(model, "gemini-2.5-flash-lite"):
		return []string{"gemini-2.5-flash-lite-thinking-*"}
	case strings.HasPrefix(model, "gemini-2.5-flash"):
		return []string{"gemini-2.5-flash-thinking-*"}
	case strings.HasPrefix(model, "gemini-2.5-pro"):
		return []string{"gemini-2.5-pro-thinking-*"}
	default:
		return nil
	}
}

func OfficialPricingAuthoritative() bool {
	return officialPricingAuthoritative.Load()
}

func officialPricingNames(name string) []string {
	formatted := FormatMatchingModelName(name)
	names := []string{formatted}
	if strings.HasSuffix(formatted, CompactModelSuffix) {
		names = append(names, strings.TrimSuffix(formatted, CompactModelSuffix))
	}
	return names
}

func HasOfficialPricing(name string) bool {
	for _, candidate := range officialPricingNames(name) {
		if _, ok := officialModelRatioMap.Get(candidate); ok {
			return true
		}
	}
	return false
}

func GetOfficialModelRatio(name string) (float64, bool) {
	for _, candidate := range officialPricingNames(name) {
		if ratio, ok := officialModelRatioMap.Get(candidate); ok {
			return ratio, true
		}
	}
	return 0, false
}

func GetOfficialCompletionRatio(name string) (float64, bool) {
	for _, candidate := range officialPricingNames(name) {
		if ratio, ok := officialCompletionRatioMap.Get(candidate); ok {
			return ratio, true
		}
	}
	return 0, false
}

func GetOfficialCacheRatio(name string) (float64, bool) {
	for _, candidate := range officialPricingNames(name) {
		if ratio, ok := officialCacheRatioMap.Get(candidate); ok {
			return ratio, true
		}
	}
	return 0, false
}

func GetOfficialCreateCacheRatio(name string) (float64, bool) {
	for _, candidate := range officialPricingNames(name) {
		if ratio, ok := officialCreateCacheRatioMap.Get(candidate); ok {
			return ratio, true
		}
	}
	return 0, false
}

func GetOfficialModelRatioCopy() map[string]float64 {
	return officialModelRatioMap.ReadAll()
}

func GetOfficialCompletionRatioCopy() map[string]float64 {
	return officialCompletionRatioMap.ReadAll()
}

func GetOfficialCacheRatioCopy() map[string]float64 {
	return officialCacheRatioMap.ReadAll()
}

func GetOfficialCreateCacheRatioCopy() map[string]float64 {
	return officialCreateCacheRatioMap.ReadAll()
}
