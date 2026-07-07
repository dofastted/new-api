package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx         *gin.Context
	TokenGroup  string
	ModelName   string
	RequestPath string
	Retry       *int
	// ExcludedChannelIds is a compatibility snapshot of request-local exclusions.
	// New retry code should update it through the methods below so durable
	// provider failures are not lost when transient rate-limit exclusions reset.
	ExcludedChannelIds    map[int]struct{}
	FailedChannelIds      map[int]struct{}
	RateLimitedChannelIds map[int]struct{}
	AllowedGroups         []string
	FallbackToAllGroups   bool
	resetNextTry          bool
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

func (p *RetryParam) ExcludeChannel(channelID int) {
	p.ExcludeFailedChannel(channelID)
}

func (p *RetryParam) ExcludeFailedChannel(channelID int) {
	if p == nil || channelID <= 0 {
		return
	}
	if p.FailedChannelIds == nil {
		p.FailedChannelIds = make(map[int]struct{})
	}
	p.FailedChannelIds[channelID] = struct{}{}
	p.syncExcludedChannelIds()
}

func (p *RetryParam) ExcludeRateLimitedChannel(channelID int) bool {
	if p == nil || channelID <= 0 || p.IsChannelExcluded(channelID) {
		return false
	}
	if p.RateLimitedChannelIds == nil {
		p.RateLimitedChannelIds = make(map[int]struct{})
	}
	p.RateLimitedChannelIds[channelID] = struct{}{}
	p.syncExcludedChannelIds()
	return true
}

func (p *RetryParam) ResetRateLimitedChannelExclusions() {
	if p == nil {
		return
	}
	p.RateLimitedChannelIds = nil
	p.syncExcludedChannelIds()
}

func (p *RetryParam) IsChannelExcluded(channelID int) bool {
	if p == nil || channelID <= 0 {
		return false
	}
	if _, ok := p.FailedChannelIds[channelID]; ok {
		return true
	}
	if _, ok := p.RateLimitedChannelIds[channelID]; ok {
		return true
	}
	_, ok := p.ExcludedChannelIds[channelID]
	return ok
}

func (p *RetryParam) SelectionExcludedChannelIds() map[int]struct{} {
	if p == nil {
		return nil
	}
	return mergeChannelExclusionMaps(p.ExcludedChannelIds, p.FailedChannelIds, p.RateLimitedChannelIds)
}

func (p *RetryParam) syncExcludedChannelIds() {
	if p == nil {
		return
	}
	p.ExcludedChannelIds = mergeChannelExclusionMaps(p.FailedChannelIds, p.RateLimitedChannelIds)
}

func mergeChannelExclusionMaps(maps ...map[int]struct{}) map[int]struct{} {
	size := 0
	for _, m := range maps {
		size += len(m)
	}
	if size == 0 {
		return nil
	}
	merged := make(map[int]struct{}, size)
	for _, m := range maps {
		for id := range m {
			merged[id] = struct{}{}
		}
	}
	return merged
}

// CacheGetRandomSatisfiedChannel tries to get a random channel that satisfies the requirements.
// 尝试获取一个满足要求的随机渠道。
//
// For "auto" tokenGroup with cross-group Retry enabled:
// 对于启用了跨分组重试的 "auto" tokenGroup：
//
//   - Each group will exhaust all its priorities before moving to the next group.
//     每个分组会用完所有优先级后才会切换到下一个分组。
//
//   - Uses ContextKeyAutoGroupIndex to track current group index.
//     使用 ContextKeyAutoGroupIndex 跟踪当前分组索引。
//
//   - Uses ContextKeyAutoGroupRetryIndex to track the global Retry count when current group started.
//     使用 ContextKeyAutoGroupRetryIndex 跟踪当前分组开始时的全局重试次数。
//
//   - priorityRetry = Retry - startRetryIndex, represents the priority level within current group.
//     priorityRetry = Retry - startRetryIndex，表示当前分组内的优先级级别。
//
//   - When GetRandomSatisfiedChannel returns nil (priorities exhausted), moves to next group.
//     当 GetRandomSatisfiedChannel 返回 nil（优先级用完）时，切换到下一个分组。
//
// Example flow (2 groups, each with 2 priorities, RetryTimes=3):
// 示例流程（2个分组，每个有2个优先级，RetryTimes=3）：
//
//	Retry=0: GroupA, priority0 (startRetryIndex=0, priorityRetry=0)
//	         分组A, 优先级0
//
//	Retry=1: GroupA, priority1 (startRetryIndex=0, priorityRetry=1)
//	         分组A, 优先级1
//
//	Retry=2: GroupA exhausted → GroupB, priority0 (startRetryIndex=2, priorityRetry=0)
//	         分组A用完 → 分组B, 优先级0
//
//	Retry=3: GroupB, priority1 (startRetryIndex=2, priorityRetry=1)
//	         分组B, 优先级1
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)

	if param.TokenGroup == "auto" {
		autoGroups := GetRequestAutoGroup(param.Ctx, userGroup)
		if len(autoGroups) == 0 {
			if accessErr := RestrictedAutoGroupAccessError(param.Ctx, userGroup); accessErr != nil {
				return nil, selectGroup, accessErr
			}
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}

		if len(param.AllowedGroups) > 0 {
			allowedGroups := filterAllowedProviderGroups(autoGroups, param.AllowedGroups)
			if len(allowedGroups) == 0 && !param.FallbackToAllGroups {
				return nil, selectGroup, model.ErrSubscriptionProviderGroupNotAllowed
			}
			if len(allowedGroups) > 0 {
				channel, selectGroup, err = selectAutoSatisfiedChannel(param, allowedGroups, selectGroup)
				if channel != nil || err != nil || !param.FallbackToAllGroups {
					return channel, selectGroup, err
				}
				resetAutoGroupRetryState(param)
			}
		}

		return selectAutoSatisfiedChannel(param, autoGroups, selectGroup)
	} else {
		if accessErr := ProviderGroupAccessError(param.Ctx, param.TokenGroup); accessErr != nil {
			return nil, param.TokenGroup, accessErr
		}
		channel, err = selectCachedHealthyChannel(param, param.TokenGroup, param.GetRetry())
		if err != nil {
			return nil, param.TokenGroup, err
		}
	}
	return channel, selectGroup, nil
}

