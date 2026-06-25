package model

type ChannelEndpoint struct {
	Id              int    `json:"id"`
	ChannelId       int    `json:"channel_id" gorm:"index;not null"`
	URL             string `json:"url" gorm:"column:url;type:text;not null"`
	Label           string `json:"label" gorm:"type:varchar(128);default:''"`
	SortOrder       int    `json:"sort_order" gorm:"default:0;index"`
	Enabled         bool   `json:"enabled" gorm:"default:true;index"`
	Healthy         bool   `json:"healthy" gorm:"default:true;index"`
	LastProbeStatus string `json:"last_probe_status" gorm:"type:varchar(64);default:''"`
	LastProbeAt     int64  `json:"last_probe_at" gorm:"bigint;default:0"`
	LastProbeError  string `json:"last_probe_error" gorm:"type:text"`
	CreatedAt       int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt       int64  `json:"updated_at" gorm:"bigint"`
}

func GetEnabledChannelEndpoints(channelID int) ([]ChannelEndpoint, error) {
	if channelID <= 0 {
		return nil, nil
	}
	if DB == nil {
		return nil, nil
	}
	var endpoints []ChannelEndpoint
	err := DB.Where("channel_id = ? AND enabled = ? AND healthy = ?", channelID, true, true).
		Order("sort_order ASC, id ASC").
		Find(&endpoints).Error
	return endpoints, err
}
