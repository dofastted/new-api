package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupModelSyncControllerOfficialPricingDB(t *testing.T) {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.OfficialModelPrice{}, &model.OfficialPricingSnapshot{}))
	require.NoError(t, db.Exec("DELETE FROM official_model_prices").Error)
	require.NoError(t, db.Exec("DELETE FROM official_pricing_snapshots").Error)
}

func seedActiveOfficialModelPrice(t *testing.T, modelName string) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.OfficialModelPrice{
		Provider:        service.OfficialPricingProviderOpenAI,
		ModelName:       modelName,
		SourceURL:       service.OfficialPricingOpenAIURL,
		ModelRatio:      1.5,
		CompletionRatio: 2.5,
		Active:          true,
	}).Error)
}

func TestSyncUpstreamPreviewUsesSeededOfficialPricingRows(t *testing.T) {
	setupModelSyncControllerOfficialPricingDB(t)
	seedActiveOfficialModelPrice(t, "gpt-preview-contract-test")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/sync-upstream-preview", nil)

	SyncUpstreamPreview(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                                 `json:"success"`
		Message string                               `json:"message"`
		Data    service.OfficialModelMetadataPreview `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Empty(t, response.Message)
	assert.Equal(t, "official-pricing", response.Data.Source.Name)
	assert.Equal(t, 1, response.Data.Source.ModelCount)
	assert.Empty(t, response.Data.Conflicts)
	require.Len(t, response.Data.Missing, 1)
	assert.Equal(t, "gpt-preview-contract-test", response.Data.Missing[0].ModelName)
	assert.Equal(t, "OpenAI", response.Data.Missing[0].Vendor)
}

func TestSyncUpstreamModelsUsesSeededOfficialPricingRows(t *testing.T) {
	setupModelSyncControllerOfficialPricingDB(t)
	seedActiveOfficialModelPrice(t, "gpt-sync-contract-test")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/sync-upstream-models", nil)

	SyncUpstreamModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                                    `json:"success"`
		Message string                                  `json:"message"`
		Data    service.OfficialModelMetadataSyncResult `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Empty(t, response.Message)
	assert.Equal(t, "official-pricing", response.Data.Source.Name)
	assert.Equal(t, 1, response.Data.Source.ModelCount)
	assert.Equal(t, 1, response.Data.CreatedModels)
	assert.Equal(t, 0, response.Data.UpdatedModels)
	assert.ElementsMatch(t, []string{"gpt-sync-contract-test"}, response.Data.CreatedList)

	var synced model.Model
	require.NoError(t, model.DB.Where("model_name = ?", "gpt-sync-contract-test").First(&synced).Error)
	assert.Equal(t, 1, synced.SyncOfficial)
	assert.NotEmpty(t, synced.PricingConfig)
}
