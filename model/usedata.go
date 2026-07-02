package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// QuotaData 柱状图数据
type QuotaData struct {
	Id        int    `json:"id"`
	UserID    int    `json:"user_id" gorm:"index"`
	Username  string `json:"username" gorm:"index:idx_qdt_model_user_name,priority:2;size:64;default:''"`
	ModelName string `json:"model_name" gorm:"index:idx_qdt_model_user_name,priority:1;size:64;default:''"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index:idx_qdt_created_at,priority:2"`
	UseGroup  string `json:"use_group" gorm:"index;size:64;default:''"`
	TokenID   int    `json:"token_id" gorm:"index;default:0"`
	ChannelID int    `json:"channel_id" gorm:"index;default:0"`
	NodeName  string `json:"node_name" gorm:"index;size:64;default:''"`
	TokenUsed int    `json:"token_used" gorm:"default:0"`
	Count     int    `json:"count" gorm:"default:0"`
	Quota     int    `json:"quota" gorm:"default:0"`
}

type QuotaDataLogParams struct {
	UserID    int
	Username  string
	ModelName string
	Quota     int
	CreatedAt int64
	TokenUsed int
	UseGroup  string
	TokenID   int
	ChannelID int
	NodeName  string
}

func UpdateQuotaData() {
	for {
		if common.DataExportEnabled {
			common.SysLog("正在更新数据看板数据...")
			SaveQuotaDataCache()
		}
		time.Sleep(time.Duration(common.DataExportInterval) * time.Minute)
	}
}

var CacheQuotaData = make(map[string]*QuotaData)
var CacheQuotaDataLock = sync.Mutex{}

func logQuotaDataCache(quotaData *QuotaData) {
	key := fmt.Sprintf("%d\x00%s\x00%s\x00%d\x00%s\x00%d\x00%d\x00%s",
		quotaData.UserID,
		quotaData.Username,
		quotaData.ModelName,
		quotaData.CreatedAt,
		quotaData.UseGroup,
		quotaData.TokenID,
		quotaData.ChannelID,
		quotaData.NodeName,
	)
	count := quotaData.Count
	quota := quotaData.Quota
	tokenUsed := quotaData.TokenUsed
	cachedQuotaData, ok := CacheQuotaData[key]
	if ok {
		cachedQuotaData.Count += count
		cachedQuotaData.Quota += quota
		cachedQuotaData.TokenUsed += tokenUsed
		quotaData = cachedQuotaData
	}
	CacheQuotaData[key] = quotaData
}

func LogQuotaData(params QuotaDataLogParams) {
	// 只精确到小时
	createdAt := params.CreatedAt - (params.CreatedAt % 3600)
	quotaData := &QuotaData{
		UserID:    params.UserID,
		Username:  params.Username,
		ModelName: params.ModelName,
		CreatedAt: createdAt,
		UseGroup:  params.UseGroup,
		TokenID:   params.TokenID,
		ChannelID: params.ChannelID,
		NodeName:  params.NodeName,
		Count:     1,
		Quota:     params.Quota,
		TokenUsed: params.TokenUsed,
	}

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	logQuotaDataCache(quotaData)
}

func SaveQuotaDataCache() {
	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	size := len(CacheQuotaData)
	// 如果缓存中有数据，就保存到数据库中
	// 1. 先查询数据库中是否有数据
	// 2. 如果有数据，就更新数据
	// 3. 如果没有数据，就插入数据
	for _, quotaData := range CacheQuotaData {
		quotaDataDB := &QuotaData{}
		DB.Table("quota_data").
			Where("user_id = ? and username = ? and model_name = ? and created_at = ? and use_group = ? and token_id = ? and channel_id = ? and node_name = ?",
				quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt, quotaData.UseGroup, quotaData.TokenID, quotaData.ChannelID, quotaData.NodeName).
			First(quotaDataDB)
		if quotaDataDB.Id > 0 {
			//quotaDataDB.Count += quotaData.Count
			//quotaDataDB.Quota += quotaData.Quota
			//DB.Table("quota_data").Save(quotaDataDB)
			increaseQuotaData(quotaData)
		} else {
			DB.Table("quota_data").Create(quotaData)
		}
	}
	CacheQuotaData = make(map[string]*QuotaData)
	common.SysLog(fmt.Sprintf("保存数据看板数据成功，共保存%d条数据", size))
}

