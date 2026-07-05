package service

import (
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCacheGetRandomSatisfiedChannelSkipsCachedFailedAnthropic(t *testing.T) {
	db := setupChannelHealthRoutingTestDB(t)
	failed := seedHealthRoutingChannel(t, db, 41001, constant.ChannelTypeAnthropic, 100)
	available := seedHealthRoutingChannel(t, db, 41002, constant.ChannelTypeAnthropic, 0)
	model.InitChannelCache()
	SetCachedChannelTestResult(ChannelTestCacheKey(failed.Id, "", "", ChannelTestUsesStream(failed)), ChannelTestCachedResult{
		Success:  false,
		Message:  "status_code=502, Upstream access forbidden, please contact administrator",
		TestedAt: 123,
	})

	param := &RetryParam{
		Ctx:        newHealthRoutingContext(),
		TokenGroup: "claude-sub",
		ModelName:  "claude-sonnet-4-6",
		Retry:      common.GetPointer(0),
	}
	channel, group, err := CacheGetRandomSatisfiedChannel(param)

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, available.Id, channel.Id)
	assert.Equal(t, "claude-sub", group)
	assert.Contains(t, param.ExcludedChannelIds, failed.Id)
}

func TestCacheGetRandomSatisfiedChannelSkipsOpenCircuitClaudeMaxFallback(t *testing.T) {
	db := setupChannelHealthRoutingTestDB(t)
	primary := seedHealthRoutingChannelForGroup(t, db, 34, constant.ChannelTypeAnthropic, "claude-max", "claude-sonnet-4-6", 100)
	fallback := seedHealthRoutingChannelForGroup(t, db, 24, constant.ChannelTypeAnthropic, "claude-max", "claude-sonnet-4-6", 0)
	model.InitChannelCache()
	RecordChannelFailure(primary.Id, ChannelCircuitClassHighLoadTemporarilyUnavailable, model.ChannelCircuitPolicy{
		FailureThreshold: 1,
		OpenSeconds:      300,
	})
	t.Cleanup(func() {
		ResetChannelCircuit(primary.Id)
		ResetChannelCircuit(fallback.Id)
	})
	require.Equal(t, model.ChannelCircuitOpen, GetChannelCircuitStatus(primary.Id).State)

	ctx := newHealthRoutingContext()
	ctx.Request = httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}]}`))
	ctx.Request.Header.Set("User-Agent", "claude-code/1.0")
	channel, group, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        ctx,
		TokenGroup: "claude-max",
		ModelName:  "claude-sonnet-4-6",
		Retry:      common.GetPointer(0),
	})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, fallback.Id, channel.Id)
	assert.NotEqual(t, primary.Id, channel.Id)
	assert.Equal(t, "claude-max", group)
}

func TestCacheGetRandomSatisfiedChannelKeepsCachedHealthyAnthropic(t *testing.T) {
	db := setupChannelHealthRoutingTestDB(t)
	healthy := seedHealthRoutingChannel(t, db, 41003, constant.ChannelTypeAnthropic, 100)
	seedHealthRoutingChannel(t, db, 41004, constant.ChannelTypeAnthropic, 0)
	model.InitChannelCache()
	SetCachedChannelTestResult(ChannelTestCacheKey(healthy.Id, "", "", ChannelTestUsesStream(healthy)), ChannelTestCachedResult{
		Success:  true,
		TestedAt: 456,
	})

	channel, _, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        newHealthRoutingContext(),
		TokenGroup: "claude-sub",
		ModelName:  "claude-sonnet-4-6",
		Retry:      common.GetPointer(0),
	})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, healthy.Id, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelKeepsUncachedAnthropic(t *testing.T) {
	db := setupChannelHealthRoutingTestDB(t)
	uncached := seedHealthRoutingChannel(t, db, 41005, constant.ChannelTypeAnthropic, 100)
	seedHealthRoutingChannel(t, db, 41006, constant.ChannelTypeAnthropic, 0)
	model.InitChannelCache()

	channel, _, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        newHealthRoutingContext(),
		TokenGroup: "claude-sub",
		ModelName:  "claude-sonnet-4-6",
		Retry:      common.GetPointer(0),
	})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, uncached.Id, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelDoesNotApplyClaudeCacheToOtherProviders(t *testing.T) {
	db := setupChannelHealthRoutingTestDB(t)
	openAI := seedHealthRoutingChannel(t, db, 41007, constant.ChannelTypeOpenAI, 100)
	seedHealthRoutingChannel(t, db, 41008, constant.ChannelTypeOpenAI, 0)
	model.InitChannelCache()
	SetCachedChannelTestResult(ChannelTestCacheKey(openAI.Id, "", "", ChannelTestUsesStream(openAI)), ChannelTestCachedResult{
		Success:  false,
		Message:  "cached probe failed",
		TestedAt: 789,
	})

	channel, _, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        newHealthRoutingContext(),
		TokenGroup: "claude-sub",
		ModelName:  "claude-sonnet-4-6",
		Retry:      common.GetPointer(0),
	})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, openAI.Id, channel.Id)
}

func setupChannelHealthRoutingTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldRedisEnabled := common.RedisEnabled
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldIsMasterNode := common.IsMasterNode
	oldSQLitePath := common.SQLitePath
	oldMainDatabaseType := common.MainDatabaseType()
	oldLogDatabaseType := common.LogDatabaseType()
	oldSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")

	common.IsMasterNode = false
	common.SQLitePath = fmt.Sprintf("file:%s_init?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, os.Setenv("SQL_DSN", "local"))
	require.NoError(t, model.InitDB())
	if model.DB != nil {
		sqlDB, err := model.DB.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = true
	PurgeChannelTestResultCache()
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))

	t.Cleanup(func() {
		PurgeChannelTestResultCache()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.RedisEnabled = oldRedisEnabled
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		common.IsMasterNode = oldIsMasterNode
		common.SQLitePath = oldSQLitePath
		common.SetDatabaseTypes(oldMainDatabaseType, oldLogDatabaseType)
		if hadSQLDSN {
			_ = os.Setenv("SQL_DSN", oldSQLDSN)
		} else {
			_ = os.Unsetenv("SQL_DSN")
		}
	})

	return db
}

func seedHealthRoutingChannel(t *testing.T, db *gorm.DB, id int, channelType int, priorityValue int64) *model.Channel {
	t.Helper()
	return seedHealthRoutingChannelForGroup(t, db, id, channelType, "claude-sub", "claude-sonnet-4-6", priorityValue)
}

func seedHealthRoutingChannelForGroup(t *testing.T, db *gorm.DB, id int, channelType int, group string, modelName string, priorityValue int64) *model.Channel {
	t.Helper()
	weight := uint(1)
	channel := &model.Channel{
		Id:       id,
		Type:     channelType,
		Key:      "sk-test",
		Name:     fmt.Sprintf("health-routing-%d", id),
		Models:   modelName,
		Status:   common.ChannelStatusEnabled,
		Priority: &priorityValue,
		Weight:   &weight,
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     group,
		Model:     modelName,
		ChannelId: id,
		Enabled:   true,
		Priority:  &priorityValue,
		Weight:    weight,
	}).Error)
	return channel
}

func newHealthRoutingContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	return ctx
}
