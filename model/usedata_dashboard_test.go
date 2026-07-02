package model

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// seedDashboardConsumeLog inserts a single consume log row into the logs table.
// These are the source of truth for dashboard aggregation after the switch
// from quota_data to logs.
func seedDashboardConsumeLog(t *testing.T, log *Log) {
	t.Helper()
	log.Type = LogTypeConsume
	require.NoError(t, LOG_DB.Create(log).Error)
}

// seedStaleQuotaData inserts a quota_data row whose values are deliberately
// wrong relative to the logs. Dashboard aggregations must ignore these.
func seedStaleQuotaData(t *testing.T, row QuotaData) {
	t.Helper()
	require.NoError(t, DB.Create(&row).Error)
}

// quotaDataKey is a stable identity for matching aggregated rows regardless of
// the (unspecified) ordering between same-bucket groups.
func quotaDataKey(model, username string, bucket int64) string {
	return model + "\x00" + username + "\x00" + strconv.FormatInt(bucket, 10)
}

// indexQuotaDataByModelBucket collects aggregated rows into a map keyed by
// model_name + hour bucket so tests do not depend on row ordering between
// groups that share a bucket (the query only orders by created_at ASC).
func indexQuotaDataByModelBucket(rows []*QuotaData) map[string]*QuotaData {
	out := make(map[string]*QuotaData, len(rows))
	for _, r := range rows {
		out[quotaDataKey(r.ModelName, "", r.CreatedAt)] = r
	}
	return out
}

func indexQuotaDataByUsernameBucket(rows []*QuotaData) map[string]*QuotaData {
	out := make(map[string]*QuotaData, len(rows))
	for _, r := range rows {
		out[quotaDataKey("", r.Username, r.CreatedAt)] = r
	}
	return out
}

// seedDashboardFixture seeds a deterministic set of consume logs across two
// hour buckets and three users, plus stale quota_data rows whose values must
// never surface in dashboard results. It returns the expected log-derived
// aggregations so each test asserts against the same source of truth.
//
// Bucket 3600:
//   - alice / gpt-a: 2 logs, quota 150, tokens 70
//   - bob   / gpt-b: 1 log,  quota 70,  tokens 30
//
// Bucket 7200:
//   - alice / gpt-a: 1 log,  quota 200, tokens 100
//   - carol / gpt-a: 1 log,  quota 80,  tokens 40
//
// A non-consume (topup) log for alice is also inserted to prove the
// type = LogTypeConsume filter excludes non-consume entries.
func seedDashboardFixture(t *testing.T) {
	t.Helper()
	truncateTables(t)

	// Bucket 3600 — alice / gpt-a (two logs that must merge into one row).
	seedDashboardConsumeLog(t, &Log{
		UserId: 1, Username: "alice", ModelName: "gpt-a",
		CreatedAt: 3601, Quota: 100, PromptTokens: 30, CompletionTokens: 20,
	})
	seedDashboardConsumeLog(t, &Log{
		UserId: 1, Username: "alice", ModelName: "gpt-a",
		CreatedAt: 3650, Quota: 50, PromptTokens: 10, CompletionTokens: 10,
	})
	// Bucket 3600 — bob / gpt-b.
	seedDashboardConsumeLog(t, &Log{
		UserId: 2, Username: "bob", ModelName: "gpt-b",
		CreatedAt: 3700, Quota: 70, PromptTokens: 15, CompletionTokens: 15,
	})
	// Bucket 7200 — alice / gpt-a.
	seedDashboardConsumeLog(t, &Log{
		UserId: 1, Username: "alice", ModelName: "gpt-a",
		CreatedAt: 7201, Quota: 200, PromptTokens: 40, CompletionTokens: 60,
	})
	// Bucket 7200 — carol / gpt-a (same model as alice, must aggregate together
	// at the model dimension but stay separate at the user dimension).
	seedDashboardConsumeLog(t, &Log{
		UserId: 3, Username: "carol", ModelName: "gpt-a",
		CreatedAt: 7250, Quota: 80, PromptTokens: 20, CompletionTokens: 20,
	})

	// Non-consume log: must be excluded from every dashboard aggregation.
	require.NoError(t, LOG_DB.Create(&Log{
		UserId: 1, Username: "alice", ModelName: "gpt-a",
		CreatedAt: 3601, Type: LogTypeTopup, Quota: 99999,
		PromptTokens: 999, CompletionTokens: 999,
	}).Error)

	// Stale quota_data rows with obviously wrong values. If any aggregation
	// reads from quota_data, these sentinel values (999) would leak through.
	seedStaleQuotaData(t, QuotaData{
		UserID: 1, Username: "alice", ModelName: "gpt-a", CreatedAt: 3600,
		Count: 999, Quota: 999, TokenUsed: 999,
	})
	seedStaleQuotaData(t, QuotaData{
		UserID: 2, Username: "bob", ModelName: "gpt-b", CreatedAt: 3600,
		Count: 999, Quota: 999, TokenUsed: 999,
	})
	seedStaleQuotaData(t, QuotaData{
		UserID: 3, Username: "carol", ModelName: "gpt-a", CreatedAt: 7200,
		Count: 999, Quota: 999, TokenUsed: 999,
	})
}

