package helper

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestOfficialResponsesAttachmentLimits(t *testing.T) {
	limits := officialResponsesAttachmentLimits()
	require.Equal(t, int64(50_000_000), limits.fileBytes)
	require.Equal(t, int64(50_000_000), limits.combinedFileBytes)
	require.Equal(t, int64(512_000_000), limits.imageBytes)
	require.Equal(t, 1_500, limits.imageInputs)
}

func TestValidateResponsesAttachmentsDoesNotLimitTextContext(t *testing.T) {
	input := attachmentTestInput(t, []any{
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": strings.Repeat("context", 100_000)},
			},
		},
	})

	err := validateResponsesAttachmentsWithLimits(input, responsesAttachmentLimits{
		fileBytes:         1,
		combinedFileBytes: 1,
		imageBytes:        1,
		imageInputs:       1,
	})
	require.Nil(t, err)
}

func TestValidateResponsesAttachmentsRejectsOversizedInlineFile(t *testing.T) {
	input := attachmentTestInput(t, responsesInputItem("input_file", "file_data", attachmentTestBase64(11)))

	err := validateResponsesAttachmentsWithLimits(input, responsesAttachmentLimits{
		fileBytes:         10,
		combinedFileBytes: 20,
		imageBytes:        100,
		imageInputs:       10,
	})

	requireAttachmentTooLarge(t, err)
	require.Contains(t, err.Error(), "each file")
}

func TestValidateResponsesAttachmentsHandlesEscapedTypeName(t *testing.T) {
	input := json.RawMessage(`[{"type":"input\u005ffile","file_data":"AAAAAAAAAAAAAAAA"}]`)

	err := validateResponsesAttachmentsWithLimits(input, responsesAttachmentLimits{
		fileBytes: 10, combinedFileBytes: 20, imageBytes: 100, imageInputs: 10,
	})

	requireAttachmentTooLarge(t, err)
}

func TestValidateResponsesAttachmentsRejectsCombinedInlineFiles(t *testing.T) {
	input := attachmentTestInput(t, []any{
		responsesInputItem("input_file", "file_data", attachmentTestBase64(6)),
		responsesInputItem("input_file", "file_data", attachmentTestBase64(6)),
	})

	err := validateResponsesAttachmentsWithLimits(input, responsesAttachmentLimits{
		fileBytes:         10,
		combinedFileBytes: 10,
		imageBytes:        100,
		imageInputs:       10,
	})

	requireAttachmentTooLarge(t, err)
	require.Contains(t, err.Error(), "combined file size")
}

func TestValidateResponsesAttachmentsRejectsImageLimits(t *testing.T) {
	t.Run("count", func(t *testing.T) {
		input := attachmentTestInput(t, []any{
			responsesInputItem("input_image", "image_url", "https://example.com/one.png"),
			responsesInputItem("input_image", "image_url", "https://example.com/two.png"),
		})
		err := validateResponsesAttachmentsWithLimits(input, responsesAttachmentLimits{
			fileBytes: 100, combinedFileBytes: 100, imageBytes: 100, imageInputs: 1,
		})
		requireAttachmentTooLarge(t, err)
		require.Contains(t, err.Error(), "too many image attachments")
	})

	t.Run("inline bytes", func(t *testing.T) {
		input := attachmentTestInput(t, responsesInputItem(
			"input_image",
			"image_url",
			"data:image/png;base64,"+attachmentTestBase64(11),
		))
		err := validateResponsesAttachmentsWithLimits(input, responsesAttachmentLimits{
			fileBytes: 100, combinedFileBytes: 100, imageBytes: 10, imageInputs: 10,
		})
		requireAttachmentTooLarge(t, err)
		require.Contains(t, err.Error(), "total image payload")
	})
}

func TestValidateResponsesAttachmentsLeavesRemoteFilesToUpstream(t *testing.T) {
	input := attachmentTestInput(t, []any{
		responsesInputItem("input_file", "file_id", "file-123"),
		responsesInputItem("input_file", "file_url", "https://example.com/large.pdf"),
	})

	err := validateResponsesAttachmentsWithLimits(input, responsesAttachmentLimits{
		fileBytes: 1, combinedFileBytes: 1, imageBytes: 1, imageInputs: 1,
	})
	require.Nil(t, err)
}

func TestValidateResponsesAttachmentsAggregatesInputAndPromptVariables(t *testing.T) {
	input := attachmentTestInput(t, responsesInputItem("input_file", "file_data", attachmentTestBase64(6)))
	prompt := attachmentTestInput(t, map[string]any{
		"id": "pmpt_test",
		"variables": map[string]any{
			"doc": map[string]any{
				"type":      "input_file",
				"file_data": attachmentTestBase64(6),
			},
		},
	})

	err := validateResponsesAttachmentValuesWithLimits(responsesAttachmentLimits{
		fileBytes:         10,
		combinedFileBytes: 10,
		imageBytes:        100,
		imageInputs:       10,
	}, input, prompt)

	requireAttachmentTooLarge(t, err)
	require.Contains(t, err.Error(), "combined file size")
}

func TestValidateResponsesAttachmentsRejectsPromptVariableOnly(t *testing.T) {
	prompt := attachmentTestInput(t, map[string]any{
		"variables": map[string]any{
			"image": map[string]any{
				"type":      "input_image",
				"image_url": "data:image/png;base64," + attachmentTestBase64(11),
			},
		},
	})

	err := validateResponsesAttachmentValuesWithLimits(responsesAttachmentLimits{
		fileBytes: 100, combinedFileBytes: 100, imageBytes: 10, imageInputs: 10,
	}, nil, prompt)

	requireAttachmentTooLarge(t, err)
	require.Contains(t, err.Error(), "total image payload")
}

func responsesInputItem(itemType, field, value string) map[string]any {
	return map[string]any{
		"role": "user",
		"content": []any{
			map[string]any{"type": itemType, field: value},
		},
	}
}

func attachmentTestInput(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}

func attachmentTestBase64(size int) string {
	return base64.StdEncoding.EncodeToString(make([]byte, size))
}

func requireAttachmentTooLarge(t *testing.T, err *types.NewAPIError) {
	t.Helper()
	require.NotNil(t, err)
	require.Equal(t, types.ErrorCodeAttachmentTooLarge, err.GetErrorCode())
	require.Equal(t, http.StatusRequestEntityTooLarge, err.StatusCode)
	require.True(t, types.IsSkipRetryError(err))
}
