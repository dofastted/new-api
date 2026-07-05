package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInitDefaultVendorMappingDoesNotPersistInferredVendors(t *testing.T) {
	oldDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&Vendor{}))

	metaMap := map[string]*Model{}
	vendorMap := map[int]*Vendor{}
	initDefaultVendorMapping(metaMap, vendorMap, []AbilityWithChannel{{
		Ability: Ability{Model: "gpt-authority-default-vendor"},
	}})

	var vendorCount int64
	require.NoError(t, db.Model(&Vendor{}).Count(&vendorCount).Error)
	require.Zero(t, vendorCount)

	metadata := metaMap["gpt-authority-default-vendor"]
	require.NotNil(t, metadata)
	require.Less(t, metadata.VendorID, 0)
	require.Equal(t, "OpenAI", vendorMap[metadata.VendorID].Name)
}
