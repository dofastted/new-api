package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ProviderGroupStatusDisabled = 0
	ProviderGroupStatusEnabled  = 1

	ProviderGroupOfflineMessage = "分组已下线"
)

const (
	ProviderRouteTypeCompletions = "completions"
	ProviderRouteTypeResponses   = "responses"
	ProviderRouteTypeMessages    = "messages"
	ProviderRouteTypeOther       = "other"
)

const (
	ProviderClientFamilyClaudeCode = "claude_code"
	ProviderClientFamilyCodex      = "codex"
)

var reservedUserProviderGroupNames = map[string]struct{}{
	"default": {},
	"premium": {},
	"vip":     {},
}

func ReservedUserProviderGroupNames() []string {
	return []string{"default", "premium", "vip"}
}

func IsReservedUserProviderGroupName(name string) bool {
	_, ok := reservedUserProviderGroupNames[strings.TrimSpace(name)]
	return ok
}

type ProviderGroup struct {
	Id                   int            `json:"id"`
	Name                 string         `json:"name" gorm:"type:varchar(64);uniqueIndex;not null"`
	DisplayName          string         `json:"display_name" gorm:"type:varchar(128);not null"`
	Description          string         `json:"description" gorm:"type:text"`
	Status               int            `json:"status" gorm:"default:1;index"`
	UsageRatio           float64        `json:"usage_ratio" gorm:"default:1"`
	RequiredClientFamily string         `json:"required_client_family,omitempty" gorm:"type:varchar(32)"`
	IsAuto               bool           `json:"is_auto" gorm:"index"`
	SortOrder            int            `json:"sort_order" gorm:"default:0;index"`
	CreatedTime          int64          `json:"created_time" gorm:"bigint"`
	UpdatedTime          int64          `json:"updated_time" gorm:"bigint"`
	DeletedAt            gorm.DeletedAt `json:"-" gorm:"index"`
}

type ProviderGroupChannel struct {
	Id              int    `json:"id"`
	ProviderGroupId int    `json:"provider_group_id" gorm:"index;not null"`
	GroupName       string `json:"group_name" gorm:"type:varchar(64);index;not null"`
	ChannelId       int    `json:"channel_id" gorm:"index;not null"`
	Priority        *int64 `json:"priority" gorm:"bigint;default:0;index"`
	Weight          *uint  `json:"weight" gorm:"default:0;index"`
	RouteTypes      string `json:"route_types" gorm:"type:text"`
	Enabled         bool   `json:"enabled" gorm:"default:true;index"`
	SortOrder       int    `json:"sort_order" gorm:"default:0;index"`
	CreatedTime     int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime     int64  `json:"updated_time" gorm:"bigint"`
}

type ProviderGroupAutoRule struct {
	Id             int    `json:"id"`
	RouteType      string `json:"route_type" gorm:"type:varchar(32);index;not null"`
	CandidateGroup string `json:"candidate_group" gorm:"type:varchar(64);index;not null"`
	SortOrder      int    `json:"sort_order" gorm:"default:0;index"`
	Enabled        bool   `json:"enabled" gorm:"default:true;index"`
	CreatedTime    int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime    int64  `json:"updated_time" gorm:"bigint"`
}

type ProviderGroupOption struct {
	Name                 string  `json:"name"`
	DisplayName          string  `json:"display_name"`
	Description          string  `json:"description"`
	UsageRatio           float64 `json:"usage_ratio"`
	RequiredClientFamily string  `json:"required_client_family,omitempty"`
	IsAuto               bool    `json:"is_auto"`
}

