package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func prepareModelPricingConfigTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(
		&Model{},
		&Vendor{},
		&Channel{},
		&Ability{},
		&Option{},
		&ModelPricingOverride{},
		&OfficialPricingSnapshot{},
		&OfficialModelPrice{},
	))
	clear := func() {
		DB.Exec("DELETE FROM abilities")
		DB.Exec("DELETE FROM channels")
		DB.Exec("DELETE FROM models")
		DB.Exec("DELETE FROM vendors")
		DB.Exec("DELETE FROM model_pricing_overrides")
		DB.Exec("DELETE FROM official_model_prices")
		DB.Exec("DELETE FROM official_pricing_snapshots")
		DB.Exec("DELETE FROM options")
		ratio_setting.ReplaceModelMetadataPricing(nil)
		ratio_setting.ReplaceOfficialPricing(nil, false)
		InvalidatePricingCache()
	}
	clear()
	t.Cleanup(clear)
}

func TestLoadManualPricingOverridesIntoRuntime(t *testing.T) {
	prepareModelPricingConfigTest(t)

	runtimeConfig, valid, err := ParseModelPricingConfig(`{"mode":"per-token","ratio":2.5,"completion_ratio":3,"cache_ratio":0.4,"create_cache_ratio":1.6,"image_ratio":1.2,"audio_ratio":0.7,"audio_completion_ratio":1.8}`)
	require.NoError(t, err)
	require.True(t, valid)
	tieredConfig, valid, err := ParseModelPricingConfig(`{"mode":"tiered_expr","billing_expr":"p * 1.25 + c * 10"}`)
	require.NoError(t, err)
	require.True(t, valid)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		if err := UpsertModelPricingOverrideTx(tx, "manual-runtime-model", runtimeConfig, ModelPricingOverrideOriginAdmin); err != nil {
			return err
		}
		return UpsertModelPricingOverrideTx(tx, "manual-tiered-model", tieredConfig, ModelPricingOverrideOriginAdmin)
	}))

	require.NoError(t, LoadManualPricingOverridesIntoRuntime())

	ratio, ok, matchName := ratio_setting.GetModelRatio("manual-runtime-model")
	require.True(t, ok)
	assert.Equal(t, "manual-runtime-model", matchName)
	assert.Equal(t, 2.5, ratio)
	assert.Equal(t, 3.0, ratio_setting.GetCompletionRatio("manual-runtime-model"))

	cacheRatio, ok := ratio_setting.GetCacheRatio("manual-runtime-model")
	require.True(t, ok)
	assert.Equal(t, 0.4, cacheRatio)

	createCacheRatio, ok := ratio_setting.GetCreateCacheRatio("manual-runtime-model")
	require.True(t, ok)
	assert.Equal(t, 1.6, createCacheRatio)

	imageRatio, ok := ratio_setting.GetImageRatio("manual-runtime-model")
	require.True(t, ok)
	assert.Equal(t, 1.2, imageRatio)
	assert.Equal(t, 0.7, ratio_setting.GetAudioRatio("manual-runtime-model"))
	assert.Equal(t, 1.8, ratio_setting.GetAudioCompletionRatio("manual-runtime-model"))
	assert.Equal(t, ModelPricingModeTieredExpr, ratio_setting.GetMetadataBillingMode("manual-tiered-model"))
	billingExpr, ok := ratio_setting.GetMetadataBillingExpr("manual-tiered-model")
	require.True(t, ok)
	assert.Equal(t, "p * 1.25 + c * 10", billingExpr)
}

