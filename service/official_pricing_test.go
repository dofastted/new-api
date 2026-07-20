package service

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupOfficialMetadataServiceTestDB(t *testing.T) {
	t.Helper()

	require.NoError(t, model.DB.AutoMigrate(
		&model.Model{},
		&model.Vendor{},
		&model.OfficialModelPrice{},
		&model.OfficialPricingSnapshot{},
		&model.ModelPricingOverride{},
		&model.Option{},
		&model.Ability{},
		&model.Channel{},
	))
	clear := func() {
		model.DB.Exec("DELETE FROM official_model_prices")
		model.DB.Exec("DELETE FROM official_pricing_snapshots")
		model.DB.Exec("DELETE FROM models")
		model.DB.Exec("DELETE FROM model_pricing_overrides")
		model.DB.Exec("DELETE FROM vendors")
		model.DB.Exec("DELETE FROM abilities")
		model.DB.Exec("DELETE FROM options")
		model.DB.Exec("DELETE FROM channels")
		ratio_setting.ReplaceModelMetadataPricing(nil)
		ratio_setting.ReplaceOfficialPricing(nil, false)
		model.InvalidatePricingCache()
	}
	clear()
	t.Cleanup(clear)
}

func TestOfficialModelMetadataSyncUpdatesExistingOfficialModelFromActivePricing(t *testing.T) {
	setupOfficialMetadataServiceTestDB(t)

	wrongVendor := model.Vendor{Name: "Wrong Vendor", Icon: "Wrong.Icon", Status: 1}
	require.NoError(t, model.DB.Create(&wrongVendor).Error)
	require.NoError(t, model.DB.Create(&model.Model{
		ModelName:     "gpt-official-metadata-test",
		Description:   "stale description",
		Icon:          "Bad.Model.Icon",
		Tags:          "stale-tag",
		VendorID:      wrongVendor.Id,
		Endpoints:     `[{"name":"stale"}]`,
		PricingConfig: `{"mode":"per-token","ratio":99,"completion_ratio":99}`,
		Status:        1,
		SyncOfficial:  1,
		NameRule:      model.NameRulePrefix,
	}).Error)

	cacheRatio := 0.2
	createCacheRatio := 0.4
	require.NoError(t, model.DB.Create(&model.OfficialModelPrice{
		Provider:         OfficialPricingProviderOpenAI,
		ModelName:        "gpt-official-metadata-test",
		SourceURL:        OfficialPricingOpenAIURL,
		ModelRatio:       1.25,
		CompletionRatio:  2.75,
		CacheRatio:       &cacheRatio,
		CreateCacheRatio: &createCacheRatio,
		Active:           true,
	}).Error)

	result, err := SyncOfficialModelMetadata(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.CreatedModels)
	assert.Equal(t, 1, result.UpdatedModels)
	assert.Equal(t, 1, result.CreatedVendors)
	assert.Equal(t, "official-pricing", result.Source.Name)
	assert.Equal(t, 1, result.Source.ModelCount)
	assert.ElementsMatch(t, []string{"gpt-official-metadata-test"}, result.UpdatedList)

	var openAIVendor model.Vendor
	require.NoError(t, model.DB.Where("name = ?", "OpenAI").First(&openAIVendor).Error)
	assert.Equal(t, "OpenAI", openAIVendor.Icon)

	var synced model.Model
	require.NoError(t, model.DB.Where("model_name = ?", "gpt-official-metadata-test").First(&synced).Error)
	assert.Equal(t, openAIVendor.Id, synced.VendorID)
	assert.Empty(t, synced.Icon)
	assert.Equal(t, model.NameRuleExact, synced.NameRule)
	assert.Equal(t, 1, synced.SyncOfficial)

	cfg, ok, err := model.ParseModelPricingConfig(synced.PricingConfig)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, model.ModelPricingModePerToken, cfg.Mode)
	require.NotNil(t, cfg.Ratio)
	assert.Equal(t, 1.25, *cfg.Ratio)
	require.NotNil(t, cfg.CompletionRatio)
	assert.Equal(t, 2.75, *cfg.CompletionRatio)
	require.NotNil(t, cfg.CacheRatio)
	assert.Equal(t, cacheRatio, *cfg.CacheRatio)
	require.NotNil(t, cfg.CreateCacheRatio)
	assert.Equal(t, createCacheRatio, *cfg.CreateCacheRatio)
}