func selectAutoSatisfiedChannel(param *RetryParam, autoGroups []string, selectGroup string) (*model.Channel, string, error) {
	// startGroupIndex: the group index to start searching from
	// startGroupIndex: 开始搜索的分组索引
	startGroupIndex := 0
	crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)

	if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
		if idx, ok := lastGroupIndex.(int); ok {
			startGroupIndex = idx
		}
	}

	for i := startGroupIndex; i < len(autoGroups); i++ {
		autoGroup := autoGroups[i]
		// Calculate priorityRetry for current group
		// 计算当前分组的 priorityRetry
		priorityRetry := param.GetRetry()
		// If moved to a new group, reset priorityRetry and update startRetryIndex
		// 如果切换到新分组，重置 priorityRetry 并更新 startRetryIndex
		if i > startGroupIndex {
			priorityRetry = 0
		}
		logger.LogDebug(param.Ctx, "Auto selecting group: %s, priorityRetry: %d", autoGroup, priorityRetry)

		channel, _ := selectCachedHealthyChannel(param, autoGroup, priorityRetry)
		if channel == nil {
			// Current group has no available channel for this model, try next group
			// 当前分组没有该模型的可用渠道，尝试下一个分组
			logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", autoGroup, param.ModelName, priorityRetry)
			// 重置状态以尝试下一个分组
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
			// Reset retry counter so outer loop can continue for next group
			// 重置重试计数器，以便外层循环可以为下一个分组继续
			param.SetRetry(0)
			continue
		}
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
		selectGroup = autoGroup
		logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)

		// Prepare state for next retry
		// 为下一次重试准备状态
		if crossGroupRetry && priorityRetry >= common.RetryTimes {
			// Current group has exhausted all retries, prepare to switch to next group
			// This request still uses current group, but next retry will use next group
			// 当前分组已用完所有重试次数，准备切换到下一个分组
			// 本次请求仍使用当前分组，但下次重试将使用下一个分组
			logger.LogDebug(param.Ctx, "Current group %s retries exhausted (priorityRetry=%d >= RetryTimes=%d), preparing switch to next group for next retry", autoGroup, priorityRetry, common.RetryTimes)
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
			// Reset retry counter so outer loop can continue for next group
			// 重置重试计数器，以便外层循环可以为下一个分组继续
			param.SetRetry(0)
			param.ResetRetryNextTry()
		} else {
			// Stay in current group, save current state
			// 保持在当前分组，保存当前状态
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
		}
		return channel, selectGroup, nil
	}
	return nil, selectGroup, nil
}

func filterAllowedProviderGroups(groups []string, allowedGroups []string) []string {
	allowed := make(map[string]struct{}, len(allowedGroups))
	for _, group := range allowedGroups {
		allowed[group] = struct{}{}
	}
	result := make([]string, 0, len(groups))
	for _, group := range groups {
		if _, ok := allowed[group]; ok {
			result = append(result, group)
		}
	}
	return result
}

func resetAutoGroupRetryState(param *RetryParam) {
	param.SetRetry(0)
	common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, 0)
	common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
}

func PrepareRetryAfterChannelFailure(param *RetryParam, channel *model.Channel) {
	if param == nil || channel == nil {
		return
	}
	param.ExcludeChannel(channel.Id)
	if param.TokenGroup != "auto" || !common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry) {
		return
	}
	selectedGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyAutoGroup)
	if selectedGroup == "" {
		return
	}
	autoGroups := retryAutoGroups(param)
	for i, group := range autoGroups {
		if group != selectedGroup {
			continue
		}
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
		param.SetRetry(0)
		param.ResetRetryNextTry()
		return
	}
}

func retryAutoGroups(param *RetryParam) []string {
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
	autoGroups := GetRequestAutoGroup(param.Ctx, userGroup)
	if len(param.AllowedGroups) == 0 {
		return autoGroups
	}
	allowedGroups := filterAllowedProviderGroups(autoGroups, param.AllowedGroups)
	if len(allowedGroups) > 0 || !param.FallbackToAllGroups {
		return allowedGroups
	}
	return autoGroups
}

func selectCachedHealthyChannel(param *RetryParam, group string, retry int) (*model.Channel, error) {
	for {
		channel, err := model.GetRandomSatisfiedChannel(group, param.ModelName, retry, param.RequestPath, param.SelectionExcludedChannelIds())
		if err != nil || channel == nil {
			return channel, err
		}
		if !excludeCachedUnhealthyChannel(param, channel) {
			return channel, nil
		}
	}
}

func excludeCachedUnhealthyChannel(param *RetryParam, channel *model.Channel) bool {
	if !ShouldExcludeChannelByCachedHealth(channel) {
		return false
	}
	param.ExcludeFailedChannel(channel.Id)
	logger.LogDebug(param.Ctx, "channel #%d skipped because cached health probe failed", channel.Id)
	return true
}
