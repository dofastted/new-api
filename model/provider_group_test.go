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

func TestProviderRouteTypeForPath(t *testing.T) {
	tests := []struct {
		name        string
		requestPath string
		expected    string
	}{
		{name: "chat completions", requestPath: "/v1/chat/completions", expected: ProviderRouteTypeCompletions},
		{name: "chat completions subpath", requestPath: "/v1/chat/completions/foo", expected: ProviderRouteTypeCompletions},
		{name: "legacy completions", requestPath: "/v1/completions", expected: ProviderRouteTypeCompletions},
		{name: "responses compact", requestPath: "/v1/responses/compact", expected: ProviderRouteTypeResponses},
		{name: "messages", requestPath: "/v1/messages?beta=true", expected: ProviderRouteTypeMessages},
		{name: "similar chat prefix", requestPath: "/v1/chat/completions-extra", expected: ProviderRouteTypeOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ProviderRouteTypeForPath(tt.requestPath))
		})
	}
}

func TestSyncProviderGroupChannelsForChannel(t *testing.T) {
	clearProviderGroupTestTables(t)

	require.NoError(t, DB.Create(&ProviderGroup{
		Name: "sync-g", DisplayName: "sync-g", Status: ProviderGroupStatusEnabled, UsageRatio: 1,
	}).Error)
	require.NoError(t, DB.Create(&ProviderGroup{
		Name: "other-g", DisplayName: "other-g", Status: ProviderGroupStatusEnabled, UsageRatio: 1,
	}).Error)

	priority := int64(3)
	require.NoError(t, DB.Create(&Channel{
		Id: 9301, Type: 1, Key: "sk-sync", Status: common.ChannelStatusEnabled,
		Name: "sync-channel", Models: "gpt-4o", Group: "sync-g", Priority: &priority,
	}).Error)

	// First sync creates membership with a neutral priority, independent of the
	// channel's legacy global priority.
	require.NoError(t, SyncProviderGroupChannelsForChannel(Channel{
		Id: 9301, Group: "sync-g", Priority: &priority, Status: common.ChannelStatusEnabled,
		Models: "gpt-4o", Type: 1,
	}))
	var member ProviderGroupChannel
	require.NoError(t, DB.Where("channel_id = ? AND group_name = ?", 9301, "sync-g").First(&member).Error)
	require.NotNil(t, member.Priority)
	assert.Equal(t, int64(0), *member.Priority)
	assert.True(t, member.Enabled)

	// Groups page owns enabled and priority; channel sync must preserve both.
	membershipPriority := int64(7)
	require.NoError(t, DB.Model(&ProviderGroupChannel{}).Where("id = ?", member.Id).
		Updates(map[string]interface{}{"enabled": false, "priority": membershipPriority}).Error)

	// Adding another group preserves the existing membership settings and gives
	// the new membership a neutral priority.
	require.NoError(t, SyncProviderGroupChannelsForChannel(Channel{
		Id: 9301, Group: "sync-g,other-g", Priority: &priority, Status: common.ChannelStatusEnabled,
		Models: "gpt-4o", Type: 1,
	}))
	var otherMember ProviderGroupChannel
	require.NoError(t, DB.Where("channel_id = ? AND group_name = ?", 9301, "other-g").First(&otherMember).Error)
	require.NotNil(t, otherMember.Priority)
	assert.Equal(t, int64(0), *otherMember.Priority)
	assert.True(t, otherMember.Enabled)

	newPriority := int64(99)
	require.NoError(t, SyncProviderGroupChannelsForChannel(Channel{
		Id: 9301, Group: "sync-g,other-g", Priority: &newPriority, Status: common.ChannelStatusEnabled,
		Models: "gpt-4o", Type: 1,
	}))
	require.NoError(t, DB.Where("id = ?", member.Id).First(&member).Error)
	require.NotNil(t, member.Priority)
	assert.Equal(t, membershipPriority, *member.Priority)
	assert.False(t, member.Enabled, "sync must preserve provider-group membership state")

	// Sync with group removed: stale membership deleted.
	require.NoError(t, SyncProviderGroupChannelsForChannel(Channel{
		Id: 9301, Group: "sync-g", Priority: &newPriority, Status: common.ChannelStatusEnabled,
		Models: "gpt-4o", Type: 1,
	}))
	var otherCount int64
	require.NoError(t, DB.Model(&ProviderGroupChannel{}).Where("channel_id = ? AND group_name = ?", 9301, "other-g").Count(&otherCount).Error)
	assert.Equal(t, int64(0), otherCount, "stale membership should be deleted")
}

