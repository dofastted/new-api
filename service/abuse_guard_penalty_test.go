package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	ags "github.com/QuantumNous/new-api/setting/abuse_guard_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetPenaltyState() {
	common.RedisEnabled = false
	abuseScoreMem.Range(func(k, _ any) bool { abuseScoreMem.Delete(k); return true })
	abuseBanMem.Range(func(k, _ any) bool { abuseBanMem.Delete(k); return true })
	abuseNow = time.Now
}

func TestApplyScoreAccumulatesAndBans(t *testing.T) {
	resetPenaltyState()
	s := &ags.AbuseGuardSetting{BanThreshold: 3, ScoreWindowHours: 24, TempBanHours: 24, PermBanAfterTempBans: 0}
	uid := 5001

	assert.Equal(t, 1, applyScore(s, uid, 1))
	assert.Equal(t, 2, applyScore(s, uid, 1))
	_, banned := getTempBan(uid)
	assert.False(t, banned, "below threshold, not banned")

	// third point reaches threshold → temp ban, score reset
	applyScore(s, uid, 1)
	_, banned = getTempBan(uid)
	assert.True(t, banned, "threshold reached → temp banned")

	// score cleared after ban
	assert.Equal(t, 1, applyScore(s, uid, 1), "score reset to fresh count after ban")
}

func TestScoreWindowExpiry(t *testing.T) {
	resetPenaltyState()
	base := time.Now()
	abuseNow = func() time.Time { return base }
	s := &ags.AbuseGuardSetting{BanThreshold: 5, ScoreWindowHours: 1, TempBanHours: 24}
	uid := 5002

	assert.Equal(t, 2, applyScore(s, uid, 2))

	// advance beyond window → memory score expires
	abuseNow = func() time.Time { return base.Add(2 * time.Hour) }
	assert.Equal(t, 2, applyScore(s, uid, 2), "expired window resets accumulation")
}

func TestGetTempBanExpiry(t *testing.T) {
	resetPenaltyState()
	base := time.Now()
	abuseNow = func() time.Time { return base }
	s := &ags.AbuseGuardSetting{BanThreshold: 1, ScoreWindowHours: 24, TempBanHours: 1, PermBanAfterTempBans: 0}
	uid := 5003

	applyScore(s, uid, 1)
	until, banned := getTempBan(uid)
	assert.True(t, banned)
	assert.Equal(t, base.Add(time.Hour).Unix(), until)

	// after ban duration → not banned
	abuseNow = func() time.Time { return base.Add(2 * time.Hour) }
	_, banned = getTempBan(uid)
	assert.False(t, banned, "temp ban auto-expires")
}

func TestPermBanTriggersAtExactThreshold(t *testing.T) {
	resetPenaltyState()
	// PermBanAfterTempBans=2:第 2 次临时封禁即应触发永久封禁。
	// 由于 RecordRiskEvent 异步写入,triggerTempBan 必须基于"记录前"的历史计数 +1 判定。
	s := &ags.AbuseGuardSetting{BanThreshold: 1, ScoreWindowHours: 24, TempBanHours: 24, PermBanAfterTempBans: 2}
	uid := 6100

	// 预置 1 条历史 temp_ban 事件(同步写入,确保可被 CountUserTempBans 计入)
	require.NoError(t, model.DB.Create(&model.RiskEvent{
		UserId: uid, CreatedAt: 100, Source: model.RiskSourcePenalty, Action: model.RiskActionTempBan,
	}).Error)

	permBanned := false
	// 第 2 次临时封禁:priorTempBans=1,+1=2 >= limit(2) → 应永封。
	// 用一个可观察 status 的用户验证 triggerPermBan 效果。
	require.NoError(t, model.DB.Create(&model.User{Id: uid, Status: common.UserStatusEnabled, Username: "ban_test"}).Error)
	triggerTempBan(s, uid)

	var u model.User
	require.NoError(t, model.DB.Where("id = ?", uid).First(&u).Error)
	permBanned = u.Status == common.UserStatusDisabled
	assert.True(t, permBanned, "second temp ban with limit=2 must escalate to permanent ban")
}

func TestClearTempBanAndResetScore(t *testing.T) {
	resetPenaltyState()
	s := &ags.AbuseGuardSetting{BanThreshold: 1, ScoreWindowHours: 24, TempBanHours: 24, PermBanAfterTempBans: 0}
	uid := 5004

	applyScore(s, uid, 1)
	_, banned := getTempBan(uid)
	assert.True(t, banned)

	ClearTempBan(uid)
	_, banned = getTempBan(uid)
	assert.False(t, banned, "admin unban clears temp ban")

	applyScore(s, uid, 1) // re-bans; but reset score should clear count
	ResetAbuseScore(uid)
	// after reset, a fresh point counts from 1
	assert.Equal(t, 1, incrScoreMem(uid, 1, 24*time.Hour))
}
