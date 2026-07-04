package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
)

const (
	defaultTimeoutSeconds         = 10
	defaultEndpoint               = "/api/pricing"
	maxConcurrentFetches          = 8
	maxRatioConfigBytes           = 10 << 20 // 10MB
	floatEpsilon                  = 1e-9
	openAIOfficialPresetID        = -102
	openAIOfficialPresetName      = "OpenAI 官方价格"
	openAIOfficialPresetBaseURL   = "https://developers.openai.com"
	openAIOfficialEndpoint        = "https://developers.openai.com/api/docs/pricing.md"
	claudeOfficialPresetID        = -103
	claudeOfficialPresetName      = "Claude 官方价格"
	claudeOfficialPresetBaseURL   = "https://platform.claude.com"
	claudeOfficialEndpoint        = "https://platform.claude.com/docs/en/about-claude/pricing.md"
	geminiOfficialPresetID        = -104
	geminiOfficialPresetName      = "Gemini 官方价格"
	geminiOfficialPresetBaseURL   = "https://ai.google.dev"
	geminiOfficialEndpoint        = "https://ai.google.dev/gemini-api/docs/pricing"
	glmOfficialPresetID           = -105
	glmOfficialPresetName         = "GLM 官方价格"
	glmOfficialPresetBaseURL      = "https://docs.bigmodel.cn"
	glmOfficialEndpoint           = "https://docs.bigmodel.cn/cn/guide/models/text/glm-4.5"
	xAIOfficialPresetID           = -106
	xAIOfficialPresetName         = "xAI 官方价格"
	xAIOfficialPresetBaseURL      = "https://docs.x.ai"
	xAIOfficialEndpoint           = "https://docs.x.ai/developers/models.md"
	deepSeekOfficialPresetID      = -107
	deepSeekOfficialPresetName    = "DeepSeek 官方价格"
	deepSeekOfficialPresetBaseURL = "https://api-docs.deepseek.com"
	deepSeekOfficialEndpoint      = "https://api-docs.deepseek.com/quick_start/pricing"
)

func nearlyEqual(a, b float64) bool {
	if a > b {
		return a-b < floatEpsilon
	}
	return b-a < floatEpsilon
}

func valuesEqual(a, b interface{}) bool {
	af, aok := a.(float64)
	bf, bok := b.(float64)
	if aok && bok {
		return nearlyEqual(af, bf)
	}
	return a == b
}

var pricingSyncFields = []string{
	"model_ratio",
	"completion_ratio",
	"cache_ratio",
	"create_cache_ratio",
	"image_ratio",
	"audio_ratio",
	"audio_completion_ratio",
	"model_price",
	billing_setting.BillingModeField,
	billing_setting.BillingExprField,
}

var numericPricingSyncFields = map[string]bool{
	"model_ratio":            true,
	"completion_ratio":       true,
	"cache_ratio":            true,
	"create_cache_ratio":     true,
	"image_ratio":            true,
	"audio_ratio":            true,
	"audio_completion_ratio": true,
	"model_price":            true,
}

type upstreamResult struct {
	Name string         `json:"name"`
	Data map[string]any `json:"data,omitempty"`
	Err  string         `json:"err,omitempty"`
}

func valueMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[string]float64:
		return lo.MapValues(typed, func(value float64, _ string) any { return value })
	case map[string]string:
		return lo.MapValues(typed, func(value string, _ string) any { return value })
	default:
		return nil
	}
}