func TestOfficialModelMetadataSyncClearsStaleOfficialVendorRowsOutsidePriceRows(t *testing.T) {
	setupOfficialMetadataServiceTestDB(t)

	openAIVendor := model.Vendor{Name: "OpenAI", Icon: "OpenAI", Status: 1}
	require.NoError(t, model.DB.Create(&openAIVendor).Error)
	existingModel := model.Model{
		ModelName:     "gpt-image-custom-icon-test",
		Description:   "stale description",
		Icon:          "Wrong.Image.Icon",
		Tags:          "stale-tag",
		Endpoints:     `[{"name":"stale"}]`,
		PricingConfig: `{"mode":"per-token","ratio":8,"completion_ratio":9}`,
		VendorID:      openAIVendor.Id,
		Status:        1,
		SyncOfficial:  1,
		NameRule:      model.NameRuleExact,
	}
	require.NoError(t, existingModel.Insert())
	require.NoError(t, model.DB.Create(&model.OfficialModelPrice{
		Provider:        OfficialPricingProviderOpenAI,
		ModelName:       "gpt-priced-icon-trigger-test",
		SourceURL:       OfficialPricingOpenAIURL,
		ModelRatio:      1.25,
		CompletionRatio: 2,
		Active:          true,
	}).Error)

	result, err := SyncOfficialModelMetadata(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.CreatedModels)
	assert.Equal(t, 1, result.UpdatedModels)
	assert.Equal(t, 0, result.CreatedVendors)

	var existing model.Model
	require.NoError(t, model.DB.Where("model_name = ?", "gpt-image-custom-icon-test").First(&existing).Error)
	assert.Empty(t, existing.Description)
	assert.Empty(t, existing.Icon)
	assert.Empty(t, existing.Tags)
	assert.Empty(t, existing.Endpoints)
	assert.Empty(t, existing.PricingConfig)
	assert.Equal(t, 1, existing.SyncOfficial)
}

func TestOfficialModelMetadataSyncPreservesStaleLocalRowsOutsidePriceRows(t *testing.T) {
	setupOfficialMetadataServiceTestDB(t)

	openAIVendor := model.Vendor{Name: "OpenAI", Icon: "OpenAI", Status: 1}
	require.NoError(t, model.DB.Create(&openAIVendor).Error)
	localPricingConfig := `{"mode":"per-token","ratio":8,"completion_ratio":9}`
	localModel := model.Model{
		ModelName:     "gpt-image-local-price-test",
		Description:   "local description",
		Icon:          "Local.Icon",
		Tags:          "local-tag",
		Endpoints:     `[{"name":"local"}]`,
		PricingConfig: localPricingConfig,
		VendorID:      openAIVendor.Id,
		Status:        1,
		SyncOfficial:  0,
		NameRule:      model.NameRuleExact,
	}
	require.NoError(t, localModel.Insert())
	require.NoError(t, model.DB.Create(&model.OfficialModelPrice{
		Provider:        OfficialPricingProviderOpenAI,
		ModelName:       "gpt-priced-local-trigger-test",
		SourceURL:       OfficialPricingOpenAIURL,
		ModelRatio:      1.25,
		CompletionRatio: 2,
		Active:          true,
	}).Error)

	result, err := SyncOfficialModelMetadata(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.CreatedModels)
	assert.Equal(t, 0, result.UpdatedModels)
	assert.Equal(t, 0, result.CreatedVendors)

	var existing model.Model
	require.NoError(t, model.DB.Where("model_name = ?", "gpt-image-local-price-test").First(&existing).Error)
	assert.Equal(t, "local description", existing.Description)
	assert.Equal(t, "Local.Icon", existing.Icon)
	assert.Equal(t, "local-tag", existing.Tags)
	assert.Equal(t, `[{"name":"local"}]`, existing.Endpoints)
	assert.Equal(t, localPricingConfig, existing.PricingConfig)
	assert.Equal(t, 0, existing.SyncOfficial)
}

