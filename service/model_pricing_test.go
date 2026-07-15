package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func pricingFloat(value float64) *float64 {
	return &value
}

func TestSaveModelPricingBatchReplacesManualSnapshotAndRestoreResolvesAuthority(t *testing.T) {
	setupOfficialMetadataServiceTestDB(t)

	officialCacheRatio := 0.2
	require.NoError(t, model.ReplaceActiveOfficialPricing("manual-snapshot-official", "test", []model.OfficialModelPrice{{
		Provider:        OfficialPricingProviderOpenAI,
		ModelName:       "gpt-manual-snapshot",
		ModelRatio:      1.25,
		CompletionRatio: 6,
		CacheRatio:      &officialCacheRatio,
	}}))

	_, err := SaveModelPricingBatch(context.Background(), ModelPricingBatchRequest{Upserts: []ModelPricingMutation{{
		ModelName: "gpt-manual-snapshot",
		Config: model.ModelPricingConfig{
			Mode:            model.ModelPricingModePerToken,
			Ratio:           pricingFloat(4),
			CompletionRatio: pricingFloat(8),
			CacheRatio:      pricingFloat(0.5),
		},
	}}})
	require.NoError(t, err)

	views, err := ListModelPricing(context.Background())
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, model.AuthorityLevelManual, views[0].Authority)
	require.NotNil(t, views[0].ManualConfig)
	require.NotNil(t, views[0].ManualConfig.CacheRatio)
	assert.Equal(t, 0.5, *views[0].ManualConfig.CacheRatio)

	_, err = SaveModelPricingBatch(context.Background(), ModelPricingBatchRequest{Upserts: []ModelPricingMutation{{
		ModelName: "gpt-manual-snapshot",
		Config: model.ModelPricingConfig{
			Mode:  model.ModelPricingModePerToken,
			Ratio: pricingFloat(7),
		},
	}}})
	require.NoError(t, err)

	views, err = ListModelPricing(context.Background())
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, model.AuthorityLevelManual, views[0].Authority)
	require.NotNil(t, views[0].ManualConfig)
	require.NotNil(t, views[0].ManualConfig.Ratio)
	assert.Equal(t, 7.0, *views[0].ManualConfig.Ratio)
	assert.Nil(t, views[0].ManualConfig.CompletionRatio)
	assert.Nil(t, views[0].ManualConfig.CacheRatio)

	_, err = SaveModelPricingBatch(context.Background(), ModelPricingBatchRequest{Restore: []string{"gpt-manual-snapshot"}})
	require.NoError(t, err)

	views, err = ListModelPricing(context.Background())
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, model.AuthorityLevelOfficial, views[0].Authority)
	assert.Nil(t, views[0].ManualConfig)
	require.NotNil(t, views[0].OfficialConfig)
	require.NotNil(t, views[0].OfficialConfig.Ratio)
	assert.Equal(t, 1.25, *views[0].OfficialConfig.Ratio)
	require.NotNil(t, views[0].OfficialConfig.CacheRatio)
	assert.Equal(t, officialCacheRatio, *views[0].OfficialConfig.CacheRatio)
}

