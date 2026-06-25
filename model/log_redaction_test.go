package model

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
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
