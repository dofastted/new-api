package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clearSubscriptionProviderGroupTestTables(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&SubscriptionPreConsumeRecord{}))
	for _, table := range []string{
		"subscription_pre_consume_records",
		"user_subscriptions",
		"subscription_orders",
		"subscription_plans",
		"users",
	} {
		require.NoError(t, DB.Exec("DELETE FROM "+table).Error)
	}
	t.Cleanup(func() {
		for _, table := range []string{
			"subscription_pre_consume_records",
			"user_subscriptions",
			"subscription_orders",
			"subscription_plans",
			"users",
		} {
			_ = DB.Exec("DELETE FROM " + table).Error
		}
	})
}

func insertSubscriptionProviderGroupUser(t *testing.T, id int) {
	t.Helper()
	require.NoError(t, DB.Create(&User{
		Id:       id,
		Username: "sub_pg_user",
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}).Error)
}

func insertSubscriptionProviderGroupPlan(t *testing.T, id int, providerGroups JSONStringList) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:             id,
		Title:          "Provider Group Plan",
		PriceAmount:    9.99,
		Currency:       "USD",
		DurationUnit:   SubscriptionDurationMonth,
		DurationValue:  1,
		Enabled:        true,
		TotalAmount:    1000,
		ProviderGroups: providerGroups,
	}
	require.NoError(t, DB.Create(plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	return plan
}

func insertSubscriptionProviderGroupSubscription(t *testing.T, sub *UserSubscription) {
	t.Helper()
	require.NoError(t, DB.Create(sub).Error)
}

func getSubscriptionProviderGroupSubscription(t *testing.T, id int) UserSubscription {
	t.Helper()
	var sub UserSubscription
	require.NoError(t, DB.Where("id = ?", id).First(&sub).Error)
	return sub
}

func TestJSONStringListProviderGroupNormalizationJSONAndDBCompatibility(t *testing.T) {
	list := NewJSONStringList([]string{" group-a ", "", "group-b", "group-a", " group-b "})
	assert.Equal(t, []string{"group-a", "group-b"}, list.Strings())
	assert.True(t, list.Allows("group-a"))
	assert.False(t, list.Allows("group-c"))

	encoded, err := common.Marshal(JSONStringList{" group-a ", "", "group-b", "group-a"})
	require.NoError(t, err)
	assert.Equal(t, `["group-a","group-b"]`, string(encoded))

	var decoded JSONStringList
	require.NoError(t, common.Unmarshal([]byte(`[" group-b ","","group-a","group-b"]`), &decoded))
	assert.Equal(t, []string{"group-b", "group-a"}, decoded.Strings())

	emptyValue, err := JSONStringList(nil).Value()
	require.NoError(t, err)
	assert.Equal(t, "", emptyValue)

	value, err := JSONStringList{" group-a ", "group-b", "group-a"}.Value()
	require.NoError(t, err)
	assert.Equal(t, `["group-a","group-b"]`, value)

	for _, tc := range []struct {
		name  string
		input any
	}{
		{name: "nil", input: nil},
		{name: "empty string", input: ""},
		{name: "empty bytes", input: []byte("")},
		{name: "json null", input: "null"},
		{name: "empty array", input: "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var scanned JSONStringList
			require.NoError(t, scanned.Scan(tc.input))
			assert.Empty(t, scanned.Strings())
			assert.True(t, scanned.Allows("any-provider-group"))
		})
	}
}

func TestCreateUserSubscriptionFromPlanTxSnapshotsProviderGroups(t *testing.T) {
	clearSubscriptionProviderGroupTestTables(t)
	insertSubscriptionProviderGroupUser(t, 61001)
	plan := insertSubscriptionProviderGroupPlan(t, 62001, NewJSONStringList([]string{" group-a ", "group-b", "group-a"}))
	created, err := CreateUserSubscriptionFromPlanTx(DB, 61001, plan, "admin")
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, []string{"group-a", "group-b"}, created.ProviderGroups.Strings())

	require.NoError(t, DB.Model(&SubscriptionPlan{}).
		Where("id = ?", plan.Id).
		Update("provider_groups", NewJSONStringList([]string{"group-c"})).Error)
	InvalidateSubscriptionPlanCache(plan.Id)

	stored := getSubscriptionProviderGroupSubscription(t, created.Id)
	assert.Equal(t, []string{"group-a", "group-b"}, stored.ProviderGroups.Strings())
}