func TestSyncProviderGroupChannelsRefreshesRouteTypes(t *testing.T) {
	clearProviderGroupTestTables(t)
	require.NoError(t, DB.Create(&ProviderGroup{
		Name: "rt-g", DisplayName: "rt-g", Status: ProviderGroupStatusEnabled, UsageRatio: 1,
	}).Error)
	ch := Channel{
		Id: 9311, Type: 1, Key: "sk-rt", Status: common.ChannelStatusEnabled,
		Name: "rt-channel", Models: "gpt-4o", Group: "rt-g",
	}
	require.NoError(t, DB.Create(&ch).Error)
	require.NoError(t, SyncProviderGroupChannelsForChannel(ch))
	var member ProviderGroupChannel
	require.NoError(t, DB.Where("channel_id = ? AND group_name = ?", 9311, "rt-g").First(&member).Error)
	assert.True(t, providerRouteTypesContain(member.RouteTypes, ProviderRouteTypeOther))

	// Change channel to advanced-custom with specific routes and re-sync.
	ch.Type = constant.ChannelTypeAdvancedCustom
	ch.SetOtherSettings(dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{
			Routes: []dto.AdvancedCustomRoute{{IncomingPath: "/v1/responses"}},
		},
	})
	require.NoError(t, SyncProviderGroupChannelsForChannel(ch))
	require.NoError(t, DB.Where("channel_id = ? AND group_name = ?", 9311, "rt-g").First(&member).Error)
	assert.True(t, providerRouteTypesContain(member.RouteTypes, ProviderRouteTypeResponses))
	assert.False(t, providerRouteTypesContain(member.RouteTypes, ProviderRouteTypeOther))
}

func TestRebuildAbilitiesRespectsGroupStatusToggle(t *testing.T) {
	clearProviderGroupTestTables(t)
	require.NoError(t, DB.Create(&ProviderGroup{
		Id: 9401, Name: "toggle-g", DisplayName: "toggle-g", Status: ProviderGroupStatusEnabled, UsageRatio: 1,
	}).Error)
	require.NoError(t, DB.Create(&Channel{
		Id: 9402, Type: 1, Key: "sk-t", Status: common.ChannelStatusEnabled,
		Name: "toggle-ch", Models: "gpt-4o",
	}).Error)
	require.NoError(t, DB.Create(&ProviderGroupChannel{
		ProviderGroupId: 9401, GroupName: "toggle-g", ChannelId: 9402,
		RouteTypes: defaultProviderRouteTypesJSON(), Enabled: true,
	}).Error)

	require.NoError(t, RebuildAbilitiesFromProviderGroups())
	var enabledCount int64
	require.NoError(t, DB.Model(&Ability{}).Where(commonGroupCol+" = ?", "toggle-g").Count(&enabledCount).Error)
	assert.Equal(t, int64(1), enabledCount)

	// Flip group offline and rebuild: abilities for this group must disappear.
	require.NoError(t, DB.Model(&ProviderGroup{}).Where("id = ?", 9401).
		Update("status", ProviderGroupStatusDisabled).Error)
	require.NoError(t, RebuildAbilitiesFromProviderGroups())
	var afterCount int64
	require.NoError(t, DB.Model(&Ability{}).Where(commonGroupCol+" = ?", "toggle-g").Count(&afterCount).Error)
	assert.Equal(t, int64(0), afterCount, "offline group abilities must be removed on rebuild")
}