func increaseQuotaData(quotaData *QuotaData) {
	err := DB.Table("quota_data").
		Where("user_id = ? and username = ? and model_name = ? and created_at = ? and use_group = ? and token_id = ? and channel_id = ? and node_name = ?",
			quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt, quotaData.UseGroup, quotaData.TokenID, quotaData.ChannelID, quotaData.NodeName).
		Updates(map[string]interface{}{
			"count":      gorm.Expr("count + ?", quotaData.Count),
			"quota":      gorm.Expr("quota + ?", quotaData.Quota),
			"token_used": gorm.Expr("token_used + ?", quotaData.TokenUsed),
		}).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("increaseQuotaData error: %s", err))
	}
}

func quotaDataLogBucketExpr(column string) string {
	switch common.LogDatabaseType() {
	case common.DatabaseTypeMySQL:
		return "FLOOR(" + column + " / 3600) * 3600"
	case common.DatabaseTypeClickHouse:
		return "intDiv(" + column + ", 3600) * 3600"
	default:
		return "(" + column + " / 3600) * 3600"
	}
}

func quotaDataLogBaseQuery(startTime int64, endTime int64) *gorm.DB {
	query := LOG_DB.Table("logs").Where("type = ?", LogTypeConsume)
	if startTime != 0 {
		query = query.Where("created_at >= ?", startTime)
	}
	if endTime != 0 {
		query = query.Where("created_at <= ?", endTime)
	}
	return query
}

func quotaDataLogSelect(groupColumns ...string) string {
	bucketExpr := quotaDataLogBucketExpr("created_at")
	columns := append([]string{}, groupColumns...)
	columns = append(columns,
		bucketExpr+" as created_at",
		"count(*) as count",
		"COALESCE(sum(quota), 0) as quota",
		"COALESCE(sum(prompt_tokens), 0) + COALESCE(sum(completion_tokens), 0) as token_used",
	)
	return strings.Join(columns, ", ")
}

func quotaDataLogGroup(groupColumns ...string) string {
	bucketExpr := quotaDataLogBucketExpr("created_at")
	columns := append([]string{}, groupColumns...)
	columns = append(columns, bucketExpr)
	return strings.Join(columns, ", ")
}