func asFloat64(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func normalizeSyncValue(field string, value any) any {
	if numericPricingSyncFields[field] {
		if parsed, ok := asFloat64(value); ok {
			return parsed
		}
	}
	return value
}

func getLocalPricingSyncData() map[string]any {
	data := billing_setting.GetPricingSyncData(map[string]any(ratio_setting.GetExposedData()))
	data["image_ratio"] = ratio_setting.GetImageRatioCopy()
	data["audio_ratio"] = ratio_setting.GetAudioRatioCopy()
	data["audio_completion_ratio"] = ratio_setting.GetAudioCompletionRatioCopy()
	return data
}

func FetchUpstreamRatios(c *gin.Context) {
	var req dto.UpstreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SysError("failed to bind upstream request: " + err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请求参数格式错误"})
		return
	}

	if req.Timeout <= 0 {
		req.Timeout = defaultTimeoutSeconds
	}

	var upstreams []dto.UpstreamDTO

	if len(req.Upstreams) > 0 {
		for _, u := range req.Upstreams {
			if strings.HasPrefix(u.BaseURL, "http") {
				if u.Endpoint == "" {
					u.Endpoint = defaultEndpoint
				}
				u.BaseURL = strings.TrimRight(u.BaseURL, "/")
				upstreams = append(upstreams, u)
			}
		}
	}

	if len(upstreams) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无有效上游渠道"})
		return
	}

	var wg sync.WaitGroup
	ch := make(chan upstreamResult, len(upstreams))

	sem := make(chan struct{}, maxConcurrentFetches)

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{MaxIdleConns: 100, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ExpectContinueTimeout: 1 * time.Second, ResponseHeaderTimeout: 10 * time.Second}
	if common.TLSInsecureSkipVerify {
		transport.TLSClientConfig = common.InsecureTLSConfig
	}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		// 对 github.io 优先尝试 IPv4，失败则回退 IPv6
		if strings.HasSuffix(host, "github.io") {
			if conn, err := dialer.DialContext(ctx, "tcp4", addr); err == nil {
				return conn, nil
			}
			return dialer.DialContext(ctx, "tcp6", addr)
		}
		return dialer.DialContext(ctx, network, addr)
	}
	client := &http.Client{Transport: transport}

	for _, chn := range upstreams {
		wg.Add(1)
		go func(chItem dto.UpstreamDTO) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			endpoint := chItem.Endpoint
			var fullURL string
			if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
				fullURL = endpoint
			} else {
				if endpoint == "" {
					endpoint = defaultEndpoint
				} else if !strings.HasPrefix(endpoint, "/") {
					endpoint = "/" + endpoint
				}
				fullURL = chItem.BaseURL + endpoint
			}
			isOpenAIOfficial := isOpenAIOfficialPricingEndpoint(fullURL)
			isClaudeOfficial := isClaudeOfficialPricingEndpoint(fullURL)
			isXAIOfficial := isXAIOfficialPricingEndpoint(fullURL)
			isGeminiOfficial := isGeminiOfficialPricingEndpoint(fullURL)
			isGLMOfficial := isGLMOfficialPricingEndpoint(fullURL)
			isDeepSeekOfficial := isDeepSeekOfficialPricingEndpoint(fullURL)
			isOfficialPricingSource := isOpenAIOfficial || isClaudeOfficial || isXAIOfficial || isGeminiOfficial || isGLMOfficial || isDeepSeekOfficial

			uniqueName := chItem.Name
			if chItem.ID != 0 {
				uniqueName = fmt.Sprintf("%s(%d)", chItem.Name, chItem.ID)
			}
			if !isOfficialPricingSource {
				ch <- upstreamResult{Name: uniqueName, Err: "仅支持官方价格源：OpenAI、Claude、xAI、Gemini、GLM、DeepSeek"}
				return
			}

			ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(req.Timeout)*time.Second)
			defer cancel()

			httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
			if err != nil {
				logger.LogWarn(c.Request.Context(), "build request failed: "+err.Error())
				ch <- upstreamResult{Name: uniqueName, Err: err.Error()}
				return
			}

			// 简单重试：最多 3 次，指数退避
			var resp *http.Response
			var lastErr error
			for attempt := 0; attempt < 3; attempt++ {
				resp, lastErr = client.Do(httpReq)
				if lastErr == nil {
					break
				}
				time.Sleep(time.Duration(200*(1<<attempt)) * time.Millisecond)
			}
			if lastErr != nil {
				logger.LogWarn(c.Request.Context(), "http error on "+chItem.Name+": "+lastErr.Error())
				ch <- upstreamResult{Name: uniqueName, Err: lastErr.Error()}
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				logger.LogWarn(c.Request.Context(), "non-200 from "+chItem.Name+": "+resp.Status)
				ch <- upstreamResult{Name: uniqueName, Err: resp.Status}
				return
			}

			limited := io.LimitReader(resp.Body, maxRatioConfigBytes)
			bodyBytes, err := io.ReadAll(limited)
			if err != nil {
				logger.LogWarn(c.Request.Context(), "read response failed from "+chItem.Name+": "+err.Error())
				ch <- upstreamResult{Name: uniqueName, Err: err.Error()}
				return
			}

			if isOpenAIOfficial {
				converted, err := convertOpenAIOfficialPricingToRatioData(bytes.NewReader(bodyBytes))
				if err != nil {
					logger.LogWarn(c.Request.Context(), "OpenAI official pricing parse failed from "+chItem.Name+": "+err.Error())
					ch <- upstreamResult{Name: uniqueName, Err: err.Error()}
					return
				}
				ch <- upstreamResult{Name: uniqueName, Data: converted}
				return
			}

			if isClaudeOfficial {
				converted, err := convertClaudeOfficialPricingToRatioData(bytes.NewReader(bodyBytes))
				if err != nil {
					logger.LogWarn(c.Request.Context(), "Claude official pricing parse failed from "+chItem.Name+": "+err.Error())
					ch <- upstreamResult{Name: uniqueName, Err: err.Error()}
					return
				}
				ch <- upstreamResult{Name: uniqueName, Data: converted}
				return
			}

			if isXAIOfficial {
				converted, err := service.ConvertXAIOfficialPricingToRatioData(bytes.NewReader(bodyBytes))
				if err != nil {
					logger.LogWarn(c.Request.Context(), "xAI official pricing parse failed from "+chItem.Name+": "+err.Error())
					ch <- upstreamResult{Name: uniqueName, Err: err.Error()}
					return
				}
				ch <- upstreamResult{Name: uniqueName, Data: converted}
				return
			}

			if isGeminiOfficial {
				converted, err := service.ConvertGeminiOfficialPricingToRatioData(bytes.NewReader(bodyBytes))
				if err != nil {
					logger.LogWarn(c.Request.Context(), "Gemini official pricing parse failed from "+chItem.Name+": "+err.Error())
					ch <- upstreamResult{Name: uniqueName, Err: err.Error()}
					return
				}
				ch <- upstreamResult{Name: uniqueName, Data: converted}
				return
			}

			if isGLMOfficial {
				converted, err := service.ConvertGLMOfficialPricingToRatioData(bytes.NewReader(bodyBytes))
				if err != nil {
					logger.LogWarn(c.Request.Context(), "GLM official pricing parse failed from "+chItem.Name+": "+err.Error())
					ch <- upstreamResult{Name: uniqueName, Err: err.Error()}
					return
				}
				ch <- upstreamResult{Name: uniqueName, Data: converted}
				return
			}

			if isDeepSeekOfficial {
				converted, err := service.ConvertDeepSeekOfficialPricingToRatioData(bytes.NewReader(bodyBytes))
				if err != nil {
					logger.LogWarn(c.Request.Context(), "DeepSeek official pricing parse failed from "+chItem.Name+": "+err.Error())
					ch <- upstreamResult{Name: uniqueName, Err: err.Error()}
					return
				}
				ch <- upstreamResult{Name: uniqueName, Data: converted}
				return
			}
			ch <- upstreamResult{Name: uniqueName, Err: "官方价格源未匹配解析器"}
		}(chn)
	}

	wg.Wait()
	close(ch)

	localData := getLocalPricingSyncData()

	var testResults []dto.TestResult
	var successfulChannels []struct {
		name string
		data map[string]any
	}

	for r := range ch {
		if r.Err != "" {
			testResults = append(testResults, dto.TestResult{
				Name:   r.Name,
				Status: "error",
				Error:  r.Err,
			})
		} else {
			testResults = append(testResults, dto.TestResult{
				Name:   r.Name,
				Status: "success",
			})
			successfulChannels = append(successfulChannels, struct {
				name string
				data map[string]any
			}{name: r.Name, data: r.Data})
		}
	}

	differences := buildDifferences(localData, successfulChannels)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"differences":  differences,
			"test_results": testResults,
		},
	})
}