func TestSaveModelPricingBatchRestoreWithoutOfficialReturnsFallbackOrUnconfigured(t *testing.T) {
	setupOfficialMetadataServiceTestDB(t)
	previousModelRatio := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatio))
	})

	require.NoError(t, model.DB.Create(&model.Option{
		Key:   "ModelRatio",
		Value: `{"fallback-after-restore":3}`,
	}).Error)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"fallback-after-restore":3}`))
	_, err := SaveModelPricingBatch(context.Background(), ModelPricingBatchRequest{Upserts: []ModelPricingMutation{
		{
			ModelName: "fallback-after-restore",
			Config: model.ModelPricingConfig{
				Mode:  model.ModelPricingModePerToken,
				Ratio: pricingFloat(8),
			},
		},
		{
			ModelName: "unconfigured-after-restore",
			Config: model.ModelPricingConfig{
				Mode:  model.ModelPricingModePerToken,
				Ratio: pricingFloat(9),
			},
		},
	}})
	require.NoError(t, err)

	_, err = SaveModelPricingBatch(context.Background(), ModelPricingBatchRequest{Restore: []string{
		"fallback-after-restore",
		"unconfigured-after-restore",
	}})
	require.NoError(t, err)

	views, err := ListModelPricing(context.Background())
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, "fallback-after-restore", views[0].ModelName)
	assert.Equal(t, model.AuthorityLevelFallback, views[0].Authority)
	require.NotNil(t, views[0].EffectiveConfig.Ratio)
	assert.Equal(t, 3.0, *views[0].EffectiveConfig.Ratio)
}

func TestManualPricingSurvivesStaleOfficialRuntimeRefresh(t *testing.T) {
	setupOfficialMetadataServiceTestDB(t)

	require.NoError(t, model.ReplaceActiveOfficialPricing("official-before-manual", "test", []model.OfficialModelPrice{{
		Provider:        OfficialPricingProviderOpenAI,
		ModelName:       "gpt-refresh-manual",
		ModelRatio:      1,
		CompletionRatio: 2,
	}}))
	_, err := SaveModelPricingBatch(context.Background(), ModelPricingBatchRequest{Upserts: []ModelPricingMutation{{
		ModelName: "gpt-refresh-manual",
		Config: model.ModelPricingConfig{
			Mode:  model.ModelPricingModePerToken,
			Ratio: pricingFloat(7),
		},
	}}})
	require.NoError(t, err)

	require.NoError(t, model.ReplaceActiveOfficialPricing("official-after-manual", "test", []model.OfficialModelPrice{{
		Provider:        OfficialPricingProviderOpenAI,
		ModelName:       "gpt-refresh-manual",
		ModelRatio:      9,
		CompletionRatio: 2,
		Stale:           true,
	}}))
	require.NoError(t, RefreshModelPricingRuntime())

	ratio, found, _ := ratio_setting.GetModelRatio("gpt-refresh-manual")
	require.True(t, found)
	assert.Equal(t, 7.0, ratio)
	views, err := ListModelPricing(context.Background())
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, model.AuthorityLevelManual, views[0].Authority)
}

func TestResetFallbackModelPricingPreservesManualOverride(t *testing.T) {
	setupOfficialMetadataServiceTestDB(t)
	previousModelRatio := ratio_setting.ModelRatio2JSONString()
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatio))
	})

	_, err := SaveModelPricingBatch(context.Background(), ModelPricingBatchRequest{Upserts: []ModelPricingMutation{{
		ModelName: "gpt-reset-manual",
		Config: model.ModelPricingConfig{
			Mode:  model.ModelPricingModePerToken,
			Ratio: pricingFloat(6),
		},
	}}})
	require.NoError(t, err)
	require.NoError(t, ResetFallbackModelPricing())

	ratio, found, _ := ratio_setting.GetModelRatio("gpt-reset-manual")
	require.True(t, found)
	assert.Equal(t, 6.0, ratio)
	views, err := ListModelPricing(context.Background())
	require.NoError(t, err)
	var manualView *ModelPricingView
	for i := range views {
		if views[i].ModelName == "gpt-reset-manual" {
			manualView = &views[i]
			break
		}
	}
	require.NotNil(t, manualView)
	assert.Equal(t, model.AuthorityLevelManual, manualView.Authority)
}

func TestReloadPricingRuntimeIfRevisionChangedLoadsManualOverride(t *testing.T) {
	setupOfficialMetadataServiceTestDB(t)

	require.NoError(t, model.ReplaceActiveOfficialPricing("revision-before", "test", []model.OfficialModelPrice{{
		Provider:        OfficialPricingProviderOpenAI,
		ModelName:       "gpt-revision-reload",
		ModelRatio:      1,
		CompletionRatio: 2,
	}}))
	require.NoError(t, model.RefreshPricingRuntime())

	ratio, found, _ := ratio_setting.GetModelRatio("gpt-revision-reload")
	require.True(t, found)
	assert.Equal(t, 1.0, ratio)

	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.UpsertModelPricingOverrideTx(tx, "gpt-revision-reload", model.ModelPricingConfig{
			Mode:  model.ModelPricingModePerToken,
			Ratio: pricingFloat(5),
		}, model.ModelPricingOverrideOriginAdmin); err != nil {
			return err
		}
		_, err := model.BumpPricingRuntimeRevisionTx(tx)
		return err
	}))
	require.NoError(t, model.ReloadPricingRuntimeIfRevisionChanged())

	ratio, found, _ = ratio_setting.GetModelRatio("gpt-revision-reload")
	require.True(t, found)
	assert.Equal(t, 5.0, ratio)
}

func TestResetFallbackModelPricingResetsCompletedLegacyFallbackWithoutPersistingOptions(t *testing.T) {
	setupOfficialMetadataServiceTestDB(t)

	previousFallbackStates := []struct {
		value   string
		restore func(string) error
	}{
		{ratio_setting.ModelPrice2JSONString(), ratio_setting.UpdateModelPriceByJSONString},
		{ratio_setting.ModelRatio2JSONString(), ratio_setting.UpdateModelRatioByJSONString},
		{ratio_setting.CompletionRatio2JSONString(), ratio_setting.UpdateCompletionRatioByJSONString},
		{ratio_setting.CacheRatio2JSONString(), ratio_setting.UpdateCacheRatioByJSONString},
		{ratio_setting.CreateCacheRatio2JSONString(), ratio_setting.UpdateCreateCacheRatioByJSONString},
		{ratio_setting.ImageRatio2JSONString(), ratio_setting.UpdateImageRatioByJSONString},
		{ratio_setting.AudioRatio2JSONString(), ratio_setting.UpdateAudioRatioByJSONString},
		{ratio_setting.AudioCompletionRatio2JSONString(), ratio_setting.UpdateAudioCompletionRatioByJSONString},
	}
	t.Cleanup(func() {
		for _, state := range previousFallbackStates {
			require.NoError(t, state.restore(state.value))
		}
	})

	legacyOptions := []model.Option{
		{Key: "ModelPrice", Value: `{"legacy-reset-price":11}`},
		{Key: "ModelRatio", Value: `{"gpt-4":777,"legacy-reset-ratio":12}`},
		{Key: "CompletionRatio", Value: `{"legacy-reset-completion":13}`},
		{Key: "CacheRatio", Value: `{"legacy-reset-cache":14}`},
		{Key: "CreateCacheRatio", Value: `{"legacy-reset-create-cache":15}`},
		{Key: "ImageRatio", Value: `{"legacy-reset-image":16}`},
		{Key: "AudioRatio", Value: `{"legacy-reset-audio":17}`},
		{Key: "AudioCompletionRatio", Value: `{"legacy-reset-audio-completion":18}`},
	}
	require.NoError(t, model.DB.Create(&legacyOptions).Error)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(legacyOptions[0].Value))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(legacyOptions[1].Value))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(legacyOptions[2].Value))
	require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(legacyOptions[3].Value))
	require.NoError(t, ratio_setting.UpdateCreateCacheRatioByJSONString(legacyOptions[4].Value))
	require.NoError(t, ratio_setting.UpdateImageRatioByJSONString(legacyOptions[5].Value))
	require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString(legacyOptions[6].Value))
	require.NoError(t, ratio_setting.UpdateAudioCompletionRatioByJSONString(legacyOptions[7].Value))

	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		return model.UpsertModelPricingOverrideTx(tx, "manual-reset-priority", model.ModelPricingConfig{
			Mode:  model.ModelPricingModePerToken,
			Ratio: pricingFloat(6),
		}, model.ModelPricingOverrideOriginAdmin)
	}))
	require.NoError(t, model.DB.Create(&model.Option{Key: "ModelPricingOverrideMigrationVersion", Value: "1"}).Error)

	require.NoError(t, ResetFallbackModelPricing())

	expectedFallback, exists := ratio_setting.GetDefaultModelRatioMap()["gpt-4"]
	require.True(t, exists)
	fallbackRatio, found, _ := ratio_setting.GetModelRatio("gpt-4")
	require.True(t, found)
	assert.Equal(t, expectedFallback, fallbackRatio)
	manualRatio, found, _ := ratio_setting.GetModelRatio("manual-reset-priority")
	require.True(t, found)
	assert.Equal(t, 6.0, manualRatio)

	views, err := ListModelPricing(context.Background())
	require.NoError(t, err)
	viewsByName := make(map[string]ModelPricingView, len(views))
	for _, view := range views {
		viewsByName[view.ModelName] = view
	}
	fallbackView, exists := viewsByName["gpt-4"]
	require.True(t, exists)
	assert.Equal(t, model.AuthorityLevelFallback, fallbackView.Authority)
	require.NotNil(t, fallbackView.EffectiveConfig.Ratio)
	assert.Equal(t, expectedFallback, *fallbackView.EffectiveConfig.Ratio)
	manualView, exists := viewsByName["manual-reset-priority"]
	require.True(t, exists)
	assert.Equal(t, model.AuthorityLevelManual, manualView.Authority)
	require.NotNil(t, manualView.EffectiveConfig.Ratio)
	assert.Equal(t, 6.0, *manualView.EffectiveConfig.Ratio)

	for _, fallback := range []struct {
		values map[string]float64
		key    string
	}{
		{ratio_setting.GetModelPriceCopy(), "legacy-reset-price"},
		{ratio_setting.GetModelRatioCopy(), "legacy-reset-ratio"},
		{ratio_setting.GetCompletionRatioCopy(), "legacy-reset-completion"},
		{ratio_setting.GetCacheRatioCopy(), "legacy-reset-cache"},
		{ratio_setting.GetCreateCacheRatioCopy(), "legacy-reset-create-cache"},
		{ratio_setting.GetImageRatioCopy(), "legacy-reset-image"},
		{ratio_setting.GetAudioRatioCopy(), "legacy-reset-audio"},
		{ratio_setting.GetAudioCompletionRatioCopy(), "legacy-reset-audio-completion"},
	} {
		_, found := fallback.values[fallback.key]
		assert.False(t, found, "reset must remove the custom legacy fallback %s", fallback.key)
	}

	for _, expected := range legacyOptions {
		var persisted model.Option
		require.NoError(t, model.DB.Where("key = ?", expected.Key).First(&persisted).Error)
		assert.Equal(t, expected.Value, persisted.Value)
	}
}

func TestSaveModelPricingBatchRestoreMissingOverrideDoesNotAdvanceRevision(t *testing.T) {
	setupOfficialMetadataServiceTestDB(t)

	previousRevision, err := model.GetPricingRuntimeRevision()
	require.NoError(t, err)
	result, err := SaveModelPricingBatch(context.Background(), ModelPricingBatchRequest{
		Restore: []string{"missing-manual-pricing-override"},
	})
	require.NoError(t, err)
	assert.Zero(t, result.Restored)
	assert.Equal(t, previousRevision, result.Revision)

	currentRevision, err := model.GetPricingRuntimeRevision()
	require.NoError(t, err)
	assert.Equal(t, previousRevision, currentRevision)
}

func TestListModelPricingAliasesMatchRuntimeAuthorityAndEffectiveConfig(t *testing.T) {
	testCases := []struct {
		name              string
		setup             func(*testing.T)
		viewModelName     string
		runtimeModelName  string
		runtimeMatchName  string
		wantAuthority     model.AuthorityLevel
		wantEffectiveRate float64
	}{
		{
			name: "official Gemini base covers thinking alias",
			setup: func(t *testing.T) {
				require.NoError(t, model.ReplaceActiveOfficialPricing("gemini-thinking-official", "test", []model.OfficialModelPrice{{
					Provider:        OfficialPricingProviderGemini,
					ModelName:       "gemini-2.5-flash",
					ModelRatio:      1.25,
					CompletionRatio: 2,
				}}))
				require.NoError(t, RefreshModelPricingRuntime())
			},
			viewModelName:     "gemini-2.5-flash-thinking-*",
			runtimeModelName:  "gemini-2.5-flash-thinking-8192",
			runtimeMatchName:  "gemini-2.5-flash-thinking-*",
			wantAuthority:     model.AuthorityLevelOfficial,
			wantEffectiveRate: 1.25,
		},
		{
			name: "base manual Gemini override covers thinking alias",
			setup: func(t *testing.T) {
				require.NoError(t, model.ReplaceActiveOfficialPricing("gemini-thinking-manual", "test", []model.OfficialModelPrice{{
					Provider:        OfficialPricingProviderGemini,
					ModelName:       "gemini-2.5-flash",
					ModelRatio:      1.25,
					CompletionRatio: 2,
				}}))
				_, err := SaveModelPricingBatch(context.Background(), ModelPricingBatchRequest{Upserts: []ModelPricingMutation{{
					ModelName: "gemini-2.5-flash",
					Config: model.ModelPricingConfig{
						Mode:  model.ModelPricingModePerToken,
						Ratio: pricingFloat(4),
					},
				}}})
				require.NoError(t, err)
			},
			viewModelName:     "gemini-2.5-flash-thinking-*",
			runtimeModelName:  "gemini-2.5-flash-thinking-8192",
			runtimeMatchName:  "gemini-2.5-flash-thinking-*",
			wantAuthority:     model.AuthorityLevelManual,
			wantEffectiveRate: 4,
		},
		{
			name: "exact manual Gemini alias wins over base manual alias",
			setup: func(t *testing.T) {
				require.NoError(t, model.ReplaceActiveOfficialPricing("gemini-thinking-exact-manual", "test", []model.OfficialModelPrice{{
					Provider:        OfficialPricingProviderGemini,
					ModelName:       "gemini-2.5-flash",
					ModelRatio:      1.25,
					CompletionRatio: 2,
				}}))
				_, err := SaveModelPricingBatch(context.Background(), ModelPricingBatchRequest{Upserts: []ModelPricingMutation{
					{
						ModelName: "gemini-2.5-flash",
						Config: model.ModelPricingConfig{
							Mode:  model.ModelPricingModePerToken,
							Ratio: pricingFloat(4),
						},
					},
					{
						ModelName: "gemini-2.5-flash-thinking-*",
						Config: model.ModelPricingConfig{
							Mode:  model.ModelPricingModePerToken,
							Ratio: pricingFloat(5),
						},
					},
				}})
				require.NoError(t, err)
			},
			viewModelName:     "gemini-2.5-flash-thinking-*",
			runtimeModelName:  "gemini-2.5-flash-thinking-8192",
			runtimeMatchName:  "gemini-2.5-flash-thinking-*",
			wantAuthority:     model.AuthorityLevelManual,
			wantEffectiveRate: 5,
		},
		{
			name: "base official config resolves compact model",
			setup: func(t *testing.T) {
				compactModelName := ratio_setting.WithCompactModelSuffix("compact-official-base")
				previousModelRatio := ratio_setting.ModelRatio2JSONString()
				t.Cleanup(func() {
					require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatio))
				})
				require.NoError(t, model.DB.Create(&model.Option{Key: "ModelRatio", Value: `{"` + compactModelName + `":99}`}).Error)
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"`+compactModelName+`":99}`))
				require.NoError(t, model.ReplaceActiveOfficialPricing("compact-official", "test", []model.OfficialModelPrice{{
					Provider:        OfficialPricingProviderOpenAI,
					ModelName:       "compact-official-base",
					ModelRatio:      1.5,
					CompletionRatio: 2,
				}}))
				require.NoError(t, RefreshModelPricingRuntime())
			},
			viewModelName:     "compact-official-base-openai-compact",
			runtimeModelName:  "compact-official-base-openai-compact",
			runtimeMatchName:  "compact-official-base-openai-compact",
			wantAuthority:     model.AuthorityLevelOfficial,
			wantEffectiveRate: 1.5,
		},
		{
			name: "base manual config resolves compact model",
			setup: func(t *testing.T) {
				compactModelName := ratio_setting.WithCompactModelSuffix("compact-manual-base")
				previousModelRatio := ratio_setting.ModelRatio2JSONString()
				t.Cleanup(func() {
					require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatio))
				})
				require.NoError(t, model.DB.Create(&model.Option{Key: "ModelRatio", Value: `{"` + compactModelName + `":99}`}).Error)
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"`+compactModelName+`":99}`))
				require.NoError(t, model.ReplaceActiveOfficialPricing("compact-manual", "test", []model.OfficialModelPrice{{
					Provider:        OfficialPricingProviderOpenAI,
					ModelName:       "compact-manual-base",
					ModelRatio:      1.5,
					CompletionRatio: 2,
				}}))
				_, err := SaveModelPricingBatch(context.Background(), ModelPricingBatchRequest{Upserts: []ModelPricingMutation{{
					ModelName: "compact-manual-base",
					Config: model.ModelPricingConfig{
						Mode:  model.ModelPricingModePerToken,
						Ratio: pricingFloat(6),
					},
				}}})
				require.NoError(t, err)
			},
			viewModelName:     "compact-manual-base-openai-compact",
			runtimeModelName:  "compact-manual-base-openai-compact",
			runtimeMatchName:  "compact-manual-base-openai-compact",
			wantAuthority:     model.AuthorityLevelManual,
			wantEffectiveRate: 6,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				require.NoError(t, model.RefreshPricingRuntime())
			})
			setupOfficialMetadataServiceTestDB(t)
			tc.setup(t)

			views, err := ListModelPricing(context.Background())
			require.NoError(t, err)
			var view *ModelPricingView
			for i := range views {
				if views[i].ModelName == tc.viewModelName {
					view = &views[i]
					break
				}
			}
			require.NotNil(t, view)
			assert.Equal(t, tc.wantAuthority, view.Authority)
			require.NotNil(t, view.EffectiveConfig.Ratio)
			assert.Equal(t, tc.wantEffectiveRate, *view.EffectiveConfig.Ratio)

			runtimeRatio, found, matchedName := ratio_setting.GetModelRatio(tc.runtimeModelName)
			require.True(t, found)
			assert.Equal(t, tc.runtimeMatchName, matchedName)
			assert.Equal(t, tc.wantEffectiveRate, runtimeRatio)
		})
	}
}

func TestSaveModelPricingBatchReportsCommittedOverrideWhenRuntimeRefreshFails(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, model.RefreshPricingRuntime())
	})
	setupOfficialMetadataServiceTestDB(t)

	revisionBefore, err := model.GetPricingRuntimeRevision()
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ModelPricingOverride{
		ModelName:     "invalid-pricing-runtime-entry",
		PricingConfig: `{"mode":"per-token","ratio":`,
		Origin:        model.ModelPricingOverrideOriginAdmin,
	}).Error)
	committedConfig := model.ModelPricingConfig{
		Mode:                 model.ModelPricingModePerToken,
		Ratio:                pricingFloat(3),
		CompletionRatio:      pricingFloat(6),
		CacheRatio:           pricingFloat(0.5),
		CreateCacheRatio:     pricingFloat(1),
		ImageRatio:           pricingFloat(2),
		AudioRatio:           pricingFloat(4),
		AudioCompletionRatio: pricingFloat(8),
	}

	result, err := SaveModelPricingBatch(context.Background(), ModelPricingBatchRequest{Upserts: []ModelPricingMutation{{
		ModelName: "valid-pricing-despite-refresh-failure",
		Config:    committedConfig,
	}}})
	require.Error(t, err)
	assert.ErrorContains(t, err, "model pricing was committed, but runtime refresh failed; automatic retry is pending")
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Updated)
	assert.NotEqual(t, revisionBefore, result.Revision)

	currentRevision, err := model.GetPricingRuntimeRevision()
	require.NoError(t, err)
	assert.Equal(t, result.Revision, currentRevision)

	var committed model.ModelPricingOverride
	require.NoError(t, model.DB.Where("model_name = ?", "valid-pricing-despite-refresh-failure").First(&committed).Error)
	savedConfig, valid, err := model.ParseModelPricingConfig(committed.PricingConfig)
	require.NoError(t, err)
	require.True(t, valid)
	assert.Equal(t, committedConfig, savedConfig)
}

func TestModelPricingConflictAcknowledgementFlow(t *testing.T) {
	setupOfficialMetadataServiceTestDB(t)
	modelName := "gpt-pricing-conflict"

	replaceOfficial := func(snapshotID string, ratio float64) {
		t.Helper()
		require.NoError(t, model.ReplaceActiveOfficialPricing(snapshotID, "test", []model.OfficialModelPrice{{
			Provider:        OfficialPricingProviderOpenAI,
			ModelName:       modelName,
			ModelRatio:      ratio,
			CompletionRatio: 2,
		}}))
	}
	findView := func() ModelPricingView {
		t.Helper()
		views, err := ListModelPricing(context.Background())
		require.NoError(t, err)
		for _, view := range views {
			if view.ModelName == modelName {
				return view
			}
		}
		require.FailNow(t, "pricing view not found")
		return ModelPricingView{}
	}

	replaceOfficial("pricing-conflict-initial", 1)
	_, err := SaveModelPricingBatch(context.Background(), ModelPricingBatchRequest{Upserts: []ModelPricingMutation{{
		ModelName: modelName,
		Config: model.ModelPricingConfig{
			Mode:            model.ModelPricingModePerToken,
			Ratio:           pricingFloat(4),
			CompletionRatio: pricingFloat(8),
		},
	}}})
	require.NoError(t, err)

	view := findView()
	assert.Equal(t, model.AuthorityLevelManual, view.Authority)
	assert.False(t, view.PricingConflict, "saving manual pricing acknowledges the current official config")
	require.NotEmpty(t, view.OfficialConfigHash)

	replaceOfficial("pricing-conflict-same-price", 1)
	view = findView()
	assert.False(t, view.PricingConflict, "a new snapshot with the same price must not prompt again")

	replaceOfficial("pricing-conflict-changed", 3)
	view = findView()
	assert.True(t, view.PricingConflict)
	require.NotNil(t, view.EffectiveConfig.Ratio)
	assert.Equal(t, 4.0, *view.EffectiveConfig.Ratio, "manual pricing remains effective during conflict")
	changedOfficialHash := view.OfficialConfigHash

	_, err = SaveModelPricingBatch(context.Background(), ModelPricingBatchRequest{Acknowledge: []ModelPricingAcknowledgement{{
		ModelName:          modelName,
		OfficialConfigHash: "stale-official-hash",
	}}})
	require.Error(t, err)
	assert.ErrorContains(t, err, "official pricing changed")
	assert.True(t, findView().PricingConflict, "stale acknowledgement must not update review state")

	acknowledged, err := SaveModelPricingBatch(context.Background(), ModelPricingBatchRequest{Acknowledge: []ModelPricingAcknowledgement{{
		ModelName:          modelName,
		OfficialConfigHash: changedOfficialHash,
	}}})
	require.NoError(t, err)
	assert.Equal(t, 1, acknowledged.Acknowledged)
	assert.False(t, findView().PricingConflict)

	replaceOfficial("pricing-conflict-changed-again", 5)
	assert.True(t, findView().PricingConflict, "a later official price change must prompt again")

	_, err = SaveModelPricingBatch(context.Background(), ModelPricingBatchRequest{Restore: []string{modelName}})
	require.NoError(t, err)
	view = findView()
	assert.Equal(t, model.AuthorityLevelOfficial, view.Authority)
	assert.False(t, view.PricingConflict)
	require.NotNil(t, view.EffectiveConfig.Ratio)
	assert.Equal(t, 5.0, *view.EffectiveConfig.Ratio)
}

func TestModelPricingConflictDoesNotRepeatForExpandedAliases(t *testing.T) {
	setupOfficialMetadataServiceTestDB(t)
	modelName := "gemini-2.5-flash"
	require.NoError(t, model.ReplaceActiveOfficialPricing("pricing-conflict-alias-initial", "test", []model.OfficialModelPrice{{
		Provider:        OfficialPricingProviderGemini,
		ModelName:       modelName,
		ModelRatio:      1,
		CompletionRatio: 2,
	}}))
	_, err := SaveModelPricingBatch(context.Background(), ModelPricingBatchRequest{Upserts: []ModelPricingMutation{{
		ModelName: modelName,
		Config: model.ModelPricingConfig{
			Mode:  model.ModelPricingModePerToken,
			Ratio: pricingFloat(4),
		},
	}}})
	require.NoError(t, err)
	require.NoError(t, model.ReplaceActiveOfficialPricing("pricing-conflict-alias-changed", "test", []model.OfficialModelPrice{{
		Provider:        OfficialPricingProviderGemini,
		ModelName:       modelName,
		ModelRatio:      2,
		CompletionRatio: 2,
	}}))

	views, err := ListModelPricing(context.Background())
	require.NoError(t, err)
	conflicts := make([]string, 0)
	for _, view := range views {
		if view.PricingConflict {
			conflicts = append(conflicts, view.ModelName)
		}
	}
	assert.Equal(t, []string{modelName}, conflicts)
}
