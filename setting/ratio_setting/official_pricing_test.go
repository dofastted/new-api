package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func preservePricingRuntimeState(t *testing.T) {
	t.Helper()

	oldModelPrice := modelPriceMap.ReadAll()
	oldModelRatio := modelRatioMap.ReadAll()
	oldOfficialAuthoritative := officialPricingAuthoritative.Load()
	oldOfficialModelRatio := officialModelRatioMap.ReadAll()
	oldOfficialCompletionRatio := officialCompletionRatioMap.ReadAll()
	oldOfficialCacheRatio := officialCacheRatioMap.ReadAll()
	oldOfficialCreateCacheRatio := officialCreateCacheRatioMap.ReadAll()
	oldMetadataModelPrice := metadataModelPriceMap.ReadAll()
	oldMetadataModelRatio := metadataModelRatioMap.ReadAll()
	oldMetadataCompletionRatio := metadataCompletionRatioMap.ReadAll()
	oldMetadataCacheRatio := metadataCacheRatioMap.ReadAll()
	oldMetadataCreateCacheRatio := metadataCreateCacheRatioMap.ReadAll()
	oldMetadataImageRatio := metadataImageRatioMap.ReadAll()
	oldMetadataAudioRatio := metadataAudioRatioMap.ReadAll()
	oldMetadataAudioCompletionRatio := metadataAudioCompletionRatioMap.ReadAll()
	oldMetadataBillingMode := metadataBillingModeMap.ReadAll()
	oldMetadataBillingExpr := metadataBillingExprMap.ReadAll()
	oldExposedCache, _ := exposedData.Load().(*exposedCache)

	t.Cleanup(func() {
		modelPriceMap.Clear()
		modelPriceMap.AddAll(oldModelPrice)
		modelRatioMap.Clear()
		modelRatioMap.AddAll(oldModelRatio)
		officialModelRatioMap.Clear()
		officialModelRatioMap.AddAll(oldOfficialModelRatio)
		officialCompletionRatioMap.Clear()
		officialCompletionRatioMap.AddAll(oldOfficialCompletionRatio)
		officialCacheRatioMap.Clear()
		officialCacheRatioMap.AddAll(oldOfficialCacheRatio)
		officialCreateCacheRatioMap.Clear()
		officialCreateCacheRatioMap.AddAll(oldOfficialCreateCacheRatio)
		officialPricingAuthoritative.Store(oldOfficialAuthoritative)
		metadataModelPriceMap.Clear()
		metadataModelPriceMap.AddAll(oldMetadataModelPrice)
		metadataModelRatioMap.Clear()
		metadataModelRatioMap.AddAll(oldMetadataModelRatio)
		metadataCompletionRatioMap.Clear()
		metadataCompletionRatioMap.AddAll(oldMetadataCompletionRatio)
		metadataCacheRatioMap.Clear()
		metadataCacheRatioMap.AddAll(oldMetadataCacheRatio)
		metadataCreateCacheRatioMap.Clear()
		metadataCreateCacheRatioMap.AddAll(oldMetadataCreateCacheRatio)
		metadataImageRatioMap.Clear()
		metadataImageRatioMap.AddAll(oldMetadataImageRatio)
		metadataAudioRatioMap.Clear()
		metadataAudioRatioMap.AddAll(oldMetadataAudioRatio)
		metadataAudioCompletionRatioMap.Clear()
		metadataAudioCompletionRatioMap.AddAll(oldMetadataAudioCompletionRatio)
		metadataBillingModeMap.Clear()
		metadataBillingModeMap.AddAll(oldMetadataBillingMode)
		metadataBillingExprMap.Clear()
		metadataBillingExprMap.AddAll(oldMetadataBillingExpr)
		exposedData.Store(oldExposedCache)
	})
}

func TestOfficialPricingUsesPerModelPriority(t *testing.T) {
	preservePricingRuntimeState(t)
	modelRatioMap.Clear()
	modelRatioMap.AddAll(map[string]float64{
		"fallback-model": 1.25,
		"official-model": 99,
		"manual-model":   99,
	})

	ReplaceOfficialPricing(map[string]OfficialPricingValues{
		"official-model": {ModelRatio: 2.5},
		"manual-model":   {ModelRatio: 3.5},
	}, true)
	manualRatio := 4.5
	ReplaceModelMetadataPricing(map[string]ModelMetadataPricingValues{
		"manual-model": {ModelRatio: &manualRatio},
	})

	cases := []struct {
		name  string
		model string
		want  float64
	}{
		{name: "official model uses official ratio", model: "official-model", want: 2.5},
		{name: "unlisted model keeps legacy fallback", model: "fallback-model", want: 1.25},
		{name: "manual metadata overrides official ratio", model: "manual-model", want: 4.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ratio, ok, matchName := GetModelRatio(tc.model)
			require.True(t, ok)
			assert.Equal(t, tc.model, matchName)
			assert.Equal(t, tc.want, ratio)
		})
	}
}

