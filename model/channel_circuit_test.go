package model

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetChannelCircuitTestCache(t *testing.T) {
	t.Helper()
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	channelCircuitOnce = sync.Once{}
	channelCircuitCache = nil
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		channelCircuitOnce = sync.Once{}
		channelCircuitCache = nil
	})
}

func TestChannelCircuitThresholdOnePolicyOpensAndPreservesOpenWindow(t *testing.T) {
	resetChannelCircuitTestCache(t)
	policy := ChannelCircuitPolicy{
		Name:                     "claude-high-load",
		FailureThreshold:         1,
		OpenSeconds:              300,
		HalfOpenSuccessThreshold: 2,
	}

	status := RecordChannelCircuitFailure(101, "high_load", policy)

	assert.Equal(t, ChannelCircuitOpen, status.State)
	assert.True(t, IsChannelCircuitOpen(101))
	assert.Equal(t, 1, status.FailureCount)
	assert.Equal(t, "high_load", status.LastCategory)
	assert.Equal(t, "claude-high-load", status.PolicyName)
	assert.Equal(t, 1, status.FailureThreshold)
	assert.Equal(t, 300, status.OpenSeconds)
	assert.Equal(t, 2, status.HalfOpenSuccessThreshold)
	assert.Equal(t, int64(300), status.NextAttemptUnix-status.OpenedAtUnix)

	preservedNextAttempt := time.Now().Add(10 * time.Minute).Unix()
	status.NextAttemptUnix = preservedNextAttempt
	require.NoError(t, getChannelCircuitCache().SetWithTTL(channelCircuitKey(101), status, time.Minute))

	status = RecordChannelCircuitFailure(101, "high_load", policy)
	assert.Equal(t, ChannelCircuitOpen, status.State)
	assert.Equal(t, 2, status.FailureCount)
	assert.Equal(t, preservedNextAttempt, status.NextAttemptUnix)
}

func TestChannelCircuitExpiredOpenBecomesHalfOpen(t *testing.T) {
	resetChannelCircuitTestCache(t)
	policy := ChannelCircuitPolicy{FailureThreshold: 1, OpenSeconds: 300, HalfOpenSuccessThreshold: 2}

	status := RecordChannelCircuitFailure(102, "high_load", policy)
	status.NextAttemptUnix = time.Now().Add(-time.Second).Unix()
	require.NoError(t, getChannelCircuitCache().SetWithTTL(channelCircuitKey(102), status, time.Minute))

	status = GetChannelCircuitStatus(102)
	assert.Equal(t, ChannelCircuitHalfOpen, status.State)
	assert.Equal(t, 0, status.HalfOpenSuccessCount)
	assert.False(t, IsChannelCircuitOpen(102))
}

func TestChannelCircuitHalfOpenSuccessThresholdClosesAfterConfiguredCount(t *testing.T) {
	resetChannelCircuitTestCache(t)
	policy := ChannelCircuitPolicy{FailureThreshold: 1, OpenSeconds: 300, HalfOpenSuccessThreshold: 2}

	status := RecordChannelCircuitFailure(103, "high_load", policy)
	status.State = ChannelCircuitHalfOpen
	status.HalfOpenSuccessCount = 0
	require.NoError(t, getChannelCircuitCache().SetWithTTL(channelCircuitKey(103), status, time.Minute))

	status = RecordChannelCircuitSuccess(103)
	assert.Equal(t, ChannelCircuitHalfOpen, status.State)
	assert.Equal(t, 1, status.HalfOpenSuccessCount)
	assert.Equal(t, 1, status.FailureCount)

	status = RecordChannelCircuitSuccess(103)
	assert.Equal(t, ChannelCircuitClosed, status.State)
	assert.Equal(t, 0, status.FailureCount)
	assert.Equal(t, 0, status.HalfOpenSuccessCount)
}

func TestChannelCircuitHalfOpenFailureReopens(t *testing.T) {
	resetChannelCircuitTestCache(t)
	policy := ChannelCircuitPolicy{FailureThreshold: 1, OpenSeconds: 300, HalfOpenSuccessThreshold: 2}

	status := RecordChannelCircuitFailure(104, "high_load", policy)
	status.State = ChannelCircuitHalfOpen
	status.HalfOpenSuccessCount = 1
	require.NoError(t, getChannelCircuitCache().SetWithTTL(channelCircuitKey(104), status, time.Minute))

	status = RecordChannelCircuitFailure(104, "high_load", policy)
	assert.Equal(t, ChannelCircuitOpen, status.State)
	assert.Equal(t, 2, status.FailureCount)
	assert.Equal(t, 0, status.HalfOpenSuccessCount)
	assert.Equal(t, int64(300), status.NextAttemptUnix-status.OpenedAtUnix)
}

func TestFilterOpenCircuitCachedAbilities(t *testing.T) {
	resetChannelCircuitTestCache(t)
	t.Setenv("CHANNEL_CIRCUIT_FAILURE_THRESHOLD", "1")

	RecordChannelCircuitFailure(201, "bad_response")
	filtered := filterOpenCircuitCachedAbilities([]cachedAbility{{channelID: 201}, {channelID: 202}})
	require.Equal(t, []cachedAbility{{channelID: 202}}, filtered)
}

func TestResetChannelCircuitIgnoresInvalidID(t *testing.T) {
	resetChannelCircuitTestCache(t)
	ResetChannelCircuit(0)
	require.Equal(t, ChannelCircuitClosed, GetChannelCircuitStatus(0).State)
}
