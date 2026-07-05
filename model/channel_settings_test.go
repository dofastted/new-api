package model

import (
	"net/http"
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

func TestChannelValidateSettingsRejectsInvalidCircuitBreaker(t *testing.T) {
	tests := []struct {
		name             string
		circuitBreaker   dto.ChannelCircuitBreakerSettings
		wantErrorMessage string
	}{
		{
			name: "negative failure threshold names field",
			circuitBreaker: dto.ChannelCircuitBreakerSettings{
				FailureThreshold: -1,
			},
			wantErrorMessage: "circuit_breaker.failure_threshold",
		},
		{
			name: "negative open seconds names field",
			circuitBreaker: dto.ChannelCircuitBreakerSettings{
				OpenSeconds: -1,
			},
			wantErrorMessage: "circuit_breaker.open_seconds",
		},
		{
			name: "negative half open success threshold names field",
			circuitBreaker: dto.ChannelCircuitBreakerSettings{
				HalfOpenSuccessThreshold: -1,
			},
			wantErrorMessage: "circuit_breaker.half_open_success_threshold",
		},
		{
			name: "status code below HTTP range names rule status codes",
			circuitBreaker: dto.ChannelCircuitBreakerSettings{
				Rules: []dto.ChannelCircuitBreakerRule{{StatusCodes: []int{99}}},
			},
			wantErrorMessage: "circuit_breaker.rules[0].status_codes",
		},
		{
			name: "status code above HTTP range names rule status codes",
			circuitBreaker: dto.ChannelCircuitBreakerSettings{
				Rules: []dto.ChannelCircuitBreakerRule{{StatusCodes: []int{600}}},
			},
			wantErrorMessage: "circuit_breaker.rules[0].status_codes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{}
			channel.SetSetting(dto.ChannelSettings{CircuitBreaker: &tt.circuitBreaker})

			err := channel.ValidateSettings()

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrorMessage)
		})
	}
}

func TestChannelValidateSettingsAllowsValidHighLoadCircuitBreaker(t *testing.T) {
	channel := &Channel{}
	channel.SetSetting(dto.ChannelSettings{
		CircuitBreaker: &dto.ChannelCircuitBreakerSettings{
			Enabled:                  true,
			FailureThreshold:         2,
			OpenSeconds:              300,
			HalfOpenSuccessThreshold: 1,
			Rules: []dto.ChannelCircuitBreakerRule{
				{
					Name:        "claude_high_load",
					Class:       "high_load_temporarily_unavailable",
					StatusCodes: []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable},
				},
			},
		},
	})

	require.NoError(t, channel.ValidateSettings())
}

func TestChannelSetSettingStoresRateLimitRPM(t *testing.T) {
	channel := &Channel{}

	channel.SetSetting(dto.ChannelSettings{RateLimitRPM: 60})

	require.NotNil(t, channel.Setting)
	assert.Contains(t, *channel.Setting, "rate_limit_rpm")
}
