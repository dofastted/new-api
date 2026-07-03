package controller

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
)

const (
	channelTestResultCacheNamespace = "channel_test_result:v1"
	channelTestResultCacheTTL       = time.Hour
)

var (
	channelTestResultCacheOnce sync.Once
	channelTestResultCache     *cachex.HybridCache[channelTestCachedResult]
)

type channelTestCachedResult struct {
	Success   bool    `json:"success"`
	Message   string  `json:"message"`
	ErrorCode string  `json:"error_code,omitempty"`
	Time      float64 `json:"time"`
	TestedAt  int64   `json:"tested_at"`
}

func getChannelTestResultCache() *cachex.HybridCache[channelTestCachedResult] {
	channelTestResultCacheOnce.Do(func() {
		channelTestResultCache = cachex.NewHybridCache[channelTestCachedResult](cachex.HybridCacheConfig[channelTestCachedResult]{
			Namespace:  cachex.Namespace(channelTestResultCacheNamespace),
			Redis:      common.RDB,
			RedisCodec: cachex.JSONCodec[channelTestCachedResult]{},
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			Memory: func() *hot.HotCache[string, channelTestCachedResult] {
				return hot.NewHotCache[string, channelTestCachedResult](hot.LRU, 100_000).
					WithTTL(channelTestResultCacheTTL).
					Build()
			},
		})
	})
	return channelTestResultCache
}

func normalizeChannelTestCachePart(value string, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	replacer := strings.NewReplacer(" ", "_", "\t", "_", "\n", "_", "/", "_", "\\", "_", ":", "_", "|", "_")
	return replacer.Replace(value)
}

func channelTestCacheKey(channelID int, modelName string, endpointType string, isStream bool) string {
	return fmt.Sprintf(
		"channel:%d:model:%s:endpoint:%s:stream:%t",
		channelID,
		normalizeChannelTestCachePart(modelName, "default"),
		normalizeChannelTestCachePart(endpointType, "auto"),
		isStream,
	)
}

func getCachedChannelTestResult(cacheKey string) (channelTestCachedResult, bool) {
	cached, found, err := getChannelTestResultCache().Get(cacheKey)
	if err != nil {
		common.SysError(fmt.Sprintf("channel test cache get failed: key=%s, err=%v", cacheKey, err))
		return channelTestCachedResult{}, false
	}
	return cached, found
}

func setCachedChannelTestResult(cacheKey string, result channelTestCachedResult) {
	if result.TestedAt == 0 {
		result.TestedAt = common.GetTimestamp()
	}
	if err := getChannelTestResultCache().SetWithTTL(cacheKey, result, channelTestResultCacheTTL); err != nil {
		common.SysError(fmt.Sprintf("channel test cache set failed: key=%s, err=%v", cacheKey, err))
	}
}

func channelTestResponseFromCachedResult(result channelTestCachedResult, fromCache bool) gin.H {
	resp := gin.H{
		"success":   result.Success,
		"message":   result.Message,
		"time":      result.Time,
		"cached":    fromCache,
		"tested_at": result.TestedAt,
	}
	if result.ErrorCode != "" {
		resp["error_code"] = result.ErrorCode
	}
	return resp
}

func channelTestCachedResultFromTestResult(result testResult, consumedTime float64) channelTestCachedResult {
	if result.localErr != nil {
		cached := channelTestCachedResult{
			Success:  false,
			Message:  result.localErr.Error(),
			Time:     0,
			TestedAt: common.GetTimestamp(),
		}
		if result.newAPIError != nil {
			cached.ErrorCode = string(result.newAPIError.GetErrorCode())
		}
		return cached
	}
	if result.newAPIError != nil {
		return channelTestCachedResult{
			Success:   false,
			Message:   result.newAPIError.Error(),
			ErrorCode: string(result.newAPIError.GetErrorCode()),
			Time:      consumedTime,
			TestedAt:  common.GetTimestamp(),
		}
	}
	return channelTestCachedResult{
		Success:  true,
		Message:  "",
		Time:     consumedTime,
		TestedAt: common.GetTimestamp(),
	}
}

func shouldBlockManualClaudeChannelHealthProbe(channel *model.Channel) bool {
	return channel != nil && channel.Type == constant.ChannelTypeAnthropic
}

func claudeChannelHealthProbeBlockedResult() channelTestCachedResult {
	return channelTestCachedResult{
		Success:  false,
		Message:  "Claude channel health probe is disabled; use cached scheduled monitor result or Claude Code traffic instead",
		Time:     0,
		TestedAt: common.GetTimestamp(),
	}
}
