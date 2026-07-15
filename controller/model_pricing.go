package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func ListModelPricing(c *gin.Context) {
	pricing, err := service.ListModelPricing(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, pricing)
}

func SaveModelPricing(c *gin.Context) {
	var request service.ModelPricingBatchRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := service.SaveModelPricingBatch(c.Request.Context(), request)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func CalibrateModelPricing(c *gin.Context) {
	task, _, err := service.EnqueueSystemTask(model.SystemTaskTypeOfficialPricingSync, officialPricingSyncTaskPayload{Calibrate: true})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, task.ToResponse())
}
