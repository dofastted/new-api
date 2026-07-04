package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	channelRPMRedisKeyPrefix      = "channel_rpm"
	channelCooldownRedisKeyPrefix = "channel_rpm_cooldown"
	channelRateLimitBusyMessage   = "We're experiencing high demand right now. Please retry in a moment."
)

type memoryChannelRPMCounter struct {
	count  atomic.Int64
	expire atomic.Int64
}

var (
	channelRateLimitNow = time.Now

	channelRPMCounters      sync.Map
	channelCooldownExpiries sync.Map
)

func TryAcquireChannelSlot(channelId int, rpm int) bool {
	if rpm <= 0 {
		return true
	}
	if channelId <= 0 {
		return true
	}
	if common.RedisEnabled && common.RDB != nil {
		ok, err := tryAcquireRedisChannelSlot(channelId, rpm)
		if err == nil {
			return ok
		}
		common.SysLog(fmt.Sprintf("failed to acquire redis channel rpm slot: channel_id=%d, error=%v", channelId, err))
	}
	return tryAcquireMemoryChannelSlot(channelId, rpm)
}

func MarkChannelRateLimited(channelId int, cooldown time.Duration) {
	if channelId <= 0 || cooldown <= 0 {
		return
	}
	if common.RedisEnabled && common.RDB != nil {
		key := channelCooldownRedisKey(channelId)
		if err := common.RDB.Set(context.Background(), key, "1", cooldown).Err(); err == nil {
			return
		} else {
			common.SysLog(fmt.Sprintf("failed to mark redis channel cooldown: channel_id=%d, error=%v", channelId, err))
		}
	}
	channelCooldownExpiries.Store(channelId, channelRateLimitNow().Add(cooldown).UnixNano())
}

func IsChannelRateLimited(channelId int) bool {
	if channelId <= 0 {
		return false
	}
	if common.RedisEnabled && common.RDB != nil {
		exists, err := common.RDB.Exists(context.Background(), channelCooldownRedisKey(channelId)).Result()
		if err == nil {
			return exists > 0
		}
		common.SysLog(fmt.Sprintf("failed to check redis channel cooldown: channel_id=%d, error=%v", channelId, err))
	}
	value, ok := channelCooldownExpiries.Load(channelId)
	if !ok {
		return false
	}
	expireAt, ok := value.(int64)
	if !ok || channelRateLimitNow().UnixNano() >= expireAt {
		channelCooldownExpiries.Delete(channelId)
		return false
	}
	return true
}

func ChannelSlotAvailable(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	if IsChannelRateLimited(channel.Id) {
		return false
	}
	return TryAcquireChannelSlot(channel.Id, channel.GetSetting().RateLimitRPM)
}

func ChannelRateLimitBusyMessage() string {
	return channelRateLimitBusyMessage
}

func tryAcquireRedisChannelSlot(channelId int, rpm int) (bool, error) {
	ctx := context.Background()
	key := channelRPMRedisKey(channelId, channelRateLimitNow())
	count, err := common.RDB.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	_, _ = common.RDB.Expire(ctx, key, 120*time.Second).Result()
	if count <= int64(rpm) {
		return true, nil
	}
	_, _ = common.RDB.Decr(ctx, key).Result()
	return false, nil
}

func tryAcquireMemoryChannelSlot(channelId int, rpm int) bool {
	now := channelRateLimitNow()
	key := channelRPMMemoryKey(channelId, now)
	value, _ := channelRPMCounters.LoadOrStore(key, newMemoryChannelRPMCounter(now.Add(2*time.Minute)))
	counter := value.(*memoryChannelRPMCounter)
	count := counter.count.Add(1)
	if count > int64(rpm) {
		counter.count.Add(-1)
		cleanupExpiredChannelRateLimitMemory(now)
		return false
	}
	cleanupExpiredChannelRateLimitMemory(now)
	return true
}

func newMemoryChannelRPMCounter(expireAt time.Time) *memoryChannelRPMCounter {
	counter := &memoryChannelRPMCounter{}
	counter.expire.Store(expireAt.UnixNano())
	return counter
}

func cleanupExpiredChannelRateLimitMemory(now time.Time) {
	nowNano := now.UnixNano()
	channelRPMCounters.Range(func(key, value any) bool {
		counter, ok := value.(*memoryChannelRPMCounter)
		if !ok || nowNano >= counter.expire.Load() {
			channelRPMCounters.Delete(key)
		}
		return true
	})
	channelCooldownExpiries.Range(func(key, value any) bool {
		expireAt, ok := value.(int64)
		if !ok || nowNano >= expireAt {
			channelCooldownExpiries.Delete(key)
		}
		return true
	})
}

func channelRPMRedisKey(channelId int, now time.Time) string {
	return fmt.Sprintf("%s:%d:%d", channelRPMRedisKeyPrefix, channelId, now.Unix()/60)
}

func channelRPMMemoryKey(channelId int, now time.Time) string {
	return fmt.Sprintf("%d:%d", channelId, now.Unix()/60)
}

func channelCooldownRedisKey(channelId int) string {
	return fmt.Sprintf("%s:%d", channelCooldownRedisKeyPrefix, channelId)
}
