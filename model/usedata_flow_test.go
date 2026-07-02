package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func seedFlowLog(t *testing.T, log Log) {
	t.Helper()
	require.NoError(t, LOG_DB.Create(&log).Error)
}

func seedFlowLookupData(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.Create(&Channel{Id: 1, Name: "east"}).Error)
	require.NoError(t, DB.Create(&Channel{Id: 2, Name: "west"}).Error)
	require.NoError(t, DB.Create(&Token{Id: 11, UserId: 1, Key: "sk-primary", Name: "primary"}).Error)
	require.NoError(t, DB.Create(&Token{Id: 22, UserId: 2, Key: "sk-backup", Name: "backup"}).Error)
	require.NoError(t, DB.Delete(&Token{Id: 11}).Error)
}

func TestGetFlowQuotaDataUsesLogBackedRoleSpecificDimensions(t *testing.T) {
	truncateTables(t)
	seedFlowLookupData(t)

	seedFlowLog(t, Log{
		UserId:           1,
		Username:         "alice",
		TokenId:          11,
		Group:            "vip",
		ModelName:        "gpt-a",
		ChannelId:        1,
		CreatedAt:        1000,
		Type:             LogTypeConsume,
		Quota:            100,
		PromptTokens:     30,
		CompletionTokens: 10,
	})
	seedFlowLog(t, Log{
		UserId:           1,
		Username:         "alice",
		TokenId:          11,
		Group:            "vip",
		ModelName:        "gpt-a",
		ChannelId:        1,
		CreatedAt:        1100,
		Type:             LogTypeConsume,
		Quota:            50,
		PromptTokens:     15,
		CompletionTokens: 5,
	})
	seedFlowLog(t, Log{
		UserId:           1,
		Username:         "alice",
		TokenId:          11,
		Group:            "vip",
		ModelName:        "gpt-a",
		ChannelId:        2,
		CreatedAt:        1200,
		Type:             LogTypeConsume,
		Quota:            25,
		PromptTokens:     7,
		CompletionTokens: 3,
	})
	seedFlowLog(t, Log{
		UserId:           2,
		Username:         "bob",
		TokenId:          22,
		Group:            "default",
		ModelName:        "gpt-b",
		ChannelId:        1,
		CreatedAt:        1300,
		Type:             LogTypeConsume,
		Quota:            70,
		PromptTokens:     20,
		CompletionTokens: 10,
	})
	seedFlowLog(t, Log{
		UserId:           1,
		Username:         "alice",
		TokenId:          11,
		Group:            "vip",
		ModelName:        "ignored-topup",
		ChannelId:        1,
		CreatedAt:        1400,
		Type:             LogTypeTopup,
		Quota:            999,
		PromptTokens:     999,
		CompletionTokens: 999,
	})
	require.NoError(t, DB.Create(&QuotaData{
		UserID:    1,
		Username:  "alice",
		NodeName:  "stale-node",
		TokenID:   11,
		UseGroup:  "vip",
		ModelName: "stale-quota-data",
		ChannelID: 1,
		CreatedAt: 1000,
		Count:     99,
		Quota:     999,
		TokenUsed: 999,
	}).Error)
	originalNodeName := common.NodeName
	common.NodeName = "node-current"
	t.Cleanup(func() { common.NodeName = originalNodeName })

	rootRows, err := GetFlowQuotaData(900, 2000, "", 0, common.RoleRootUser)
	require.NoError(t, err)
	require.Len(t, rootRows, 3)
	// Token 11 was soft-deleted, so its name is intentionally left empty for the
	// frontend to render a localized "deleted (id)" label instead.
	require.Equal(t, FlowQuotaData{
		UserID:      1,
		Username:    "alice",
		NodeName:    "node-current",
		TokenID:     11,
		TokenName:   "",
		UseGroup:    "vip",
		ChannelID:   1,
		ChannelName: "east",
		ModelName:   "gpt-a",
		TokenUsed:   60,
		Count:       2,
		Quota:       150,
	}, *rootRows[0])
	// A token that still exists resolves to its current name.
	require.Equal(t, 22, rootRows[1].TokenID)
	require.Equal(t, "backup", rootRows[1].TokenName)

	adminRows, err := GetFlowQuotaData(900, 2000, "alice", 0, common.RoleAdminUser)
	require.NoError(t, err)
	require.Len(t, adminRows, 2)
	require.Equal(t, 0, adminRows[0].TokenID)
	require.Empty(t, adminRows[0].TokenName)
	require.Empty(t, adminRows[0].NodeName)
	require.Equal(t, "alice", adminRows[0].Username)
	require.Equal(t, "vip", adminRows[0].UseGroup)
	require.Equal(t, "east", adminRows[0].ChannelName)
	require.Equal(t, 150, adminRows[0].Quota)

	selfRows, err := GetFlowQuotaData(900, 2000, "", 1, common.RoleCommonUser)
	require.NoError(t, err)
	require.Len(t, selfRows, 1)
	require.Empty(t, selfRows[0].Username)
	require.Equal(t, 0, selfRows[0].ChannelID)
	require.Empty(t, selfRows[0].ChannelName)
	require.Empty(t, selfRows[0].TokenName)
	require.Equal(t, "vip", selfRows[0].UseGroup)
	require.Equal(t, 175, selfRows[0].Quota)
}

func TestLogQuotaDataSplitsRowsByUseGroupTokenChannelAndNode(t *testing.T) {
	truncateTables(t)
	CacheQuotaDataLock.Lock()
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()

	LogQuotaData(QuotaDataLogParams{
		UserID:    1,
		Username:  "alice",
		ModelName: "gpt-a",
		CreatedAt: 3661,
		UseGroup:  "vip",
		TokenID:   11,
		ChannelID: 1,
		NodeName:  "node-a",
		Quota:     100,
		TokenUsed: 40,
	})
	LogQuotaData(QuotaDataLogParams{
		UserID:    1,
		Username:  "alice",
		ModelName: "gpt-a",
		CreatedAt: 3700,
		UseGroup:  "vip",
		TokenID:   11,
		ChannelID: 1,
		NodeName:  "node-a",
		Quota:     50,
		TokenUsed: 20,
	})
	LogQuotaData(QuotaDataLogParams{
		UserID:    1,
		Username:  "alice",
		ModelName: "gpt-a",
		CreatedAt: 3700,
		UseGroup:  "default",
		TokenID:   11,
		ChannelID: 1,
		NodeName:  "node-a",
		Quota:     25,
		TokenUsed: 10,
	})

	SaveQuotaDataCache()

	var rows []QuotaData
	require.NoError(t, DB.Order("quota DESC").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, int64(3600), rows[0].CreatedAt)
	require.Equal(t, "vip", rows[0].UseGroup)
	require.Equal(t, 11, rows[0].TokenID)
	require.Equal(t, 1, rows[0].ChannelID)
	require.Equal(t, "node-a", rows[0].NodeName)
	require.Equal(t, 2, rows[0].Count)
	require.Equal(t, 150, rows[0].Quota)
	require.Equal(t, 60, rows[0].TokenUsed)
	require.Equal(t, "default", rows[1].UseGroup)
	require.Equal(t, 25, rows[1].Quota)
}
