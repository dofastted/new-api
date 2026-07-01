package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepareModelPricingConfigTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Model{}, &Option{}, &Ability{}))
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM models").Error)
	require.NoError(t, DB.Exec("DELETE FROM options").Error)
	ratio_setting.ReplaceModelMetadataPricing(nil)
	ratio_setting.ReplaceOfficialPricing(nil, false)
	InvalidatePricingCache()
	t.Cleanup(func() {
		DB.Exec("DELETE FROM abilities")
		DB.Exec("DELETE FROM models")
		DB.Exec("DELETE FROM options")
		ratio_setting.ReplaceModelMetadataPricing(nil)
		ratio_setting.ReplaceOfficialPricing(nil, false)
		InvalidatePricingCache()
	})
}

func TestLoadModelPricingConfigsIntoRuntime(t *testing.T) {
	prepareModelPricingConfigTest(t)

	require.NoError(t, DB.Create(&Model{
		ModelName:    "metadata-runtime-model",
		Status:       1,
		SyncOfficial: 1,
		PricingConfig: `{"mode":"per-token","ratio":2.5,"completion_ratio":3,"cache_ratio":0.4,"create_cache_ratio":1.6,` +
			`"image_ratio":1.2,"audio_ratio":0.7,"audio_completion_ratio":1.8}`,
	}).Error)
	require.NoError(t, DB.Create(&Model{
		ModelName:     "metadata-tiered-model",
		Status:        1,
		SyncOfficial:  1,
		PricingConfig: `{"mode":"tiered_expr","billing_expr":"p * 1.25 + c * 10"}`,
	}).Error)

	require.NoError(t, LoadModelPricingConfigsIntoRuntime())

	ratio, ok, matchName := ratio_setting.GetModelRatio("metadata-runtime-model")
	require.True(t, ok)
	assert.Equal(t, "metadata-runtime-model", matchName)
	assert.Equal(t, 2.5, ratio)
	assert.Equal(t, 3.0, ratio_setting.GetCompletionRatio("metadata-runtime-model"))

	cacheRatio, ok := ratio_setting.GetCacheRatio("metadata-runtime-model")
	require.True(t, ok)
	assert.Equal(t, 0.4, cacheRatio)

	createCacheRatio, ok := ratio_setting.GetCreateCacheRatio("metadata-runtime-model")
	require.True(t, ok)
	assert.Equal(t, 1.6, createCacheRatio)

	imageRatio, ok := ratio_setting.GetImageRatio("metadata-runtime-model")
	require.True(t, ok)
	assert.Equal(t, 1.2, imageRatio)
	assert.Equal(t, 0.7, ratio_setting.GetAudioRatio("metadata-runtime-model"))
	assert.Equal(t, 1.8, ratio_setting.GetAudioCompletionRatio("metadata-runtime-model"))
	assert.Equal(t, ModelPricingModeTieredExpr, ratio_setting.GetMetadataBillingMode("metadata-tiered-model"))
	billingExpr, ok := ratio_setting.GetMetadataBillingExpr("metadata-tiered-model")
	require.True(t, ok)
	assert.Equal(t, "p * 1.25 + c * 10", billingExpr)
}

func TestTieredModelPricingConfigVisibleWhenOfficialPricingAuthoritative(t *testing.T) {
	prepareModelPricingConfigTest(t)
	ratio_setting.ReplaceOfficialPricing(nil, true)

	require.NoError(t, DB.Create(&Model{
		ModelName:     "metadata-authoritative-tiered-model",
		Status:        1,
		SyncOfficial:  1,
		PricingConfig: `{"mode":"tiered_expr","billing_expr":"p * 1.25 + c * 10"}`,
	}).Error)
	require.NoError(t, DB.Create(&Ability{
		Group:     "default",
		Model:     "metadata-authoritative-tiered-model",
		ChannelId: 1,
		Enabled:   true,
	}).Error)
	require.NoError(t, LoadModelPricingConfigsIntoRuntime())
	RefreshPricing()

	pricingByName := make(map[string]Pricing)
	for _, item := range GetPricing() {
		pricingByName[item.ModelName] = item
	}
	pricing, ok := pricingByName["metadata-authoritative-tiered-model"]
	require.True(t, ok)
	assert.Equal(t, ModelPricingModeTieredExpr, pricing.BillingMode)
	assert.Equal(t, "p * 1.25 + c * 10", pricing.BillingExpr)
}

func TestMigrateModelPricingConfigFromLegacyOptionsOnlyCustomModels(t *testing.T) {
	prepareModelPricingConfigTest(t)

	customModel := Model{ModelName: "legacy-custom-model", Status: 1, SyncOfficial: 0}
	require.NoError(t, customModel.Insert())
	officialModel := Model{ModelName: "legacy-official-model", Status: 1, SyncOfficial: 1}
	require.NoError(t, officialModel.Insert())
	require.NoError(t, DB.Create(&[]Option{
		{Key: "ModelRatio", Value: `{"legacy-custom-model":2.5,"legacy-official-model":9}`},
		{Key: "CompletionRatio", Value: `{"legacy-custom-model":3}`},
		{Key: "CacheRatio", Value: `{"legacy-custom-model":0.4}`},
		{Key: "CreateCacheRatio", Value: `{"legacy-custom-model":1.6}`},
	}).Error)

	require.NoError(t, MigrateModelPricingConfigFromLegacyOptions())

	var migrated Model
	require.NoError(t, DB.First(&migrated, customModel.Id).Error)
	cfg, ok, err := ParseModelPricingConfig(migrated.PricingConfig)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, ModelPricingModePerToken, cfg.Mode)
	require.NotNil(t, cfg.Ratio)
	assert.Equal(t, 2.5, *cfg.Ratio)
	require.NotNil(t, cfg.CompletionRatio)
	assert.Equal(t, 3.0, *cfg.CompletionRatio)
	require.NotNil(t, cfg.CacheRatio)
	assert.Equal(t, 0.4, *cfg.CacheRatio)
	require.NotNil(t, cfg.CreateCacheRatio)
	assert.Equal(t, 1.6, *cfg.CreateCacheRatio)

	var unchanged Model
	require.NoError(t, DB.First(&unchanged, officialModel.Id).Error)
	assert.Empty(t, unchanged.PricingConfig)
}
