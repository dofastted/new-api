package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

func TestTryAcquireChannelSlotMemory(t *testing.T) {
	restoreChannelRateLimitTestState(t)

	channelRateLimitNow = func() time.Time { return time.Unix(120, 0) }

	assert.True(t, TryAcquireChannelSlot(10, 2))
	assert.True(t, TryAcquireChannelSlot(10, 2))
	assert.False(t, TryAcquireChannelSlot(10, 2))
	assert.True(t, TryAcquireChannelSlot(10, 0))
}

func TestTryAcquireChannelSlotMinuteBucketRollover(t *testing.T) {
	restoreChannelRateLimitTestState(t)

	now := time.Unix(120, 0)
	channelRateLimitNow = func() time.Time { return now }

	assert.True(t, TryAcquireChannelSlot(10, 1))
	assert.False(t, TryAcquireChannelSlot(10, 1))

	now = now.Add(time.Minute)
	assert.True(t, TryAcquireChannelSlot(10, 1))
	assert.False(t, TryAcquireChannelSlot(10, 1))
}

func TestChannelRateLimitedCooldownMemory(t *testing.T) {
	restoreChannelRateLimitTestState(t)

	now := time.Unix(120, 0)
	channelRateLimitNow = func() time.Time { return now }

	MarkChannelRateLimited(10, 30*time.Second)
	assert.True(t, IsChannelRateLimited(10))

	now = now.Add(31 * time.Second)
	assert.False(t, IsChannelRateLimited(10))
}

func restoreChannelRateLimitTestState(t *testing.T) {
	t.Helper()

	origRedisEnabled := common.RedisEnabled
	origRDB := common.RDB
	origNow := channelRateLimitNow
	common.RedisEnabled = false
	common.RDB = nil
	clearChannelRateLimitMemory()

	t.Cleanup(func() {
		common.RedisEnabled = origRedisEnabled
		common.RDB = origRDB
		channelRateLimitNow = origNow
		clearChannelRateLimitMemory()
	})
}

func clearChannelRateLimitMemory() {
	channelRPMCounters.Range(func(key, value any) bool {
		channelRPMCounters.Delete(key)
		return true
	})
	channelCooldownExpiries.Range(func(key, value any) bool {
		channelCooldownExpiries.Delete(key)
		return true
	})
}
