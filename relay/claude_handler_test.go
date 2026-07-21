package relay

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
)

func TestShouldUseClaudeStreamResponse(t *testing.T) {
	sseResponse := &http.Response{Header: http.Header{"Content-Type": []string{"text/event-stream; charset=utf-8"}}}

	t.Run("advanced custom trusts non-stream request", func(t *testing.T) {
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeAdvancedCustom}}
		assert.False(t, shouldUseClaudeStreamResponse(info, sseResponse))
	})

	t.Run("advanced custom preserves stream request", func(t *testing.T) {
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeAdvancedCustom}, IsStream: true}
		assert.True(t, shouldUseClaudeStreamResponse(info, sseResponse))
	})

	t.Run("legacy channels retain response header detection", func(t *testing.T) {
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeAnthropic}}
		assert.True(t, shouldUseClaudeStreamResponse(info, sseResponse))
	})
}