func TestTieredModelPricingConfigVisibleWhenOfficialPricingAuthoritative(t *testing.T) {
	prepareModelPricingConfigTest(t)
	ratio_setting.ReplaceOfficialPricing(nil, true)

	tieredConfig, valid, err := ParseModelPricingConfig(`{"mode":"tiered_expr","billing_expr":"p * 1.25 + c * 10"}`)
	require.NoError(t, err)
	require.True(t, valid)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return UpsertModelPricingOverrideTx(tx, "manual-authoritative-tiered-model", tieredConfig, ModelPricingOverrideOriginAdmin)
	}))
	require.NoError(t, DB.Create(&Channel{
		Id:     1,
		Key:    "manual-tiered-pricing-key",
		Name:   "manual-tiered-pricing-channel",
		Status: common.ChannelStatusEnabled,
	}).Error)
	require.NoError(t, DB.Create(&Ability{
		Group:     "default",
		Model:     "manual-authoritative-tiered-model",
		ChannelId: 1,
		Enabled:   true,
	}).Error)
	require.NoError(t, LoadManualPricingOverridesIntoRuntime())
	RefreshPricing()

	pricingByName := make(map[string]Pricing)
	for _, item := range GetPricing() {
		pricingByName[item.ModelName] = item
	}
	pricing, ok := pricingByName["manual-authoritative-tiered-model"]
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
	require.NoError(t, ReplaceActiveOfficialPricing("legacy-pricing-test-snapshot", "test", []OfficialModelPrice{{
		Provider:        "test",
		ModelName:       "legacy-official-model",
		ModelRatio:      9,
		CompletionRatio: 3,
	}}))
	require.NoError(t, DB.Create(&[]Option{
		{Key: "ModelRatio", Value: `{"legacy-custom-model":2.5,"legacy-official-model":9}`},
		{Key: "CompletionRatio", Value: `{"legacy-custom-model":3}`},
		{Key: "CacheRatio", Value: `{"legacy-custom-model":0.4}`},
		{Key: "CreateCacheRatio", Value: `{"legacy-custom-model":1.6}`},
	}).Error)

	require.NoError(t, MigrateModelPricingOverridesFromLegacy())

	var migrated ModelPricingOverride
	require.NoError(t, DB.Where("model_name = ?", customModel.ModelName).First(&migrated).Error)
	assert.Equal(t, ModelPricingOverrideOriginLegacyOption, migrated.Origin)
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

	var officialOverrideCount int64
	require.NoError(t, DB.Model(&ModelPricingOverride{}).Where("model_name = ?", officialModel.ModelName).Count(&officialOverrideCount).Error)
	assert.Zero(t, officialOverrideCount)
}

func TestMigrateManualMetadataWithoutOfficialSnapshotDefersLegacyOptions(t *testing.T) {
	prepareModelPricingConfigTest(t)

	manualModel := Model{
		ModelName:     "manual-metadata-without-official-snapshot",
		PricingConfig: `{"mode":"per-token","ratio":4,"completion_ratio":8}`,
		Status:        1,
		SyncOfficial:  0,
	}
	require.NoError(t, manualModel.Insert())
	require.NoError(t, DB.Create(&Option{
		Key:   "ModelRatio",
		Value: `{"legacy-option-without-official-snapshot":6}`,
	}).Error)

	require.NoError(t, MigrateModelPricingOverridesFromLegacy())

	var manualOverride ModelPricingOverride
	require.NoError(t, DB.Where("model_name = ?", manualModel.ModelName).First(&manualOverride).Error)
	assert.Equal(t, ModelPricingOverrideOriginModelMetadata, manualOverride.Origin)
	require.NoError(t, LoadManualPricingOverridesIntoRuntime())
	ratio, found, _ := ratio_setting.GetModelRatio(manualModel.ModelName)
	require.True(t, found)
	assert.Equal(t, 4.0, ratio)

	var legacyOverrideCount int64
	require.NoError(t, DB.Model(&ModelPricingOverride{}).Where("model_name = ?", "legacy-option-without-official-snapshot").Count(&legacyOverrideCount).Error)
	assert.Zero(t, legacyOverrideCount)

	var migrationVersion Option
	err := DB.Where("key = ?", modelPricingOverrideMigrationVersionKey).First(&migrationVersion).Error
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestModelPricingConfigHashUsesNormalizedCompleteConfig(t *testing.T) {
	zero := 0.0
	left := ModelPricingConfig{
		Mode:        " tiered_expr ",
		BillingExpr: "  p * 2 + c * 4  ",
		CacheRatio:  &zero,
	}
	right := ModelPricingConfig{
		Mode:        ModelPricingModeTieredExpr,
		BillingExpr: "p * 2 + c * 4",
		CacheRatio:  &zero,
	}

	leftHash, err := ModelPricingConfigHash(left)
	require.NoError(t, err)
	rightHash, err := ModelPricingConfigHash(right)
	require.NoError(t, err)
	assert.Equal(t, leftHash, rightHash)
	assert.True(t, ModelPricingConfigsEqual(left, right))

	withoutExplicitZero := right
	withoutExplicitZero.CacheRatio = nil
	withoutZeroHash, err := ModelPricingConfigHash(withoutExplicitZero)
	require.NoError(t, err)
	assert.NotEqual(t, rightHash, withoutZeroHash)
	assert.False(t, ModelPricingConfigsEqual(right, withoutExplicitZero))
}