// TestGetAllQuotaDatesAggregatesLogsByModelAndHourIgnoringStaleQuotaData
// defends the contract that the dashboard "all models" view aggregates from
// logs grouped by model_name and hour bucket, and that stale quota_data rows
// never contribute. A flipped data source (reading quota_data instead of logs)
// or a dropped GROUP BY model would redden this test: the stale sentinel 999
// values would appear, or same-model cross-user logs would fail to merge.
func TestGetAllQuotaDatesAggregatesLogsByModelAndHourIgnoringStaleQuotaData(t *testing.T) {
	seedDashboardFixture(t)

	rows, err := GetAllQuotaDates(0, 0, "")
	require.NoError(t, err)

	byKey := indexQuotaDataByModelBucket(rows)
	require.Len(t, byKey, 3, "expected one row per (model, hour-bucket) pair")

	// Bucket 3600: gpt-a (alice's two logs merged) and gpt-b (bob).
	gptA3600, ok := byKey[quotaDataKey("gpt-a", "", 3600)]
	require.True(t, ok, "missing gpt-a @ 3600")
	require.Equal(t, 2, gptA3600.Count, "gpt-a @ 3600 count must merge alice's two logs")
	require.Equal(t, 150, gptA3600.Quota, "gpt-a @ 3600 quota must sum log quotas, not stale quota_data")
	require.Equal(t, 70, gptA3600.TokenUsed, "gpt-a @ 3600 tokens must sum prompt+completion")
	require.NotEqual(t, 999, gptA3600.Quota, "stale quota_data value leaked into dashboard")

	gptB3600, ok := byKey[quotaDataKey("gpt-b", "", 3600)]
	require.True(t, ok, "missing gpt-b @ 3600")
	require.Equal(t, 1, gptB3600.Count)
	require.Equal(t, 70, gptB3600.Quota)
	require.Equal(t, 30, gptB3600.TokenUsed)
	require.NotEqual(t, 999, gptB3600.Quota, "stale quota_data value leaked into dashboard")

	// Bucket 7200: gpt-a merges alice + carol (same model, different users).
	gptA7200, ok := byKey[quotaDataKey("gpt-a", "", 7200)]
	require.True(t, ok, "missing gpt-a @ 7200")
	require.Equal(t, 2, gptA7200.Count, "gpt-a @ 7200 must merge alice and carol logs by model")
	require.Equal(t, 280, gptA7200.Quota, "gpt-a @ 7200 quota = 200 + 80")
	require.Equal(t, 140, gptA7200.TokenUsed, "gpt-a @ 7200 tokens = 100 + 40")
	require.NotEqual(t, 999, gptA7200.Quota, "stale quota_data value leaked into dashboard")
}

// TestGetAllQuotaDatesByUsernameDelegatesToLogAggregation defends the
// username-filtered branch of GetAllQuotaDates (which delegates to
// GetQuotaDataByUsername): it must still aggregate from logs by model+hour and
// ignore stale quota_data, restricted to the named user.
func TestGetAllQuotaDatesByUsernameDelegatesToLogAggregation(t *testing.T) {
	seedDashboardFixture(t)

	rows, err := GetAllQuotaDates(0, 0, "alice")
	require.NoError(t, err)

	byKey := indexQuotaDataByModelBucket(rows)
	require.Len(t, byKey, 2, "alice has logs in two buckets")

	gptA3600, ok := byKey[quotaDataKey("gpt-a", "", 3600)]
	require.True(t, ok)
	require.Equal(t, 2, gptA3600.Count)
	require.Equal(t, 150, gptA3600.Quota)
	require.NotEqual(t, 999, gptA3600.Quota, "stale quota_data value leaked into filtered dashboard")

	gptA7200, ok := byKey[quotaDataKey("gpt-a", "", 7200)]
	require.True(t, ok)
	require.Equal(t, 1, gptA7200.Count)
	require.Equal(t, 200, gptA7200.Quota)

	// No other user's rows may appear under alice's filter.
	for _, r := range rows {
		require.Equal(t, 1, r.UserID, "username filter leaked another user's row")
	}
}