func TestPreConsumeUserSubscriptionFiltersProviderGroupBeforeEarliestExpiry(t *testing.T) {
	clearSubscriptionProviderGroupTestTables(t)
	insertSubscriptionProviderGroupUser(t, 61002)
	plan := insertSubscriptionProviderGroupPlan(t, 62002, nil)
	now := GetDBTimestamp()

	insertSubscriptionProviderGroupSubscription(t, &UserSubscription{
		Id:                  63001,
		UserId:              61002,
		PlanId:              plan.Id,
		AmountTotal:         1000,
		StartTime:           now - 10,
		EndTime:             now + 100,
		Status:              "active",
		ProviderGroups:      NewJSONStringList([]string{"other-group"}),
		AllowWalletOverflow: true,
	})
	insertSubscriptionProviderGroupSubscription(t, &UserSubscription{
		Id:                  63020,
		UserId:              61002,
		PlanId:              plan.Id,
		AmountTotal:         1000,
		StartTime:           now - 10,
		EndTime:             now + 150,
		Status:              "active",
		ProviderGroups:      NewJSONStringList([]string{"target-group"}),
		AllowWalletOverflow: true,
	})
	insertSubscriptionProviderGroupSubscription(t, &UserSubscription{
		Id:                  63010,
		UserId:              61002,
		PlanId:              plan.Id,
		AmountTotal:         1000,
		StartTime:           now - 10,
		EndTime:             now + 150,
		Status:              "active",
		ProviderGroups:      NewJSONStringList([]string{"target-group"}),
		AllowWalletOverflow: true,
	})
	insertSubscriptionProviderGroupSubscription(t, &UserSubscription{
		Id:                  63002,
		UserId:              61002,
		PlanId:              plan.Id,
		AmountTotal:         1000,
		StartTime:           now - 10,
		EndTime:             now + 200,
		Status:              "active",
		ProviderGroups:      NewJSONStringList([]string{"target-group"}),
		AllowWalletOverflow: true,
	})

	result, err := PreConsumeUserSubscription("provider-group-filter-order", 61002, "gpt-5.5", 0, 100, "target-group")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 63010, result.UserSubscriptionId)
	assert.Equal(t, int64(100), result.PreConsumed)
	assert.Equal(t, []string{"target-group"}, result.ProviderGroups)

	wrongGroup := getSubscriptionProviderGroupSubscription(t, 63001)
	chosen := getSubscriptionProviderGroupSubscription(t, 63010)
	higherTie := getSubscriptionProviderGroupSubscription(t, 63020)
	assert.Zero(t, wrongGroup.AmountUsed)
	assert.Equal(t, int64(100), chosen.AmountUsed)
	assert.Zero(t, higherTie.AmountUsed)
}

func TestPreConsumeUserSubscriptionReturnsProviderGroupNotAllowed(t *testing.T) {
	clearSubscriptionProviderGroupTestTables(t)
	insertSubscriptionProviderGroupUser(t, 61003)
	plan := insertSubscriptionProviderGroupPlan(t, 62003, nil)
	now := GetDBTimestamp()
	insertSubscriptionProviderGroupSubscription(t, &UserSubscription{
		Id:                  63030,
		UserId:              61003,
		PlanId:              plan.Id,
		AmountTotal:         1000,
		StartTime:           now - 10,
		EndTime:             now + 100,
		Status:              "active",
		ProviderGroups:      NewJSONStringList([]string{"allowed-group"}),
		AllowWalletOverflow: true,
	})

	result, err := PreConsumeUserSubscription("provider-group-not-allowed", 61003, "gpt-5.5", 0, 100, "blocked-group")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSubscriptionProviderGroupNotAllowed))
	assert.Nil(t, result)

	stored := getSubscriptionProviderGroupSubscription(t, 63030)
	assert.Zero(t, stored.AmountUsed)
	var recordCount int64
	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).Where("request_id = ?", "provider-group-not-allowed").Count(&recordCount).Error)
	assert.Zero(t, recordCount)
}

