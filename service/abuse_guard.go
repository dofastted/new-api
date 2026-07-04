package service

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	ags "github.com/QuantumNous/new-api/setting/abuse_guard_setting"
)

const roleAdminThreshold = common.RoleAdminUser

// AbuseDecision 为同步风控网关的裁决结果。
type AbuseDecision int

const (
	AbuseAllow      AbuseDecision = iota // 放行
	AbuseBlock                           // 命中硬拦截,返回 400
	AbuseTempBanned                      // 用户处于临时封禁期,返回 403
)

// AbuseCheckResult 是 AbuseGuardCheck 的返回值。
type AbuseCheckResult struct {
	Decision AbuseDecision
	// BanUntil 仅在 Decision==AbuseTempBanned 时有效,为解封时间戳(秒)
	BanUntil int64
	// Message 面向用户的提示(不含命中词等敏感信息)
	Message string
}

// syncDetection 为纯检测结果,不含任何副作用,便于单测与基准。
type syncDetection struct {
	blocked     bool
	source      string // sync_word | sync_pattern
	score       int
	patternHits []patternHit
	matchedWord string // 命中的硬拦截词(仅用于内部 snippet,不回显用户)
	forceReview bool   // 有模式命中但未达拦截阈值
}

// detectSync 对文本执行同步检测:硬拦截词表优先,其次破限模式评分。纯函数。
func detectSync(s *ags.AbuseGuardSetting, text string) syncDetection {
	scan := truncateForScan(text, s.EffectiveScanWindowBytes())
	lower := strings.ToLower(scan)

	if len(s.BlockWords) > 0 {
		if ok, words := AcSearch(lower, s.BlockWords, true); ok {
			matched := ""
			if len(words) > 0 {
				matched = words[0]
			}
			return syncDetection{blocked: true, source: model.RiskSourceSyncWord, score: s.BanThreshold, matchedWord: matched}
		}
	}

	ps := getPatternSet(s)
	hits, total := ps.scan(lower)
	if len(hits) == 0 {
		return syncDetection{}
	}
	threshold := s.PatternBlockScore
	if threshold <= 0 {
		threshold = 10
	}
	if total >= threshold {
		return syncDetection{blocked: true, source: model.RiskSourceSyncPattern, score: patternScore(total), patternHits: hits}
	}
	return syncDetection{forceReview: true, patternHits: hits}
}

// patternScore 将模式权重和折算为记分,单次违规最多贡献 1 分(除非命中硬拦截词直达阈值)。
// 破限模式命中记 1 分,交由累计机制升级处罚,避免单次高权重直接永封。
func patternScore(total int) int {
	return 1
}

// truncateForScan 对超长文本做首尾窗口截断(按 rune 边界),控制扫描耗时。
func truncateForScan(text string, windowBytes int) string {
	if windowBytes <= 0 || len(text) <= 2*windowBytes {
		return text
	}
	head := safeRunePrefix(text, windowBytes)
	tail := safeRuneSuffix(text, windowBytes)
	return head + "\n" + tail
}

