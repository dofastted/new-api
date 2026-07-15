package service

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func initRealtimeQuotaColumnNames(t *testing.T) {
	t.Helper()

	originalDB := model.DB
	originalIsMasterNode := common.IsMasterNode
	originalSQLitePath := common.SQLitePath
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")
	defer func() {
		if model.DB != nil && model.DB != originalDB {
			sqlDB, err := model.DB.DB()
			if err == nil {
				_ = sqlDB.Close()
			}
		}
		model.DB = originalDB
		common.IsMasterNode = originalIsMasterNode
		common.SQLitePath = originalSQLitePath
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	}()

	common.IsMasterNode = false
	common.SQLitePath = fmt.Sprintf("file:%s_init?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, os.Setenv("SQL_DSN", "local"))
	require.NoError(t, model.InitDB())
}

func setupRealtimeQuotaProviderGroupTest(t *testing.T) *gorm.DB {
	t.Helper()
	initRealtimeQuotaColumnNames(t)

	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
	})

	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.ProviderGroup{}))
	return db
}

func TestPreWssConsumeQuotaUsesProviderGroupRatioBeforeLegacyRatios(t *testing.T) {
	db := setupRealtimeQuotaProviderGroupTest(t)

	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	oldGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(oldGroupGroupRatio))
		ratio_setting.ReplaceOfficialPricing(nil, false)
	})
	ratio_setting.ReplaceOfficialPricing(nil, false)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"realtime-authority-model":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"realtime-provider":9,"default":1}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"default":{"realtime-provider":9}}`))

	require.NoError(t, db.Create(&model.ProviderGroup{
		Name:        "realtime-provider",
		DisplayName: "realtime-provider",
		Status:      model.ProviderGroupStatusEnabled,
		UsageRatio:  0.25,
	}).Error)
	require.NoError(t, db.Create(&model.User{
		Id:    99101,
		Quota: 300,
		Group: "default",
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		Id:          99102,
		UserId:      99101,
		Key:         "realtime-quota-test",
		RemainQuota: 300,
		Group:       "auto",
	}).Error)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyAutoGroup, "realtime-provider")
	relayInfo := &relaycommon.RelayInfo{
		UserId:          99101,
		TokenKey:        "sk-realtime-quota-test",
		UserGroup:       "default",
		UsingGroup:      "auto",
		OriginModelName: "realtime-authority-model",
	}
	usage := &dto.RealtimeUsage{
		InputTokenDetails: dto.InputTokenDetails{TextTokens: 1000},
	}

	require.NoError(t, PreWssConsumeQuota(ctx, relayInfo, usage))
	require.Equal(t, "realtime-provider", relayInfo.UsingGroup)
}