func TestOfficialModelMetadataSyncPreservesExistingModelOptedOutOfOfficialSync(t *testing.T) {
	setupOfficialMetadataServiceTestDB(t)

	customVendor := model.Vendor{Name: "Custom Vendor", Icon: "CustomVendor.Icon", Status: 1}
	require.NoError(t, model.DB.Create(&customVendor).Error)
	customPricingConfig := `{"mode":"per-token","ratio":8,"completion_ratio":9}`
	optedOutModel := model.Model{
		ModelName:     "gemini-custom-opt-out-test",
		Description:   "custom description",
		Icon:          "Custom.Model.Icon",
		Tags:          "custom-tag",
		VendorID:      customVendor.Id,
		Endpoints:     `[{"name":"custom"}]`,
		PricingConfig: customPricingConfig,
		Status:        1,
		SyncOfficial:  0,
		NameRule:      model.NameRuleContains,
	}
	require.NoError(t, optedOutModel.Insert())
	require.NoError(t, model.DB.Create(&model.OfficialModelPrice{
		Provider:        OfficialPricingProviderGemini,
		ModelName:       "gemini-custom-opt-out-test",
		SourceURL:       OfficialPricingGeminiURL,
		ModelRatio:      0.5,
		CompletionRatio: 3.5,
		Active:          true,
	}).Error)

	result, err := SyncOfficialModelMetadata(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.CreatedModels)
	assert.Equal(t, 0, result.UpdatedModels)
	assert.Equal(t, 1, result.CreatedVendors)
	assert.ElementsMatch(t, []string{"gemini-custom-opt-out-test"}, result.SkippedModels)
	assert.Empty(t, result.UpdatedList)

	var preserved model.Model
	require.NoError(t, model.DB.Where("model_name = ?", "gemini-custom-opt-out-test").First(&preserved).Error)
	assert.Equal(t, customVendor.Id, preserved.VendorID)
	assert.Equal(t, "custom description", preserved.Description)
	assert.Equal(t, "Custom.Model.Icon", preserved.Icon)
	assert.Equal(t, "custom-tag", preserved.Tags)
	assert.Equal(t, `[{"name":"custom"}]`, preserved.Endpoints)
	assert.Equal(t, 0, preserved.SyncOfficial)
	assert.Equal(t, model.NameRuleContains, preserved.NameRule)
	assert.Equal(t, customPricingConfig, preserved.PricingConfig)
}

func TestConvertGeminiOfficialPricingToRatioData(t *testing.T) {
	body := `
## Gemini 2.5 Flash

*` + "`gemini-2.5-flash`" + `*

### Standard

|  | Free Tier | Paid Tier, per 1M tokens in USD |
| --- | --- | --- |
| Input price | Free of charge | $0.30 (text / image / video)   $1.00 (audio) |
| Output price (including thinking tokens) | Free of charge | $2.50 |
| Context caching price | Not available | $0.03 (text / image / video)   $0.1 (audio)   $1.00 / 1,000,000 tokens per hour (storage price) |

### Batch

|  | Free Tier | Paid Tier, per 1M tokens in USD |
| --- | --- | --- |
| Input price | Not available | $0.15 |
| Output price (including thinking tokens) | Not available | $1.25 |

## Gemini 2.5 Flash Image

*` + "`gemini-2.5-flash-image`" + `*

### Standard

|  | Free Tier | Paid Tier, per 1M tokens in USD |
| --- | --- | --- |
| Input price | Not available | $0.30 (text / image) |
| Output price | Not available | $0.039 per image* |
`

	converted, err := ConvertGeminiOfficialPricingToRatioData(strings.NewReader(body))
	require.NoError(t, err)

	modelRatio := converted["model_ratio"].(map[string]any)
	completionRatio := converted["completion_ratio"].(map[string]any)
	cacheRatio := converted["cache_ratio"].(map[string]any)

	assert.Equal(t, 0.15, modelRatio["gemini-2.5-flash"])
	assert.Equal(t, 8.33333333, completionRatio["gemini-2.5-flash"])
	assert.Equal(t, 0.1, cacheRatio["gemini-2.5-flash"])
	assert.NotContains(t, modelRatio, "gemini-2.5-flash-image")
}

