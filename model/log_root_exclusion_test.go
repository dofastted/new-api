package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedLogFilterUser(t *testing.T, id int, username string, role int) {
	t.Helper()
	require.NoError(t, DB.Create(&User{
		Id:       id,
		Username: username,
		Role:     role,
		AffCode:  username + "-aff",
	}).Error)
}

func TestGetAdminUserIdsExcludesAdminAndRootRoles(t *testing.T) {
	truncateTables(t)

	seedLogFilterUser(t, 1, "root", common.RoleRootUser)
	seedLogFilterUser(t, 2, "admin", common.RoleAdminUser)
	seedLogFilterUser(t, 3, "user", common.RoleCommonUser)

	ids, err := GetAdminUserIds()
	require.NoError(t, err)
	assert.ElementsMatch(t, []int{1, 2}, ids)
}

func TestLogQueriesExcludeAdminAndRootUsers(t *testing.T) {
	truncateTables(t)

	seedLogFilterUser(t, 1, "root", common.RoleRootUser)
	seedLogFilterUser(t, 2, "admin", common.RoleAdminUser)
	seedLogFilterUser(t, 3, "user", common.RoleCommonUser)

	logs := []Log{
		{UserId: 1, Username: "root", CreatedAt: 1000, Type: LogTypeConsume, ModelName: "gpt-a", Quota: 100, PromptTokens: 10, CompletionTokens: 1},
		{UserId: 2, Username: "admin", CreatedAt: 1001, Type: LogTypeConsume, ModelName: "gpt-a", Quota: 70, PromptTokens: 7, CompletionTokens: 2},
		{UserId: 3, Username: "user", CreatedAt: 1002, Type: LogTypeConsume, ModelName: "gpt-a", Quota: 30, PromptTokens: 3, CompletionTokens: 3},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)

	excludeUserIds, err := GetAdminUserIds()
	require.NoError(t, err)

	rows, total, err := GetAllLogs(LogTypeConsume, 900, 1100, "", "", "", 0, 10, 0, "", "", "", excludeUserIds)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, "user", rows[0].Username)

	stat, err := SumUsedQuota(LogTypeConsume, 900, 1100, "", "", "", 0, "", excludeUserIds)
	require.NoError(t, err)
	assert.Equal(t, 30, stat.Quota)
}