func EnsureProviderGroupsSeededFromLegacy() error {
	if DB == nil || !DB.Migrator().HasTable(&ProviderGroup{}) {
		return nil
	}
	var count int64
	if err := DB.Model(&ProviderGroup{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return seedProviderGroupsFromLegacy()
}

func seedProviderGroupsFromLegacy() error {
	now := common.GetTimestamp()
	return DB.Transaction(func(tx *gorm.DB) error {
		legacyRatios := ratio_setting.GetGroupRatioCopy()
		legacyRatios["auto"] = 1
		groups := make([]ProviderGroup, 0, len(legacyRatios))
		for name, ratio := range legacyRatios {
			name = strings.TrimSpace(name)
			if name == "" || IsReservedUserProviderGroupName(name) {
				continue
			}
			groups = append(groups, ProviderGroup{
				Name:        name,
				DisplayName: name,
				Status:      ProviderGroupStatusEnabled,
				UsageRatio:  ratio,
				IsAuto:      name == "auto",
				CreatedTime: now,
				UpdatedTime: now,
			})
		}
		if len(groups) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&groups).Error; err != nil {
				return err
			}
		}

		var channels []Channel
		if err := tx.Find(&channels).Error; err != nil {
			return err
		}
		members := make([]ProviderGroupChannel, 0)
		for _, channel := range channels {
			for _, groupName := range strings.Split(channel.Group, ",") {
				groupName = strings.TrimSpace(groupName)
				if groupName == "" || IsReservedUserProviderGroupName(groupName) {
					continue
				}
				providerGroupID, err := getProviderGroupID(tx, groupName)
				if err != nil {
					return err
				}
				weight := uint(channel.GetWeight())
				members = append(members, ProviderGroupChannel{
					ProviderGroupId: providerGroupID,
					GroupName:       groupName,
					ChannelId:       channel.Id,
					Priority:        channel.Priority,
					Weight:          &weight,
					RouteTypes:      ProviderRouteTypesForChannel(channel),
					Enabled:         channel.Status == common.ChannelStatusEnabled,
					CreatedTime:     now,
					UpdatedTime:     now,
				})
			}
		}
		for _, chunk := range chunkProviderGroupChannels(members, 50) {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error; err != nil {
				return err
			}
		}

		autoRules := make([]ProviderGroupAutoRule, 0)
		for index, groupName := range setting.GetAutoGroups() {
			groupName = strings.TrimSpace(groupName)
			if groupName == "" || IsReservedUserProviderGroupName(groupName) {
				continue
			}
			for _, routeType := range []string{ProviderRouteTypeCompletions, ProviderRouteTypeResponses, ProviderRouteTypeMessages, ProviderRouteTypeOther} {
				autoRules = append(autoRules, ProviderGroupAutoRule{
					RouteType:      routeType,
					CandidateGroup: groupName,
					SortOrder:      index,
					Enabled:        true,
					CreatedTime:    now,
					UpdatedTime:    now,
				})
			}
		}
		if len(autoRules) > 0 {
			return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&autoRules).Error
		}
		return nil
	})
}

func getProviderGroupID(tx *gorm.DB, name string) (int, error) {
	name = strings.TrimSpace(name)
	if name == "" || IsReservedUserProviderGroupName(name) {
		return 0, gorm.ErrRecordNotFound
	}
	var group ProviderGroup
	if err := tx.Where("name = ?", name).First(&group).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			return 0, err
		}
		now := common.GetTimestamp()
		group = ProviderGroup{
			Name:        name,
			DisplayName: name,
			Status:      ProviderGroupStatusEnabled,
			UsageRatio:  ratio_setting.GetGroupRatio(name),
			CreatedTime: now,
			UpdatedTime: now,
		}
		if createErr := tx.Create(&group).Error; createErr != nil {
			return 0, createErr
		}
	}
	return group.Id, nil
}

func chunkProviderGroupChannels(items []ProviderGroupChannel, size int) [][]ProviderGroupChannel {
	if len(items) == 0 {
		return nil
	}
	chunks := make([][]ProviderGroupChannel, 0, (len(items)+size-1)/size)
	for start := 0; start < len(items); start += size {
		end := start + size
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[start:end])
	}
	return chunks
}

func defaultProviderRouteTypesJSON() string {
	data, err := common.Marshal([]string{
		ProviderRouteTypeCompletions,
		ProviderRouteTypeResponses,
		ProviderRouteTypeMessages,
		ProviderRouteTypeOther,
	})
	if err != nil {
		return "[]"
	}
	return string(data)
}