func TestParseGeminiOfficialTokenPricingCollapsedOfficialTextUsesStandardPrices(t *testing.T) {
	body := `Gemini 2.5 Flash gemini-2.5-flash Standard Free Tier Paid Tier, per 1M tokens in USD Input price Free of charge $0.30 (text / image / video) $1.00 (audio) Output price (including thinking tokens) Free of charge $2.50 Context caching price Not available $0.03 (text / image / video) Batch Free Tier Paid Tier, per 1M tokens in USD Input price Not available $0.15 Output price (including thinking tokens) Not available $1.25 Context caching price Not available $0.015`

	entries, err := ParseGeminiOfficialTokenPricing(strings.NewReader(body), "https://ai.google.dev/gemini-api/docs/pricing")
	require.NoError(t, err)
	require.Len(t, entries, 1)

	entry := entries[0]
	assert.Equal(t, OfficialPricingProviderGemini, entry.Provider)
	assert.Equal(t, "gemini-2.5-flash", entry.Model)
	assert.Equal(t, "https://ai.google.dev/gemini-api/docs/pricing", entry.SourceURL)
	assert.Equal(t, 0.30, entry.InputUSDPerMTok)
	assert.Equal(t, 2.50, entry.OutputUSDPerMTok)
	require.NotNil(t, entry.CacheReadUSDPerMTok)
	assert.Equal(t, 0.03, *entry.CacheReadUSDPerMTok)
	require.NotNil(t, entry.CacheReadRatio)
	assert.Equal(t, 0.1, *entry.CacheReadRatio)
}

func TestConvertKimiOfficialPricingToRatioData(t *testing.T) {
	body := `
<DocTable
  rows={[
["kimi-k3", "1M tokens", <>{"$"}0.30</>, <>{"$"}3.00</>, <>{"$"}15.00</>, "1,048,576 tokens"],
]}
/>
`

	entries, err := ParseKimiOfficialTokenPricing(strings.NewReader(body), OfficialPricingKimiURL)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	entry := entries[0]
	assert.Equal(t, OfficialPricingProviderKimi, entry.Provider)
	assert.Equal(t, "kimi-k3", entry.Model)
	assert.Equal(t, OfficialPricingKimiURL, entry.SourceURL)
	assert.Equal(t, 3.0, entry.InputUSDPerMTok)
	assert.Equal(t, 15.0, entry.OutputUSDPerMTok)
	require.NotNil(t, entry.CacheReadUSDPerMTok)
	assert.Equal(t, 0.30, *entry.CacheReadUSDPerMTok)
	require.NotNil(t, entry.CacheReadRatio)
	assert.InDelta(t, 0.1, *entry.CacheReadRatio, 1e-12)

	converted, err := ConvertKimiOfficialPricingToRatioData(strings.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, 1.5, converted["model_ratio"].(map[string]any)["kimi-k3"])
	assert.Equal(t, 5.0, converted["completion_ratio"].(map[string]any)["kimi-k3"])
	assert.Equal(t, 0.1, converted["cache_ratio"].(map[string]any)["kimi-k3"])
}

func TestOfficialModelMetadataSyncCreatesKimiVendor(t *testing.T) {
	setupOfficialMetadataServiceTestDB(t)

	cacheRatio := 0.1
	require.NoError(t, model.DB.Create(&model.OfficialModelPrice{
		Provider:        OfficialPricingProviderKimi,
		ModelName:       "kimi-k3",
		SourceURL:       OfficialPricingKimiURL,
		ModelRatio:      1.5,
		CompletionRatio: 5,
		CacheRatio:      &cacheRatio,
		Active:          true,
	}).Error)

	result, err := SyncOfficialModelMetadata(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, result.CreatedModels)
	assert.Equal(t, 1, result.CreatedVendors)

	var vendor model.Vendor
	require.NoError(t, model.DB.Where("name = ?", "Moonshot").First(&vendor).Error)
	assert.Equal(t, "Moonshot", vendor.Icon)

	var synced model.Model
	require.NoError(t, model.DB.Where("model_name = ?", "kimi-k3").First(&synced).Error)
	assert.Equal(t, vendor.Id, synced.VendorID)
	assert.Equal(t, 1, synced.SyncOfficial)
}

