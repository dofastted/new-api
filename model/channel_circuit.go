package model

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/samber/hot"
)

type ChannelCircuitState string

const (
	ChannelCircuitClosed   ChannelCircuitState = "closed"
	ChannelCircuitOpen     ChannelCircuitState = "open"
	ChannelCircuitHalfOpen ChannelCircuitState = "half_open"
)

type ChannelCircuitStatus struct {
	ChannelID       int                 `json:"channel_id"`
	State           ChannelCircuitState `json:"state"`
	FailureCount    int                 `json:"failure_count"`
	OpenedAtUnix    int64               `json:"opened_at_unix,omitempty"`
	UpdatedAtUnix   int64               `json:"updated_at_unix"`
	NextAttemptUnix int64               `json:"next_attempt_unix,omitempty"`
	LastCategory    string              `json:"last_category,omitempty"`
}

type channelCircuitConfig struct {
	FailureThreshold int
	OpenSeconds      int
	CacheTTL         time.Duration
}

const channelCircuitCacheNamespace = "new-api:channel_circuit:v1"

var (
	channelCircuitOnce  sync.Once
	channelCircuitCache *cachex.HybridCache[ChannelCircuitStatus]
)

func getChannelCircuitConfig() channelCircuitConfig {
	threshold := common.GetEnvOrDefault("CHANNEL_CIRCUIT_FAILURE_THRESHOLD", 3)
	if threshold <= 0 {
		threshold = 3
	}
	openSeconds := common.GetEnvOrDefault("CHANNEL_CIRCUIT_OPEN_SECONDS", 60)
	if openSeconds <= 0 {
		openSeconds = 60
	}
	return channelCircuitConfig{
		FailureThreshold: threshold,
		OpenSeconds:      openSeconds,
		CacheTTL:         time.Duration(openSeconds*4) * time.Second,
	}
}

func getChannelCircuitCache() *cachex.HybridCache[ChannelCircuitStatus] {
	channelCircuitOnce.Do(func() {
		channelCircuitCache = cachex.NewHybridCache[ChannelCircuitStatus](cachex.HybridCacheConfig[ChannelCircuitStatus]{
			Namespace: cachex.Namespace(channelCircuitCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[ChannelCircuitStatus]{},
			Memory: func() *hot.HotCache[string, ChannelCircuitStatus] {
				return hot.NewHotCache[string, ChannelCircuitStatus](hot.LRU, 100_000).
					WithTTL(getChannelCircuitConfig().CacheTTL).
					Build()
			},
		})
	})
	return channelCircuitCache
}

func channelCircuitKey(channelID int) string {
	return fmt.Sprintf("channel:%d", channelID)
}

func GetChannelCircuitStatus(channelID int) ChannelCircuitStatus {
	status, found, err := getChannelCircuitCache().Get(channelCircuitKey(channelID))
	if err != nil || !found {
		return ChannelCircuitStatus{ChannelID: channelID, State: ChannelCircuitClosed}
	}
	status.ChannelID = channelID
	if status.State == "" {
		status.State = ChannelCircuitClosed
	}
	return normalizeChannelCircuitStatus(status, time.Now())
}

func IsChannelCircuitOpen(channelID int) bool {
	status := GetChannelCircuitStatus(channelID)
	return status.State == ChannelCircuitOpen
}

func RecordChannelCircuitSuccess(channelID int) {
	if channelID <= 0 {
		return
	}
	ResetChannelCircuit(channelID)
}

func RecordChannelCircuitFailure(channelID int, category string) {
	if channelID <= 0 {
		return
	}
	now := time.Now()
	config := getChannelCircuitConfig()
	status := GetChannelCircuitStatus(channelID)
	status.ChannelID = channelID
	status.FailureCount++
	status.LastCategory = category
	status.UpdatedAtUnix = now.Unix()
	if status.State == ChannelCircuitHalfOpen || status.FailureCount >= config.FailureThreshold {
		status.State = ChannelCircuitOpen
		status.OpenedAtUnix = now.Unix()
		status.NextAttemptUnix = now.Add(time.Duration(config.OpenSeconds) * time.Second).Unix()
	}
	_ = getChannelCircuitCache().SetWithTTL(channelCircuitKey(channelID), status, config.CacheTTL)
}

func ResetChannelCircuit(channelID int) {
	if channelID <= 0 {
		return
	}
	now := time.Now()
	status := ChannelCircuitStatus{
		ChannelID:     channelID,
		State:         ChannelCircuitClosed,
		UpdatedAtUnix: now.Unix(),
	}
	_ = getChannelCircuitCache().SetWithTTL(channelCircuitKey(channelID), status, getChannelCircuitConfig().CacheTTL)
}

func normalizeChannelCircuitStatus(status ChannelCircuitStatus, now time.Time) ChannelCircuitStatus {
	if status.State != ChannelCircuitOpen || status.NextAttemptUnix == 0 || now.Unix() < status.NextAttemptUnix {
		return status
	}
	status.State = ChannelCircuitHalfOpen
	status.UpdatedAtUnix = now.Unix()
	_ = getChannelCircuitCache().SetWithTTL(channelCircuitKey(status.ChannelID), status, getChannelCircuitConfig().CacheTTL)
	return status
}