func GetQuotaDataByUsername(username string, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	quotaDatas := make([]*QuotaData, 0)
	err = quotaDataLogBaseQuery(startTime, endTime).
		Select(quotaDataLogSelect("user_id", "username", "model_name")).
		Where("username = ?", username).
		Group(quotaDataLogGroup("user_id", "username", "model_name")).
		Order("created_at ASC").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataByUserId(userId int, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	quotaDatas := make([]*QuotaData, 0)
	err = quotaDataLogBaseQuery(startTime, endTime).
		Select(quotaDataLogSelect("user_id", "username", "model_name")).
		Where("user_id = ?", userId).
		Group(quotaDataLogGroup("user_id", "username", "model_name")).
		Order("created_at ASC").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataGroupByUser(startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	quotaDatas := make([]*QuotaData, 0)
	err = quotaDataLogBaseQuery(startTime, endTime).
		Select(quotaDataLogSelect("username")).
		Group(quotaDataLogGroup("username")).
		Order("created_at ASC").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetAllQuotaDates(startTime int64, endTime int64, username string) (quotaData []*QuotaData, err error) {
	if username != "" {
		return GetQuotaDataByUsername(username, startTime, endTime)
	}
	quotaDatas := make([]*QuotaData, 0)
	err = quotaDataLogBaseQuery(startTime, endTime).
		Select(quotaDataLogSelect("model_name")).
		Group(quotaDataLogGroup("model_name")).
		Order("created_at ASC").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

type QuotaDataRebuildParams struct {
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
	ModelName string `json:"model_name,omitempty"`
	BatchSize int    `json:"batch_size,omitempty"`
}

type QuotaDataRebuildResult struct {
	StartTime    int64  `json:"start_time"`
	EndTime      int64  `json:"end_time"`
	ModelName    string `json:"model_name,omitempty"`
	DeletedRows  int64  `json:"deleted_rows"`
	InsertedRows int64  `json:"inserted_rows"`
	LogRows      int64  `json:"log_rows"`
	Quota        int64  `json:"quota"`
	TokenUsed    int64  `json:"token_used"`
}

func RebuildQuotaDataFromLogs(ctx context.Context, params QuotaDataRebuildParams) (QuotaDataRebuildResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if params.StartTime <= 0 {
		return QuotaDataRebuildResult{}, errors.New("start time is required")
	}
	if params.EndTime <= 0 {
		return QuotaDataRebuildResult{}, errors.New("end time is required")
	}
	if params.EndTime < params.StartTime {
		return QuotaDataRebuildResult{}, errors.New("end time must be greater than or equal to start time")
	}
	if params.BatchSize <= 0 {
		params.BatchSize = 1000
	}

	rebuildStart := params.StartTime - params.StartTime%3600
	rebuildEnd := params.EndTime - params.EndTime%3600
	aggregateParams := params
	aggregateParams.StartTime = rebuildStart
	aggregateParams.EndTime = rebuildEnd + 3599
	result := QuotaDataRebuildResult{
		StartTime: rebuildStart,
		EndTime:   rebuildEnd,
		ModelName: params.ModelName,
	}

	rows, err := aggregateQuotaDataRowsFromLogs(ctx, aggregateParams)
	if err != nil {
		return result, err
	}
	for _, row := range rows {
		result.LogRows += int64(row.Count)
		result.Quota += int64(row.Quota)
		result.TokenUsed += int64(row.TokenUsed)
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		deleteQuery := tx.Table("quota_data").Where("created_at >= ? and created_at <= ?", rebuildStart, rebuildEnd)
		if params.ModelName != "" {
			deleteQuery = deleteQuery.Where("model_name = ?", params.ModelName)
		}
		deleteResult := deleteQuery.Delete(&QuotaData{})
		if deleteResult.Error != nil {
			return deleteResult.Error
		}
		result.DeletedRows = deleteResult.RowsAffected
		if len(rows) == 0 {
			return nil
		}
		if err := tx.CreateInBatches(rows, params.BatchSize).Error; err != nil {
			return err
		}
		result.InsertedRows = int64(len(rows))
		return nil
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

func aggregateQuotaDataRowsFromLogs(ctx context.Context, params QuotaDataRebuildParams) ([]*QuotaData, error) {
	bucketExpr := quotaDataLogBucketExpr("created_at")
	selectColumns := []string{
		"user_id",
		"username",
		"model_name",
		logGroupCol + " as use_group",
		"token_id",
		"channel_id",
		"? as node_name",
		bucketExpr + " as created_at",
		"count(*) as count",
		"COALESCE(sum(quota), 0) as quota",
		"COALESCE(sum(prompt_tokens), 0) + COALESCE(sum(completion_tokens), 0) as token_used",
	}
	groupColumns := []string{
		"user_id",
		"username",
		"model_name",
		logGroupCol,
		"token_id",
		"channel_id",
		bucketExpr,
	}
	rows := make([]*QuotaData, 0)
	query := quotaDataLogBaseQuery(params.StartTime, params.EndTime).WithContext(ctx)
	if params.ModelName != "" {
		query = query.Where("model_name = ?", params.ModelName)
	}
	err := query.
		Select(strings.Join(selectColumns, ", "), common.NodeName).
		Group(strings.Join(groupColumns, ", ")).
		Order("created_at ASC").
		Find(&rows).Error
	return rows, err
}
