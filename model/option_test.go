package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateOptionLeavesRuntimeValueUntouchedWhenPersistenceFails(t *testing.T) {
	// This package has no parallel tests; restore both process globals so the
	// temporary closed database cannot affect another test.
	oldDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))

	common.OptionMapRWMutex.Lock()
	oldOptionMap := common.OptionMap
	common.OptionMap = map[string]string{"test.persistence.failure": "persisted-runtime-value"}
	common.OptionMapRWMutex.Unlock()
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	err = UpdateOption("test.persistence.failure", "new-runtime-value")
	require.Error(t, err)

	common.OptionMapRWMutex.RLock()
	value, exists := common.OptionMap["test.persistence.failure"]
	common.OptionMapRWMutex.RUnlock()
	assert.True(t, exists)
	assert.Equal(t, "persisted-runtime-value", value)
}