// TestGetQuotaDataGroupByUserAggregatesLogsByUsernameAndHour defends that the
// per-user dashboard view groups logs by username and hour bucket. All of a
// user's consume logs in the same bucket merge into one row regardless of model.
// The stale quota_data values must not appear.
func TestGetQuotaDataGroupByUserAggregatesLogsByUsernameAndHour(t *testing.T) {
	seedDashboardFixture(t)

	rows, err := GetQuotaDataGroupByUser(0, 0)
	require.NoError(t, err)

	byKey := indexQuotaDataByUsernameBucket(rows)
	require.Len(t, byKey, 4, "expected one row per (username, hour-bucket)")

	alice3600, ok := byKey[quotaDataKey("", "alice", 3600)]
	require.True(t, ok, "missing alice @ 3600")
	require.Equal(t, 2, alice3600.Count)
	require.Equal(t, 150, alice3600.Quota)
	require.Equal(t, 70, alice3600.TokenUsed)
	require.NotEqual(t, 999, alice3600.Quota, "stale quota_data value leaked into group-by-user")

	bob3600, ok := byKey[quotaDataKey("", "bob", 3600)]
	require.True(t, ok, "missing bob @ 3600")
	require.Equal(t, 1, bob3600.Count)
	require.Equal(t, 70, bob3600.Quota)
	require.NotEqual(t, 999, bob3600.Quota, "stale quota_data value leaked into group-by-user")

	alice7200, ok := byKey[quotaDataKey("", "alice", 7200)]
	require.True(t, ok, "missing alice @ 7200")
	require.Equal(t, 1, alice7200.Count)
	require.Equal(t, 200, alice7200.Quota)

	carol7200, ok := byKey[quotaDataKey("", "carol", 7200)]
	require.True(t, ok, "missing carol @ 7200")
	require.Equal(t, 1, carol7200.Count)
	require.Equal(t, 80, carol7200.Quota)
	require.NotEqual(t, 999, carol7200.Quota, "stale quota_data value leaked into group-by-user")
}

// TestGetQuotaDataByUserIdRestrictsToRequestedUser defends the contract that
// the per-user-id dashboard view only returns rows derived from the requested
// user's logs. A bug that drops the user_id filter (or reads cross-user
// quota_data) would surface bob/carol rows or stale 999 values.
func TestGetQuotaDataByUserIdRestrictsToRequestedUser(t *testing.T) {
	seedDashboardFixture(t)

	// User 1 (alice): two buckets, both gpt-a.
	aliceRows, err := GetQuotaDataByUserId(1, 0, 0)
	require.NoError(t, err)
	require.Len(t, aliceRows, 2, "alice should have one row per bucket")
	for _, r := range aliceRows {
		require.Equal(t, 1, r.UserID, "GetQuotaDataByUserId leaked another user's row")
		require.Equal(t, "alice", r.Username)
		require.NotEqual(t, 999, r.Quota, "stale quota_data value leaked into per-user view")
	}
	// Bucket 3600 merges alice's two gpt-a logs.
	byBucket := map[int64]*QuotaData{}
	for _, r := range aliceRows {
		byBucket[r.CreatedAt] = r
	}
	require.Equal(t, 2, byBucket[3600].Count)
	require.Equal(t, 150, byBucket[3600].Quota)
	require.Equal(t, 1, byBucket[7200].Count)
	require.Equal(t, 200, byBucket[7200].Quota)

	// User 2 (bob): one bucket only.
	bobRows, err := GetQuotaDataByUserId(2, 0, 0)
	require.NoError(t, err)
	require.Len(t, bobRows, 1)
	require.Equal(t, 2, bobRows[0].UserID)
	require.Equal(t, "bob", bobRows[0].Username)
	require.Equal(t, int64(3600), bobRows[0].CreatedAt)
	require.Equal(t, 70, bobRows[0].Quota)
	require.NotEqual(t, 999, bobRows[0].Quota, "stale quota_data value leaked into per-user view")

	// User 3 (carol): one bucket only.
	carolRows, err := GetQuotaDataByUserId(3, 0, 0)
	require.NoError(t, err)
	require.Len(t, carolRows, 1)
	require.Equal(t, 3, carolRows[0].UserID)
	require.Equal(t, "carol", carolRows[0].Username)
	require.Equal(t, int64(7200), carolRows[0].CreatedAt)
	require.Equal(t, 80, carolRows[0].Quota)

	// User 4 has no logs: no rows, no error.
	noneRows, err := GetQuotaDataByUserId(4, 0, 0)
	require.NoError(t, err)
	require.Empty(t, noneRows, "user with no consume logs must get no dashboard rows")
}