func buildDifferences(localData map[string]any, successfulChannels []struct {
	name string
	data map[string]any
}) map[string]map[string]dto.DifferenceItem {
	differences := make(map[string]map[string]dto.DifferenceItem)

	allModels := make(map[string]struct{})

	for _, field := range pricingSyncFields {
		for modelName := range valueMap(localData[field]) {
			allModels[modelName] = struct{}{}
		}
	}

	for _, channel := range successfulChannels {
		for _, field := range pricingSyncFields {
			for modelName := range valueMap(channel.data[field]) {
				allModels[modelName] = struct{}{}
			}
		}
	}

	confidenceMap := make(map[string]map[string]bool)

	// 预处理阶段：检查pricing接口的可信度
	for _, channel := range successfulChannels {
		confidenceMap[channel.name] = make(map[string]bool)

		modelRatios := valueMap(channel.data["model_ratio"])
		completionRatios := valueMap(channel.data["completion_ratio"])

		if len(modelRatios) > 0 && len(completionRatios) > 0 {
			// 遍历所有模型，检查是否满足不可信条件
			for modelName := range allModels {
				// 默认为可信
				confidenceMap[channel.name][modelName] = true

				// 检查是否满足不可信条件：model_ratio为37.5且completion_ratio为1
				if modelRatioVal, ok := modelRatios[modelName]; ok {
					if completionRatioVal, ok := completionRatios[modelName]; ok {
						// 转换为float64进行比较
						modelRatioFloat, modelRatioOK := asFloat64(modelRatioVal)
						completionRatioFloat, completionRatioOK := asFloat64(completionRatioVal)
						if modelRatioOK && completionRatioOK && nearlyEqual(modelRatioFloat, 37.5) && nearlyEqual(completionRatioFloat, 1.0) {
							confidenceMap[channel.name][modelName] = false
						}
					}
				}
			}
		} else {
			// 如果不是从pricing接口获取的数据，则全部标记为可信
			for modelName := range allModels {
				confidenceMap[channel.name][modelName] = true
			}
		}
	}

	for modelName := range allModels {
		for _, ratioType := range pricingSyncFields {
			var localValue interface{} = nil
			if val, exists := valueMap(localData[ratioType])[modelName]; exists {
				localValue = normalizeSyncValue(ratioType, val)
			}

			upstreamValues := make(map[string]interface{})
			confidenceValues := make(map[string]bool)
			hasUpstreamValue := false
			hasDifference := false

			for _, channel := range successfulChannels {
				var upstreamValue interface{} = nil

				if val, exists := valueMap(channel.data[ratioType])[modelName]; exists {
					upstreamValue = normalizeSyncValue(ratioType, val)
					hasUpstreamValue = true

					if localValue != nil && !valuesEqual(localValue, upstreamValue) {
						hasDifference = true
					} else if valuesEqual(localValue, upstreamValue) {
						upstreamValue = "same"
					}
				}
				if upstreamValue == nil && localValue == nil {
					upstreamValue = "same"
				}

				if localValue == nil && upstreamValue != nil && upstreamValue != "same" {
					hasDifference = true
				}

				upstreamValues[channel.name] = upstreamValue

				confidenceValues[channel.name] = confidenceMap[channel.name][modelName]
			}

			shouldInclude := false

			if localValue != nil {
				if hasDifference {
					shouldInclude = true
				}
			} else {
				if hasUpstreamValue {
					shouldInclude = true
				}
			}

			if shouldInclude {
				if differences[modelName] == nil {
					differences[modelName] = make(map[string]dto.DifferenceItem)
				}
				differences[modelName][ratioType] = dto.DifferenceItem{
					Current:    localValue,
					Upstreams:  upstreamValues,
					Confidence: confidenceValues,
				}
			}
		}
	}

	channelHasDiff := make(map[string]bool)
	for _, ratioMap := range differences {
		for _, item := range ratioMap {
			for chName, val := range item.Upstreams {
				if val != nil && val != "same" {
					channelHasDiff[chName] = true
				}
			}
		}
	}

	for modelName, ratioMap := range differences {
		for ratioType, item := range ratioMap {
			for chName := range item.Upstreams {
				if !channelHasDiff[chName] {
					delete(item.Upstreams, chName)
					delete(item.Confidence, chName)
				}
			}

			allSame := true
			for _, v := range item.Upstreams {
				if v != "same" {
					allSame = false
					break
				}
			}
			if len(item.Upstreams) == 0 || allSame {
				delete(ratioMap, ratioType)
			} else {
				differences[modelName][ratioType] = item
			}
		}

		if len(ratioMap) == 0 {
			delete(differences, modelName)
		}
	}

	return differences
}

