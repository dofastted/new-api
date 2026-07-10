package ratio_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOfficialPricingAuthoritativeOverridesLegacyRatios(t *testing.T) {
	oldSelfUse := operation_setting.SelfUseModeEnabled
	t.Cleanup(func() {
		operation_setting.SelfUseModeEnabled = oldSelfUse
		ReplaceOfficialPricing(nil, false)
	})

	cacheRatio := 0.1
	createCacheRatio := 1.25
	ReplaceOfficialPricing(map[string]OfficialPricingValues{
		"gpt-5-codex": {
			ModelRatio:       0.625,
			CompletionRatio:  8,
			CacheRatio:       &cacheRatio,
			CreateCacheRatio: &createCacheRatio,
		},
	}, true)

	modelPrice, hasPrice := GetModelPrice("gpt-5-codex", false)
	assert.False(t, hasPrice)
	assert.Equal(t, -1.0, modelPrice)

	ratio, ok, matchName := GetModelRatio("gpt-5-codex")
	require.True(t, ok)
	assert.Equal(t, "gpt-5-codex", matchName)
	assert.Equal(t, 0.625, ratio)
	assert.Equal(t, 8.0, GetCompletionRatio("gpt-5-codex"))

	compactRatio, compactOK, _ := GetModelRatio("gpt-5-codex-openai-compact")
	require.True(t, compactOK)
	assert.Equal(t, 0.625, compactRatio)

	readRatio, readOK := GetCacheRatio("gpt-5-codex")
	require.True(t, readOK)
	assert.Equal(t, 0.1, readRatio)

	writeRatio, writeOK := GetCreateCacheRatio("gpt-5-codex")
	require.True(t, writeOK)
	assert.Equal(t, 1.25, writeRatio)

	operation_setting.SelfUseModeEnabled = true
	legacyRatio, legacyOK, legacyMatch := GetModelRatio("deepseek-chat")
	assert.False(t, legacyOK)
	assert.Equal(t, "deepseek-chat", legacyMatch)
	assert.Equal(t, 37.5, legacyRatio)
}

func TestOfficialPricingAddsGeminiThinkingAliases(t *testing.T) {
	t.Cleanup(func() {
		ReplaceOfficialPricing(nil, false)
	})

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
	t.Cleanup(func() {
		ReplaceModelMetadataPricing(nil)
		ReplaceOfficialPricing(nil, false)
	})

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

func TestGPT56SeriesDefaultRatiosAndCompletion(t *testing.T) {
	t.Cleanup(func() {
		ReplaceOfficialPricing(nil, false)
		ReplaceModelMetadataPricing(nil)
	})
	ReplaceOfficialPricing(nil, false)
	ReplaceModelMetadataPricing(nil)

	// Ensure defaults are present for offline/fallback billing.
	InitRatioSettings()

	cases := []struct {
		name       string
		modelRatio float64
	}{
		{"gpt-5.6-sol", 2.5},
		{"gpt-5.6-terra", 1.25},
		{"gpt-5.6-luna", 0.5},
	}
	for _, tc := range cases {
		ratio, ok, matchName := GetModelRatio(tc.name)
		require.Truef(t, ok, "missing default model ratio for %s", tc.name)
		assert.Equal(t, tc.name, matchName)
		assert.Equal(t, tc.modelRatio, ratio)
		assert.Equal(t, 6.0, GetCompletionRatio(tc.name))
		cacheRatio, cacheOK := GetCacheRatio(tc.name)
		require.Truef(t, cacheOK, "missing default cache ratio for %s", tc.name)
		assert.Equal(t, 0.1, cacheRatio)
	}
}
