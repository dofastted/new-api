package openai

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func isGrokSchemaCompatModel(info *relaycommon.RelayInfo, requestModel string) bool {
	if info != nil && info.ChannelMeta != nil && strings.HasPrefix(info.UpstreamModelName, "grok-") {
		return true
	}
	return strings.HasPrefix(requestModel, "grok-")
}

func SanitizeGrokOpenAIRequestSchema(request *dto.GeneralOpenAIRequest) {
	if request == nil {
		return
	}

	for i := range request.Tools {
		if request.Tools[i].Function.Parameters != nil {
			request.Tools[i].Function.Parameters = sanitizeGrokJSONSchemaValue(request.Tools[i].Function.Parameters)
		}
	}

	request.Functions = sanitizeGrokRawJSON(request.Functions)
	if request.ResponseFormat != nil {
		request.ResponseFormat.JsonSchema = sanitizeGrokRawJSON(request.ResponseFormat.JsonSchema)
	}
}

func SanitizeGrokOpenAIResponsesRequestSchema(request *dto.OpenAIResponsesRequest) {
	if request == nil {
		return
	}

	request.Tools = sanitizeGrokRawJSON(request.Tools)
	request.Text = sanitizeGrokRawJSON(request.Text)
}

func sanitizeGrokRawJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}

	var decoded any
	if err := common.Unmarshal(raw, &decoded); err != nil {
		return raw
	}
	cleaned := sanitizeGrokJSONSchemaValue(decoded)
	data, err := common.Marshal(cleaned)
	if err != nil {
		return raw
	}
	return data
}

func sanitizeGrokJSONSchemaValue(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case map[string]any:
		cleaned := make(map[string]any, len(v))
		for key, item := range v {
			if key == "additionalProperties" {
				if _, ok := item.(bool); ok {
					continue
				}
			}
			cleaned[key] = sanitizeGrokJSONSchemaValue(item)
		}
		return cleaned
	case []any:
		cleaned := make([]any, len(v))
		for i, item := range v {
			cleaned[i] = sanitizeGrokJSONSchemaValue(item)
		}
		return cleaned
	case json.RawMessage:
		var decoded any
		if err := common.Unmarshal(v, &decoded); err != nil {
			return v
		}
		return sanitizeGrokJSONSchemaValue(decoded)
	case string, bool, float64, float32, int, int64, uint, uint64:
		return v
	default:
		data, err := common.Marshal(v)
		if err != nil {
			return v
		}
		var decoded any
		if err := common.Unmarshal(data, &decoded); err != nil {
			return v
		}
		return sanitizeGrokJSONSchemaValue(decoded)
	}
}
