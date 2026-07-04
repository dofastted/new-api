package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	ags "github.com/QuantumNous/new-api/setting/abuse_guard_setting"
)

const (
	abuseScoreRedisPrefix = "ag:score"
	abuseBanRedisPrefix   = "ag:ban"
)

// 内存兜底(Redis 不可用时,单实例语义)
type memScore struct {
	score  int
	expire int64
}

var (
	abuseScoreMem   sync.Map // userId -> *memScore
	abuseScoreMemMu sync.Mutex
	abuseBanMem     sync.Map // userId -> untilTs
	abuseNow        = time.Now
)

func abuseScoreKey(userId int) string { return fmt.Sprintf("%s:%d", abuseScoreRedisPrefix, userId) }
func abuseBanKey(userId int) string   { return fmt.Sprintf("%s:%d", abuseBanRedisPrefix, userId) }

// getTempBan 返回用户临时封禁的解封时间戳与是否处于封禁期。
func getTempBan(userId int) (int64, bool) {
	if common.RedisEnabled && common.RDB != nil {
		v, err := common.RDB.Get(context.Background(), abuseBanKey(userId)).Result()
		if err == nil {
			var until int64
			fmt.Sscanf(v, "%d", &until)
			if until > abuseNow().Unix() {
				return until, true
			}
			return 0, false
		}
		// redis.Nil 或错误 → 回退内存判定
	}
	if v, ok := abuseBanMem.Load(userId); ok {
		until := v.(int64)
		if until > abuseNow().Unix() {
			return until, true
		}
		abuseBanMem.Delete(userId)
	}
	return 0, false
}

// recordSyncViolation 记录同步命中的违规事件并累计分数,可能触发临时/永久封禁。
// 返回该用户当前窗口内的累计分数,用于面向用户的计数提示。
func recordSyncViolation(s *ags.AbuseGuardSetting, in AbuseGuardInput, det syncDetection) int {
	detail := buildSyncDetail(det, s.MonitorOnly)
	model.RecordRiskEvent(&model.RiskEvent{
		UserId:    in.UserId,
		Source:    det.source,
		Action:    model.RiskActionBlocked,
		Detail:    detail,
		Snippet:   buildSnippet(in.CombineText, s.EffectiveSnippetBytes()),
		Score:     det.score,
		RequestId: in.RequestId,
		ModelName: in.ModelName,
		Ip:        in.Ip,
	})

	if s.MonitorOnly {
		return det.score
	}
	return applyScore(s, in.UserId, det.score)
}

// applyScore 累计违规分数,达到阈值时触发临时封禁,并在临时封禁次数超限时永久封禁。
// 返回累计后的当前分数。
func applyScore(s *ags.AbuseGuardSetting, userId, score int) int {
	if score <= 0 {
		score = 1
	}
	window := time.Duration(s.ScoreWindowHours) * time.Hour
	if window <= 0 {
		window = 24 * time.Hour
	}
	current := incrScore(userId, score, window)

	threshold := s.BanThreshold
	if threshold <= 0 {
		threshold = 5
	}
	if current >= threshold {
		triggerTempBan(s, userId)
		resetScore(userId)
	}
	return current
}

// incrScore 原子累计分数(自首次违规起 window 内有效),返回累计值。
func incrScore(userId, delta int, window time.Duration) int {
	if common.RedisEnabled && common.RDB != nil {
		ctx := context.Background()
		key := abuseScoreKey(userId)
		val, err := common.RDB.IncrBy(ctx, key, int64(delta)).Result()
		if err == nil {
			if val == int64(delta) {
				// 首次写入,设置窗口过期
				common.RDB.Expire(ctx, key, window)
			}
			return int(val)
		}
		common.SysError("abuse_guard: redis incr score failed: " + err.Error())
	}
	return incrScoreMem(userId, delta, window)
}