// TestDashboardAggregationRespectsTimeBoundsAndLogType defends the time-window
// and log-type filters in the log-based aggregation: only consume logs within
// [start, end] contribute, and the hour bucket is derived from the log's
// created_at, not from any quota_data row.
func TestDashboardAggregationRespectsTimeBoundsAndLogType(t *testing.T) {
	seedDashboardFixture(t)

	// start=3700 excludes alice's two logs at 3601 and 3650 (bucket 3600)
	// but keeps bob's log at 3700 (same bucket). end=7200 excludes both
	// bucket-7200 logs (7201 and 7250 are > 7200).
	rows, err := GetAllQuotaDates(3700, 7200, "")
	require.NoError(t, err)

	byKey := indexQuotaDataByModelBucket(rows)
	require.Len(t, byKey, 1, "time window must select exactly one (model, bucket) row")
	gptB, ok := byKey[quotaDataKey("gpt-b", "", 3600)]
	require.True(t, ok, "expected only bob/gpt-b @ 3600 within the time window")
	require.Equal(t, 1, gptB.Count)
	require.Equal(t, 70, gptB.Quota)

	// The non-consume topup log for alice at 3601 (quota 99999) must never
	// appear, even with a window that includes its timestamp.
	for _, r := range rows {
		require.NotEqual(t, 99999, r.Quota, "non-consume log leaked into dashboard aggregation")
	}
}

func TestRebuildQuotaDataFromLogsIsIdempotentAndScopedByModel(t *testing.T) {
	seedDashboardFixture(t)

	params := QuotaDataRebuildParams{
		StartTime: 3601,
		EndTime:   7250,
		ModelName: "gpt-a",
		BatchSize: 2,
	}
	firstResult, err := RebuildQuotaDataFromLogs(context.Background(), params)
	require.NoError(t, err)
	require.Equal(t, int64(2), firstResult.DeletedRows, "first rebuild should replace stale gpt-a quota_data rows")
	require.Equal(t, int64(3), firstResult.InsertedRows, "gpt-a rebuild should materialize one row per user/hour bucket")
	require.Equal(t, int64(4), firstResult.LogRows)
	require.Equal(t, int64(430), firstResult.Quota)
	require.Equal(t, int64(210), firstResult.TokenUsed)

	assertRebuiltGPTAQuotaData(t)

	secondResult, err := RebuildQuotaDataFromLogs(context.Background(), params)
	require.NoError(t, err)
	require.Equal(t, int64(3), secondResult.DeletedRows, "second rebuild should replace prior materialized rows instead of appending")
	require.Equal(t, int64(3), secondResult.InsertedRows)
	require.Equal(t, int64(4), secondResult.LogRows)

	assertRebuiltGPTAQuotaData(t)

	var staleOtherModel QuotaData
	require.NoError(t, DB.Table("quota_data").Where("model_name = ?", "gpt-b").First(&staleOtherModel).Error)
	require.Equal(t, 999, staleOtherModel.Quota, "model-scoped rebuild must not delete other models")
}

func assertRebuiltGPTAQuotaData(t *testing.T) {
	t.Helper()

	var rows []*QuotaData
	require.NoError(t, DB.Table("quota_data").Where("model_name = ?", "gpt-a").Find(&rows).Error)
	require.Len(t, rows, 3)

	var countSum, quotaSum, tokenSum int
	for _, row := range rows {
		countSum += row.Count
		quotaSum += row.Quota
		tokenSum += row.TokenUsed
		require.NotEqual(t, 999, row.Quota, "stale quota_data value survived rebuild")
	}
	require.Equal(t, 4, countSum)
	require.Equal(t, 430, quotaSum)
	require.Equal(t, 210, tokenSum)
}