func roundRatioValue(value float64) float64 {
	return math.Round(value*1e6) / 1e6
}

func isOpenAIOfficialPricingEndpoint(rawURL string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if strings.ToLower(parsedURL.Hostname()) != "developers.openai.com" {
		return false
	}
	path := strings.TrimSuffix(parsedURL.Path, "/")
	return path == "/api/docs/pricing" || path == "/api/docs/pricing.md"
}

func isClaudeOfficialPricingEndpoint(rawURL string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if strings.ToLower(parsedURL.Hostname()) != "platform.claude.com" {
		return false
	}
	path := strings.TrimSuffix(parsedURL.Path, "/")
	return path == "/docs/en/about-claude/pricing" || path == "/docs/en/about-claude/pricing.md"
}

func isXAIOfficialPricingEndpoint(rawURL string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if strings.ToLower(parsedURL.Hostname()) != "docs.x.ai" {
		return false
	}
	path := strings.TrimSuffix(parsedURL.Path, "/")
	return path == "/developers/models" || path == "/developers/models.md" || path == "/developers/pricing" || path == "/developers/pricing.md"
}

func isGeminiOfficialPricingEndpoint(rawURL string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if strings.ToLower(parsedURL.Hostname()) != "ai.google.dev" {
		return false
	}
	path := strings.TrimSuffix(parsedURL.Path, "/")
	return path == "/gemini-api/docs/pricing"
}

