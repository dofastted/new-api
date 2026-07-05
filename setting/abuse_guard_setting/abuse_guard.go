package abuse_guard_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// AbusePattern 破限/滥用检测模式。Kind 为 keyword 时走 AC 自动机(子串匹配,
// 大小写不敏感);为 regex 时按 Go RE2 正则编译。Weight 为命中权重(1-10)。
type AbusePattern struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"` // keyword | regex
	Pattern string `json:"pattern"`
	Weight  int    `json:"weight"`
	Note    string `json:"note,omitempty"`
}

const (
	PatternKindKeyword = "keyword"
	PatternKindRegex   = "regex"
)

type AbuseGuardSetting struct {
	Enabled     bool `json:"enabled"`
	MonitorOnly bool `json:"monitor_only"`
	// ExemptGroups 中的用户分组跳过全部检测与送审
	ExemptGroups []string `json:"exempt_groups"`
	// ModelScopePatterns 受检模型通配列表(仅支持前缀 + 尾部 `*`),按用户请求的原始模型名匹配
	ModelScopePatterns []string `json:"model_scope_patterns"`

	// 同步层
	BlockWords         []string       `json:"block_words"`
	CustomPatterns     []AbusePattern `json:"custom_patterns"`
	DisabledBuiltinIDs []string       `json:"disabled_builtin_ids"`
	PatternBlockScore  int            `json:"pattern_block_score"`
	ScanWindowKB       int            `json:"scan_window_kb"`

	// 异步审查层
	ModerationAPIKey     string         `json:"moderation_api_key"`
	ModerationBaseURL    string         `json:"moderation_base_url"`
	ModerationModel      string         `json:"moderation_model"`
	SampleRatePercent    float64        `json:"sample_rate_percent"`
	ReviewSnippetKB      int            `json:"review_snippet_kb"`
	QueueSize            int            `json:"queue_size"`
	WorkerCount          int            `json:"worker_count"`
	CategoryScores       map[string]int `json:"category_scores"`
	InstantBanCategories []string       `json:"instant_ban_categories"`

	// 处罚
	ScoreWindowHours     int `json:"score_window_hours"`
	BanThreshold         int `json:"ban_threshold"`
	TempBanHours         int `json:"temp_ban_hours"`
	PermBanAfterTempBans int `json:"perm_ban_after_temp_bans"`
}

var defaultBlockWords = []string{
	"steal api key",
	"steal cookies",
	"dump saved passwords",
	"extract oauth token",
	"extract refresh token",
	"session hijack",
	"bypass login",
	"account takeover",
	"write ransomware",
	"create ransomware",
	"build ransomware",
	"create keylogger",
	"write keylogger",
	"credential stealer",
	"cookie stealer",
	"undetectable malware",
	"bypass antivirus",
	"disable antivirus",
	"phishing kit",
	"窃取cookie",
	"盗取cookie",
	"抓取用户token",
	"导出用户token",
	"窃取密码",
	"盗取密码",
	"提取oauth",
	"劫持会话",
	"接管账号",
	"绕过登录",
	"生成勒索软件",
	"制作勒索软件",
	"编写键盘记录器",
	"绕过杀毒软件",
	"关闭杀毒软件",
	"免杀木马",
	"钓鱼网站源码",
}

var abuseGuardSetting = AbuseGuardSetting{
	Enabled:     false,
	MonitorOnly: false,
	ModelScopePatterns: []string{
		"claude*", "gpt*", "o1*", "o3*", "o4*", "chatgpt*", "codex*",
	},
	BlockWords:           append([]string(nil), defaultBlockWords...),
	PatternBlockScore:    10,
	ScanWindowKB:         32,
	ModerationBaseURL:    "https://api.openai.com",
	ModerationModel:      "omni-moderation-latest",
	SampleRatePercent:    5,
	ReviewSnippetKB:      16,
	QueueSize:            1024,
	WorkerCount:          4,
	InstantBanCategories: []string{"sexual/minors"},
	ScoreWindowHours:     24,
	BanThreshold:         5,
	TempBanHours:         24,
	PermBanAfterTempBans: 3,
}

func init() {
	config.GlobalConfig.Register("abuse_guard", &abuseGuardSetting)
}

func GetAbuseGuardSetting() *AbuseGuardSetting {
	return &abuseGuardSetting
}

// ModelInScope 判断模型名是否属于受检范围。模式仅支持精确匹配和尾部 `*` 前缀匹配,
// 大小写不敏感;空模式列表视为不限制(全部受检)。
func (s *AbuseGuardSetting) ModelInScope(modelName string) bool {
	if len(s.ModelScopePatterns) == 0 {
		return true
	}
	name := strings.ToLower(modelName)
	for _, p := range s.ModelScopePatterns {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if strings.HasSuffix(p, "*") {
			if strings.HasPrefix(name, strings.TrimSuffix(p, "*")) {
				return true
			}
		} else if name == p {
			return true
		}
	}
	return false
}

func (s *AbuseGuardSetting) IsExemptGroup(group string) bool {
	for _, g := range s.ExemptGroups {
		if g != "" && g == group {
			return true
		}
	}
	return false
}

// CategoryScore 返回 moderation 类别的记分值,未配置的类别默认 1 分。
func (s *AbuseGuardSetting) CategoryScore(category string) int {
	if v, ok := s.CategoryScores[category]; ok && v > 0 {
		return v
	}
	return 1
}

func (s *AbuseGuardSetting) IsInstantBanCategory(category string) bool {
	for _, c := range s.InstantBanCategories {
		if c != "" && strings.EqualFold(c, category) {
			return true
		}
	}
	return false
}

// EffectiveScanWindowBytes 返回首尾扫描窗口字节数(单侧)。
func (s *AbuseGuardSetting) EffectiveScanWindowBytes() int {
	kb := s.ScanWindowKB
	if kb <= 0 {
		kb = 32
	}
	return kb * 1024
}

func (s *AbuseGuardSetting) EffectiveSnippetBytes() int {
	kb := s.ReviewSnippetKB
	if kb <= 0 {
		kb = 16
	}
	return kb * 1024
}
