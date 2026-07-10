package model

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ModelPricingOverrideOriginAdmin         = "admin"
	ModelPricingOverrideOriginLegacyOption  = "legacy_option"
	ModelPricingOverrideOriginModelMetadata = "model_metadata"

	modelPricingOverrideMigrationVersionKey = "ModelPricingOverrideMigrationVersion"
	modelPricingOverrideMigrationVersion    = 1
	PricingRuntimeRevisionOptionKey         = "PricingRuntimeRevision"
)

var loadedPricingRuntimeRevision atomic.Int64

// ModelPricingOverride is the single persisted source of manual model pricing.
// The row's existence means official pricing must not change this model's price.
type ModelPricingOverride struct {
	ID            int64  `json:"id" gorm:"primary_key"`
	ModelName     string `json:"model_name" gorm:"type:varchar(191);uniqueIndex"`
	PricingConfig string `json:"pricing_config" gorm:"type:text"`
	Origin        string `json:"origin" gorm:"type:varchar(32);index"`
	CreatedAt     int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt     int64  `json:"updated_at" gorm:"bigint;index"`
}

func (item *ModelPricingOverride) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if item.CreatedAt == 0 {
		item.CreatedAt = now
	}
	if item.UpdatedAt == 0 {
		item.UpdatedAt = now
	}
	return nil
}

func (item *ModelPricingOverride) BeforeUpdate(_ *gorm.DB) error {
	item.UpdatedAt = common.GetTimestamp()
	return nil
}

func GetAllModelPricingOverrides() ([]ModelPricingOverride, error) {
	var overrides []ModelPricingOverride
	err := DB.Order("model_name ASC").Find(&overrides).Error
	return overrides, err
}

func GetModelPricingOverridesByNames(modelNames []string) (map[string]ModelPricingOverride, error) {
	result := make(map[string]ModelPricingOverride)
	modelNames = normalizeLookupValues(modelNames)
	if len(modelNames) == 0 {
		return result, nil
	}
	var overrides []ModelPricingOverride
	if err := DB.Where("model_name IN ?", modelNames).Find(&overrides).Error; err != nil {
		return nil, err
	}
	for _, item := range overrides {
		result[item.ModelName] = item
	}
	return result, nil
}

func UpsertModelPricingOverrideTx(tx *gorm.DB, modelName string, cfg ModelPricingConfig, origin string) error {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return fmt.Errorf("model name is required")
	}
	if origin == "" {
		origin = ModelPricingOverrideOriginAdmin
	}
	payload, err := common.Marshal(cfg)
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	item := ModelPricingOverride{
		ModelName:     modelName,
		PricingConfig: string(payload),
		Origin:        origin,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "model_name"}},
		DoUpdates: clause.Assignments(map[string]any{
			"pricing_config": item.PricingConfig,
			"origin":         item.Origin,
			"updated_at":     now,
		}),
	}).Create(&item).Error
}

func DeleteModelPricingOverridesTx(tx *gorm.DB, modelNames []string) (int64, error) {
	modelNames = normalizeLookupValues(modelNames)
	if len(modelNames) == 0 {
		return 0, nil
	}
	result := tx.Where("model_name IN ?", modelNames).Delete(&ModelPricingOverride{})
	return result.RowsAffected, result.Error
}

func LoadManualPricingOverridesIntoRuntime() error {
	overrides, err := GetAllModelPricingOverrides()
	if err != nil {
		return err
	}
	entries := make(map[string]ratio_setting.ModelMetadataPricingValues, len(overrides))
	for _, item := range overrides {
		cfg, ok, err := ParseModelPricingConfig(item.PricingConfig)
		if err != nil {
			return fmt.Errorf("parse manual pricing override for %s: %w", item.ModelName, err)
		}
		if !ok {
			continue
		}
		values, hasValues := cfg.metadataValues()
		if hasValues {
			entries[item.ModelName] = values
		}
	}
	ratio_setting.ReplaceModelMetadataPricing(entries)
	return nil
}

func RefreshPricingRuntime() error {
	migrationCompleted, err := modelPricingOverrideMigrationCompleted()
	if err != nil {
		return err
	}
	if migrationCompleted {
		ratio_setting.ResetFallbackPricingToDefaults()
	}
	if err := LoadActiveOfficialPricingIntoRuntime(); err != nil {
		return err
	}
	if err := LoadManualPricingOverridesIntoRuntime(); err != nil {
		return err
	}
	RefreshPricing()
	revision, err := GetPricingRuntimeRevision()
	if err != nil {
		return err
	}
	loadedPricingRuntimeRevision.Store(revision)
	return nil
}

func ReloadPricingRuntimeIfRevisionChanged() error {
	revision, err := GetPricingRuntimeRevision()
	if err != nil {
		return err
	}
	if revision == loadedPricingRuntimeRevision.Load() {
		return nil
	}
	return RefreshPricingRuntime()
}

func GetPricingRuntimeRevision() (int64, error) {
	var option Option
	if err := DB.Where("key = ?", PricingRuntimeRevisionOptionKey).First(&option).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, nil
		}
		return 0, err
	}
	revision, err := strconv.ParseInt(strings.TrimSpace(option.Value), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse pricing runtime revision: %w", err)
	}
	return revision, nil
}

func BumpPricingRuntimeRevisionTx(tx *gorm.DB) (int64, error) {
	revision := time.Now().UnixNano()
	option := Option{Key: PricingRuntimeRevisionOptionKey}
	if err := tx.FirstOrCreate(&option, Option{Key: PricingRuntimeRevisionOptionKey}).Error; err != nil {
		return 0, err
	}
	option.Value = strconv.FormatInt(revision, 10)
	if err := tx.Save(&option).Error; err != nil {
		return 0, err
	}
	return revision, nil
}

func modelPricingOverrideMigrationCompleted() (bool, error) {
	var option Option
	if err := DB.Where("key = ?", modelPricingOverrideMigrationVersionKey).First(&option).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}
	version, err := strconv.Atoi(strings.TrimSpace(option.Value))
	if err != nil {
		return false, nil
	}
	return version >= modelPricingOverrideMigrationVersion, nil
}

func markModelPricingOverrideMigrationCompletedTx(tx *gorm.DB) error {
	option := Option{Key: modelPricingOverrideMigrationVersionKey}
	if err := tx.FirstOrCreate(&option, Option{Key: modelPricingOverrideMigrationVersionKey}).Error; err != nil {
		return err
	}
	option.Value = strconv.Itoa(modelPricingOverrideMigrationVersion)
	return tx.Save(&option).Error
}
