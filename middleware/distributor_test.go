package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutoGroupForRequestPath(t *testing.T) {
	tests := []struct {
		name            string
		usingGroup      string
		requestPath     string
		expectedGroup   string
		expectedChanged bool
	}{
		{
			name:            "routes chat completions",
			usingGroup:      "auto",
			requestPath:     "/v1/chat/completions",
			expectedGroup:   "codex-completions",
			expectedChanged: true,
		},
		{
			name:          "keeps responses auto",
			usingGroup:    "auto",
			requestPath:   "/v1/responses",
			expectedGroup: "auto",
		},
		{
			name:          "keeps explicit group",
			usingGroup:    "codex",
			requestPath:   "/v1/chat/completions",
			expectedGroup: "codex",
		},
		{
			name:          "ignores embedded chat completions fragment",
			usingGroup:    "auto",
			requestPath:   "/proxy/v1/chat/completions",
			expectedGroup: "auto",
		},
		{
			name:          "ignores similar chat completions prefix",
			usingGroup:    "auto",
			requestPath:   "/v1/chat/completions-extra",
			expectedGroup: "auto",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := autoGroupForRequestPath(tt.usingGroup, tt.requestPath)

			assert.Equal(t, tt.expectedGroup, got)
			assert.Equal(t, tt.expectedChanged, changed)
		})
	}
}

func TestRouteAutoGroupForRequestPathUpdatesRetryTokenGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")

	routedGroup := routeAutoGroupForRequestPath(c, "auto")

	assert.Equal(t, "codex-completions", routedGroup)
	assert.Equal(t, "codex-completions", common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
	assert.Equal(t, "codex-completions", common.GetContextKeyString(c, constant.ContextKeyTokenGroup))
}

func TestRouteAutoGroupForRequestPathKeepsResponsesRetryAuto(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")

	routedGroup := routeAutoGroupForRequestPath(c, "auto")

	assert.Equal(t, "auto", routedGroup)
	assert.Equal(t, "auto", common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
	assert.Equal(t, "auto", common.GetContextKeyString(c, constant.ContextKeyTokenGroup))
	assert.Equal(t, []string{"codex", "codex-pro"}, common.GetContextKeyStringSlice(c, constant.ContextKeyRouteAutoGroups))
}

func TestDetectRequestFormatByEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		path        string
		expectation types.RequestFormat
	}{
		{name: "openai chat completions", path: "/v1/chat/completions", expectation: types.RequestFormatOpenAI},
		{name: "openai completions", path: "/v1/completions", expectation: types.RequestFormatOpenAI},
		{name: "responses", path: "/v1/responses", expectation: types.RequestFormatResponses},
		{name: "claude", path: "/v1/messages", expectation: types.RequestFormatClaude},
		{name: "gemini v1beta", path: "/v1beta/models/gemini-2.0-flash:generateContent", expectation: types.RequestFormatGemini},
		{name: "gemini v1", path: "/v1/models/gemini-2.0-flash:streamGenerateContent", expectation: types.RequestFormatGemini},
		{name: "unknown", path: "/v1/moderations", expectation: types.RequestFormatUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, nil)

			assert.Equal(t, tt.expectation, detectRequestFormat(c))
		})
	}
}

func TestDetectRequestFormatFromBodyFallbacksRestoreBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		body        string
		expectation types.RequestFormat
	}{
		{
			name:        "gemini cli contents",
			body:        `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`,
			expectation: types.RequestFormatGeminiCLI,
		},
		{
			name:        "gemini cli batch contents",
			body:        `{"requests":[{"contents":[{"parts":[{"text":"hello"}]}]}]}`,
			expectation: types.RequestFormatGeminiCLI,
		},
		{
			name:        "openai shaped unknown endpoint",
			body:        `{"model":"gpt-4.1","messages":[{"role":"user","content":"hello"}]}`,
			expectation: types.RequestFormatUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/unknown", strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")

			got := detectRequestFormat(c)

			restored, err := io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			assert.Equal(t, tt.expectation, got)
			assert.JSONEq(t, tt.body, string(restored))
		})
	}
}