func TestConvertGLMOfficialPricingToRatioData(t *testing.T) {
	body := `
# GLM-4.5

<Card><h3>GLM-4.5</h3></Card>
<Card><h3>GLM-4.5-Air</h3></Card>
<Card><h3>GLM-4.7</h3></Card>

API 调用价格低至**输入 0.8 元/百万 tokens，输出 2 元/百万 tokens**
`

	converted, err := ConvertGLMOfficialPricingToRatioData(strings.NewReader(body))
	require.NoError(t, err)

	modelRatio := converted["model_ratio"].(map[string]any)
	completionRatio := converted["completion_ratio"].(map[string]any)

	expectedRatio := 0.8 / ratio_setting.USD2RMB * float64(ratio_setting.USD) / 1000
	assert.Equal(t, roundOfficialRatioValue(expectedRatio), modelRatio["glm-4.5"])
	assert.Equal(t, roundOfficialRatioValue(expectedRatio), modelRatio["glm-4.5-air"])
	assert.Equal(t, 2.5, completionRatio["glm-4.5"])
	assert.NotContains(t, modelRatio, "glm-4.7")
}

func TestConvertOpenAIOfficialPricingIncludesCodex(t *testing.T) {
	body := `
<div data-content-switcher-pane data-value="standard">
<TextTokenPricingTables tier="standard" rows={[
  ["gpt-5-codex", 1.25, 0.125, 10],
]} />
</div>`

	converted, err := ConvertOpenAIOfficialPricingToRatioData(strings.NewReader(body))
	require.NoError(t, err)

	modelRatio := converted["model_ratio"].(map[string]any)
	completionRatio := converted["completion_ratio"].(map[string]any)
	cacheRatio := converted["cache_ratio"].(map[string]any)

	assert.Equal(t, 0.625, modelRatio["gpt-5-codex"])
	assert.Equal(t, 8.0, completionRatio["gpt-5-codex"])
	assert.Equal(t, 0.1, cacheRatio["gpt-5-codex"])
}