func ProviderRouteTypesForChannel(channel Channel) string {
	routeTypes := ProviderRouteTypesForChannelValue(channel)
	data, err := common.Marshal(routeTypes)
	if err != nil {
		return defaultProviderRouteTypesJSON()
	}
	return string(data)
}

func ProviderRouteTypesForChannelValue(channel Channel) []string {
	if channel.Type != constant.ChannelTypeAdvancedCustom {
		return []string{
			ProviderRouteTypeCompletions,
			ProviderRouteTypeResponses,
			ProviderRouteTypeMessages,
			ProviderRouteTypeOther,
		}
	}
	settings := channel.GetOtherSettings()
	if settings.AdvancedCustom == nil || len(settings.AdvancedCustom.Routes) == 0 {
		return []string{
			ProviderRouteTypeCompletions,
			ProviderRouteTypeResponses,
			ProviderRouteTypeMessages,
			ProviderRouteTypeOther,
		}
	}
	seen := make(map[string]struct{}, 4)
	for _, route := range settings.AdvancedCustom.Routes {
		routeType := ProviderRouteTypeForPath(route.IncomingPath)
		seen[routeType] = struct{}{}
	}
	ordered := []string{ProviderRouteTypeCompletions, ProviderRouteTypeResponses, ProviderRouteTypeMessages, ProviderRouteTypeOther}
	result := make([]string, 0, len(seen))
	for _, routeType := range ordered {
		if _, ok := seen[routeType]; ok {
			result = append(result, routeType)
		}
	}
	if len(result) == 0 {
		return ordered
	}
	return result
}

func ListOnlineProviderGroupOptions() ([]ProviderGroupOption, error) {
	groups, err := ListOnlineProviderGroups()
	if err != nil {
		return nil, err
	}
	options := make([]ProviderGroupOption, 0, len(groups))
	for _, group := range groups {
		options = append(options, ProviderGroupOption{
			Name:                 group.Name,
			DisplayName:          group.DisplayName,
			Description:          group.Description,
			UsageRatio:           group.UsageRatio,
			RequiredClientFamily: group.RequiredClientFamily,
			IsAuto:               group.IsAuto,
		})
	}
	return options, nil
}

func providerGroupTableReady(modelValue interface{}) bool {
	return DB != nil && DB.Migrator().HasTable(modelValue)
}

func ListOnlineProviderGroups() ([]ProviderGroup, error) {
	if !providerGroupTableReady(&ProviderGroup{}) {
		legacyRatios := ratio_setting.GetGroupRatioCopy()
		legacyRatios["auto"] = 1
		groups := make([]ProviderGroup, 0, len(legacyRatios))
		for name, ratio := range legacyRatios {
			name = strings.TrimSpace(name)
			if name == "" || IsReservedUserProviderGroupName(name) {
				continue
			}
			groups = append(groups, ProviderGroup{
				Name:        name,
				DisplayName: name,
				Status:      ProviderGroupStatusEnabled,
				UsageRatio:  ratio,
				IsAuto:      name == "auto",
			})
		}
		return groups, nil
	}
	var groups []ProviderGroup
	err := DB.Where("status = ? AND name NOT IN ?", ProviderGroupStatusEnabled, ReservedUserProviderGroupNames()).
		Order("sort_order ASC, id ASC").
		Find(&groups).Error
	return groups, err
}

func IsProviderGroupOnline(name string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return true, nil
	}
	if !providerGroupTableReady(&ProviderGroup{}) {
		return true, nil
	}
	var count int64
	err := DB.Model(&ProviderGroup{}).
		Where("name = ? AND status = ?", name, ProviderGroupStatusEnabled).
		Count(&count).Error
	return count > 0, err
}

func ProviderGroupUsageRatio(name string) (float64, bool) {
	name = strings.TrimSpace(name)
	if name == "" || !providerGroupTableReady(&ProviderGroup{}) {
		return 1, false
	}
	var group ProviderGroup
	err := DB.Where("name = ? AND status = ?", name, ProviderGroupStatusEnabled).First(&group).Error
	if err != nil {
		return 1, false
	}
	if group.UsageRatio == 0 {
		return 1, true
	}
	return group.UsageRatio, true
}

