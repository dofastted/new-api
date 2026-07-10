package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type modelPricingControllerResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    model.Model `json:"data"`
}

type modelPricingOptionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func setupModelPricingControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.ModelPricingOverride{},
		&model.OfficialPricingSnapshot{},
		&model.OfficialModelPrice{},
		&model.Option{},
	))
	t.Cleanup(func() {
		ratio_setting.ReplaceModelMetadataPricing(nil)
		ratio_setting.ReplaceOfficialPricing(nil, false)
		model.InvalidatePricingCache()
	})
	return db
}

func modelPricingControllerBody(t *testing.T, item model.Model) *bytes.Reader {
	t.Helper()
	payload, err := common.Marshal(item)
	require.NoError(t, err)
	return bytes.NewReader(payload)
}

func invokeModelPricingController(t *testing.T, method string, target string, body *bytes.Reader, params gin.Params, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	if body == nil {
		ctx.Request = httptest.NewRequest(method, target, nil)
	} else {
		ctx.Request = httptest.NewRequest(method, target, body)
	}
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = params
	handler(ctx)
	return recorder
}

func parseModelPricingControllerResponse(t *testing.T, recorder *httptest.ResponseRecorder) modelPricingControllerResponse {
	t.Helper()
	var response modelPricingControllerResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func seedModelPricingControllerOfficialPrice(t *testing.T, modelName string, ratio float64, completionRatio float64) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.OfficialModelPrice{
		Provider:        "test",
		ModelName:       modelName,
		ModelRatio:      ratio,
		CompletionRatio: completionRatio,
		Active:          true,
	}).Error)
}

func TestCreateModelMetaCreatesManualPricingOverride(t *testing.T) {
	setupModelPricingControllerTestDB(t)
	pricingConfig := `{"mode":"per-token","ratio":3,"completion_ratio":5}`

	recorder := invokeModelPricingController(t, http.MethodPost, "/api/model-meta", modelPricingControllerBody(t, model.Model{
		ModelName:     "controller-create-manual-price",
		Description:   "manual price created with model",
		PricingConfig: pricingConfig,
		Status:        1,
		SyncOfficial:  1,
	}), nil, CreateModelMeta)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := parseModelPricingControllerResponse(t, recorder)
	assert.True(t, response.Success)
	assert.Empty(t, response.Message)

	var persisted model.Model
	require.NoError(t, model.DB.Where("model_name = ?", "controller-create-manual-price").First(&persisted).Error)
	assert.Empty(t, persisted.PricingConfig, "model metadata must not retain a manual pricing source")

	var override model.ModelPricingOverride
	require.NoError(t, model.DB.Where("model_name = ?", persisted.ModelName).First(&override).Error)
	assert.Equal(t, model.ModelPricingOverrideOriginAdmin, override.Origin)
	config, valid, err := model.ParseModelPricingConfig(override.PricingConfig)
	require.NoError(t, err)
	require.True(t, valid)
	require.NotNil(t, config.Ratio)
	assert.Equal(t, 3.0, *config.Ratio)
	require.NotNil(t, config.CompletionRatio)
	assert.Equal(t, 5.0, *config.CompletionRatio)
}

func TestUpdateModelMetaUnchangedEffectivePriceDoesNotCreateOverride(t *testing.T) {
	tests := []struct {
		name          string
		modelName     string
		pricingConfig string
		seed          func(t *testing.T, db *gorm.DB, modelName string)
	}{
		{
			name:          "official price",
			modelName:     "controller-official-unchanged-price",
			pricingConfig: `{"mode":"per-token","ratio":2,"completion_ratio":4}`,
			seed: func(t *testing.T, _ *gorm.DB, modelName string) {
				seedModelPricingControllerOfficialPrice(t, modelName, 2, 4)
			},
		},
		{
			name:          "fallback price",
			modelName:     "controller-fallback-unchanged-price",
			pricingConfig: `{"mode":"per-token","ratio":2}`,
			seed: func(t *testing.T, db *gorm.DB, modelName string) {
				previousModelRatio := ratio_setting.ModelRatio2JSONString()
				t.Cleanup(func() {
					require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatio))
				})
				fallbackRatio := `{"` + modelName + `":2}`
				require.NoError(t, db.Create(&model.Option{Key: "ModelRatio", Value: fallbackRatio}).Error)
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(fallbackRatio))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupModelPricingControllerTestDB(t)
			existing := model.Model{
				ModelName:    test.modelName,
				Description:  "before metadata edit",
				Status:       1,
				SyncOfficial: 1,
			}
			require.NoError(t, existing.Insert())
			test.seed(t, db, existing.ModelName)

			recorder := invokeModelPricingController(t, http.MethodPut, "/api/model-meta", modelPricingControllerBody(t, model.Model{
				Id:            existing.Id,
				ModelName:     existing.ModelName,
				Description:   "metadata changed without changing price authority",
				PricingConfig: test.pricingConfig,
				Status:        1,
				SyncOfficial:  1,
			}), nil, UpdateModelMeta)

			require.Equal(t, http.StatusOK, recorder.Code)
			response := parseModelPricingControllerResponse(t, recorder)
			assert.True(t, response.Success)
			assert.Empty(t, response.Message)

			var overrideCount int64
			require.NoError(t, db.Model(&model.ModelPricingOverride{}).Where("model_name = ?", existing.ModelName).Count(&overrideCount).Error)
			assert.Zero(t, overrideCount, "resubmitting the current effective price must preserve its authority")

			var persisted model.Model
			require.NoError(t, db.First(&persisted, existing.Id).Error)
			assert.Equal(t, "metadata changed without changing price authority", persisted.Description)
			assert.Empty(t, persisted.PricingConfig)
		})
	}
}

