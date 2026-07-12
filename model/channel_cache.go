package model

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

type cachedAbility struct {
	channelID int
	priority  int64
	weight    uint
}

var group2model2abilities map[string]map[string][]cachedAbility // enabled abilities
var channelsIDM map[int]*Channel                                // all channels include disabled
// channel2advancedCustomConfig caches parsed Advanced Custom (type 58) configs so
// path-aware selection avoids re-parsing JSON per request. Refreshed on full sync.
var channel2advancedCustomConfig map[int]*dto.AdvancedCustomConfig
var channelSyncLock sync.RWMutex

func InitChannelCache() {
	if !common.MemoryCacheEnabled {
		return
	}
	newChannelId2channel := make(map[int]*Channel)
	newChannel2advancedCustomConfig := make(map[int]*dto.AdvancedCustomConfig)
	var channels []*Channel
	DB.Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
		if channel.Type == constant.ChannelTypeAdvancedCustom {
			if config := channel.GetOtherSettings().AdvancedCustom; config != nil {
				newChannel2advancedCustomConfig[channel.Id] = config
			}
		}
	}
	var abilities []*Ability
	DB.Where("enabled = ?", true).Find(&abilities)
	newGroup2model2abilities := make(map[string]map[string][]cachedAbility)
	for _, ability := range abilities {
		channel, ok := newChannelId2channel[ability.ChannelId]
		if !ok || channel.Status != common.ChannelStatusEnabled {
			continue
		}
		model2abilities, ok := newGroup2model2abilities[ability.Group]
		if !ok {
			model2abilities = make(map[string][]cachedAbility)
			newGroup2model2abilities[ability.Group] = model2abilities
		}
		priority := int64(0)
		if ability.Priority != nil {
			priority = *ability.Priority
		}
		model2abilities[ability.Model] = append(model2abilities[ability.Model], cachedAbility{
			channelID: ability.ChannelId,
			priority:  priority,
			weight:    ability.Weight,
		})
	}

	for _, model2abilities := range newGroup2model2abilities {
		for model, abilities := range model2abilities {
			sort.SliceStable(abilities, func(i, j int) bool {
				if abilities[i].priority == abilities[j].priority {
					return abilities[i].channelID < abilities[j].channelID
				}
				return abilities[i].priority > abilities[j].priority
			})
			model2abilities[model] = abilities
		}
	}

	channelSyncLock.Lock()
	group2model2abilities = newGroup2model2abilities
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			channel.Keys = channel.GetKeys()
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channel2advancedCustomConfig = newChannel2advancedCustomConfig
	channelSyncLock.Unlock()
	common.SysLog("channels synced from database")
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

func GetRandomSatisfiedChannel(group string, model string, retry int, requestPath string, excluded map[int]struct{}) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannel(group, model, retry, requestPath, excluded)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	abilities := filterCachedAbilitiesByRequestPath(group, group2model2abilities[group][model], requestPath)
	if len(abilities) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		abilities = filterCachedAbilitiesByRequestPath(group, group2model2abilities[group][normalizedModel], requestPath)
	}
	if len(abilities) == 0 {
		return nil, nil
	}

	abilities = filterOpenCircuitCachedAbilities(abilities)
	if len(abilities) == 0 {
		return nil, nil
	}
	abilities = filterExcludedCachedAbilities(abilities, excluded)
	if len(abilities) == 0 {
		return nil, nil
	}

	if len(abilities) == 1 {
		if channel, ok := channelsIDM[abilities[0].channelID]; ok {
			return channel, nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", abilities[0].channelID)
	}

	uniquePriorities := make(map[int64]struct{})
	for _, ability := range abilities {
		if _, ok := channelsIDM[ability.channelID]; !ok {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", ability.channelID)
		}
		uniquePriorities[ability.priority] = struct{}{}
	}
	sortedUniquePriorities := make([]int64, 0, len(uniquePriorities))
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Slice(sortedUniquePriorities, func(i, j int) bool {
		return sortedUniquePriorities[i] > sortedUniquePriorities[j]
	})
	if retry >= len(sortedUniquePriorities) {
		retry = len(sortedUniquePriorities) - 1
	}
	targetPriority := sortedUniquePriorities[retry]

	sumWeight := 0
	targetCount := 0
	for _, ability := range abilities {
		if ability.priority == targetPriority {
			sumWeight += int(ability.weight)
			targetCount++
		}
	}
	if targetCount == 0 {
		return nil, fmt.Errorf("no channel found, group: %s, model: %s, priority: %d", group, model, targetPriority)
	}

	smoothingFactor := 1
	smoothingAdjustment := 0
	if sumWeight == 0 {
		sumWeight = targetCount * 100
		smoothingAdjustment = 100
	} else if sumWeight/targetCount < 10 {
		smoothingFactor = 100
	}
	randomWeight := rand.Intn(sumWeight * smoothingFactor)
	for _, ability := range abilities {
		if ability.priority != targetPriority {
			continue
		}
		randomWeight -= int(ability.weight)*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channelsIDM[ability.channelID], nil
		}
	}
	return nil, errors.New("channel not found")
}

