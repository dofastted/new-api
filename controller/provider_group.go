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
	if err := model.DB.Order("sort_order ASC, id ASC").Find(&groups).Error; err != nil {
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
	common.ApiSuccess(c, group)
}

func DeleteProviderGroup(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效分组 ID")
		return
	}
	if err := model.DB.Delete(&model.ProviderGroup{}, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func GetProviderGroupChannels(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效分组 ID")
		return
	}
	var items []model.ProviderGroupChannel
	if err := model.DB.Where("provider_group_id = ?", id).Order("sort_order ASC, id ASC").Find(&items).Error; err != nil {
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
	var group model.ProviderGroup
	if err := model.DB.First(&group, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	now := common.GetTimestamp()
	for i := range req.Items {
		req.Items[i].ProviderGroupId = id
		req.Items[i].GroupName = group.Name
		req.Items[i].UpdatedTime = now
		if req.Items[i].CreatedTime == 0 {
			req.Items[i].CreatedTime = now
		}
	}
	if err := replaceProviderGroupChannels(id, req.Items); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.RebuildAbilitiesFromProviderGroups(); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	common.ApiSuccess(c, req.Items)
}

func replaceProviderGroupChannels(groupID int, items []model.ProviderGroupChannel) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("provider_group_id = ?", groupID).Delete(&model.ProviderGroupChannel{}).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		return tx.Create(&items).Error
	})
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
