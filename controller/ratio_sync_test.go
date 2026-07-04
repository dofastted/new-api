package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIOfficialPricingToRatioData(t *testing.T) {
	body := `
<div data-content-switcher-pane data-value="standard">
<TextTokenPricingTables tier="standard" rows={[
  ["gpt-5.5 (<272K context length)", 5, 0.5, 30],
  ["gpt-5.4-mini", 0.75, 0.075, 4.5],
  ["gpt-5-pro", 15, null, 120],
]} />
</div>
<div data-content-switcher-pane data-value="batch" hidden>
<TextTokenPricingTables rows={[
  ["gpt-5.5 (<272K context length)", 2.5, 0.25, 15],
]} />
</div>`

	converted, err := convertOpenAIOfficialPricingToRatioData(strings.NewReader(body))
	require.NoError(t, err)
	modelRatio := converted["model_ratio"].(map[string]any)
	completionRatio := converted["completion_ratio"].(map[string]any)
	cacheRatio := converted["cache_ratio"].(map[string]any)
	require.Equal(t, 2.5, modelRatio["gpt-5.5"])
	require.Equal(t, 0.375, modelRatio["gpt-5.4-mini"])
	require.Equal(t, 7.5, modelRatio["gpt-5-pro"])
	require.Equal(t, 6.0, completionRatio["gpt-5.5"])
	require.Equal(t, 0.1, cacheRatio["gpt-5.5"])
	require.NotContains(t, modelRatio, "gpt-5.5 (<272K context length)")
}

func TestConvertClaudeOfficialPricingToRatioData(t *testing.T) {
	body := `
| Model             | Base Input Tokens | 5m Cache Writes | 1h Cache Writes | Cache Hits & Refreshes | Output Tokens |
|-------------------|-------------------|-----------------|-----------------|----------------------|---------------|
| Claude Fable 5      | $10 / MTok        | $12.50 / MTok   | $20 / MTok      | $1 / MTok | $50 / MTok    |
| Claude Opus 4.8     | $5 / MTok         | $6.25 / MTok    | $10 / MTok      | $0.50 / MTok | $25 / MTok    |
| Claude Sonnet 4.6   | $3 / MTok         | $3.75 / MTok    | $6 / MTok       | $0.30 / MTok | $15 / MTok    |
| Claude Haiku 4.5  | $1 / MTok         | $1.25 / MTok    | $2 / MTok       | $0.10 / MTok | $5 / MTok     |
`

	converted, err := convertClaudeOfficialPricingToRatioData(strings.NewReader(body))

	require.NoError(t, err)
	modelRatio := converted["model_ratio"].(map[string]any)
	completionRatio := converted["completion_ratio"].(map[string]any)
	cacheRatio := converted["cache_ratio"].(map[string]any)
	createCacheRatio := converted["create_cache_ratio"].(map[string]any)
	require.Equal(t, 5.0, modelRatio["claude-fable-5"])
	require.Equal(t, 2.5, modelRatio["claude-opus-4-8"])
	require.Equal(t, 1.5, modelRatio["claude-sonnet-4-6"])
	require.Equal(t, 0.5, modelRatio["claude-haiku-4-5"])
	require.Equal(t, 5.0, completionRatio["claude-opus-4-8"])
	require.Equal(t, 0.1, cacheRatio["claude-opus-4-8"])
	require.Equal(t, 1.25, createCacheRatio["claude-opus-4-8"])
	require.Equal(t, 2.5, modelRatio["claude-opus-4-8-xhigh"])
}

func TestOfficialPricingEndpointDetection(t *testing.T) {
	require.True(t, isOpenAIOfficialPricingEndpoint("https://developers.openai.com/api/docs/pricing.md"))
	require.True(t, isOpenAIOfficialPricingEndpoint("https://developers.openai.com/api/docs/pricing"))
	require.False(t, isOpenAIOfficialPricingEndpoint("https://openai.com/api/pricing/"))
	require.True(t, isClaudeOfficialPricingEndpoint("https://platform.claude.com/docs/en/about-claude/pricing.md"))
	require.True(t, isClaudeOfficialPricingEndpoint("https://platform.claude.com/docs/en/about-claude/pricing"))
	require.False(t, isClaudeOfficialPricingEndpoint("https://claude.com/pricing"))
	require.True(t, isXAIOfficialPricingEndpoint("https://docs.x.ai/developers/models.md"))
	require.True(t, isXAIOfficialPricingEndpoint("https://docs.x.ai/developers/pricing"))
	require.False(t, isXAIOfficialPricingEndpoint("https://x.ai/api"))
	require.True(t, isDeepSeekOfficialPricingEndpoint("https://api-docs.deepseek.com/quick_start/pricing"))
	require.False(t, isDeepSeekOfficialPricingEndpoint("https://deepseek.com/pricing"))
}

func TestGetSyncableChannelsOnlyOfficialPricingSources(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	GetSyncableChannels(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Success bool `json:"success"`
		Data    []struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			BaseURL string `json:"base_url"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Len(t, body.Data, 6)
	names := make([]string, 0, len(body.Data))
	for _, item := range body.Data {
		names = append(names, item.Name)
		require.Less(t, item.ID, 0)
		require.NotEmpty(t, item.BaseURL)
	}
	require.ElementsMatch(t, []string{
		"OpenAI 官方价格",
		"Claude 官方价格",
		"xAI 官方价格",
		"Gemini 官方价格",
		"GLM 官方价格",
		"DeepSeek 官方价格",
	}, names)
	require.NotContains(t, names, "官方倍率预设")
	require.NotContains(t, names, "models.dev 价格预设")
}
