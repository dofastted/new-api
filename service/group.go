package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func GetUserUsableGroups(userGroup string) map[string]string {
	groupsCopy := setting.GetUserUsableGroupsCopy()
	if userGroup != "" {
		specialSettings, b := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup)
		if b {
			// 处理特殊可用分组
			for specialGroup, desc := range specialSettings {
				if strings.HasPrefix(specialGroup, "-:") {
					// 移除分组
					groupToRemove := strings.TrimPrefix(specialGroup, "-:")
					delete(groupsCopy, groupToRemove)
				} else if strings.HasPrefix(specialGroup, "+:") {
					// 添加分组
					groupToAdd := strings.TrimPrefix(specialGroup, "+:")
					groupsCopy[groupToAdd] = desc
				} else {
					// 直接添加分组
					groupsCopy[specialGroup] = desc
				}
			}
		}
		// 如果userGroup不在UserUsableGroups中，返回UserUsableGroups + userGroup
		if _, ok := groupsCopy[userGroup]; !ok {
			groupsCopy[userGroup] = "用户分组"
		}
	}
	return groupsCopy
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	_, ok := GetUserUsableGroups(userGroup)[groupName]
	return ok
}

// GetUserAutoGroup returns the admin-managed auto provider-group order. The
// userGroup argument is kept for compatibility; provider group access is not
// user-tier scoped in the redesigned contract.
func GetUserAutoGroup(userGroup string) []string {
	groups, err := model.GetProviderAutoGroups(model.ProviderRouteTypeOther)
	if err == nil && len(groups) > 0 {
		return filterOnlineProviderGroups(groups)
	}
	return filterOnlineProviderGroups(setting.GetAutoGroups())
}

func GetRequestAutoGroup(c *gin.Context, userGroup string) []string {
	autoGroups := GetUserAutoGroup(userGroup)
	if c != nil && c.Request != nil && c.Request.URL != nil {
		groups, err := model.GetProviderAutoGroups(model.ProviderRouteTypeForPath(c.Request.URL.Path))
		if err == nil && len(groups) > 0 {
			autoGroups = filterOnlineProviderGroups(groups)
		}
	}
	routeGroups := common.GetContextKeyStringSlice(c, constant.ContextKeyRouteAutoGroups)
	if len(routeGroups) == 0 {
		return autoGroups
	}

	filteredGroups := make([]string, 0, len(routeGroups))
	for _, group := range autoGroups {
		if containsGroup(routeGroups, group) {
			filteredGroups = append(filteredGroups, group)
		}
	}
	return filteredGroups
}
func filterOnlineProviderGroups(groups []string) []string {
	filtered := make([]string, 0, len(groups))
	for _, group := range groups {
		online, err := model.IsProviderGroupOnline(group)
		if err == nil && online {
			filtered = append(filtered, group)
		}
	}
	return filtered
}

func containsGroup(groups []string, group string) bool {
	for _, item := range groups {
		if item == group {
			return true
		}
	}
	return false
}

// GetUserGroupRatio 获取用户使用某个分组的倍率
// userGroup 用户分组
// group 需要获取倍率的分组
func GetUserGroupRatio(userGroup, group string) float64 {
	ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, group)
	if ok {
		return ratio
	}
	return ratio_setting.GetGroupRatio(group)
}