func TestGetActiveSubscriptionProviderGroupScopeFiniteUnion(t *testing.T) {
	clearSubscriptionProviderGroupTestTables(t)
	insertSubscriptionProviderGroupUser(t, 61004)
	plan := insertSubscriptionProviderGroupPlan(t, 62004, nil)
	now := GetDBTimestamp()
	insertSubscriptionProviderGroupSubscription(t, &UserSubscription{
		Id:                  63040,
		UserId:              61004,
		PlanId:              plan.Id,
		AmountTotal:         1000,
		StartTime:           now - 10,
		EndTime:             now + 100,
		Status:              "active",
		ProviderGroups:      NewJSONStringList([]string{"group-b"}),
		AllowWalletOverflow: true,
	})
	insertSubscriptionProviderGroupSubscription(t, &UserSubscription{
		Id:                  63041,
		UserId:              61004,
		PlanId:              plan.Id,
		AmountTotal:         1000,
		StartTime:           now - 10,
		EndTime:             now + 200,
		Status:              "active",
		ProviderGroups:      NewJSONStringList([]string{" group-a ", "group-b"}),
		AllowWalletOverflow: true,
	})
	insertSubscriptionProviderGroupSubscription(t, &UserSubscription{
		Id:                  63042,
		UserId:              61004,
		PlanId:              plan.Id,
		AmountTotal:         1000,
		StartTime:           now - 200,
		EndTime:             now - 100,
		Status:              "active",
		ProviderGroups:      NewJSONStringList([]string{"expired-group"}),
		AllowWalletOverflow: true,
	})
	insertSubscriptionProviderGroupSubscription(t, &UserSubscription{
		Id:                  63043,
		UserId:              61004,
		PlanId:              plan.Id,
		AmountTotal:         1000,
		StartTime:           now - 10,
		EndTime:             now + 300,
		Status:              "cancelled",
		ProviderGroups:      NewJSONStringList([]string{"cancelled-group"}),
		AllowWalletOverflow: true,
	})

	scope, err := GetActiveSubscriptionProviderGroupScope(61004)
	require.NoError(t, err)
	assert.True(t, scope.HasActive)
	assert.False(t, scope.Unrestricted)
	assert.Equal(t, []string{"group-b", "group-a"}, scope.Groups)
	assert.Equal(t, []string{"group-a", "group-b"}, scope.Filter([]string{"group-a", "group-c", "group-b"}))
}

func TestGetActiveSubscriptionProviderGroupScopeUnrestrictedWhenAnyActiveSnapshotIsEmpty(t *testing.T) {
	clearSubscriptionProviderGroupTestTables(t)
	insertSubscriptionProviderGroupUser(t, 61005)
	plan := insertSubscriptionProviderGroupPlan(t, 62005, nil)
	now := GetDBTimestamp()
	insertSubscriptionProviderGroupSubscription(t, &UserSubscription{
		Id:                  63050,
		UserId:              61005,
		PlanId:              plan.Id,
		AmountTotal:         1000,
		StartTime:           now - 10,
		EndTime:             now + 100,
		Status:              "active",
		ProviderGroups:      NewJSONStringList([]string{"group-a"}),
		AllowWalletOverflow: true,
	})
	insertSubscriptionProviderGroupSubscription(t, &UserSubscription{
		Id:                  63051,
		UserId:              61005,
		PlanId:              plan.Id,
		AmountTotal:         1000,
		StartTime:           now - 10,
		EndTime:             now + 200,
		Status:              "active",
		ProviderGroups:      nil,
		AllowWalletOverflow: true,
	})

	scope, err := GetActiveSubscriptionProviderGroupScope(61005)
	require.NoError(t, err)
	assert.True(t, scope.HasActive)
	assert.True(t, scope.Unrestricted)
	assert.Empty(t, scope.Groups)
	assert.Equal(t, []string{"group-a", "group-c"}, scope.Filter([]string{"group-a", "group-c"}))
}