func TestUpdateModelMetaChangesPriceAndDetailReportsManualAuthority(t *testing.T) {
	setupModelPricingControllerTestDB(t)
	existing := model.Model{
		ModelName:    "controller-update-manual-price",
		Description:  "before edit",
		Status:       1,
		SyncOfficial: 1,
	}
	require.NoError(t, existing.Insert())
	seedModelPricingControllerOfficialPrice(t, existing.ModelName, 2, 4)
	pricingConfig := `{"mode":"per-token","ratio":7,"completion_ratio":9}`

	updateRecorder := invokeModelPricingController(t, http.MethodPut, "/api/model-meta", modelPricingControllerBody(t, model.Model{
		Id:            existing.Id,
		ModelName:     existing.ModelName,
		Description:   "metadata and price changed together",
		PricingConfig: pricingConfig,
		Status:        1,
		SyncOfficial:  1,
	}), nil, UpdateModelMeta)

	require.Equal(t, http.StatusOK, updateRecorder.Code)
	updateResponse := parseModelPricingControllerResponse(t, updateRecorder)
	assert.True(t, updateResponse.Success)
	assert.Empty(t, updateResponse.Message)

	var persisted model.Model
	require.NoError(t, model.DB.First(&persisted, existing.Id).Error)
	assert.Equal(t, "metadata and price changed together", persisted.Description)
	assert.Empty(t, persisted.PricingConfig)
	var override model.ModelPricingOverride
	require.NoError(t, model.DB.Where("model_name = ?", existing.ModelName).First(&override).Error)

	detailRecorder := invokeModelPricingController(t, http.MethodGet, "/api/model-meta/1", nil, gin.Params{{Key: "id", Value: "1"}}, GetModelMeta)
	require.Equal(t, http.StatusOK, detailRecorder.Code)
	detailResponse := parseModelPricingControllerResponse(t, detailRecorder)
	assert.True(t, detailResponse.Success)
	assert.Empty(t, detailResponse.Message)
	assert.Equal(t, model.AuthorityLevelManual, detailResponse.Data.PricingAuthority)

	effective, valid, err := model.ParseModelPricingConfig(detailResponse.Data.PricingConfig)
	require.NoError(t, err)
	require.True(t, valid)
	require.NotNil(t, effective.Ratio)
	assert.Equal(t, 7.0, *effective.Ratio)
	require.NotNil(t, effective.CompletionRatio)
	assert.Equal(t, 9.0, *effective.CompletionRatio)
}

func TestUpdateModelMetaClearingManualPriceRestoresOfficialAuthority(t *testing.T) {
	setupModelPricingControllerTestDB(t)
	existing := model.Model{
		ModelName:    "controller-restore-official-price",
		Description:  "manual override exists",
		Status:       1,
		SyncOfficial: 1,
	}
	require.NoError(t, existing.Insert())
	seedModelPricingControllerOfficialPrice(t, existing.ModelName, 2, 4)

	manualRatio := 7.0
	manualCompletion := 9.0
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		return model.UpsertModelPricingOverrideTx(tx, existing.ModelName, model.ModelPricingConfig{
			Mode:            model.ModelPricingModePerToken,
			Ratio:           &manualRatio,
			CompletionRatio: &manualCompletion,
		}, model.ModelPricingOverrideOriginAdmin)
	}))

	updateRecorder := invokeModelPricingController(t, http.MethodPut, "/api/model-meta", modelPricingControllerBody(t, model.Model{
		Id:           existing.Id,
		ModelName:    existing.ModelName,
		Description:  "manual override cleared",
		Status:       1,
		SyncOfficial: 1,
	}), nil, UpdateModelMeta)

	require.Equal(t, http.StatusOK, updateRecorder.Code)
	updateResponse := parseModelPricingControllerResponse(t, updateRecorder)
	assert.True(t, updateResponse.Success)
	assert.Empty(t, updateResponse.Message)

	var overrideCount int64
	require.NoError(t, model.DB.Model(&model.ModelPricingOverride{}).Where("model_name = ?", existing.ModelName).Count(&overrideCount).Error)
	assert.Zero(t, overrideCount)

	detailRecorder := invokeModelPricingController(t, http.MethodGet, "/api/model-meta/1", nil, gin.Params{{Key: "id", Value: "1"}}, GetModelMeta)
	require.Equal(t, http.StatusOK, detailRecorder.Code)
	detailResponse := parseModelPricingControllerResponse(t, detailRecorder)
	assert.True(t, detailResponse.Success)
	assert.Equal(t, model.AuthorityLevelOfficial, detailResponse.Data.PricingAuthority)
	effective, valid, err := model.ParseModelPricingConfig(detailResponse.Data.PricingConfig)
	require.NoError(t, err)
	require.True(t, valid)
	require.NotNil(t, effective.Ratio)
	assert.Equal(t, 2.0, *effective.Ratio)
}

