package abuse_guard_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAbuseGuardSettingOptionRoundTrip(t *testing.T) {
	src := AbuseGuardSetting{
		Enabled:            true,
		MonitorOnly:        true,
		ExemptGroups:       []string{"vip"},
		ModelScopePatterns: []string{"claude*", "gpt-4o"},
		BlockWords:         []string{"badword"},
		CustomPatterns: []AbusePattern{
			{ID: "c1", Kind: PatternKindRegex, Pattern: `(?i)ignore\s+previous`, Weight: 4},
		},
		DisabledBuiltinIDs:   []string{"b1"},
		PatternBlockScore:    8,
		ScanWindowKB:         16,
		ModerationAPIKey:     "sk-test",
		ModerationBaseURL:    "https://proxy.example.com",
		ModerationModel:      "omni-moderation-latest",
		SampleRatePercent:    2.5,
		ReviewSnippetKB:      8,
		QueueSize:            256,
		WorkerCount:          2,
		CategoryScores:       map[string]int{"violence": 2},
		InstantBanCategories: []string{"sexual/minors"},
		ScoreWindowHours:     12,
		BanThreshold:         3,
		TempBanHours:         48,
		PermBanAfterTempBans: 2,
	}

	m, err := config.ConfigToMap(&src)
	require.NoError(t, err)

	var dst AbuseGuardSetting
	require.NoError(t, config.UpdateConfigFromMap(&dst, m))
	assert.Equal(t, src, dst)
}

func TestDefaultAbuseGuardSettingIncludesHardBlockWords(t *testing.T) {
	defaults := GetAbuseGuardSetting()

	require.NotEmpty(t, defaults.BlockWords)
	assert.Contains(t, defaults.BlockWords, "steal api key")
	assert.Contains(t, defaults.BlockWords, "生成勒索软件")
}

func TestModelInScope(t *testing.T) {
	s := AbuseGuardSetting{ModelScopePatterns: []string{"claude*", "gpt*", "o1*", "chatgpt*", "text-moderation-exact"}}

	tests := []struct {
		model string
		want  bool
	}{
		{"claude-sonnet-4-6", true},
		{"Claude-Opus-4-8", true},
		{"gpt-4o-mini", true},
		{"o1-preview", true},
		{"chatgpt-4o-latest", true},
		{"text-moderation-exact", true},
		{"gemini-2.5-pro", false},
		{"deepseek-chat", false},
		{"text-moderation-exact-v2", false},
		{"", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, s.ModelInScope(tt.model), "model=%s", tt.model)
	}

	empty := AbuseGuardSetting{}
	assert.True(t, empty.ModelInScope("anything"), "empty scope means unrestricted")
}

func TestCategoryScoreAndInstantBan(t *testing.T) {
	s := AbuseGuardSetting{
		CategoryScores:       map[string]int{"violence": 3, "hate": 0},
		InstantBanCategories: []string{"sexual/minors"},
	}
	assert.Equal(t, 3, s.CategoryScore("violence"))
	assert.Equal(t, 1, s.CategoryScore("hate"), "non-positive configured score falls back to 1")
	assert.Equal(t, 1, s.CategoryScore("unknown"))
	assert.True(t, s.IsInstantBanCategory("sexual/minors"))
	assert.True(t, s.IsInstantBanCategory("SEXUAL/MINORS"))
	assert.False(t, s.IsInstantBanCategory("violence"))
}
