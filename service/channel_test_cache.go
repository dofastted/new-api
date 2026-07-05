package service

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/samber/hot"
)

const (
	channelTestResultCacheNamespace = "channel_test_result:v1"
	channelTestResultCacheTTL       = time.Hour
)

var (
	channelTestResultCacheOnce sync.Once
	channelTestResultCache     *cachex.HybridCache[ChannelTestCachedResult]
)

type ChannelTestCachedResult struct {
	Success   bool    `json:"success"`
	Message   string  `json:"message"`
	ErrorCode string  `json:"error_code,omitempty"`
	Time      float64 `json:"time"`
	TestedAt  int64   `json:"tested_at"`
}

func getChannelTestResultCache() *cachex.HybridCache[ChannelTestCachedResult] {
	channelTestResultCacheOnce.Do(func() {
		channelTestResultCache = cachex.NewHybridCache[ChannelTestCachedResult](cachex.HybridCacheConfig[ChannelTestCachedResult]{
			Namespace:  cachex.Namespace(channelTestResultCacheNamespace),
			Redis:      common.RDB,
			RedisCodec: cachex.JSONCodec[ChannelTestCachedResult]{},
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			Memory: func() *hot.HotCache[string, ChannelTestCachedResult] {
				return hot.NewHotCache[string, ChannelTestCachedResult](hot.LRU, 100_000).
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

func ChannelTestCacheKey(channelID int, modelName string, endpointType string, isStream bool) string {
	return fmt.Sprintf(
		"channel:%d:model:%s:endpoint:%s:stream:%t",
		channelID,
		normalizeChannelTestCachePart(modelName, "default"),
		normalizeChannelTestCachePart(endpointType, "auto"),
		isStream,
	)
}

func GetCachedChannelTestResult(cacheKey string) (ChannelTestCachedResult, bool) {
	cached, found, err := getChannelTestResultCache().Get(cacheKey)
	if err != nil {
		common.SysError(fmt.Sprintf("channel test cache get failed: key=%s, err=%v", cacheKey, err))
		return ChannelTestCachedResult{}, false
	}
	return cached, found
}

func SetCachedChannelTestResult(cacheKey string, result ChannelTestCachedResult) {
	if result.TestedAt == 0 {
		result.TestedAt = common.GetTimestamp()
	}
	if err := getChannelTestResultCache().SetWithTTL(cacheKey, result, channelTestResultCacheTTL); err != nil {
		common.SysError(fmt.Sprintf("channel test cache set failed: key=%s, err=%v", cacheKey, err))
	}
}

func PurgeChannelTestResultCache() {
	if err := getChannelTestResultCache().Purge(); err != nil {
		common.SysError(fmt.Sprintf("channel test cache purge failed: err=%v", err))
	}
}

func ChannelTestUsesStream(channel *model.Channel) bool {
	return channel != nil && channel.Type == constant.ChannelTypeCodex
}

func ShouldBlockManualClaudeChannelHealthProbe(channel *model.Channel) bool {
	return channel != nil && channel.Type == constant.ChannelTypeAnthropic
}

func ClaudeChannelHealthProbeBlockedResult() ChannelTestCachedResult {
	return ChannelTestCachedResult{
		Success:  false,
		Message:  "Claude channel health probe is disabled; use cached scheduled monitor result or Claude Code traffic instead",
		Time:     0,
		TestedAt: common.GetTimestamp(),
	}
}

func ShouldExcludeChannelByCachedHealth(channel *model.Channel) bool {
	if channel == nil || channel.Type != constant.ChannelTypeAnthropic {
		return false
	}
	cached, found := GetCachedChannelTestResult(ChannelTestCacheKey(channel.Id, "", "", ChannelTestUsesStream(channel)))
	return found && !cached.Success
}