func TestParseOpenAIOfficialTokenPricingSupportsFourPriceColumns(t *testing.T) {
	body := `
<div data-content-switcher-pane data-value="standard">
<TextTokenPricingTables
  client:load
  tier="standard"
  rows={[
    ["gpt-5.6-sol", 5, 0.5, 6.25, 30],
    ["gpt-5.6-terra", 2.5, 0.25, 3.125, 15],
    ["gpt-5.6-luna", 1, 0.1, 1.25, 6],
    ["gpt-5.5 (<272K context length)", 5, 0.5, "-", 30],
    ["gpt-5.5-pro (<272K context length)", 30, "-", "-", 180],
    ["gpt-5.4-mini", 0.75, 0.075, "-", 4.5],
    ["gpt-5.2", 1.75, 0.175, 14],
    ["gpt-5-pro", 15, null, 120],
  ]}
/>
</div>
<div data-content-switcher-pane data-value="batch" hidden>
  <TextTokenPricingTables rows={[
    ["gpt-5.5 (<272K context length)", 2.5, 0.25, "-", 15],
  ]} />
</div>`

	entries, err := ParseOpenAIOfficialTokenPricing(strings.NewReader(body), OfficialPricingOpenAIURL)
	require.NoError(t, err)

	byModel := make(map[string]OfficialTokenPricing, len(entries))
	for _, entry := range entries {
		byModel[entry.Model] = entry
	}

	require.Contains(t, byModel, "gpt-5.6-sol")
	assert.Equal(t, 5.0, byModel["gpt-5.6-sol"].InputUSDPerMTok)
	assert.Equal(t, 30.0, byModel["gpt-5.6-sol"].OutputUSDPerMTok)
	require.NotNil(t, byModel["gpt-5.6-sol"].CacheReadUSDPerMTok)
	assert.Equal(t, 0.5, *byModel["gpt-5.6-sol"].CacheReadUSDPerMTok)

	require.Contains(t, byModel, "gpt-5.6-terra")
	assert.Equal(t, 2.5, byModel["gpt-5.6-terra"].InputUSDPerMTok)
	assert.Equal(t, 15.0, byModel["gpt-5.6-terra"].OutputUSDPerMTok)
	require.NotNil(t, byModel["gpt-5.6-terra"].CacheReadUSDPerMTok)
	assert.Equal(t, 0.25, *byModel["gpt-5.6-terra"].CacheReadUSDPerMTok)

	require.Contains(t, byModel, "gpt-5.6-luna")
	assert.Equal(t, 1.0, byModel["gpt-5.6-luna"].InputUSDPerMTok)
	assert.Equal(t, 6.0, byModel["gpt-5.6-luna"].OutputUSDPerMTok)
	require.NotNil(t, byModel["gpt-5.6-luna"].CacheReadUSDPerMTok)
	assert.Equal(t, 0.1, *byModel["gpt-5.6-luna"].CacheReadUSDPerMTok)

	converted, err := ConvertOfficialTokenPricingToRatioData([]OfficialTokenPricing{
		byModel["gpt-5.6-sol"],
		byModel["gpt-5.6-terra"],
		byModel["gpt-5.6-luna"],
	})
	require.NoError(t, err)
	modelRatio := converted["model_ratio"].(map[string]any)
	completionRatio := converted["completion_ratio"].(map[string]any)
	cacheRatio := converted["cache_ratio"].(map[string]any)
	assert.Equal(t, 2.5, modelRatio["gpt-5.6-sol"])
	assert.Equal(t, 1.25, modelRatio["gpt-5.6-terra"])
	assert.Equal(t, 0.5, modelRatio["gpt-5.6-luna"])
	assert.Equal(t, 6.0, completionRatio["gpt-5.6-sol"])
	assert.Equal(t, 6.0, completionRatio["gpt-5.6-terra"])
	assert.Equal(t, 6.0, completionRatio["gpt-5.6-luna"])
	assert.Equal(t, 0.1, cacheRatio["gpt-5.6-sol"])
	assert.Equal(t, 0.1, cacheRatio["gpt-5.6-terra"])
	assert.Equal(t, 0.1, cacheRatio["gpt-5.6-luna"])

	require.Contains(t, byModel, "gpt-5.5")
	assert.Equal(t, 5.0, byModel["gpt-5.5"].InputUSDPerMTok)
	assert.Equal(t, 30.0, byModel["gpt-5.5"].OutputUSDPerMTok)
	require.NotNil(t, byModel["gpt-5.5"].CacheReadUSDPerMTok)
	assert.Equal(t, 0.5, *byModel["gpt-5.5"].CacheReadUSDPerMTok)

	require.Contains(t, byModel, "gpt-5.5-pro")
	assert.Equal(t, 30.0, byModel["gpt-5.5-pro"].InputUSDPerMTok)
	assert.Equal(t, 180.0, byModel["gpt-5.5-pro"].OutputUSDPerMTok)
	assert.Nil(t, byModel["gpt-5.5-pro"].CacheReadUSDPerMTok)

	require.Contains(t, byModel, "gpt-5.4-mini")
	assert.Equal(t, 0.75, byModel["gpt-5.4-mini"].InputUSDPerMTok)
	assert.Equal(t, 4.5, byModel["gpt-5.4-mini"].OutputUSDPerMTok)

	require.Contains(t, byModel, "gpt-5.2")
	assert.Equal(t, 1.75, byModel["gpt-5.2"].InputUSDPerMTok)
	assert.Equal(t, 14.0, byModel["gpt-5.2"].OutputUSDPerMTok)

	require.Contains(t, byModel, "gpt-5-pro")
	assert.Equal(t, 15.0, byModel["gpt-5-pro"].InputUSDPerMTok)
	assert.Equal(t, 120.0, byModel["gpt-5-pro"].OutputUSDPerMTok)
	assert.Nil(t, byModel["gpt-5-pro"].CacheReadUSDPerMTok)
}