func TestProviderGroupPriorityMatchesWithAndWithoutMemoryCache(t *testing.T) {
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		if oldMemoryCacheEnabled {
			InitChannelCache()
		}
	})
	clearProviderGroupTestTables(t)

	require.NoError(t, DB.Create(&[]ProviderGroup{
		{Id: 9451, Name: "cache-g1", DisplayName: "cache-g1", Status: ProviderGroupStatusEnabled, UsageRatio: 1},
		{Id: 9452, Name: "cache-g2", DisplayName: "cache-g2", Status: ProviderGroupStatusEnabled, UsageRatio: 1},
	}).Error)
	globalHigh := int64(100)
	globalLow := int64(0)
	globalHighest := int64(999)
	require.NoError(t, DB.Create(&[]Channel{
		{Id: 9453, Type: 1, Key: "sk-a", Status: common.ChannelStatusEnabled, Name: "channel-a", Models: "gpt-4o", Priority: &globalHigh},
		{Id: 9454, Type: 1, Key: "sk-b", Status: common.ChannelStatusEnabled, Name: "channel-b", Models: "gpt-4o", Priority: &globalLow},
		{Id: 9455, Type: 1, Key: "sk-c", Status: common.ChannelStatusEnabled, Name: "channel-c", Models: "gpt-4o", Priority: &globalHighest},
	}).Error)
	priority1 := int64(1)
	priority10 := int64(10)
	require.NoError(t, DB.Create(&[]ProviderGroupChannel{
		{ProviderGroupId: 9451, GroupName: "cache-g1", ChannelId: 9453, Priority: &priority1, Weight: uintPointer(1), Enabled: true},
		{ProviderGroupId: 9451, GroupName: "cache-g1", ChannelId: 9454, Priority: &priority10, Weight: uintPointer(1), Enabled: true},
		{ProviderGroupId: 9452, GroupName: "cache-g2", ChannelId: 9453, Priority: &priority10, Weight: uintPointer(1), Enabled: true},
		{ProviderGroupId: 9452, GroupName: "cache-g2", ChannelId: 9455, Priority: nil, Weight: uintPointer(1), Enabled: true},
	}).Error)
	require.NoError(t, RebuildAbilitiesFromProviderGroups())

	var nilPriorityAbility Ability
	require.NoError(t, DB.Where(commonGroupCol+" = ? AND model = ? AND channel_id = ?", "cache-g2", "gpt-4o", 9455).
		First(&nilPriorityAbility).Error)
	require.NotNil(t, nilPriorityAbility.Priority)
	assert.Equal(t, int64(0), *nilPriorityAbility.Priority)

	for _, memoryCacheEnabled := range []bool{false, true} {
		common.MemoryCacheEnabled = memoryCacheEnabled
		if memoryCacheEnabled {
			InitChannelCache()
		}

		selected, err := GetRandomSatisfiedChannel("cache-g1", "gpt-4o", 0, "", nil)
		require.NoError(t, err)
		require.NotNil(t, selected)
		assert.Equal(t, 9454, selected.Id, "membership priority must override global channel priority")

		fallback, err := GetRandomSatisfiedChannel("cache-g1", "gpt-4o", 1, "", nil)
		require.NoError(t, err)
		require.NotNil(t, fallback)
		assert.Equal(t, 9453, fallback.Id)

		otherGroup, err := GetRandomSatisfiedChannel("cache-g2", "gpt-4o", 0, "", nil)
		require.NoError(t, err)
		require.NotNil(t, otherGroup)
		assert.Equal(t, 9453, otherGroup.Id, "the same channel may have a different priority in another group")
	}
}

func uintPointer(value uint) *uint {
	return &value
}

// TestProviderGroupChannelSupportsPathPrefersMemberRouteTypes guards the
// route_types-first contract of ProviderGroupChannelSupportsPath: a
// provider_group_channels membership with route_types=["messages"] must
// support /v1/messages and reject /v1/responses, regardless of the
// underlying channel's own route types (the member row is the SSOT and the
// channel fallback must not be reached when route_types is non-empty).
func TestProviderGroupChannelSupportsPathPrefersMemberRouteTypes(t *testing.T) {
	clearProviderGroupTestTables(t)

	require.NoError(t, DB.Create(&ProviderGroup{
		Id: 9501, Name: "msg-only-g", DisplayName: "msg-only-g",
		Status: ProviderGroupStatusEnabled, UsageRatio: 1,
	}).Error)
	// Non-advanced channel whose own route types would otherwise span every
	// route; the membership's route_types must override it.
	require.NoError(t, DB.Create(&Channel{
		Id: 9502, Type: 1, Key: "sk-msg", Status: common.ChannelStatusEnabled,
		Name: "msg-channel", Models: "claude-3-5-sonnet",
	}).Error)
	routeTypes, err := common.Marshal([]string{ProviderRouteTypeMessages})
	require.NoError(t, err)
	require.NoError(t, DB.Create(&ProviderGroupChannel{
		ProviderGroupId: 9501, GroupName: "msg-only-g", ChannelId: 9502,
		RouteTypes: string(routeTypes), Enabled: true,
	}).Error)

	assert.True(t, ProviderGroupChannelSupportsPath("msg-only-g", 9502, "/v1/messages"),
		"member route_types=[messages] must support /v1/messages")
	assert.False(t, ProviderGroupChannelSupportsPath("msg-only-g", 9502, "/v1/responses"),
		"member route_types=[messages] must not support /v1/responses")
}
