package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
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

func TestEnsureProviderGroupsSeededFromLegacySkipsUserGroups(t *testing.T) {
	clearProviderGroupTestTables(t)
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
	})

	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"premium":1,"provider-a":0.5}`))
	require.NoError(t, DB.Create(&Channel{
		Id:     9102,
		Type:   1,
		Key:    "sk-user-group",
		Status: common.ChannelStatusEnabled,
		Name:   "user-group-channel",
		Models: "gpt-5.5",
		Group:  "default,provider-a,vip",
	}).Error)

	require.NoError(t, EnsureProviderGroupsSeededFromLegacy())

	var reservedCount int64
	require.NoError(t, DB.Model(&ProviderGroup{}).Where("name IN ?", ReservedUserProviderGroupNames()).Count(&reservedCount).Error)
	assert.Equal(t, int64(0), reservedCount)

	var memberCount int64
	require.NoError(t, DB.Model(&ProviderGroupChannel{}).Where("group_name IN ?", ReservedUserProviderGroupNames()).Count(&memberCount).Error)
	assert.Equal(t, int64(0), memberCount)
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

func TestRebuildAbilitiesSkipsDisabledProviderGroups(t *testing.T) {
	clearProviderGroupTestTables(t)

	require.NoError(t, DB.Create(&ProviderGroup{
		Id:          9201,
		Name:        "active-provider",
		DisplayName: "active-provider",
		Status:      ProviderGroupStatusEnabled,
		UsageRatio:  1,
	}).Error)
	require.NoError(t, DB.Create(&ProviderGroup{
		Id:          9202,
		Name:        "disabled-provider",
		DisplayName: "disabled-provider",
		UsageRatio:  1,
	}).Error)
	require.NoError(t, DB.Model(&ProviderGroup{}).
		Where("id = ?", 9202).
		Update("status", ProviderGroupStatusDisabled).Error)
	require.NoError(t, DB.Create(&Channel{
		Id:     9203,
		Type:   1,
		Key:    "sk-active",
		Status: common.ChannelStatusEnabled,
		Name:   "active-channel",
		Models: "gpt-5.5",
	}).Error)
	require.NoError(t, DB.Create(&Channel{
		Id:     9204,
		Type:   1,
		Key:    "sk-disabled",
		Status: common.ChannelStatusEnabled,
		Name:   "disabled-channel",
		Models: "gpt-5.5",
	}).Error)
	require.NoError(t, DB.Create(&ProviderGroupChannel{
		ProviderGroupId: 9201,
		GroupName:       "active-provider",
		ChannelId:       9203,
		RouteTypes:      defaultProviderRouteTypesJSON(),
		Enabled:         true,
	}).Error)
	require.NoError(t, DB.Create(&ProviderGroupChannel{
		ProviderGroupId: 9202,
		GroupName:       "disabled-provider",
		ChannelId:       9204,
		RouteTypes:      defaultProviderRouteTypesJSON(),
		Enabled:         true,
	}).Error)

	require.NoError(t, RebuildAbilitiesFromProviderGroups())

	var activeCount int64
	require.NoError(t, DB.Model(&Ability{}).Where("\"group\" = ?", "active-provider").Count(&activeCount).Error)
	assert.Equal(t, int64(1), activeCount)

	var disabledCount int64
	require.NoError(t, DB.Model(&Ability{}).Where("\"group\" = ?", "disabled-provider").Count(&disabledCount).Error)
	assert.Equal(t, int64(0), disabledCount)
}

func TestProviderRouteTypesForAdvancedCustomChannel(t *testing.T) {
	channel := Channel{Type: constant.ChannelTypeAdvancedCustom}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{
			Routes: []dto.AdvancedCustomRoute{
				{IncomingPath: "/v1/responses"},
				{IncomingPath: "/v1/messages"},
			},
		},
	})

	assert.Equal(t, []string{ProviderRouteTypeResponses, ProviderRouteTypeMessages}, ProviderRouteTypesForChannelValue(channel))
}
