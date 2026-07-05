package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettleTestQuotaUsesTieredBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:   "tiered_expr",
			ExprString:    `param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`,
			ExprHash:      billingexpr.ExprHashString(`param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`),
			GroupRatio:    1,
			EstimatedTier: "stream",
			QuotaPerUnit:  common.QuotaPerUnit,
			ExprVersion:   1,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}

	quota, result := settleTestQuota(info, types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 2,
	}, &dto.Usage{
		PromptTokens: 1000,
	})

	require.Equal(t, 1500, quota)
	require.NotNil(t, result)
	require.Equal(t, "stream", result.MatchedTier)
}

func TestBuildTestLogOtherInjectsTieredInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
			ExprString:  `tier("base", p * 2)`,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12,
		},
	}

	other := buildTestLogOther(ctx, info, priceData, usage, &billingexpr.TieredResult{
		MatchedTier: "base",
	})

	require.Equal(t, "tiered_expr", other["billing_mode"])
	require.Equal(t, "base", other["matched_tier"])
	require.NotEmpty(t, other["expr_b64"])
}

func TestResolveChannelTestUserIDUsesRequestUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("id", 2)

	userID, err := resolveChannelTestUserID(ctx)

	require.NoError(t, err)
	require.Equal(t, 2, userID)
}

func TestSelectChannelsForAutomaticTestPassiveRecoveryOnlyUsesAutoDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModePassiveRecovery)

	require.Len(t, selected, 1)
	require.Equal(t, 2, selected[0].Id)
}

func TestSelectChannelsForAutomaticTestScheduledSkipsManualDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeScheduledAll)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 2, selected[1].Id)
}

func TestTestAllChannelsRejectsExistingActiveTask(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))

	existing, err := model.CreateSystemTask(model.SystemTaskTypeChannelTest, nil, nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/test", nil)

	TestAllChannels(ctx)

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), existing.TaskID)
	require.Contains(t, recorder.Body.String(), "已有通道测试任务正在运行或等待中")
}

func resetChannelTestResultCacheForTest(t *testing.T) {
	t.Helper()
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	service.PurgeChannelTestResultCache()
	t.Cleanup(func() {
		service.PurgeChannelTestResultCache()
		common.RedisEnabled = oldRedisEnabled
	})
}

func TestChannelReturnsCachedClaudeChannelTest(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	resetChannelTestResultCacheForTest(t)
	require.NoError(t, db.Create(&model.Channel{
		Id:     101,
		Type:   constant.ChannelTypeAnthropic,
		Name:   "claude-cached",
		Key:    "sk-test",
		Models: "claude-sonnet-4-6",
		Status: common.ChannelStatusEnabled,
	}).Error)

	setCachedChannelTestResult(channelTestCacheKey(101, "", "", false), channelTestCachedResult{
		Success:  true,
		Message:  "",
		Time:     1.25,
		TestedAt: 12345,
	})
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "101"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/test/101", nil)

	TestChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, true, body["success"])
	require.Equal(t, true, body["cached"])
	require.Equal(t, float64(12345), body["tested_at"])
}

func TestChannelReturnsScheduledCachedClaudeChannelTestForSpecificModel(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	resetChannelTestResultCacheForTest(t)
	require.NoError(t, db.Create(&model.Channel{
		Id:     103,
		Type:   constant.ChannelTypeAnthropic,
		Name:   "claude-specific-model-cached",
		Key:    "sk-test",
		Models: "claude-sonnet-4-6",
		Status: common.ChannelStatusEnabled,
	}).Error)

	setCachedChannelTestResult(channelTestCacheKey(103, "", "", false), channelTestCachedResult{
		Success:  true,
		Message:  "",
		Time:     1.5,
		TestedAt: 23456,
	})
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "103"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/test/103?model=claude-sonnet-4-6", nil)

	TestChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, true, body["success"])
	require.Equal(t, true, body["cached"])
	require.Equal(t, float64(23456), body["tested_at"])
}

func TestChannelBlocksUncachedClaudeChannelTest(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	resetChannelTestResultCacheForTest(t)
	require.NoError(t, db.Create(&model.Channel{
		Id:     102,
		Type:   constant.ChannelTypeAnthropic,
		Name:   "claude-blocked",
		Key:    "sk-test",
		Models: "claude-sonnet-4-6",
		Status: common.ChannelStatusEnabled,
	}).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "102"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/test/102", nil)

	TestChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, false, body["success"])
	require.Equal(t, false, body["cached"])
	require.Contains(t, body["message"], "Claude channel health probe is disabled")
}