func TestOfficialPricingAddsGeminiThinkingAliases(t *testing.T) {
	preservePricingRuntimeState(t)
	ReplaceOfficialPricing(map[string]OfficialPricingValues{
		"gemini-2.5-flash": {
			ModelRatio:      0.15,
			CompletionRatio: 8.33333333,
		},
	}, true)

	ratio, ok, matchName := GetModelRatio("gemini-2.5-flash-thinking-8192")
	require.True(t, ok)
	assert.Equal(t, "gemini-2.5-flash-thinking-*", matchName)
	assert.Equal(t, 0.15, ratio)
	assert.Equal(t, 8.33333333, GetCompletionRatio("gemini-2.5-flash-thinking-8192"))
}

func TestModelMetadataPricingOverridesOfficialPricing(t *testing.T) {
	preservePricingRuntimeState(t)

	officialCacheRatio := 0.1
	officialCreateCacheRatio := 1.25
	ReplaceOfficialPricing(map[string]OfficialPricingValues{
		"gpt-5-codex": {
			ModelRatio:       0.625,
			CompletionRatio:  8,
			CacheRatio:       &officialCacheRatio,
			CreateCacheRatio: &officialCreateCacheRatio,
		},
	}, true)

	metadataRatio := 2.5
	metadataCompletionRatio := 3.0
	metadataCacheRatio := 0.4
	metadataCreateCacheRatio := 1.6
	ReplaceModelMetadataPricing(map[string]ModelMetadataPricingValues{
		"gpt-5-codex": {
			ModelRatio:       &metadataRatio,
			CompletionRatio:  &metadataCompletionRatio,
			CacheRatio:       &metadataCacheRatio,
			CreateCacheRatio: &metadataCreateCacheRatio,
		},
	})

	ratio, ok, matchName := GetModelRatio("gpt-5-codex")
	require.True(t, ok)
	assert.Equal(t, "gpt-5-codex", matchName)
	assert.Equal(t, 2.5, ratio)
	assert.Equal(t, 3.0, GetCompletionRatio("gpt-5-codex"))

	cacheRatio, ok := GetCacheRatio("gpt-5-codex")
	require.True(t, ok)
	assert.Equal(t, 0.4, cacheRatio)

	createCacheRatio, ok := GetCreateCacheRatio("gpt-5-codex")
	require.True(t, ok)
	assert.Equal(t, 1.6, createCacheRatio)
}

func TestGetExposedDataLayersFallbackOfficialAndManualPricing(t *testing.T) {
	preservePricingRuntimeState(t)
	modelPriceMap.Clear()
	modelPriceMap.AddAll(map[string]float64{
		"official-per-token": 0.12,
	})
	modelRatioMap.Clear()
	modelRatioMap.AddAll(map[string]float64{
		"fallback-model": 1.25,
		"official-model": 99,
		"manual-model":   99,
	})

	ReplaceOfficialPricing(map[string]OfficialPricingValues{
		"official-model":     {ModelRatio: 2.5},
		"manual-model":       {ModelRatio: 3.5},
		"official-per-token": {ModelRatio: 4.5},
	}, true)
	manualRatio := 5.5
	ReplaceModelMetadataPricing(map[string]ModelMetadataPricingValues{
		"manual-model": {ModelRatio: &manualRatio},
	})

	data := GetExposedData()
	modelRatio, ok := data["model_ratio"].(map[string]float64)
	require.True(t, ok)
	modelPrice, ok := data["model_price"].(map[string]float64)
	require.True(t, ok)

	assert.Equal(t, 1.25, modelRatio["fallback-model"])
	assert.Equal(t, 2.5, modelRatio["official-model"])
	assert.Equal(t, 5.5, modelRatio["manual-model"])
	assert.Equal(t, 4.5, modelRatio["official-per-token"])
	assert.NotContains(t, modelPrice, "official-per-token")
}
