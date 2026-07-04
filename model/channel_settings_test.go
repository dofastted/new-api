package model

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelValidateSettingsRejectsNegativeRateLimitRPM(t *testing.T) {
	setting := `{"rate_limit_rpm":-1}`
	channel := &Channel{Setting: &setting}

	err := channel.ValidateSettings()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate_limit_rpm")
}

func TestChannelValidateSettingsAllowsZeroRateLimitRPM(t *testing.T) {
	setting := `{"rate_limit_rpm":0}`
	channel := &Channel{Setting: &setting}

	require.NoError(t, channel.ValidateSettings())
}

func TestChannelSetSettingStoresRateLimitRPM(t *testing.T) {
	channel := &Channel{}

	channel.SetSetting(dto.ChannelSettings{RateLimitRPM: 60})

	require.NotNil(t, channel.Setting)
	assert.Contains(t, *channel.Setting, "rate_limit_rpm")
}
