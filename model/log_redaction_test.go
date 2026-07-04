package model

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatUserLogsRedactsRoutingDecisionMetadata(t *testing.T) {
	other := map[string]interface{}{
		"request_format":     "openai",
		"channel_chain":      []map[string]interface{}{{"channel_id": 12}},
		"selected_endpoint":  map[string]interface{}{"url": "https://upstream.example/v1"},
		"initial_channel_id": float64(11),
		"final_channel_id":   float64(12),
		"body_shape":         "*dto.GeneralOpenAIRequest",
		"admin_info":         map[string]interface{}{"use_channel": []int{11, 12}},
		"audit_info":         map[string]interface{}{"route": "/api/test"},
		"stream_status":      map[string]interface{}{"status": "error"},
	}
	logs := []*Log{{Other: common.MapToJsonStr(other)}}

	formatUserLogs(logs, 0)

	var redacted map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(logs[0].Other), &redacted))
	require.Equal(t, "openai", redacted["request_format"])
	require.NotContains(t, redacted, "channel_chain")
	require.NotContains(t, redacted, "selected_endpoint")
	require.NotContains(t, redacted, "initial_channel_id")
	require.NotContains(t, redacted, "final_channel_id")
	require.NotContains(t, redacted, "body_shape")
	require.NotContains(t, redacted, "admin_info")
	require.NotContains(t, redacted, "audit_info")
	require.NotContains(t, redacted, "stream_status")
}

func TestRecordTopupLogPersistsTopupMetadata(t *testing.T) {
	truncateTables(t)

	const userID = 91001
	recordedBalance := 123456
	require.NoError(t, DB.Create(&User{
		Id:       userID,
		Username: "topup-user",
		Role:     common.RoleCommonUser,
		AffCode:  "topup-user-aff",
	}).Error)

	RecordTopupLog(TopupLogDetails{
		UserID:                userID,
		Content:               "online top-up completed",
		CallerIP:              "203.0.113.10",
		PaymentMethod:         "stripe",
		CallbackPaymentMethod: "card",
		QuotaDelta:            5000,
		BalanceAfter:          &recordedBalance,
		PayAmount:             12.34,
	})

	var log Log
	require.NoError(t, LOG_DB.Where("user_id = ? AND type = ?", userID, LogTypeTopup).First(&log).Error)
	assert.Equal(t, LogTypeTopup, log.Type)
	assert.Equal(t, userID, log.UserId)
	assert.Equal(t, "topup-user", log.Username)
	assert.Equal(t, "online top-up completed", log.Content)
	assert.Equal(t, "203.0.113.10", log.Ip)

	var other struct {
		AdminInfo map[string]interface{} `json:"admin_info"`
		Topup     struct {
			PaymentMethod         string  `json:"payment_method"`
			CallbackPaymentMethod string  `json:"callback_payment_method"`
			QuotaDelta            int     `json:"quota_delta"`
			BalanceAfter          int     `json:"balance_after"`
			PayAmount             float64 `json:"pay_amount"`
		} `json:"topup"`
	}
	require.NoError(t, common.Unmarshal([]byte(log.Other), &other))
	require.NotNil(t, other.AdminInfo)
	assert.Equal(t, "203.0.113.10", other.AdminInfo["caller_ip"])
	assert.Equal(t, "stripe", other.AdminInfo["payment_method"])
	assert.Equal(t, "card", other.AdminInfo["callback_payment_method"])
	assert.Equal(t, "stripe", other.Topup.PaymentMethod)
	assert.Equal(t, "card", other.Topup.CallbackPaymentMethod)
	assert.Equal(t, 5000, other.Topup.QuotaDelta)
	assert.Equal(t, recordedBalance, other.Topup.BalanceAfter)
	assert.Equal(t, 12.34, other.Topup.PayAmount)
}
