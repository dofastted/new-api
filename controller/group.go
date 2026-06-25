package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func GetGroups(c *gin.Context) {
	groups, err := model.ListOnlineProviderGroupOptions()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	groupNames := make([]string, 0, len(groups))
	for _, group := range groups {
		groupNames = append(groupNames, group.Name)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

func GetUserGroups(c *gin.Context) {
	groups, err := model.ListOnlineProviderGroupOptions()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	usableGroups := make(map[string]map[string]interface{}, len(groups))
	for _, group := range groups {
		ratio := interface{}(group.UsageRatio)
		if group.IsAuto {
			ratio = "自动"
		}
		usableGroups[group.Name] = map[string]interface{}{
			"ratio": ratio,
			"desc":  group.Description,
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usableGroups,
	})
}