func safeRunePrefix(s string, n int) string {
	if n >= len(s) {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func safeRuneSuffix(s string, n int) string {
	if n >= len(s) {
		return s
	}
	start := len(s) - n
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return s[start:]
}

// AbuseGuardInput 为同步网关的输入参数。
type AbuseGuardInput struct {
	UserId      int
	UserRole    int // 若 >0 则直接用于豁免判定,避免额外 DB 查询;0 表示未知,回退 IsAdmin
	Group       string
	ModelName   string
	CombineText string
	RequestId   string
	Ip          string
}

// AbuseGuardCheck 是请求路径上的同步风控网关。返回裁决结果;命中时已完成记分与事件落库,
// 并在需要时投递异步审查。绝不调用外部服务,耗时为毫秒级。
func AbuseGuardCheck(in AbuseGuardInput) AbuseCheckResult {
	s := ags.GetAbuseGuardSetting()
	if !s.Enabled {
		return AbuseCheckResult{Decision: AbuseAllow}
	}

	// 豁免:管理员角色或豁免分组
	if isAbuseExempt(in.UserId, in.UserRole, in.Group, s) {
		return AbuseCheckResult{Decision: AbuseAllow}
	}

	// 临时封禁检查(全局,不限模型)
	if until, banned := getTempBan(in.UserId); banned {
		if s.MonitorOnly {
			return AbuseCheckResult{Decision: AbuseAllow}
		}
		return AbuseCheckResult{Decision: AbuseTempBanned, BanUntil: until, Message: tempBanMessage(until)}
	}

	// 模型范围过滤:范围外模型跳过检测与送审
	if !s.ModelInScope(in.ModelName) {
		return AbuseCheckResult{Decision: AbuseAllow}
	}

	det := detectSync(s, in.CombineText)

	if det.blocked {
		score := recordSyncViolation(s, in, det)
		if s.MonitorOnly {
			return AbuseCheckResult{Decision: AbuseAllow}
		}
		return AbuseCheckResult{Decision: AbuseBlock, Message: blockMessage(s, score)}
	}

	// 未拦截:判定是否送异步审查
	if shouldReview(s, in.UserId, in.RequestId, det.forceReview) {
		enqueueReview(reviewJob{
			UserId:    in.UserId,
			Text:      truncateForScan(in.CombineText, s.EffectiveSnippetBytes()),
			ModelName: in.ModelName,
			RequestId: in.RequestId,
			Ip:        in.Ip,
			Monitor:   s.MonitorOnly,
		})
	}
	return AbuseCheckResult{Decision: AbuseAllow}
}

// blockMessage 面向用户的拦截提示,不回显命中词,附带当前计数与阈值以传达封禁预警。
func blockMessage(s *ags.AbuseGuardSetting, currentScore int) string {
	return fmt.Sprintf(
		"检测到疑似违规内容,请修改提示词后重试。当前违规计数 %d/%d,累计达到上限将被临时封禁。",
		currentScore, s.BanThreshold,
	)
}

func tempBanMessage(until int64) string {
	remain := until - common.GetTimestamp()
	if remain < 0 {
		remain = 0
	}
	mins := (remain + 59) / 60
	return fmt.Sprintf(
		"由于多次触发内容风控,您的账号已被临时限制访问,预计 %d 分钟后自动恢复(解封时间 %s)。",
		mins, time.Unix(until, 0).Format("2006-01-02 15:04:05"),
	)
}

// buildSyncDetail 构造违规事件的 Detail JSON,记录命中来源与模式 ID(不含原文)。
func buildSyncDetail(det syncDetection, monitor bool) string {
	ids := make([]string, 0, len(det.patternHits))
	for _, h := range det.patternHits {
		ids = append(ids, h.ID)
	}
	payload := map[string]any{
		"source":   det.source,
		"patterns": ids,
		"monitor":  monitor,
	}
	if det.matchedWord != "" {
		payload["hit_word"] = true
	}
	b, err := common.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(b)
}

func buildAbuseNotify() dto.Notify {
	return dto.NewNotify(
		dto.NotifyTypeAbuseWarning,
		"内容风控警告",
		"我们检测到您的部分请求可能违反使用政策。请调整提示词内容,多次违规将导致账号被限制访问。",
		nil,
	)
}

// buildSnippet 截取用于管理端排查的命中上下文片段。
func buildSnippet(text string, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = 2048
	}
	return truncateForScan(text, maxBytes)
}

func isAbuseExempt(userId, role int, group string, s *ags.AbuseGuardSetting) bool {
	if role >= roleAdminThreshold {
		return true
	}
	if s.IsExemptGroup(group) {
		return true
	}
	// relay(令牌鉴权)路径未在 context 写入 role,role 为 0;此处回退到带缓存的
	// 管理员判定,避免每个受检请求都查库(角色极少变动,TTL 缓存足够)。
	if role == 0 && isAdminCached(userId) {
		return true
	}
	return false
}

type adminCacheEntry struct {
	isAdmin bool
	expire  int64
}

var abuseAdminCache sync.Map // userId -> adminCacheEntry

const abuseAdminCacheTTLSeconds = 300

// isAdminCached 带 TTL 缓存的管理员判定,避免请求路径上重复查库。
func isAdminCached(userId int) bool {
	if userId <= 0 {
		return false
	}
	now := common.GetTimestamp()
	if v, ok := abuseAdminCache.Load(userId); ok {
		e := v.(adminCacheEntry)
		if e.expire > now {
			return e.isAdmin
		}
	}
	admin := model.IsAdmin(userId)
	abuseAdminCache.Store(userId, adminCacheEntry{isAdmin: admin, expire: now + abuseAdminCacheTTLSeconds})
	return admin
}
