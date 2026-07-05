package model

import (
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
	Id          int            `json:"id"`
	Name        string         `json:"name" gorm:"type:varchar(64);uniqueIndex;not null"`
	DisplayName string         `json:"display_name" gorm:"type:varchar(128);not null"`
	Description string         `json:"description" gorm:"type:text"`
	Status      int            `json:"status" gorm:"default:1;index"`
	UsageRatio  float64        `json:"usage_ratio" gorm:"default:1"`
	IsAuto      bool           `json:"is_auto" gorm:"index"`
	SortOrder   int            `json:"sort_order" gorm:"default:0;index"`
	CreatedTime int64          `json:"created_time" gorm:"bigint"`
	UpdatedTime int64          `json:"updated_time" gorm:"bigint"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
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
	Name        string  `json:"name"`
	DisplayName string  `json:"display_name"`
	Description string  `json:"description"`
	UsageRatio  float64 `json:"usage_ratio"`
	IsAuto      bool    `json:"is_auto"`
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
			Name:        group.Name,
			DisplayName: group.DisplayName,
			Description: group.Description,
			UsageRatio:  group.UsageRatio,
			IsAuto:      group.IsAuto,
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

func RebuildAbilitiesFromProviderGroups() error {
	return rebuildAbilitiesFromProviderGroups()
}

// SyncProviderGroupChannelsForChannel mirrors a channel's Group/Priority into
// provider_group_channels, the single source of truth for routing abilities.
// When syncPriority is true, existing memberships get their priority updated to
// channel.Priority; enabled is always left to the groups page. Stale memberships
// (group no longer in channel.Group) are deleted. No-op when the PGC table is
// absent (legacy deployments keep the channel.Group-driven ability path).
func SyncProviderGroupChannelsForChannel(channel Channel, syncPriority bool) error {
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
			weight := uint(channel.GetWeight())
			toCreate = append(toCreate, ProviderGroupChannel{
				ProviderGroupId: pgID,
				GroupName:       g,
				ChannelId:       channel.Id,
				Priority:        channel.Priority,
				Weight:          &weight,
				RouteTypes:      ProviderRouteTypesForChannel(channel),
				Enabled:         channel.Status == common.ChannelStatusEnabled,
				CreatedTime:     now,
				UpdatedTime:     now,
			})
		} else {
			// Always refresh route_types (derived from channel config); priority
			// only when syncPriority. Enabled stays groups-page authoritative.
			m.RouteTypes = ProviderRouteTypesForChannel(channel)
			if syncPriority {
				m.Priority = channel.Priority
			}
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
			if syncPriority {
				patch["priority"] = m.Priority
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

func rebuildAbilitiesFromProviderGroups() error {
	if DB == nil || !DB.Migrator().HasTable(&ProviderGroupChannel{}) {
		return nil
	}
	var members []ProviderGroupChannel
	if err := DB.Table("provider_group_channels").
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
	if err := DB.Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
		return err
	}
	channelByID := make(map[int]Channel, len(channels))
	for _, channel := range channels {
		channelByID[channel.Id] = channel
	}
	return DB.Transaction(func(tx *gorm.DB) error {
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
			priority := member.Priority
			if priority == nil {
				priority = channel.Priority
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
					Priority:  priority,
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
	})
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

// GetProviderAutoModelGroups returns every enabled provider auto candidate that
// can make a model visible through auto. /v1/models is a capability listing, not
// a relay route, so it must not be limited to the "other" route candidates.
func GetProviderAutoModelGroups() ([]string, error) {
	if !providerGroupTableReady(&ProviderGroupAutoRule{}) {
		return nil, nil
	}
	var rules []ProviderGroupAutoRule
	err := DB.Where("enabled = ?", true).
		Order("sort_order ASC, id ASC").
		Find(&rules).Error
	if err != nil {
		return nil, err
	}
	groups := make([]string, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		group := strings.TrimSpace(rule.CandidateGroup)
		if group == "" {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		groups = append(groups, group)
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