func ProviderGroupRequiredClientFamily(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	if providerGroupTableReady(&ProviderGroup{}) {
		var group ProviderGroup
		if err := DB.Select("required_client_family").Where("name = ? AND status = ?", name, ProviderGroupStatusEnabled).First(&group).Error; err == nil {
			family := strings.TrimSpace(group.RequiredClientFamily)
			if family != "" {
				return family, true
			}
		}
	}
	return inferProviderGroupRequiredClientFamily(name)
}

func inferProviderGroupRequiredClientFamily(name string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(normalized, "claude-max"):
		return ProviderClientFamilyClaudeCode, true
	case normalized == "codex-pro" || strings.HasPrefix(normalized, "codex-pro-"):
		return ProviderClientFamilyCodex, true
	default:
		return "", false
	}
}

func RebuildAbilitiesFromProviderGroups() error {
	if DB == nil || !DB.Migrator().HasTable(&ProviderGroupChannel{}) {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return rebuildAbilitiesFromProviderGroups(tx)
	})
}

// ProviderGroupMetadataUpdate is the metadata half of a unified group save.
// Fields map 1:1 to the editable provider-group detail form.
type ProviderGroupMetadataUpdate struct {
	DisplayName          string  `json:"display_name"`
	Description          string  `json:"description"`
	Status               int     `json:"status"`
	UsageRatio           float64 `json:"usage_ratio"`
	RequiredClientFamily string  `json:"required_client_family,omitempty"`
	SortOrder            *int    `json:"sort_order,omitempty"`
}

// ProviderGroupConfigurationUpdate is the transactional save payload for the
// groups page. Omitting Metadata or Members leaves that side unchanged;
// Members set to an empty slice clears all providers in the group.
type ProviderGroupConfigurationUpdate struct {
	Metadata *ProviderGroupMetadataUpdate `json:"metadata,omitempty"`
	Members  *[]ProviderGroupChannel      `json:"members,omitempty"`
}

// ProviderGroupChannelInfo contains only the channel fields required by the
// provider-group membership editor.
type ProviderGroupChannelInfo struct {
	Id     int    `json:"id"`
	Type   int    `json:"type"`
	Status int    `json:"status"`
	Name   string `json:"name"`
	Models string `json:"models"`
}

// ProviderGroupChannelDetail keeps membership routing state and its display
// metadata together so the editor never depends on a truncated channel page.
type ProviderGroupChannelDetail struct {
	ProviderGroupChannel
	Channel ProviderGroupChannelInfo `json:"channel"`
}

// ProviderGroupConfigurationResult is returned after a successful unified save.
type ProviderGroupConfigurationResult struct {
	Group   ProviderGroup                `json:"group"`
	Members []ProviderGroupChannelDetail `json:"members,omitempty"`
}

func ListProviderGroupChannelDetails(id int) ([]ProviderGroupChannelDetail, error) {
	return listProviderGroupChannelDetails(DB, id)
}

func listProviderGroupChannelDetails(tx *gorm.DB, id int) ([]ProviderGroupChannelDetail, error) {
	var members []ProviderGroupChannel
	if err := tx.Where("provider_group_id = ?", id).
		Order("sort_order ASC, id ASC").
		Find(&members).Error; err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return []ProviderGroupChannelDetail{}, nil
	}

	channelIDs := make([]int, 0, len(members))
	for _, member := range members {
		channelIDs = append(channelIDs, member.ChannelId)
	}
	var channels []ProviderGroupChannelInfo
	if err := tx.Model(&Channel{}).
		Select("id", "type", "status", "name", "models").
		Where("id IN ?", channelIDs).
		Scan(&channels).Error; err != nil {
		return nil, err
	}
	channelByID := make(map[int]ProviderGroupChannelInfo, len(channels))
	for _, channel := range channels {
		channelByID[channel.Id] = channel
	}

	details := make([]ProviderGroupChannelDetail, 0, len(members))
	for _, member := range members {
		channel, ok := channelByID[member.ChannelId]
		if !ok {
			return nil, fmt.Errorf("channel %d not found", member.ChannelId)
		}
		details = append(details, ProviderGroupChannelDetail{
			ProviderGroupChannel: member,
			Channel:              channel,
		})
	}
	return details, nil
}

