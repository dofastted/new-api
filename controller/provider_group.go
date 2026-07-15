package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type providerGroupChannelsRequest struct {
	Items []model.ProviderGroupChannel `json:"items"`
}

type providerGroupAutoRulesRequest struct {
	Items []model.ProviderGroupAutoRule `json:"items"`
}

func GetProviderGroups(c *gin.Context) {
	var groups []model.ProviderGroup
	if err := model.DB.
		Where("name NOT IN ?", model.ReservedUserProviderGroupNames()).
		Order("sort_order ASC, id ASC").
		Find(&groups).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, groups)
}

func CreateProviderGroup(c *gin.Context) {
	var group model.ProviderGroup
	if err := c.ShouldBindJSON(&group); err != nil {
		common.ApiError(c, err)
		return
	}
	if group.Name == "" {
		common.ApiErrorMsg(c, "分组名称不能为空")
		return
	}
	if model.IsReservedUserProviderGroupName(group.Name) {
		common.ApiErrorMsg(c, "用户等级分组不能作为 provider 分组")
		return
	}
	if group.DisplayName == "" {
		group.DisplayName = group.Name
	}
	if group.UsageRatio == 0 {
		group.UsageRatio = 1
	}
	if group.Status == 0 {
		group.Status = model.ProviderGroupStatusEnabled
	}
	group.CreatedTime = common.GetTimestamp()
	group.UpdatedTime = group.CreatedTime
	if err := model.DB.Create(&group).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, group)
}

func UpdateProviderGroup(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效分组 ID")
		return
	}
	var input model.ProviderGroup
	if err := c.ShouldBindJSON(&input); err != nil {
		common.ApiError(c, err)
		return
	}
	var group model.ProviderGroup
	if err := model.DB.First(&group, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	updates := map[string]interface{}{
		"display_name": input.DisplayName,
		"description":  input.Description,
		"status":       input.Status,
		"usage_ratio":  input.UsageRatio,
		"is_auto":      input.IsAuto,
		"sort_order":   input.SortOrder,
		"updated_time": common.GetTimestamp(),
	}
	if input.UsageRatio == 0 {
		updates["usage_ratio"] = 1
	}
	if err := model.DB.Model(&group).Updates(updates).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	// Enabled status is the groups-page authoritative control over routing:
	// flipping a group offline must remove its abilities immediately.
	if input.Status != group.Status {
		if err := model.RebuildAbilitiesFromProviderGroups(); err != nil {
			common.ApiError(c, err)
			return
		}
		model.InitChannelCache()
	}
	common.ApiSuccess(c, group)
}

func DeleteProviderGroup(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效分组 ID")
		return
	}
	var group model.ProviderGroup
	if err := model.DB.First(&group, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	// Auto is a system routing entity; deleting it would orphan auto rules.
	if group.IsAuto || group.Name == "auto" {
		common.ApiErrorMsg(c, "Auto 分组不可删除")
		return
	}
	var affectedChannelIDs []int
	if err := model.DB.Model(&model.ProviderGroupChannel{}).
		Where("provider_group_id = ?", id).
		Pluck("channel_id", &affectedChannelIDs).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Delete(&model.ProviderGroup{}, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.RebuildAbilitiesFromProviderGroups(); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	common.ApiSuccess(c, nil)
}

func GetProviderGroupChannels(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效分组 ID")
		return
	}
	items, err := model.ListProviderGroupChannelDetails(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, items)
}

func UpdateProviderGroupChannels(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效分组 ID")
		return
	}
	var req providerGroupChannelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	// Keep the legacy members-only endpoint as a thin wrapper over the
	// transactional configuration save so partial-update semantics stay shared.
	result, err := model.ApplyProviderGroupConfiguration(id, model.ProviderGroupConfigurationUpdate{
		Members: &req.Items,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result.Members)
}

// UpdateProviderGroupConfiguration applies metadata and/or membership changes
// as a single transactional operation used by the unified groups-page save.
func UpdateProviderGroupConfiguration(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效分组 ID")
		return
	}
	var req model.ProviderGroupConfigurationUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := model.ApplyProviderGroupConfiguration(id, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func GetProviderGroupAutoRules(c *gin.Context) {
	var items []model.ProviderGroupAutoRule
	if err := model.DB.Order("route_type ASC, sort_order ASC, id ASC").Find(&items).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, items)
}

func UpdateProviderGroupAutoRules(c *gin.Context) {
	var req providerGroupAutoRulesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	now := common.GetTimestamp()
	for i := range req.Items {
		req.Items[i].UpdatedTime = now
		if req.Items[i].CreatedTime == 0 {
			req.Items[i].CreatedTime = now
		}
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&model.ProviderGroupAutoRule{}).Error; err != nil {
			return err
		}
		if len(req.Items) == 0 {
			return nil
		}
		return tx.Create(&req.Items).Error
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, req.Items)
}
