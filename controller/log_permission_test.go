package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetUserLogsRedactsRoutingTraceDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupModelListControllerTestDB(t)
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}))
	require.NoError(t, model.LOG_DB.Exec("DELETE FROM logs").Error)
	routingOther := map[string]interface{}{
		"request_format":     "openai",
		"channel_chain":      []map[string]interface{}{{"channel_id": 12}},
		"selected_endpoint":  map[string]interface{}{"url": "https://upstream.example/v1"},
		"initial_channel_id": 11,
		"final_channel_id":   12,
		"body_shape":         "*dto.GeneralOpenAIRequest",
		"admin_info":         map[string]interface{}{"use_channel": []int{11, 12}},
	}
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId:    41,
		Type:      model.LogTypeConsume,
		Content:   "consume",
		Other:     common.MapToJsonStr(routingOther),
		CreatedAt: 100,
	}).Error)
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId:    42,
		Type:      model.LogTypeConsume,
		Content:   "other user",
		Other:     common.MapToJsonStr(routingOther),
		CreatedAt: 101,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/self?type=2&p=0&page_size=10", nil)
	ctx.Set("id", 41)

	GetUserLogs(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Items []model.Log `json:"items"`
			Total int         `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, 1, response.Data.Total)
	require.Len(t, response.Data.Items, 1)
	require.Equal(t, 41, response.Data.Items[0].UserId)

	var redacted map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(response.Data.Items[0].Other), &redacted))
	require.Equal(t, "openai", redacted["request_format"])
	require.NotContains(t, redacted, "channel_chain")
	require.NotContains(t, redacted, "selected_endpoint")
	require.NotContains(t, redacted, "initial_channel_id")
	require.NotContains(t, redacted, "final_channel_id")
	require.NotContains(t, redacted, "body_shape")
	require.NotContains(t, redacted, "admin_info")
}
