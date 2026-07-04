package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRiskEventTable(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&RiskEvent{}))
	require.NoError(t, DB.Where("1 = 1").Delete(&RiskEvent{}).Error)
}

func TestCountUserTempBans(t *testing.T) {
	setupRiskEventTable(t)

	rows := []*RiskEvent{
		{UserId: 1, CreatedAt: 100, Source: RiskSourcePenalty, Action: RiskActionTempBan},
		{UserId: 1, CreatedAt: 200, Source: RiskSourcePenalty, Action: RiskActionTempBan},
		{UserId: 1, CreatedAt: 300, Source: RiskSourceSyncWord, Action: RiskActionBlocked},
		{UserId: 2, CreatedAt: 400, Source: RiskSourcePenalty, Action: RiskActionTempBan},
	}
	for _, r := range rows {
		require.NoError(t, DB.Create(r).Error)
	}

	count, err := CountUserTempBans(1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	count, err = CountUserTempBans(3)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestGetRiskEventsFilters(t *testing.T) {
	setupRiskEventTable(t)

	rows := []*RiskEvent{
		{UserId: 1, CreatedAt: 100, Source: RiskSourceSyncWord, Action: RiskActionBlocked, ModelName: "gpt-4o"},
		{UserId: 1, CreatedAt: 200, Source: RiskSourceModeration, Action: RiskActionFlagged, ModelName: "claude-sonnet-4-6"},
		{UserId: 2, CreatedAt: 300, Source: RiskSourceSyncPattern, Action: RiskActionBlocked, ModelName: "gpt-4o"},
		{UserId: 2, CreatedAt: 400, Source: RiskSourcePenalty, Action: RiskActionTempBan},
	}
	for _, r := range rows {
		require.NoError(t, DB.Create(r).Error)
	}

	events, total, err := GetRiskEvents(RiskEventQuery{UserId: 1, Num: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, events, 2)
	assert.Equal(t, int64(200), events[0].CreatedAt, "results ordered by id desc")

	events, total, err = GetRiskEvents(RiskEventQuery{Action: RiskActionBlocked, Num: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)

	events, total, err = GetRiskEvents(RiskEventQuery{StartTimestamp: 250, EndTimestamp: 350, Num: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, events, 1)
	assert.Equal(t, RiskSourceSyncPattern, events[0].Source)

	events, total, err = GetRiskEvents(RiskEventQuery{Num: 2, StartIdx: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(4), total)
	assert.Len(t, events, 2)
}