func TestMergePreservedOfficialPricingRowsKeepsFailedProviderCatalog(t *testing.T) {
	setupOfficialMetadataServiceTestDB(t)

	const (
		freshLastConfirmedAt = int64(12345)
		staleLastConfirmedAt = int64(67890)
	)

	require.NoError(t, model.DB.Create(&model.OfficialModelPrice{
		Provider:        OfficialPricingProviderOpenAI,
		ModelName:       "gpt-preserved-failed-provider",
		SourceURL:       OfficialPricingOpenAIURL,
		ModelRatio:      1.25,
		CompletionRatio: 6,
		Active:          true,
		LastConfirmedAt: freshLastConfirmedAt,
	}).Error)
	require.NoError(t, model.DB.Create(&model.OfficialModelPrice{
		Provider:        OfficialPricingProviderOpenAI,
		ModelName:       "gpt-stale-failed-provider",
		SourceURL:       OfficialPricingOpenAIURL,
		ModelRatio:      1.5,
		CompletionRatio: 6,
		Active:          true,
		Stale:           true,
		LastConfirmedAt: staleLastConfirmedAt,
	}).Error)
	require.NoError(t, model.DB.Create(&model.OfficialModelPrice{
		Provider:        OfficialPricingProviderClaude,
		ModelName:       "claude-should-not-preserve",
		SourceURL:       OfficialPricingClaudeURL,
		ModelRatio:      1.5,
		CompletionRatio: 5,
		Active:          true,
	}).Error)

	fresh := []model.OfficialModelPrice{{
		Provider:        OfficialPricingProviderClaude,
		ModelName:       "claude-fresh-ok",
		SourceURL:       OfficialPricingClaudeURL,
		ModelRatio:      2.5,
		CompletionRatio: 5,
	}}
	result := &OfficialPricingSyncResult{}
	merged, err := mergeOfficialPricingRows("op_test_merge", fresh, map[string]string{
		OfficialPricingProviderOpenAI: "openai source unavailable",
	}, result)
	require.NoError(t, err)

	byName := make(map[string]model.OfficialModelPrice, len(merged))
	for _, row := range merged {
		byName[row.ModelName] = row
	}
	require.Contains(t, byName, "claude-fresh-ok")
	require.Contains(t, byName, "gpt-preserved-failed-provider")
	require.Contains(t, byName, "gpt-stale-failed-provider")
	require.NotContains(t, byName, "claude-should-not-preserve")
	freshPreserved := byName["gpt-preserved-failed-provider"]
	assert.Equal(t, "op_test_merge", freshPreserved.SnapshotID)
	assert.Equal(t, int64(0), freshPreserved.ID)
	assert.True(t, freshPreserved.Active)
	assert.False(t, freshPreserved.Stale)
	assert.Equal(t, freshLastConfirmedAt, freshPreserved.LastConfirmedAt)
	stalePreserved := byName["gpt-stale-failed-provider"]
	assert.True(t, stalePreserved.Stale)
	assert.Equal(t, staleLastConfirmedAt, stalePreserved.LastConfirmedAt)
	assert.Equal(t, 2, result.CarriedCount)
	assert.Equal(t, 1, result.StaleCount)
}

func TestMergeOfficialPricingRowsMarksEnabledMissingModelsStaleAndClearsOnReappearance(t *testing.T) {
	setupOfficialMetadataServiceTestDB(t)

	const modelName = "gpt-stale-lifecycle"
	const lastConfirmedAt = int64(12345)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:     801,
		Key:    "stale-pricing-key",
		Name:   "stale-pricing-channel",
		Status: common.ChannelStatusEnabled,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group:     "default",
		Model:     modelName,
		ChannelId: 801,
		Enabled:   true,
	}).Error)
	require.NoError(t, model.DB.Create(&model.OfficialModelPrice{
		Provider:        OfficialPricingProviderOpenAI,
		ModelName:       modelName,
		SourceURL:       OfficialPricingOpenAIURL,
		ModelRatio:      1.25,
		CompletionRatio: 6,
		Active:          true,
		LastConfirmedAt: lastConfirmedAt,
	}).Error)

	staleResult := &OfficialPricingSyncResult{}
	staleRows, err := mergeOfficialPricingRows("op_stale", []model.OfficialModelPrice{{
		Provider:        OfficialPricingProviderClaude,
		ModelName:       "claude-fresh",
		SourceURL:       OfficialPricingClaudeURL,
		ModelRatio:      2,
		CompletionRatio: 5,
	}}, nil, staleResult)
	require.NoError(t, err)
	require.Len(t, staleRows, 2)
	assert.Equal(t, 1, staleResult.FreshCount)
	assert.Equal(t, 1, staleResult.CarriedCount)
	assert.Equal(t, 1, staleResult.StaleCount)
	assert.Zero(t, staleResult.RemovedCount)

	var staleRow model.OfficialModelPrice
	for _, row := range staleRows {
		if row.ModelName == modelName {
			staleRow = row
		}
	}
	assert.True(t, staleRow.Stale)
	assert.Equal(t, lastConfirmedAt, staleRow.LastConfirmedAt)
	assert.Equal(t, 1.25, staleRow.ModelRatio)
	require.NoError(t, model.ReplaceActiveOfficialPricing("op_stale", "test", staleRows))

	freshResult := &OfficialPricingSyncResult{}
	freshRows, err := mergeOfficialPricingRows("op_fresh", []model.OfficialModelPrice{{
		Provider:        OfficialPricingProviderOpenAI,
		ModelName:       modelName,
		SourceURL:       OfficialPricingOpenAIURL,
		ModelRatio:      2.5,
		CompletionRatio: 6,
		LastConfirmedAt: 23456,
	}}, nil, freshResult)
	require.NoError(t, err)
	require.Len(t, freshRows, 1)
	assert.False(t, freshRows[0].Stale)
	assert.Equal(t, int64(23456), freshRows[0].LastConfirmedAt)
	assert.Equal(t, 2.5, freshRows[0].ModelRatio)
	assert.Equal(t, 1, freshResult.FreshCount)
	assert.Zero(t, freshResult.CarriedCount)
	assert.Zero(t, freshResult.StaleCount)
	assert.Equal(t, 1, freshResult.RemovedCount)
}