func isGLMOfficialPricingEndpoint(rawURL string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if strings.ToLower(parsedURL.Hostname()) != "docs.bigmodel.cn" {
		return false
	}
	path := strings.TrimSuffix(parsedURL.Path, "/")
	return path == "/cn/guide/models/text/glm-4.5" || path == "/cn/guide/models/text/glm-4.5.md"
}

func isDeepSeekOfficialPricingEndpoint(rawURL string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if strings.ToLower(parsedURL.Hostname()) != "api-docs.deepseek.com" {
		return false
	}
	path := strings.TrimSuffix(parsedURL.Path, "/")
	return path == "/quick_start/pricing"
}

func convertOpenAIOfficialPricingToRatioData(reader io.Reader) (map[string]any, error) {
	return service.ConvertOpenAIOfficialPricingToRatioData(reader)
}

func convertClaudeOfficialPricingToRatioData(reader io.Reader) (map[string]any, error) {
	return service.ConvertClaudeOfficialPricingToRatioData(reader)
}

func GetSyncableChannels(c *gin.Context) {
	syncableChannels := []dto.SyncableChannel{
		{ID: openAIOfficialPresetID, Name: openAIOfficialPresetName, BaseURL: openAIOfficialPresetBaseURL, Status: 1},
		{ID: claudeOfficialPresetID, Name: claudeOfficialPresetName, BaseURL: claudeOfficialPresetBaseURL, Status: 1},
		{ID: xAIOfficialPresetID, Name: xAIOfficialPresetName, BaseURL: xAIOfficialPresetBaseURL, Status: 1},
		{ID: geminiOfficialPresetID, Name: geminiOfficialPresetName, BaseURL: geminiOfficialPresetBaseURL, Status: 1},
		{ID: glmOfficialPresetID, Name: glmOfficialPresetName, BaseURL: glmOfficialPresetBaseURL, Status: 1},
		{ID: deepSeekOfficialPresetID, Name: deepSeekOfficialPresetName, BaseURL: deepSeekOfficialPresetBaseURL, Status: 1},
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    syncableChannels,
	})
}
