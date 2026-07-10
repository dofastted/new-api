package model

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"gorm.io/gorm"
)

const (
	OfficialPricingSnapshotStatusSucceeded = "succeeded"
	OfficialPricingSnapshotStatusFailed    = "failed"
)

// OfficialPricingSnapshot records one successful or failed official pricing sync.
// The active runtime directory only switches after a succeeded snapshot and its
// price rows are committed in the same DB transaction.
type OfficialPricingSnapshot struct {
	ID           int64  `json:"id" gorm:"primary_key"`
	SnapshotID   string `json:"snapshot_id" gorm:"type:varchar(64);uniqueIndex"`
	Status       string `json:"status" gorm:"type:varchar(32);index"`
	Sources      string `json:"sources" gorm:"type:text"`
	EntriesCount int    `json:"entries_count"`
	Error        string `json:"error" gorm:"type:text"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index"`
	ActivatedAt  int64  `json:"activated_at" gorm:"bigint;index"`
}

// OfficialModelPrice is the authoritative token-price directory normalized from
// official provider documentation. Prices are USD per 1M tokens; ratio fields are
// materialized compatibility values for legacy billing paths.
type OfficialModelPrice struct {
	ID                      int64    `json:"id" gorm:"primary_key"`
	SnapshotID              string   `json:"snapshot_id" gorm:"type:varchar(64);index"`
	Provider                string   `json:"provider" gorm:"type:varchar(32);index"`
	ModelName               string   `json:"model_name" gorm:"type:varchar(191);index"`
	SourceURL               string   `json:"source_url" gorm:"type:text"`
	InputUSDPerMTokens      float64  `json:"input_usd_per_m_tokens"`
	OutputUSDPerMTokens     float64  `json:"output_usd_per_m_tokens"`
	CacheReadUSDPerMTokens  *float64 `json:"cache_read_usd_per_m_tokens,omitempty"`
	CacheWriteUSDPerMTokens *float64 `json:"cache_write_usd_per_m_tokens,omitempty"`
	ModelRatio              float64  `json:"model_ratio"`
	CompletionRatio         float64  `json:"completion_ratio"`
	CacheRatio              *float64 `json:"cache_ratio,omitempty"`
	CreateCacheRatio        *float64 `json:"create_cache_ratio,omitempty"`
	Active                  bool     `json:"active" gorm:"index"`
	Stale                   bool     `json:"stale" gorm:"index"`
	LastConfirmedAt         int64    `json:"last_confirmed_at" gorm:"bigint;index"`
	CreatedAt               int64    `json:"created_at" gorm:"bigint;index"`
	UpdatedAt               int64    `json:"updated_at" gorm:"bigint;index"`
}

func (snapshot *OfficialPricingSnapshot) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if snapshot.CreatedAt == 0 {
		snapshot.CreatedAt = now
	}
	if snapshot.ActivatedAt == 0 && snapshot.Status == OfficialPricingSnapshotStatusSucceeded {
		snapshot.ActivatedAt = now
	}
	return nil
}

func (price *OfficialModelPrice) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if price.CreatedAt == 0 {
		price.CreatedAt = now
	}
	if price.UpdatedAt == 0 {
		price.UpdatedAt = now
	}
	return nil
}

func (price *OfficialModelPrice) BeforeUpdate(_ *gorm.DB) error {
	price.UpdatedAt = common.GetTimestamp()
	return nil
}

func NewOfficialPricingSnapshotID() string {
	return fmt.Sprintf("op_%d", time.Now().UnixNano())
}

func ReplaceActiveOfficialPricing(snapshotID string, sources string, prices []OfficialModelPrice) error {
	if len(prices) == 0 {
		return fmt.Errorf("official pricing snapshot has no prices")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		now := common.GetTimestamp()
		snapshot := OfficialPricingSnapshot{
			SnapshotID:   snapshotID,
			Status:       OfficialPricingSnapshotStatusSucceeded,
			Sources:      sources,
			EntriesCount: len(prices),
			ActivatedAt:  now,
		}
		if err := tx.Create(&snapshot).Error; err != nil {
			return err
		}
		if err := tx.Model(&OfficialModelPrice{}).Where("active = ?", true).Updates(map[string]any{"active": false, "updated_at": now}).Error; err != nil {
			return err
		}
		for i := range prices {
			prices[i].SnapshotID = snapshotID
			prices[i].Active = true
			prices[i].CreatedAt = now
			prices[i].UpdatedAt = now
		}
		if err := tx.CreateInBatches(prices, 200).Error; err != nil {
			return err
		}
		if _, err := BumpPricingRuntimeRevisionTx(tx); err != nil {
			return err
		}
		return nil
	})
}

func RecordFailedOfficialPricingSnapshot(snapshotID string, sources string, syncErr error) error {
	message := ""
	if syncErr != nil {
		message = syncErr.Error()
	}
	snapshot := OfficialPricingSnapshot{
		SnapshotID: snapshotID,
		Status:     OfficialPricingSnapshotStatusFailed,
		Sources:    sources,
		Error:      message,
	}
	return DB.Create(&snapshot).Error
}

func GetActiveOfficialPricingRows() ([]OfficialModelPrice, error) {
	var rows []OfficialModelPrice
	err := DB.Where("active = ?", true).Find(&rows).Error
	return rows, err
}

func GetEnabledAbilityModelNames(modelNames []string) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	modelNames = normalizeLookupValues(modelNames)
	if len(modelNames) == 0 {
		return result, nil
	}
	var names []string
	err := DB.Table("abilities").
		Select("DISTINCT abilities.model").
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where("abilities.model IN ? AND abilities.enabled = ? AND channels.status = ?", modelNames, true, common.ChannelStatusEnabled).
		Pluck("abilities.model", &names).Error
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		result[name] = struct{}{}
	}
	return result, nil
}

func LoadActiveOfficialPricingIntoRuntime() error {
	rows, err := GetActiveOfficialPricingRows()
	if err != nil {
		return err
	}
	entries := make(map[string]ratio_setting.OfficialPricingValues, len(rows))
	for _, row := range rows {
		if row.ModelName == "" {
			continue
		}
		entries[row.ModelName] = ratio_setting.OfficialPricingValues{
			ModelRatio:       row.ModelRatio,
			CompletionRatio:  row.CompletionRatio,
			CacheRatio:       row.CacheRatio,
			CreateCacheRatio: row.CreateCacheRatio,
		}
	}
	ratio_setting.ReplaceOfficialPricing(entries, len(entries) > 0)
	InvalidatePricingCache()
	return nil
}
