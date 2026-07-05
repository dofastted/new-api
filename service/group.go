package service

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	ClaudeCodeFamilyRequiredMessage = "This model requires the official Claude Code CLI. Please use Claude Code and retry."
	CodexFamilyRequiredMessage      = "This model requires the official Codex CLI. Please use Codex CLI and retry."
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

// GetRequestAutoModelGroups returns the provider groups whose models should be
// visible to an auto token for the current request. Model listing is route-neutral,
// so provider auto candidates from every route type are authoritative here, but
// official-client family gates still apply.
func GetRequestAutoModelGroups(c *gin.Context, userGroup string) []string {
	groups, err := model.GetProviderAutoModelGroups()
	if err == nil && len(groups) > 0 {
		groups = filterOnlineProviderGroups(groups)
		return filterAutoGroupsByRequestFamily(c, groups)
	}
	return GetRequestAutoGroup(c, userGroup)
}

func GetRequestAutoGroup(c *gin.Context, userGroup string) []string {
	autoGroups := requestAutoGroupCandidates(c, userGroup)
	autoGroups = filterAutoGroupsByRequestFamily(c, autoGroups)
	return filterRouteAutoGroups(c, autoGroups)
}

func requestAutoGroupCandidates(c *gin.Context, userGroup string) []string {
	autoGroups := GetUserAutoGroup(userGroup)
	if c != nil && c.Request != nil && c.Request.URL != nil {
		groups, err := model.GetProviderAutoGroups(model.ProviderRouteTypeForPath(c.Request.URL.Path))
		if err == nil && len(groups) > 0 {
			autoGroups = filterOnlineProviderGroups(groups)
		}
	}
	return autoGroups
}

func filterRouteAutoGroups(c *gin.Context, autoGroups []string) []string {
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

func RestrictedAutoGroupAccessError(c *gin.Context, userGroup string) *types.NewAPIError {
	autoGroups := filterRouteAutoGroups(c, requestAutoGroupCandidates(c, userGroup))
	if len(autoGroups) == 0 {
		return nil
	}
	var firstErr *types.NewAPIError
	for _, group := range autoGroups {
		accessErr := ProviderGroupAccessError(c, group)
		if accessErr == nil {
			return nil
		}
		if firstErr == nil {
			firstErr = accessErr
		}
	}
	return firstErr
}

func filterAutoGroupsByRequestFamily(c *gin.Context, groups []string) []string {
	if c == nil || len(groups) == 0 {
		return groups
	}
	filtered := make([]string, 0, len(groups))
	claudeCodeFamily := false
	claudeCodeFamilyChecked := false
	codexFamily := false
	codexFamilyChecked := false
	for _, group := range groups {
		normalized := strings.ToLower(strings.TrimSpace(group))
		if strings.HasPrefix(normalized, "claude-max") {
			if !claudeCodeFamilyChecked {
				claudeCodeFamily = isClaudeCodeFamilyRequest(c)
				claudeCodeFamilyChecked = true
			}
			if !claudeCodeFamily {
				continue
			}
		}
		if normalized == "codex-pro" || strings.HasPrefix(normalized, "codex-pro-") {
			if !codexFamilyChecked {
				codexFamily = isCodexFamilyRequest(c)
				codexFamilyChecked = true
			}
			if !codexFamily {
				continue
			}
		}
		filtered = append(filtered, group)
	}
	return filtered
}

func ProviderGroupAccessError(c *gin.Context, group string) *types.NewAPIError {
	normalized := strings.ToLower(strings.TrimSpace(group))
	switch {
	case strings.HasPrefix(normalized, "claude-max"):
		if isClaudeCodeFamilyRequest(c) {
			return nil
		}
		return providerFamilyAccessError(ClaudeCodeFamilyRequiredMessage)
	case normalized == "codex-pro" || strings.HasPrefix(normalized, "codex-pro-"):
		if isCodexFamilyRequest(c) {
			return nil
		}
		return providerFamilyAccessError(CodexFamilyRequiredMessage)
	default:
		return nil
	}
}

func providerFamilyAccessError(message string) *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		errors.New(message),
		types.ErrorCodeAccessDenied,
		http.StatusForbidden,
		types.ErrOptionWithSkipRetry(),
		types.ErrOptionWithNoRecordErrorLog(),
	)
}

func IsProviderFamilyAccessDeniedError(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	if types.IsSkipRetryError(err) && err.GetErrorCode() == types.ErrorCodeAccessDenied {
		return true
	}
	message := strings.ToLower(err.ErrorWithStatusCode())
	return strings.Contains(message, "official claude cli") ||
		strings.Contains(message, "official claude code cli") ||
		strings.Contains(message, "official codex cli")
}

func isClaudeCodeFamilyRequest(c *gin.Context) bool {
	if requestHeadersContainAny(c, []string{"claude-code", "claude_code", "claude-cli", "claude_cli", "claude code"}) {
		return true
	}
	return requestBodyHasClaudeCodeTools(c)
}

func isCodexFamilyRequest(c *gin.Context) bool {
	if requestHeadersContainAny(c, []string{"codex"}) {
		return true
	}
	body := requestJSONBody(c)
	return len(body) > 0 && gjson.GetBytes(body, "prompt_cache_key").Exists()
}

func requestHeadersContainAny(c *gin.Context, markers []string) bool {
	if c == nil || c.Request == nil {
		return false
	}
	for _, values := range c.Request.Header {
		for _, value := range values {
			lowerValue := strings.ToLower(value)
			for _, marker := range markers {
				if strings.Contains(lowerValue, marker) {
					return true
				}
			}
		}
	}
	return false
}

func requestBodyHasClaudeCodeTools(c *gin.Context) bool {
	body := requestJSONBody(c)
	if len(body) == 0 {
		return false
	}
	toolNames := gjson.GetBytes(body, "tools.#.name").Array()
	if len(toolNames) == 0 {
		return false
	}
	seen := 0
	for _, toolName := range toolNames {
		switch strings.ToLower(toolName.String()) {
		case "bash", "read", "edit", "write", "glob", "grep", "ls", "todowrite", "multiedit":
			seen++
		}
		if seen >= 2 {
			return true
		}
	}
	return false
}

func requestJSONBody(c *gin.Context) []byte {
	if c == nil || c.Request == nil || !strings.HasPrefix(c.Request.Header.Get("Content-Type"), "application/json") {
		return nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil
	}
	body, err := storage.Bytes()
	if _, seekErr := storage.Seek(0, io.SeekStart); seekErr == nil {
		c.Request.Body = io.NopCloser(storage)
	}
	if err != nil {
		return nil
	}
	return body
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