// ApplyProviderGroupConfiguration updates metadata, memberships, and derived
// database routing state in one transaction. The in-memory cache refreshes
// only after the transaction commits. Either side may be omitted.
func ApplyProviderGroupConfiguration(id int, update ProviderGroupConfigurationUpdate) (*ProviderGroupConfigurationResult, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid provider group id")
	}
	if update.Metadata == nil && update.Members == nil {
		return nil, fmt.Errorf("no configuration changes provided")
	}

	var group ProviderGroup
	if err := DB.First(&group, id).Error; err != nil {
		return nil, err
	}

	// Snapshot channels that currently belong to this group so removals still
	// get their legacy channels.group field resynced after the transaction.
	var previousChannelIDs []int
	if err := DB.Model(&ProviderGroupChannel{}).
		Where("provider_group_id = ?", id).
		Pluck("channel_id", &previousChannelIDs).Error; err != nil {
		return nil, err
	}

	membersChanged := update.Members != nil
	now := common.GetTimestamp()
	var preparedMembers []ProviderGroupChannel
	affectedChannelIDs := append([]int{}, previousChannelIDs...)

	// Precompute metadata fields outside the transaction so a rolled-back
	// write never leaves statusChanged=true for a post-commit rebuild.
	var metadataUpdates map[string]interface{}
	statusChanged := false
	if update.Metadata != nil {
		displayName := strings.TrimSpace(update.Metadata.DisplayName)
		if displayName == "" {
			displayName = group.Name
		}
		usageRatio := update.Metadata.UsageRatio
		if usageRatio == 0 {
			usageRatio = 1
		}
		status := update.Metadata.Status
		if status != ProviderGroupStatusEnabled && status != ProviderGroupStatusDisabled {
			return nil, fmt.Errorf("invalid provider group status")
		}
		statusChanged = status != group.Status
		metadataUpdates = map[string]interface{}{
			"display_name":           displayName,
			"description":            update.Metadata.Description,
			"status":                 status,
			"usage_ratio":            usageRatio,
			"required_client_family": update.Metadata.RequiredClientFamily,
			"updated_time":           now,
		}
		if update.Metadata.SortOrder != nil {
			metadataUpdates["sort_order"] = *update.Metadata.SortOrder
		}
	}

	if update.Members != nil {
		channelIDs := make([]int, 0, len(*update.Members))
		seenChannel := make(map[int]struct{}, len(*update.Members))
		for _, item := range *update.Members {
			if item.ChannelId <= 0 {
				return nil, fmt.Errorf("invalid channel id in membership")
			}
			if _, exists := seenChannel[item.ChannelId]; exists {
				return nil, fmt.Errorf("duplicate channel id in membership: %d", item.ChannelId)
			}
			seenChannel[item.ChannelId] = struct{}{}
			channelIDs = append(channelIDs, item.ChannelId)
			affectedChannelIDs = append(affectedChannelIDs, item.ChannelId)
		}

		channelsByID := make(map[int]Channel, len(channelIDs))
		if len(channelIDs) > 0 {
			var channels []Channel
			if err := DB.Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
				return nil, err
			}
			for _, channel := range channels {
				channelsByID[channel.Id] = channel
			}
			for _, channelID := range channelIDs {
				if _, ok := channelsByID[channelID]; !ok {
					return nil, fmt.Errorf("channel %d not found", channelID)
				}
			}
		}

		preparedMembers = make([]ProviderGroupChannel, 0, len(*update.Members))
		for index, item := range *update.Members {
			member := item
			member.Id = 0
			member.ProviderGroupId = id
			member.GroupName = group.Name
			if channel, ok := channelsByID[member.ChannelId]; ok {
				member.RouteTypes = ProviderRouteTypesForChannel(channel)
			} else {
				member.RouteTypes = ""
			}
			if member.Priority == nil {
				priority := int64(len(*update.Members) - index)
				member.Priority = &priority
			}
			member.SortOrder = index
			member.Enabled = true
			member.UpdatedTime = now
			if member.CreatedTime == 0 {
				member.CreatedTime = now
			}
			preparedMembers = append(preparedMembers, member)
		}
	}

	result := &ProviderGroupConfigurationResult{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if metadataUpdates != nil {
			if err := tx.Model(&ProviderGroup{}).Where("id = ?", id).Updates(metadataUpdates).Error; err != nil {
				return err
			}
		}

		if update.Members != nil {
			if err := tx.Where("provider_group_id = ?", id).Delete(&ProviderGroupChannel{}).Error; err != nil {
				return err
			}
			if len(preparedMembers) > 0 {
				if err := tx.Create(&preparedMembers).Error; err != nil {
					return err
				}
			}
		}

		if statusChanged || membersChanged {
			if err := rebuildAbilitiesFromProviderGroups(tx); err != nil {
				return err
			}
			if membersChanged {
				if err := syncChannelGroupsFromProviderGroupChannels(tx, affectedChannelIDs); err != nil {
					return err
				}
			}
		}

		if err := tx.First(&result.Group, id).Error; err != nil {
			return err
		}
		if update.Members != nil {
			members, err := listProviderGroupChannelDetails(tx, id)
			if err != nil {
				return err
			}
			result.Members = members
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if statusChanged || membersChanged {
		InitChannelCache()
	}
	return result, nil
}

// SyncProviderGroupChannelsForChannel mirrors a channel's routing groups into
// provider_group_channels. Membership priority and enabled state remain owned
// by the provider groups page. No-op when the PGC table is absent.
func SyncProviderGroupChannelsForChannel(channel Channel) error {
	if !providerGroupTableReady(&ProviderGroupChannel{}) {
		return nil
	}
	routingGroups := make([]string, 0)
	for _, g := range channel.GetGroups() {
		if !IsReservedUserProviderGroupName(g) {
			routingGroups = append(routingGroups, g)
		}
	}
	routingSet := make(map[string]struct{}, len(routingGroups))
	for _, g := range routingGroups {
		routingSet[g] = struct{}{}
	}

	var existing []ProviderGroupChannel
	if err := DB.Where("channel_id = ?", channel.Id).Find(&existing).Error; err != nil {
		return err
	}
	existingByName := make(map[string]ProviderGroupChannel, len(existing))
	for _, m := range existing {
		existingByName[m.GroupName] = m
	}

	now := common.GetTimestamp()
	toCreate := make([]ProviderGroupChannel, 0)
	toUpdate := make([]ProviderGroupChannel, 0)
	for _, g := range routingGroups {
		m, ok := existingByName[g]
		if !ok {
			pgID, err := getProviderGroupID(DB, g)
			if err != nil {
				continue
			}
			priority := int64(0)
			weight := uint(channel.GetWeight())
			toCreate = append(toCreate, ProviderGroupChannel{
				ProviderGroupId: pgID,
				GroupName:       g,
				ChannelId:       channel.Id,
				Priority:        &priority,
				Weight:          &weight,
				RouteTypes:      ProviderRouteTypesForChannel(channel),
				Enabled:         channel.Status == common.ChannelStatusEnabled,
				CreatedTime:     now,
				UpdatedTime:     now,
			})
		} else {
			// route_types is derived from channel config. Priority and enabled
			// remain provider-group membership settings.
			m.RouteTypes = ProviderRouteTypesForChannel(channel)
			m.UpdatedTime = now
			toUpdate = append(toUpdate, m)
		}
	}
	toDelete := make([]int, 0)
	for _, m := range existing {
		if _, ok := routingSet[m.GroupName]; !ok {
			toDelete = append(toDelete, m.Id)
		}
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if len(toCreate) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&toCreate).Error; err != nil {
				return err
			}
		}
		for _, m := range toUpdate {
			patch := map[string]interface{}{
				"route_types":  m.RouteTypes,
				"updated_time": m.UpdatedTime,
			}
			if err := tx.Model(&ProviderGroupChannel{}).Where("id = ?", m.Id).
				Updates(patch).Error; err != nil {
				return err
			}
		}
		if len(toDelete) > 0 {
			if err := tx.Where("id IN ?", toDelete).Delete(&ProviderGroupChannel{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// SyncChannelGroupsFromProviderGroupChannels mirrors provider-group memberships
// back to channels.group for legacy display and APIs. Routing remains derived
// from provider_group_channels via RebuildAbilitiesFromProviderGroups.
func SyncChannelGroupsFromProviderGroupChannels(channelIDs []int) error {
	if DB == nil || !DB.Migrator().HasTable(&ProviderGroupChannel{}) || len(channelIDs) == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return syncChannelGroupsFromProviderGroupChannels(tx, channelIDs)
	})
}

func syncChannelGroupsFromProviderGroupChannels(tx *gorm.DB, channelIDs []int) error {
	seenIDs := make(map[int]struct{}, len(channelIDs))
	uniqueIDs := make([]int, 0, len(channelIDs))
	for _, id := range channelIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seenIDs[id]; ok {
			continue
		}
		seenIDs[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return nil
	}

	var members []ProviderGroupChannel
	if err := tx.Table("provider_group_channels").
		Select("provider_group_channels.*").
		Joins("JOIN provider_groups ON provider_groups.id = provider_group_channels.provider_group_id").
		Where("provider_group_channels.channel_id IN ? AND provider_groups.deleted_at IS NULL", uniqueIDs).
		Find(&members).Error; err != nil {
		return err
	}
	groupsByChannel := make(map[int][]string, len(uniqueIDs))
	seenGroupByChannel := make(map[int]map[string]struct{}, len(uniqueIDs))
	for _, member := range members {
		groupName := strings.TrimSpace(member.GroupName)
		if groupName == "" {
			continue
		}
		if seenGroupByChannel[member.ChannelId] == nil {
			seenGroupByChannel[member.ChannelId] = make(map[string]struct{})
		}
		if _, ok := seenGroupByChannel[member.ChannelId][groupName]; ok {
			continue
		}
		seenGroupByChannel[member.ChannelId][groupName] = struct{}{}
		groupsByChannel[member.ChannelId] = append(groupsByChannel[member.ChannelId], groupName)
	}

	for _, channelID := range uniqueIDs {
		groups := groupsByChannel[channelID]
		sort.Strings(groups)
		if err := tx.Model(&Channel{}).Where("id = ?", channelID).Update("group", strings.Join(groups, ",")).Error; err != nil {
			return err
		}
	}
	return nil
}

func rebuildAbilitiesFromProviderGroups(tx *gorm.DB) error {
	if tx == nil || !tx.Migrator().HasTable(&ProviderGroupChannel{}) {
		return nil
	}
	var members []ProviderGroupChannel
	if err := tx.Table("provider_group_channels").
		Select("provider_group_channels.*").
		Joins("JOIN provider_groups ON provider_groups.id = provider_group_channels.provider_group_id").
		Where("provider_group_channels.enabled = ? AND provider_groups.status = ? AND provider_groups.deleted_at IS NULL", true, ProviderGroupStatusEnabled).
		Find(&members).Error; err != nil {
		return err
	}
	// Always clear the derived abilities table, even when no enabled members
	// remain, so disabling the last group / member removes stale routing.
	channelIDs := make([]int, 0, len(members))
	for _, member := range members {
		channelIDs = append(channelIDs, member.ChannelId)
	}
	var channels []Channel
	if err := tx.Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
		return err
	}
	channelByID := make(map[int]Channel, len(channels))
	for _, channel := range channels {
		channelByID[channel.Id] = channel
	}
	if err := tx.Where("1 = 1").Delete(&Ability{}).Error; err != nil {
		return err
	}
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0)
	for _, member := range members {
		channel, ok := channelByID[member.ChannelId]
		if !ok {
			continue
		}
		priority := int64(0)
		if member.Priority != nil {
			priority = *member.Priority
		}
		weight := uint(channel.GetWeight())
		if member.Weight != nil {
			weight = *member.Weight
		}
		for _, modelName := range strings.Split(channel.Models, ",") {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}
			key := member.GroupName + "|" + modelName + "|" + strconv.Itoa(channel.Id)
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			abilities = append(abilities, Ability{
				Group:     member.GroupName,
				Model:     modelName,
				ChannelId: channel.Id,
				Enabled:   channel.Status == common.ChannelStatusEnabled && member.Enabled,
				Priority:  &priority,
				Weight:    weight,
				Tag:       channel.Tag,
			})
		}
	}
	for _, chunk := range chunkAbilities(abilities, 50) {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error; err != nil {
			return err
		}
	}
	return nil
}

