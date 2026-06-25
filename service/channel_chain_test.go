package service

import (
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAppendChannelChainEntrySanitizesSensitiveEndpointQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := &gin.Context{}

	AppendChannelChainEntry(ctx, types.ChannelChainEntry{
		ChannelId:   7,
		ChannelName: " primary\nchannel ",
		Endpoint:    "https://example.test/v1?api_key=secret",
		Reason:      ChannelChainReasonSelected,
	})

	chain := ChannelChainForLog(ctx)
	require.Len(t, chain, 1)
	require.Equal(t, "primary channel", chain[0].ChannelName)
	require.Equal(t, "https://example.test/v1", chain[0].Endpoint)
	require.NotContains(t, chain[0].Endpoint, "secret")
}

func TestAppendChannelFailureTraceRecordsErrorCategory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := &gin.Context{}
	err := types.NewErrorWithStatusCode(assertErr("upstream failed"), types.ErrorCodeBadResponse, 502)

	AppendChannelFailureTrace(ctx, 11, 1, "openai", err)

	chain := ChannelChainForLog(ctx)
	require.Len(t, chain, 1)
	require.Equal(t, ChannelChainReasonFailure, chain[0].Reason)
	require.Equal(t, string(types.ErrorCodeBadResponse), chain[0].ErrorCode)
	require.NotContains(t, chain[0].ErrorCategory, "upstream failed")
}

type assertErr string

func (e assertErr) Error() string {
	return string(e)
}
