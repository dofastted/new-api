package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clearProviderGroupTestTables(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)
	require.NoError(t, DB.Exec("DELETE FROM provider_group_auto_rules").Error)
	require.NoError(t, DB.Exec("DELETE FROM provider_group_channels").Error)
	require.NoError(t, DB.Exec("DELETE FROM provider_groups").Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM abilities")
		DB.Exec("DELETE FROM channels")
		DB.Exec("DELETE FROM provider_group_auto_rules")
		DB.Exec("DELETE FROM provider_group_channels")
		DB.Exec("DELETE FROM provider_groups")
	})
}

func TestEnsureProviderGroupsSeededFromLegacy(t *testing.T) {
	clearProviderGroupTestTables(t)
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	oldAutoGroups := setting.AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(oldAutoGroups))
	})

	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"gpt":0.175}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	priority := int64(7)
	weight := uint(42)
	require.NoError(t, DB.Create(&Channel{
		Id:       9101,
		Type:     1,
		Key:      "sk-test",
		Status:   common.ChannelStatusEnabled,
		Name:     "seed-channel",
		Models:   "gpt-5.5",
		Group:    "gpt",
		Priority: &priority,
		Weight:   &weight,
	}).Error)

	require.NoError(t, EnsureProviderGroupsSeededFromLegacy())

	var group ProviderGroup
	require.NoError(t, DB.Where("name = ?", "gpt").First(&group).Error)
	assert.Equal(t, 0.175, group.UsageRatio)
	assert.Equal(t, ProviderGroupStatusEnabled, group.Status)

	var member ProviderGroupChannel
	require.NoError(t, DB.Where("group_name = ? AND channel_id = ?", "gpt", 9101).First(&member).Error)
	require.NotNil(t, member.Priority)
	assert.Equal(t, priority, *member.Priority)
	require.NotNil(t, member.Weight)
	assert.Equal(t, weight, *member.Weight)
	assert.True(t, providerRouteTypesContain(member.RouteTypes, ProviderRouteTypeResponses))
}

func TestProviderGroupOnlineStatusAndDelete(t *testing.T) {
	clearProviderGroupTestTables(t)

	require.NoError(t, DB.Create(&ProviderGroup{
		Name:        "offline-test",
		DisplayName: "offline-test",
		Status:      ProviderGroupStatusEnabled,
		UsageRatio:  0.25,
	}).Error)

	online, err := IsProviderGroupOnline("offline-test")
	require.NoError(t, err)
	assert.True(t, online)
	ratio, ok := ProviderGroupUsageRatio("offline-test")
	assert.True(t, ok)
	assert.Equal(t, 0.25, ratio)

	require.NoError(t, DB.Where("name = ?", "offline-test").Delete(&ProviderGroup{}).Error)
	online, err = IsProviderGroupOnline("offline-test")
	require.NoError(t, err)
	assert.False(t, online)
}