func chunkAbilities(items []Ability, size int) [][]Ability {
	if len(items) == 0 {
		return nil
	}
	chunks := make([][]Ability, 0, (len(items)+size-1)/size)
	for start := 0; start < len(items); start += size {
		end := start + size
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[start:end])
	}
	return chunks
}

func GetProviderAutoGroups(routeType string) ([]string, error) {
	if !providerGroupTableReady(&ProviderGroupAutoRule{}) {
		return nil, nil
	}
	var rules []ProviderGroupAutoRule
	err := DB.Where("route_type = ? AND enabled = ?", routeType, true).
		Order("sort_order ASC, id ASC").
		Find(&rules).Error
	if err != nil {
		return nil, err
	}
	groups := make([]string, 0, len(rules))
	for _, rule := range rules {
		if strings.TrimSpace(rule.CandidateGroup) != "" {
			groups = append(groups, rule.CandidateGroup)
		}
	}
	return groups, nil
}

func ProviderRouteTypeForPath(requestPath string) string {
	path := strings.Split(requestPath, "?")[0]
	switch {
	case path == "/v1/chat/completions" || strings.HasPrefix(path, "/v1/chat/completions/") ||
		path == "/v1/completions" || strings.HasPrefix(path, "/v1/completions/"):
		return ProviderRouteTypeCompletions
	case path == "/v1/responses" || strings.HasPrefix(path, "/v1/responses/"):
		return ProviderRouteTypeResponses
	case path == "/v1/messages" || strings.HasPrefix(path, "/v1/messages/"):
		return ProviderRouteTypeMessages
	default:
		return ProviderRouteTypeOther
	}
}