func TestConvertXAIOfficialPricingToRatioData(t *testing.T) {
	body := `
| Model | Context | Input / 1M tokens | Output / 1M tokens |
| --- | --- | --- | --- |
| grok-4.3 | 1M | $1.25 | $2.50 |
| grok-build-0.1 | 256k | $1.00 | $2.00 |
`

	converted, err := ConvertXAIOfficialPricingToRatioData(strings.NewReader(body))
	require.NoError(t, err)

	modelRatio := converted["model_ratio"].(map[string]any)
	completionRatio := converted["completion_ratio"].(map[string]any)

	assert.Equal(t, 0.625, modelRatio["grok-4.3"])
	assert.Equal(t, 2.0, completionRatio["grok-4.3"])
	assert.Equal(t, 0.5, modelRatio["grok-build-0.1"])
}

func TestConvertDeepSeekOfficialPricingToRatioData(t *testing.T) {
	body := `
**- MODEL deepseek-v4-flash(1) deepseek-v4-pro
- BASE URL (OpenAI Format) https://api.deepseek.com
- PRICING 1M INPUT TOKENS (CACHE HIT) $0.0028 $0.003625
- 1M INPUT TOKENS (CACHE MISS) $0.14 $0.435
- 1M OUTPUT TOKENS $0.28 $0.87**

(1) The model names ` + "`deepseek-chat`" + ` and ` + "`deepseek-reasoner`" + ` correspond to deepseek-v4-flash.
`

	converted, err := ConvertDeepSeekOfficialPricingToRatioData(strings.NewReader(body))
	require.NoError(t, err)

	modelRatio := converted["model_ratio"].(map[string]any)
	completionRatio := converted["completion_ratio"].(map[string]any)
	cacheRatio := converted["cache_ratio"].(map[string]any)

	expectedFlashRatio := 0.14 * float64(ratio_setting.USD) / 1000
	expectedProRatio := 0.435 * float64(ratio_setting.USD) / 1000
	assert.Equal(t, roundOfficialRatioValue(expectedFlashRatio), modelRatio["deepseek-v4-flash"])
	assert.Equal(t, roundOfficialRatioValue(expectedFlashRatio), modelRatio["deepseek-chat"])
	assert.Equal(t, roundOfficialRatioValue(expectedFlashRatio), modelRatio["deepseek-reasoner"])
	assert.Equal(t, roundOfficialRatioValue(expectedProRatio), modelRatio["deepseek-v4-pro"])
	assert.Equal(t, 2.0, completionRatio["deepseek-v4-flash"])
	assert.Equal(t, 2.0, completionRatio["deepseek-v4-pro"])
	assert.Equal(t, roundOfficialRatioValue(0.0028/0.14), cacheRatio["deepseek-v4-flash"])
	assert.Equal(t, roundOfficialRatioValue(0.003625/0.435), cacheRatio["deepseek-v4-pro"])
}
