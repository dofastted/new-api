package model

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
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

func TestChannelCircuitFailureThresholdOpens(t *testing.T) {
	resetChannelCircuitTestCache(t)
	t.Setenv("CHANNEL_CIRCUIT_FAILURE_THRESHOLD", "2")

	RecordChannelCircuitFailure(101, "bad_response")
	require.False(t, IsChannelCircuitOpen(101))

	RecordChannelCircuitFailure(101, "bad_response")
	status := GetChannelCircuitStatus(101)
	require.Equal(t, ChannelCircuitOpen, status.State)
	require.True(t, IsChannelCircuitOpen(101))
	require.Equal(t, 2, status.FailureCount)
}

func TestChannelCircuitOpenDurationAllowsHalfOpen(t *testing.T) {
	resetChannelCircuitTestCache(t)
	t.Setenv("CHANNEL_CIRCUIT_FAILURE_THRESHOLD", "1")
	t.Setenv("CHANNEL_CIRCUIT_OPEN_SECONDS", "1")

	RecordChannelCircuitFailure(102, "bad_response")
	status := GetChannelCircuitStatus(102)
	status.NextAttemptUnix = time.Now().Add(-time.Second).Unix()
	require.NoError(t, getChannelCircuitCache().SetWithTTL(channelCircuitKey(102), status, time.Minute))

	status = GetChannelCircuitStatus(102)
	require.Equal(t, ChannelCircuitHalfOpen, status.State)
}

func TestChannelCircuitSuccessCloses(t *testing.T) {
	resetChannelCircuitTestCache(t)
	t.Setenv("CHANNEL_CIRCUIT_FAILURE_THRESHOLD", "1")

	RecordChannelCircuitFailure(103, "bad_response")
	require.True(t, IsChannelCircuitOpen(103))

	RecordChannelCircuitSuccess(103)
	status := GetChannelCircuitStatus(103)
	require.Equal(t, ChannelCircuitClosed, status.State)
	require.Equal(t, 0, status.FailureCount)
}

func TestFilterOpenCircuitChannelIDs(t *testing.T) {
	resetChannelCircuitTestCache(t)
	t.Setenv("CHANNEL_CIRCUIT_FAILURE_THRESHOLD", "1")

	RecordChannelCircuitFailure(201, "bad_response")
	filtered := filterOpenCircuitChannelIDs([]int{201, 202})
	require.Equal(t, []int{202}, filtered)
}

func TestResetChannelCircuitIgnoresInvalidID(t *testing.T) {
	resetChannelCircuitTestCache(t)
	ResetChannelCircuit(0)
	require.Equal(t, ChannelCircuitClosed, GetChannelCircuitStatus(0).State)
}

func TestChannelCircuitUniqueKeys(t *testing.T) {
	require.Equal(t, "channel:42", channelCircuitKey(42))
	require.Equal(t, "channel:-1", channelCircuitKey(-1))
	require.NotEqual(t, channelCircuitKey(1), fmt.Sprintf("channel:%d", 2))
}
