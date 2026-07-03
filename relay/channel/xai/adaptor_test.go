package xai

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIRequestSanitizesBooleanAdditionalProperties(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model: "grok-composer-2.5-fast",
		Tools: []dto.ToolCallRequest{
			{
				Type: "function",
				Function: dto.FunctionRequest{
					Name: "emit",
					Parameters: map[string]any{
						"type":                 "object",
						"additionalProperties": false,
					},
				},
			},
		},
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-composer-2.5-fast"}}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	parameters, ok := convertedRequest.Tools[0].Function.Parameters.(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, parameters, "additionalProperties")
}

func TestConvertOpenAIRequestSanitizesSearchVariantBeforeMapConversion(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model: "grok-4-fast-reasoning-search",
		Tools: []dto.ToolCallRequest{
			{
				Type: "function",
				Function: dto.FunctionRequest{
					Name: "emit",
					Parameters: map[string]any{
						"type":                 "object",
						"additionalProperties": false,
					},
				},
			},
		},
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-4-fast-reasoning-search"}}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedMap, ok := converted.(map[string]any)
	require.True(t, ok)
	tools, ok := convertedMap["tools"].([]any)
	require.True(t, ok)
	tool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	function, ok := tool["function"].(map[string]any)
	require.True(t, ok)
	parameters, ok := function["parameters"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, parameters, "additionalProperties")
	assert.Equal(t, map[string]any{"mode": "on"}, convertedMap["search_parameters"])
}