func TestUpdateOptionRejectsLegacyModelPricingKeysWithoutPersisting(t *testing.T) {
	pricingKeys := []string{
		"ModelPrice",
		"ModelRatio",
		"CompletionRatio",
		"CacheRatio",
		"CreateCacheRatio",
		"ImageRatio",
		"AudioRatio",
		"AudioCompletionRatio",
	}

	for _, key := range pricingKeys {
		t.Run(key, func(t *testing.T) {
			db := setupModelPricingControllerTestDB(t)
			payload, err := common.Marshal(OptionUpdateRequest{Key: key, Value: `{"controller-option-rejected":7}`})
			require.NoError(t, err)

			recorder := invokeModelPricingController(t, http.MethodPut, "/api/option", bytes.NewReader(payload), nil, UpdateOption)
			require.Equal(t, http.StatusOK, recorder.Code)
			var response modelPricingOptionResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Contains(t, response.Message, "模型价格配置已迁移")

			var count int64
			require.NoError(t, db.Model(&model.Option{}).Where("key = ?", key).Count(&count).Error)
			assert.Zero(t, count, "rejected pricing options must not become a second persisted authority")
		})
	}
}

func TestDeleteModelMetaRemovesManualOverrideBeforeSameNameRecreation(t *testing.T) {
	setupModelPricingControllerTestDB(t)
	existing := model.Model{
		ModelName:    "controller-delete-manual-override",
		Description:  "model to remove",
		Status:       1,
		SyncOfficial: 1,
	}
	require.NoError(t, existing.Insert())
	manualRatio := 7.0
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		return model.UpsertModelPricingOverrideTx(tx, existing.ModelName, model.ModelPricingConfig{
			Mode:  model.ModelPricingModePerToken,
			Ratio: &manualRatio,
		}, model.ModelPricingOverrideOriginAdmin)
	}))

	deleteRecorder := invokeModelPricingController(t, http.MethodDelete, "/api/model-meta/"+strconv.Itoa(existing.Id), nil, gin.Params{{Key: "id", Value: strconv.Itoa(existing.Id)}}, DeleteModelMeta)
	require.Equal(t, http.StatusOK, deleteRecorder.Code)
	deleteResponse := parseModelPricingControllerResponse(t, deleteRecorder)
	assert.True(t, deleteResponse.Success)

	var overrideCount int64
	require.NoError(t, model.DB.Model(&model.ModelPricingOverride{}).Where("model_name = ?", existing.ModelName).Count(&overrideCount).Error)
	assert.Zero(t, overrideCount)

	recreateRecorder := invokeModelPricingController(t, http.MethodPost, "/api/model-meta", modelPricingControllerBody(t, model.Model{
		ModelName:    existing.ModelName,
		Description:  "same name, no manual price",
		Status:       1,
		SyncOfficial: 1,
	}), nil, CreateModelMeta)
	require.Equal(t, http.StatusOK, recreateRecorder.Code)
	recreateResponse := parseModelPricingControllerResponse(t, recreateRecorder)
	assert.True(t, recreateResponse.Success)

	var recreated model.Model
	require.NoError(t, model.DB.Where("model_name = ?", existing.ModelName).First(&recreated).Error)
	require.NoError(t, model.DB.Model(&model.ModelPricingOverride{}).Where("model_name = ?", existing.ModelName).Count(&overrideCount).Error)
	assert.Zero(t, overrideCount)

	detailRecorder := invokeModelPricingController(t, http.MethodGet, "/api/model-meta/"+strconv.Itoa(recreated.Id), nil, gin.Params{{Key: "id", Value: strconv.Itoa(recreated.Id)}}, GetModelMeta)
	require.Equal(t, http.StatusOK, detailRecorder.Code)
	detailResponse := parseModelPricingControllerResponse(t, detailRecorder)
	assert.True(t, detailResponse.Success)
	assert.NotEqual(t, model.AuthorityLevelManual, detailResponse.Data.PricingAuthority)
}
