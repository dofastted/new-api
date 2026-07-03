package openai

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIRequestSanitizesGrokBooleanAdditionalProperties(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model: "grok-composer-2.5-fast",
		Tools: []dto.ToolCallRequest{
			{
				Type: "function",
				Function: dto.FunctionRequest{
					Name: "emit_properties",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"properties": map[string]any{
								"type":                 "object",
								"additionalProperties": false,
							},
						},
						"additionalProperties": false,
					},
				},
			},
		},
		Functions: json.RawMessage(`[{"name":"legacy","parameters":{"type":"object","additionalProperties":false}}]`),
		ResponseFormat: &dto.ResponseFormat{
			Type:       "json_schema",
			JsonSchema: json.RawMessage(`{"name":"out","schema":{"type":"object","properties":{"ok":{"type":"string"}},"additionalProperties":false},"strict":true}`),
		},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "grok-composer-2.5-fast",
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	parameters, ok := convertedRequest.Tools[0].Function.Parameters.(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, parameters, "additionalProperties")
	properties, ok := parameters["properties"].(map[string]any)
	require.True(t, ok)
	nested, ok := properties["properties"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, nested, "additionalProperties")

	var legacy []map[string]any
	require.NoError(t, common.Unmarshal(convertedRequest.Functions, &legacy))
	legacyParams, ok := legacy[0]["parameters"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, legacyParams, "additionalProperties")

	var responseFormat map[string]any
	require.NoError(t, common.Unmarshal(convertedRequest.ResponseFormat.JsonSchema, &responseFormat))
	responseSchema, ok := responseFormat["schema"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, responseSchema, "additionalProperties")
	assert.Equal(t, true, responseFormat["strict"])
}

func TestConvertOpenAIRequestPreservesNonGrokBooleanAdditionalProperties(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model: "gpt-4o",
		Tools: []dto.ToolCallRequest{
			{
				Type: "function",
				Function: dto.FunctionRequest{
					Name: "emit_properties",
					Parameters: map[string]any{
						"type":                 "object",
						"additionalProperties": false,
					},
				},
			},
		},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "gpt-4o",
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	parameters, ok := convertedRequest.Tools[0].Function.Parameters.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, parameters, "additionalProperties")
	assert.Equal(t, false, parameters["additionalProperties"])
}

func TestIsGrokSchemaCompatModelHandlesNilChannelMeta(t *testing.T) {
	assert.False(t, isGrokSchemaCompatModel(&relaycommon.RelayInfo{}, "gpt-4o"))
	assert.True(t, isGrokSchemaCompatModel(&relaycommon.RelayInfo{}, "grok-composer-2.5-fast"))
}

func TestConvertOpenAIResponsesRequestSanitizesGrokTextAndTools(t *testing.T) {
	request := dto.OpenAIResponsesRequest{
		Model: "grok-composer-2.5-fast",
		Tools: json.RawMessage(`[{"type":"function","name":"emit","parameters":{"type":"object","additionalProperties":false}}]`),
		Text:  json.RawMessage(`{"format":{"type":"json_schema","name":"out","schema":{"type":"object","additionalProperties":false}}}`),
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "grok-composer-2.5-fast",
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest, ok := converted.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	var tools []map[string]any
	require.NoError(t, common.Unmarshal(convertedRequest.Tools, &tools))
	toolParams, ok := tools[0]["parameters"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, toolParams, "additionalProperties")

	var text map[string]any
	require.NoError(t, common.Unmarshal(convertedRequest.Text, &text))
	format, ok := text["format"].(map[string]any)
	require.True(t, ok)
	schema, ok := format["schema"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, schema, "additionalProperties")
}