func ProviderGroupChannelSupportsPath(groupName string, channelID int, requestPath string) bool {
	if groupName == "" || channelID <= 0 || requestPath == "" {
		return true
	}
	if !providerGroupTableReady(&ProviderGroupChannel{}) {
		return true
	}
	var member ProviderGroupChannel
	err := DB.Where("group_name = ? AND channel_id = ? AND enabled = ?", groupName, channelID, true).
		First(&member).Error
	if err != nil {
		return true
	}
	if strings.TrimSpace(member.RouteTypes) != "" {
		return providerRouteTypesContain(member.RouteTypes, ProviderRouteTypeForPath(requestPath))
	}
	var channel Channel
	if err := DB.First(&channel, channelID).Error; err != nil {
		return true
	}
	return providerRouteTypesContain(ProviderRouteTypesForChannel(channel), ProviderRouteTypeForPath(requestPath))
}

func providerRouteTypesContain(routeTypesJSON string, routeType string) bool {
	if strings.TrimSpace(routeTypesJSON) == "" {
		return true
	}
	var routeTypes []string
	if err := common.Unmarshal([]byte(routeTypesJSON), &routeTypes); err != nil {
		return true
	}
	if len(routeTypes) == 0 {
		return true
	}
	for _, item := range routeTypes {
		if item == routeType {
			return true
		}
	}
	return false
}
