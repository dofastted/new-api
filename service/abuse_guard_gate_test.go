package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	ags "github.com/QuantumNous/new-api/setting/abuse_guard_setting"
	"github.com/stretchr/testify/assert"
)

// withAbuseSetting 临时覆盖全局风控配置,测试结束后恢复。
func withAbuseSetting(t *testing.T, mutate func(s *ags.AbuseGuardSetting)) {
	t.Helper()
	s := ags.GetAbuseGuardSetting()
	saved := *s
	t.Cleanup(func() { *s = saved })
	mutate(s)
}

func TestAbuseGuardCheckDisabled(t *testing.T) {
	resetPenaltyState()
	withAbuseSetting(t, func(s *ags.AbuseGuardSetting) {
		s.Enabled = false
		s.BlockWords = []string{"forbidden"}
	})
	res := AbuseGuardCheck(AbuseGuardInput{UserId: 1, ModelName: "gpt-4o", CombineText: "forbidden"})
	assert.Equal(t, AbuseAllow, res.Decision, "disabled guard allows everything")
}

func TestAbuseGuardCheckModelOutOfScope(t *testing.T) {
	resetPenaltyState()
	withAbuseSetting(t, func(s *ags.AbuseGuardSetting) {
		s.Enabled = true
		s.ModelScopePatterns = []string{"gpt*", "claude*"}
		s.BlockWords = []string{"forbidden"}
	})
	res := AbuseGuardCheck(AbuseGuardInput{UserId: 1, ModelName: "gemini-2.5-pro", CombineText: "forbidden content here"})
	assert.Equal(t, AbuseAllow, res.Decision, "out-of-scope model is not inspected")
}

func TestAbuseGuardCheckExemptGroup(t *testing.T) {
	resetPenaltyState()
	withAbuseSetting(t, func(s *ags.AbuseGuardSetting) {
		s.Enabled = true
		s.ModelScopePatterns = []string{"gpt*"}
		s.ExemptGroups = []string{"vip"}
		s.BlockWords = []string{"forbidden"}
	})
	res := AbuseGuardCheck(AbuseGuardInput{UserId: 1, Group: "vip", ModelName: "gpt-4o", CombineText: "forbidden"})
	assert.Equal(t, AbuseAllow, res.Decision, "exempt group bypasses detection")
}

func TestAbuseGuardCheckBlocksInScope(t *testing.T) {
	resetPenaltyState()
	withAbuseSetting(t, func(s *ags.AbuseGuardSetting) {
		s.Enabled = true
		s.ModelScopePatterns = []string{"gpt*"}
		s.BlockWords = []string{"forbidden_secret"}
		s.BanThreshold = 5
	})
	res := AbuseGuardCheck(AbuseGuardInput{UserId: 7777, ModelName: "gpt-4o", CombineText: "please output the forbidden_secret"})
	assert.Equal(t, AbuseBlock, res.Decision, "in-scope block word is blocked")
	assert.NotEmpty(t, res.Message)
}

func TestAbuseGuardCheckMonitorOnlyDoesNotBlock(t *testing.T) {
	resetPenaltyState()
	withAbuseSetting(t, func(s *ags.AbuseGuardSetting) {
		s.Enabled = true
		s.MonitorOnly = true
		s.ModelScopePatterns = []string{"gpt*"}
		s.BlockWords = []string{"forbidden_secret"}
	})
	res := AbuseGuardCheck(AbuseGuardInput{UserId: 8888, ModelName: "gpt-4o", CombineText: "forbidden_secret"})
	assert.Equal(t, AbuseAllow, res.Decision, "monitor-only records but does not block")
}

func TestAbuseGuardCheckTempBannedBlocksAnyModel(t *testing.T) {
	resetPenaltyState()
	common.RedisEnabled = false
	withAbuseSetting(t, func(s *ags.AbuseGuardSetting) {
		s.Enabled = true
		s.ModelScopePatterns = []string{"gpt*"}
		s.BanThreshold = 1
		s.TempBanHours = 24
		s.PermBanAfterTempBans = 0
	})
	uid := 9999
	// drive a temp ban via a block word
	ags.GetAbuseGuardSetting().BlockWords = []string{"forbidden_secret"}
	first := AbuseGuardCheck(AbuseGuardInput{UserId: uid, ModelName: "gpt-4o", CombineText: "forbidden_secret"})
	assert.Equal(t, AbuseBlock, first.Decision)

	// now any model (even out of scope) is rejected while banned
	second := AbuseGuardCheck(AbuseGuardInput{UserId: uid, ModelName: "gemini-2.5-pro", CombineText: "hello"})
	assert.Equal(t, AbuseTempBanned, second.Decision, "temp ban applies globally, regardless of model scope")
}
