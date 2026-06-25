package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChannelSelectionTraceRecordsObservableRoutingMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	channel := &model.Channel{Id: 12, Name: " primary\nchannel ", Type: constant.ChannelTypeOpenAI}
	priority := int64(3)
	weight := uint(7)
	channel.Priority = &priority
	channel.Weight = &weight

	AppendChannelSelectionTrace(ctx, channel, "default", 1, ChannelChainReasonRetrySelected, ChannelChainSelectionWeighted)

	chain := ChannelChainForLog(ctx)
	require.Len(t, chain, 1)
	require.Equal(t, 12, chain[0].ChannelId)
	require.Equal(t, "primary channel", chain[0].ChannelName)
	require.Equal(t, "default", chain[0].Group)
	require.Equal(t, ChannelChainReasonRetrySelected, chain[0].Reason)
	require.Equal(t, ChannelChainSelectionWeighted, chain[0].Selection)
	require.Equal(t, 2, chain[0].Attempt)
	require.Equal(t, 1, chain[0].RetryIndex)
	require.Equal(t, ChannelChainCircuitStateClosed, chain[0].CircuitState)
	require.Equal(t, priority, chain[0].Decision.Priority)
	require.Equal(t, 7, chain[0].Decision.Weight)
}

func TestGenerateTextOtherInfoPersistsRoutingDecisionMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("use_channel", []string{"11", "12"})
	common.SetContextKey(ctx, constant.ContextKeyRequestFormat, string(types.RequestFormatOpenAI))
	AppendChannelChainEntry(ctx, types.ChannelChainEntry{
		ChannelId: 12,
		Reason:    ChannelChainReasonSelected,
		Selection: ChannelChainSelectionWeighted,
	})

	relayInfo := &relaycommon.RelayInfo{
		StartTime:         time.Unix(10, 0),
		FirstResponseTime: time.Unix(10, int64(250*time.Millisecond)),
		OriginModelName:   "gpt-4.1",
		RequestFormat:     types.RequestFormatOpenAI,
		Request:           &dto.GeneralOpenAIRequest{Model: "gpt-4.1"},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:            12,
			ChannelEndpointId:    3,
			ChannelEndpointLabel: "east",
			ChannelEndpointUrl:   "https://upstream.example/v1?api_key=secret",
			UpstreamModelName:    "gpt-4.1-mini",
		},
	}

	other := GenerateTextOtherInfo(ctx, relayInfo, 1, 1, 1, 0, 0, 0, 1)

	require.Equal(t, "openai", other["request_format"])
	require.Equal(t, "gpt-4.1", other["original_model"])
	require.Equal(t, "gpt-4.1-mini", other["upstream_model"])
	require.Equal(t, 11, other["initial_channel_id"])
	require.Equal(t, 12, other["final_channel_id"])
	require.Equal(t, "*dto.GeneralOpenAIRequest", other["body_shape"])
	require.Len(t, other["channel_chain"], 1)

	endpoint, ok := other["selected_endpoint"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, 3, endpoint["id"])
	require.Equal(t, "east", endpoint["label"])
	require.Equal(t, "https://upstream.example/v1", endpoint["url"])
}
