package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
)

// RiskEvent 记录滥用风控产生的违规/处罚事件,仅管理端可见。
type RiskEvent struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	UserId    int    `json:"user_id" gorm:"index;index:idx_risk_user_action,priority:1"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index"`
	Source    string `json:"source" gorm:"type:varchar(32);index"`
	Action    string `json:"action" gorm:"type:varchar(32);index:idx_risk_user_action,priority:2"`
	Detail    string `json:"detail"`
	Snippet   string `json:"snippet"`
	Score     int    `json:"score" gorm:"default:0"`
	RequestId string `json:"request_id" gorm:"type:varchar(64);default:''"`
	ModelName string `json:"model_name" gorm:"index;default:''"`
	Ip        string `json:"ip" gorm:"default:''"`
}

const (
	RiskSourceSyncWord    = "sync_word"
	RiskSourceSyncPattern = "sync_pattern"
	RiskSourceModeration  = "moderation"
	RiskSourcePenalty     = "penalty"

	RiskActionBlocked      = "blocked"
	RiskActionFlagged      = "flagged"
	RiskActionForcedReview = "forced_review"
	RiskActionTempBan      = "temp_ban"
	RiskActionPermBan      = "perm_ban"
)

const riskSnippetMaxBytes = 2048

// RecordRiskEvent 异步写入违规事件,不阻塞请求路径。
func RecordRiskEvent(event *RiskEvent) {
	if event.CreatedAt == 0 {
		event.CreatedAt = common.GetTimestamp()
	}
	if len(event.Snippet) > riskSnippetMaxBytes {
		event.Snippet = event.Snippet[:riskSnippetMaxBytes]
	}
	gopool.Go(func() {
		if err := DB.Create(event).Error; err != nil {
			common.SysError("failed to record risk event: " + err.Error())
		}
	})
}

// EnableAbuseBannedUser 将被自动永久封禁的用户恢复为启用状态,并失效相关缓存。
// 仅当用户当前为 disabled 时才更新,避免误改其他状态。
func EnableAbuseBannedUser(userId int) error {
	var user User
	if err := DB.Where("id = ?", userId).First(&user).Error; err != nil {
		return err
	}
	if user.Status != common.UserStatusDisabled {
		return nil
	}
	if err := DB.Model(&User{}).Where("id = ?", userId).Update("status", common.UserStatusEnabled).Error; err != nil {
		return err
	}
	_ = InvalidateUserCache(userId)
	_ = InvalidateUserTokensCache(userId)
	return nil
}

// CountUserTempBans 统计用户历史临时封禁次数,用于判定是否升级为永久封禁。
func CountUserTempBans(userId int) (int64, error) {
	var count int64
	err := DB.Model(&RiskEvent{}).
		Where("user_id = ? AND action = ?", userId, RiskActionTempBan).
		Count(&count).Error
	return count, err
}

type RiskEventQuery struct {
	UserId         int
	Source         string
	Action         string
	StartTimestamp int64
	EndTimestamp   int64
	StartIdx       int
	Num            int
}

func GetRiskEvents(q RiskEventQuery) ([]*RiskEvent, int64, error) {
	tx := DB.Model(&RiskEvent{})
	if q.UserId != 0 {
		tx = tx.Where("user_id = ?", q.UserId)
	}
	if q.Source != "" {
		tx = tx.Where("source = ?", q.Source)
	}
	if q.Action != "" {
		tx = tx.Where("action = ?", q.Action)
	}
	if q.StartTimestamp != 0 {
		tx = tx.Where("created_at >= ?", q.StartTimestamp)
	}
	if q.EndTimestamp != 0 {
		tx = tx.Where("created_at <= ?", q.EndTimestamp)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var events []*RiskEvent
	err := tx.Order("id desc").Limit(q.Num).Offset(q.StartIdx).Find(&events).Error
	return events, total, err
}
