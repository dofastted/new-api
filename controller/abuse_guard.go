package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// GetRiskEvents 返回违规风控事件列表(管理端,分页 + 筛选)。
func GetRiskEvents(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userId, _ := strconv.Atoi(c.Query("user_id"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	events, total, err := model.GetRiskEvents(model.RiskEventQuery{
		UserId:         userId,
		Source:         c.Query("source"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		StartIdx:       pageInfo.GetStartIdx(),
		Num:            pageInfo.GetPageSize(),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(events)
	common.ApiSuccess(c, pageInfo)
}

// UnbanAbuseUser 解除用户的临时封禁;若用户已被自动永久封禁则恢复为启用。
func UnbanAbuseUser(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		common.ApiErrorMsg(c, "invalid user id")
		return
	}
	service.ClearTempBan(userId)
	if err := model.EnableAbuseBannedUser(userId); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// ResetAbuseScore 清零用户当前窗口的违规分数。
func ResetAbuseScore(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		common.ApiErrorMsg(c, "invalid user id")
		return
	}
	service.ResetAbuseScore(userId)
	common.ApiSuccess(c, nil)
}