func incrScoreMem(userId, delta int, window time.Duration) int {
	now := abuseNow().Unix()
	expire := now + int64(window.Seconds())

	abuseScoreMemMu.Lock()
	defer abuseScoreMemMu.Unlock()

	v, ok := abuseScoreMem.Load(userId)
	if !ok {
		abuseScoreMem.Store(userId, &memScore{score: delta, expire: expire})
		return delta
	}
	ms := v.(*memScore)
	if ms.expire <= now {
		abuseScoreMem.Store(userId, &memScore{score: delta, expire: expire})
		return delta
	}
	ms.score += delta
	return ms.score
}

func resetScore(userId int) {
	if common.RedisEnabled && common.RDB != nil {
		common.RDB.Del(context.Background(), abuseScoreKey(userId))
	}
	abuseScoreMem.Delete(userId)
}

// triggerTempBan 设置临时封禁,并在历史临时封禁次数达上限时升级为永久封禁。
func triggerTempBan(s *ags.AbuseGuardSetting, userId int) {
	dur := time.Duration(s.TempBanHours) * time.Hour
	if dur <= 0 {
		dur = 24 * time.Hour
	}
	until := abuseNow().Add(dur).Unix()

	if common.RedisEnabled && common.RDB != nil {
		if err := common.RDB.Set(context.Background(), abuseBanKey(userId), fmt.Sprintf("%d", until), dur).Err(); err != nil {
			common.SysError("abuse_guard: redis set ban failed: " + err.Error())
			abuseBanMem.Store(userId, until)
		}
	} else {
		abuseBanMem.Store(userId, until)
	}

	// 永久封禁判定:历史临时封禁次数(含本次)达到上限。
	// 必须在记录本次事件之前查询历史计数——RecordRiskEvent 为异步写入,
	// 若先记录再查询,本次事件几乎不会落库,导致计数少一(晚一次才永封)。
	limit := s.PermBanAfterTempBans
	priorTempBans := int64(-1)
	if limit > 0 {
		if c, err := model.CountUserTempBans(userId); err != nil {
			common.SysError("abuse_guard: count temp bans failed: " + err.Error())
		} else {
			priorTempBans = c
		}
	}

	model.RecordRiskEvent(&model.RiskEvent{
		UserId: userId,
		Source: model.RiskSourcePenalty,
		Action: model.RiskActionTempBan,
		Detail: fmt.Sprintf(`{"until":%d,"temp_ban_hours":%d}`, until, s.TempBanHours),
	})

	// priorTempBans 不含本次;本次为第 priorTempBans+1 次临时封禁。
	if limit > 0 && priorTempBans >= 0 && int(priorTempBans)+1 >= limit {
		triggerPermBan(userId)
	}
}

// triggerPermBan 永久封禁用户:置为 disabled 并失效缓存,复用管理端禁用路径的效果。
func triggerPermBan(userId int) {
	if err := model.DB.Model(&model.User{}).Where("id = ?", userId).Update("status", common.UserStatusDisabled).Error; err != nil {
		common.SysError("abuse_guard: perm ban update status failed: " + err.Error())
		return
	}
	if err := model.InvalidateUserCache(userId); err != nil {
		common.SysError("abuse_guard: perm ban invalidate cache failed: " + err.Error())
	}
	if err := model.InvalidateUserTokensCache(userId); err != nil {
		common.SysError("abuse_guard: perm ban invalidate tokens cache failed: " + err.Error())
	}
	model.RecordRiskEvent(&model.RiskEvent{
		UserId: userId,
		Source: model.RiskSourcePenalty,
		Action: model.RiskActionPermBan,
		Detail: `{"auto":true}`,
	})
	common.SysLog(fmt.Sprintf("abuse_guard: user %d permanently banned after repeated temp bans", userId))
}

// ClearTempBan 供管理端解除临时封禁。
func ClearTempBan(userId int) {
	if common.RedisEnabled && common.RDB != nil {
		common.RDB.Del(context.Background(), abuseBanKey(userId))
	}
	abuseBanMem.Delete(userId)
}

// ResetAbuseScore 供管理端清零用户当前窗口的违规分数。
func ResetAbuseScore(userId int) {
	resetScore(userId)
}
