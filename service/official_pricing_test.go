package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertGeminiOfficialPricingToRatioData(t *testing.T) {
	body := `
## Gemini 2.5 Flash

*` + "`gemini-2.5-flash`" + `*

### Standard

|  | Free Tier | Paid Tier, per 1M tokens in USD |
| --- | --- | --- |
| Input price | Free of charge | $0.30 (text / image / video)   $1.00 (audio) |
| Output price (including thinking tokens) | Free of charge | $2.50 |
| Context caching price | Not available | $0.03 (text / image / video)   $0.1 (audio)   $1.00 / 1,000,000 tokens per hour (storage price) |

### Batch

|  | Free Tier | Paid Tier, per 1M tokens in USD |
| --- | --- | --- |
| Input price | Not available | $0.15 |
| Output price (including thinking tokens) | Not available | $1.25 |

## Gemini 2.5 Flash Image

*` + "`gemini-2.5-flash-image`" + `*

### Standard

|  | Free Tier | Paid Tier, per 1M tokens in USD |
| --- | --- | --- |
| Input price | Not available | $0.30 (text / image) |
| Output price | Not available | $0.039 per image* |
`

	converted, err := ConvertGeminiOfficialPricingToRatioData(strings.NewReader(body))
	require.NoError(t, err)

	modelRatio := converted["model_ratio"].(map[string]any)
	completionRatio := converted["completion_ratio"].(map[string]any)
	cacheRatio := converted["cache_ratio"].(map[string]any)

	assert.Equal(t, 0.15, modelRatio["gemini-2.5-flash"])
	assert.Equal(t, 8.33333333, completionRatio["gemini-2.5-flash"])
	assert.Equal(t, 0.1, cacheRatio["gemini-2.5-flash"])
	assert.NotContains(t, modelRatio, "gemini-2.5-flash-image")
}

func TestConvertGLMOfficialPricingToRatioData(t *testing.T) {
	body := `
# GLM-4.5

<Card><h3>GLM-4.5</h3></Card>
<Card><h3>GLM-4.5-Air</h3></Card>
<Card><h3>GLM-4.7</h3></Card>

API 调用价格低至**输入 0.8 元/百万 tokens，输出 2 元/百万 tokens**
`

	converted, err := ConvertGLMOfficialPricingToRatioData(strings.NewReader(body))
	require.NoError(t, err)

	modelRatio := converted["model_ratio"].(map[string]any)
	completionRatio := converted["completion_ratio"].(map[string]any)

	expectedRatio := 0.8 / ratio_setting.USD2RMB * float64(ratio_setting.USD) / 1000
	assert.Equal(t, roundOfficialRatioValue(expectedRatio), modelRatio["glm-4.5"])
	assert.Equal(t, roundOfficialRatioValue(expectedRatio), modelRatio["glm-4.5-air"])
	assert.Equal(t, 2.5, completionRatio["glm-4.5"])
	assert.NotContains(t, modelRatio, "glm-4.7")
}

func TestConvertOpenAIOfficialPricingIncludesCodex(t *testing.T) {
	body := `
<div data-content-switcher-pane data-value="standard">
<TextTokenPricingTables tier="standard" rows={[
  ["gpt-5-codex", 1.25, 0.125, 10],
]} />
</div>`

	converted, err := ConvertOpenAIOfficialPricingToRatioData(strings.NewReader(body))
	require.NoError(t, err)

	modelRatio := converted["model_ratio"].(map[string]any)
	completionRatio := converted["completion_ratio"].(map[string]any)
	cacheRatio := converted["cache_ratio"].(map[string]any)

	assert.Equal(t, 0.625, modelRatio["gpt-5-codex"])
	assert.Equal(t, 8.0, completionRatio["gpt-5-codex"])
	assert.Equal(t, 0.1, cacheRatio["gpt-5-codex"])
}
