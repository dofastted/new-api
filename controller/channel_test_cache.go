package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type channelTestCachedResult = service.ChannelTestCachedResult

func channelTestCacheKey(channelID int, modelName string, endpointType string, isStream bool) string {
	return service.ChannelTestCacheKey(channelID, modelName, endpointType, isStream)
}

func getCachedChannelTestResult(cacheKey string) (channelTestCachedResult, bool) {
	return service.GetCachedChannelTestResult(cacheKey)
}

func setCachedChannelTestResult(cacheKey string, result channelTestCachedResult) {
	service.SetCachedChannelTestResult(cacheKey, result)
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
	return service.ShouldBlockManualClaudeChannelHealthProbe(channel)
}

func claudeChannelHealthProbeBlockedResult() channelTestCachedResult {
	return service.ClaudeChannelHealthProbeBlockedResult()
}
