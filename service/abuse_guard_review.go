package service

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	ags "github.com/QuantumNous/new-api/setting/abuse_guard_setting"
)

const (
	abuseFirstSeenRedisPrefix = "ag:first"
	abuseFirstSeenTTL         = 30 * 24 * time.Hour
)

type reviewJob struct {
	UserId    int
	Text      string
	ModelName string
	RequestId string
	Ip        string
	Monitor   bool
}

var (
	reviewQueue     chan reviewJob
	reviewQueueOnce sync.Once
	reviewDropped   uint64
	reviewDropMu    sync.Mutex
	abuseFirstMem   sync.Map // userId -> struct{}
	// moderationHTTPClient 可在测试中替换
	moderationHTTPClient = &http.Client{Timeout: 10 * time.Second}
)

// initReviewWorkers 惰性启动有界队列与 worker 池。
func initReviewWorkers(s *ags.AbuseGuardSetting) {
	reviewQueueOnce.Do(func() {
		size := s.QueueSize
		if size <= 0 {
			size = 1024
		}
		workers := s.WorkerCount
		if workers <= 0 {
			workers = 4
		}
		reviewQueue = make(chan reviewJob, size)
		for i := 0; i < workers; i++ {
			go reviewWorker()
		}
	})
}

func reviewWorker() {
	for job := range reviewQueue {
		processReviewJob(job)
	}
}

// enqueueReview 非阻塞投递审查任务;队列满则丢弃并计数,绝不阻塞请求路径。
func enqueueReview(job reviewJob) {
	s := ags.GetAbuseGuardSetting()
	initReviewWorkers(s)
	select {
	case reviewQueue <- job:
	default:
		reviewDropMu.Lock()
		reviewDropped++
		reviewDropMu.Unlock()
	}
}

// shouldReview 判定是否送异步审查:用户首条必审;同步层强制送审;其余按抽样率。
func shouldReview(s *ags.AbuseGuardSetting, userId int, requestId string, forced bool) bool {
	if s.ModerationAPIKey == "" {
		return false
	}
	if forced {
		return true
	}
	if isFirstRequest(userId) {
		return true
	}
	return sampleHit(requestId, s.SampleRatePercent)
}

// isFirstRequest 判定并标记用户是否首次请求(以是否成功写入首见标记为准)。
func isFirstRequest(userId int) bool {
	if common.RedisEnabled && common.RDB != nil {
		ok, err := common.RDB.SetNX(context.Background(), abuseFirstSeenKey(userId), "1", abuseFirstSeenTTL).Result()
		if err == nil {
			return ok
		}
	}
	_, loaded := abuseFirstMem.LoadOrStore(userId, struct{}{})
	return !loaded
}

func abuseFirstSeenKey(userId int) string {
	return fmt.Sprintf("%s:%d", abuseFirstSeenRedisPrefix, userId)
}

// sampleHit 基于 requestId 的哈希做确定性抽样,避免随机源。
func sampleHit(requestId string, ratePercent float64) bool {
	if ratePercent <= 0 {
		return false
	}
	if ratePercent >= 100 {
		return true
	}
	h := fnv.New32a()
	h.Write([]byte(requestId))
	bucket := h.Sum32() % 10000
	return float64(bucket) < ratePercent*100
}

func processReviewJob(job reviewJob) {
	s := ags.GetAbuseGuardSetting()
	flagged, category, err := callModeration(s, job.Text)
	if err != nil {
		common.SysLog(fmt.Sprintf("abuse_guard: moderation call failed (user %d): %s", job.UserId, err.Error()))
		return
	}
	if !flagged {
		return
	}

	score := s.CategoryScore(category)
	if s.IsInstantBanCategory(category) {
		score = s.BanThreshold
	}

	model.RecordRiskEvent(&model.RiskEvent{
		UserId:    job.UserId,
		Source:    model.RiskSourceModeration,
		Action:    model.RiskActionFlagged,
		Detail:    fmt.Sprintf(`{"category":%q,"monitor":%t}`, category, job.Monitor),
		Snippet:   job.Text,
		Score:     score,
		RequestId: job.RequestId,
		ModelName: job.ModelName,
		Ip:        job.Ip,
	})

	if job.Monitor {
		return
	}
	applyScore(s, job.UserId, score)
	notifyAbuseWarning(job.UserId)
}

// notifyAbuseWarning 经现有通知机制向用户发送违规警告(尽力而为,失败不影响流程)。
func notifyAbuseWarning(userId int) {
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		return
	}
	setting := userCache.GetSetting()
	_ = NotifyUser(userId, userCache.Email, setting, buildAbuseNotify())
}

type moderationRequest struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

type moderationResponse struct {
	Results []struct {
		Flagged    bool               `json:"flagged"`
		Categories map[string]bool    `json:"categories"`
		Scores     map[string]float64 `json:"category_scores"`
	} `json:"results"`
}

// callModeration 调用 OpenAI Moderation API,返回是否命中及最高分类别。
func callModeration(s *ags.AbuseGuardSetting, text string) (bool, string, error) {
	reqBody := moderationRequest{Input: text, Model: s.ModerationModel}
	payload, err := common.Marshal(reqBody)
	if err != nil {
		return false, "", err
	}

	url := s.ModerationBaseURL
	if url == "" {
		url = "https://api.openai.com"
	}
	url += "/v1/moderations"

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return false, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.ModerationAPIKey)

	resp, err := moderationHTTPClient.Do(req)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("moderation status %d", resp.StatusCode)
	}

	var mr moderationResponse
	if err := common.DecodeJson(resp.Body, &mr); err != nil {
		return false, "", err
	}
	if len(mr.Results) == 0 || !mr.Results[0].Flagged {
		return false, "", nil
	}
	return true, topCategory(mr.Results[0].Scores, mr.Results[0].Categories), nil
}

// topCategory 返回分值最高的命中类别;无分值时回退首个为 true 的类别。
func topCategory(scores map[string]float64, cats map[string]bool) string {
	best := ""
	bestScore := -1.0
	for c, sc := range scores {
		if cats[c] && sc > bestScore {
			best = c
			bestScore = sc
		}
	}
	if best != "" {
		return best
	}
	for c, flagged := range cats {
		if flagged {
			return c
		}
	}
	return "unknown"
}

// ReviewQueueDropped 返回累计丢弃的审查任务数(供监控/管理端展示)。
func ReviewQueueDropped() uint64 {
	reviewDropMu.Lock()
	defer reviewDropMu.Unlock()
	return reviewDropped
}