// filterCachedAbilitiesByRequestPath restricts candidates by request path. Only Advanced
// Custom (type 58) channels are path-checked: they are kept only when one of their
// configured routes matches requestPath. All other channel types always pass.
// When requestPath is empty (non-relay callers) filtering is skipped.
// Caller must hold channelSyncLock (read lock). The cached slice is never mutated.
func filterCachedAbilitiesByRequestPath(group string, abilities []cachedAbility, requestPath string) []cachedAbility {
	if requestPath == "" || len(abilities) == 0 {
		return abilities
	}
	filtered := make([]cachedAbility, 0, len(abilities))
	for _, ability := range abilities {
		channel, ok := channelsIDM[ability.channelID]
		if !ok {
			filtered = append(filtered, ability)
			continue
		}
		if !ProviderGroupChannelSupportsPath(group, ability.channelID, requestPath) {
			continue
		}
		if channel.Type != constant.ChannelTypeAdvancedCustom {
			filtered = append(filtered, ability)
			continue
		}
		if config := channel2advancedCustomConfig[ability.channelID]; config != nil && config.SupportsPath(requestPath) {
			filtered = append(filtered, ability)
		}
	}
	return filtered
}

func filterOpenCircuitCachedAbilities(abilities []cachedAbility) []cachedAbility {
	if len(abilities) == 0 {
		return abilities
	}
	filtered := make([]cachedAbility, 0, len(abilities))
	for _, ability := range abilities {
		if !IsChannelCircuitOpen(ability.channelID) {
			filtered = append(filtered, ability)
		}
	}
	return filtered
}

func filterExcludedCachedAbilities(abilities []cachedAbility, excluded map[int]struct{}) []cachedAbility {
	if len(abilities) == 0 || len(excluded) == 0 {
		return abilities
	}
	filtered := make([]cachedAbility, 0, len(abilities))
	for _, ability := range abilities {
		if _, ok := excluded[ability.channelID]; !ok {
			filtered = append(filtered, ability)
		}
	}
	return filtered
}

func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return c, nil
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return &c.ChannelInfo, nil
}

func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel, ok := channelsIDM[id]; ok {
		channel.Status = status
	}
	if status != common.ChannelStatusEnabled {
		for group, model2abilities := range group2model2abilities {
			for model, abilities := range model2abilities {
				for i, ability := range abilities {
					if ability.channelID == id {
						group2model2abilities[group][model] = append(abilities[:i], abilities[i+1:]...)
						break
					}
				}
			}
		}
	}
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel == nil {
		return
	}

	if channelsIDM == nil {
		channelsIDM = make(map[int]*Channel)
	}
	if oldChannel, ok := channelsIDM[channel.Id]; ok {
		logger.LogDebug(nil, "CacheUpdateChannel before: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, oldChannel.ChannelInfo.MultiKeyPollingIndex)
	}
	channelsIDM[channel.Id] = channel
	logger.LogDebug(nil, "CacheUpdateChannel after: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, channel.ChannelInfo.MultiKeyPollingIndex)
}
