package helper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/types"
)

const (
	// OpenAI file inputs guide: each file must be under 50 MB and all files in
	// one request are limited to 50 MB combined. OpenAI documents MB in decimal.
	maxResponsesFileBytes         int64 = 50_000_000
	maxResponsesCombinedFileBytes int64 = 50_000_000

	// OpenAI images and vision guide: up to 512 MB total payload and 1,500
	// image inputs per request.
	maxResponsesImageBytes  int64 = 512_000_000
	maxResponsesImageInputs       = 1_500
)

type responsesAttachmentLimits struct {
	fileBytes         int64
	combinedFileBytes int64
	imageBytes        int64
	imageInputs       int
}

func officialResponsesAttachmentLimits() responsesAttachmentLimits {
	return responsesAttachmentLimits{
		fileBytes:         maxResponsesFileBytes,
		combinedFileBytes: maxResponsesCombinedFileBytes,
		imageBytes:        maxResponsesImageBytes,
		imageInputs:       maxResponsesImageInputs,
	}
}

// validateResponsesAttachments applies OpenAI's documented attachment limits
// to inline Base64 inputs. file_id and remote URLs are deliberately not fetched:
// their sizes must be validated by the file host or upstream provider.
func validateResponsesAttachments(inputs ...json.RawMessage) *types.NewAPIError {
	return validateResponsesAttachmentValuesWithLimits(officialResponsesAttachmentLimits(), inputs...)
}

func validateResponsesAttachmentsWithLimits(input json.RawMessage, limits responsesAttachmentLimits) *types.NewAPIError {
	return validateResponsesAttachmentValuesWithLimits(limits, input)
}

func validateResponsesAttachmentValuesWithLimits(limits responsesAttachmentLimits, inputs ...json.RawMessage) *types.NewAPIError {
	stats := responsesAttachmentStats{}
	for _, input := range inputs {
		if len(input) == 0 {
			continue
		}
		var value any
		if err := json.Unmarshal(input, &value); err != nil {
			continue // The normal request decoder reports malformed JSON.
		}
		walkResponsesAttachments(value, &stats)
	}
	for _, size := range stats.fileSizes {
		if size >= limits.fileBytes {
			return attachmentTooLargeError(fmt.Sprintf(
				"file attachment is too large: each file must be under 50 MB (received %.2f MB)",
				float64(size)/1_000_000,
			))
		}
	}
	if stats.combinedFileBytes > limits.combinedFileBytes {
		return attachmentTooLargeError(fmt.Sprintf(
			"file attachments are too large: combined file size must not exceed 50 MB (received %.2f MB)",
			float64(stats.combinedFileBytes)/1_000_000,
		))
	}
	if stats.imageInputs > limits.imageInputs {
		return attachmentTooLargeError(fmt.Sprintf(
			"too many image attachments: at most 1500 images are allowed per request (received %d)",
			stats.imageInputs,
		))
	}
	if stats.inlineImageBytes > limits.imageBytes {
		return attachmentTooLargeError(fmt.Sprintf(
			"image attachments are too large: total image payload must not exceed 512 MB (received %.2f MB)",
			float64(stats.inlineImageBytes)/1_000_000,
		))
	}
	return nil
}

type responsesAttachmentStats struct {
	fileSizes         []int64
	combinedFileBytes int64
	imageInputs       int
	inlineImageBytes  int64
}

func walkResponsesAttachments(value any, stats *responsesAttachmentStats) {
	switch node := value.(type) {
	case []any:
		for _, child := range node {
			walkResponsesAttachments(child, stats)
		}
	case map[string]any:
		typeName, _ := node["type"].(string)
		switch typeName {
		case "input_file":
			if fileData, ok := node["file_data"].(string); ok && fileData != "" {
				if size, ok := decodedBase64Size(fileData); ok {
					stats.fileSizes = append(stats.fileSizes, size)
					stats.combinedFileBytes += size
				}
			}
		case "input_image":
			stats.imageInputs++
			if imageURL := attachmentStringValue(node["image_url"]); strings.HasPrefix(strings.ToLower(imageURL), "data:") {
				if size, ok := decodedBase64Size(imageURL); ok {
					stats.inlineImageBytes += size
				}
			}
		}

		for _, child := range node {
			walkResponsesAttachments(child, stats)
		}
	}
}

func attachmentStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		text, _ := typed["url"].(string)
		return text
	default:
		return ""
	}
}

func decodedBase64Size(value string) (int64, bool) {
	payload := value
	if strings.HasPrefix(strings.ToLower(payload), "data:") {
		comma := strings.IndexByte(payload, ',')
		if comma < 0 || !strings.Contains(strings.ToLower(payload[:comma]), ";base64") {
			return 0, false
		}
		payload = payload[comma+1:]
	}

	payload = strings.TrimSpace(payload)
	if payload == "" {
		return 0, true
	}

	encodedLen := int64(len(payload))
	padding := int64(0)
	if strings.HasSuffix(payload, "=") {
		padding++
	}
	if strings.HasSuffix(payload, "==") {
		padding++
	}
	return encodedLen*3/4 - padding, true
}

func attachmentTooLargeError(message string) *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		fmt.Errorf("%s", message),
		types.ErrorCodeAttachmentTooLarge,
		http.StatusRequestEntityTooLarge,
		types.ErrOptionWithSkipRetry(),
	)
}
