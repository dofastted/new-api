package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	ModelRequestRateLimitCountMark        = "MRRL"
	ModelRequestRateLimitSuccessCountMark = "MRRLS"
)

const modelRateLimitBusyMessage = "We're experiencing high demand right now. Please retry in a moment."

var modelRateLimitWaitForSlot = service.WaitForSlot

var modelRateLimitWaitingCounters sync.Map

type modelRateLimitCheck func(context.Context) (bool, error)

type modelRateLimitWaitingCount struct {
	value atomic.Int64
}

// 检查Redis中的请求限制
func checkRedisRateLimit(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64) (bool, error) {
	// 如果maxCount为0，表示不限制
	if maxCount == 0 {
		return true, nil
	}

	// 获取当前计数
	length, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		return false, err
	}

	// 如果未达到限制，允许请求
	if length < int64(maxCount) {
		return true, nil
	}

	// 检查时间窗口
	oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
	oldTime, err := time.Parse(timeFormat, oldTimeStr)
	if err != nil {
		return false, err
	}

	nowTimeStr := time.Now().Format(timeFormat)
	nowTime, err := time.Parse(timeFormat, nowTimeStr)
	if err != nil {
		return false, err
	}
	// 如果在时间窗口内已达到限制，拒绝请求
	subTime := nowTime.Sub(oldTime).Seconds()
	if int64(subTime) < duration {
		rdb.Expire(ctx, key, time.Duration(setting.ModelRequestRateLimitDurationMinutes)*time.Minute)
		return false, nil
	}

	return true, nil
}

// 记录Redis请求
func recordRedisRequest(ctx context.Context, rdb *redis.Client, key string, maxCount int) {
	// 如果maxCount为0，不记录请求
	if maxCount == 0 {
		return
	}

	now := time.Now().Format(timeFormat)
	rdb.LPush(ctx, key, now)
	rdb.LTrim(ctx, key, 0, int64(maxCount-1))
	rdb.Expire(ctx, key, time.Duration(setting.ModelRequestRateLimitDurationMinutes)*time.Minute)
}

func waitForModelRateLimitSlot(c *gin.Context, userId string, tryAcquire modelRateLimitCheck) bool {
	if !setting.ModelRequestRateLimitWaitEnabled || setting.RateLimitWaitTimeoutSeconds <= 0 {
		abortWithOpenAiMessage(c, http.StatusTooManyRequests, modelRateLimitBusyMessage)
		return false
	}

	release, ok, err := acquireModelRateLimitWaitingSlot(c.Request.Context(), userId, setting.RateLimitMaxWaitingPerUser)
	if err != nil {
		fmt.Println("增加排队计数失败:", err.Error())
		abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
		return false
	}
	if !ok {
		abortWithOpenAiMessage(c, http.StatusTooManyRequests, modelRateLimitBusyMessage)
		return false
	}
	defer release()

	budget := service.RateWaitBudget(c, setting.RateLimitWaitTimeoutSeconds)
	if budget <= 0 {
		abortWithOpenAiMessage(c, http.StatusTooManyRequests, modelRateLimitBusyMessage)
		return false
	}

	var acquireErr error
	waitErr := modelRateLimitWaitForSlot(c.Request.Context(), func() bool {
		allowed, err := tryAcquire(c.Request.Context())
		if err != nil {
			acquireErr = err
			return true
		}
		return allowed
	}, budget)
	if acquireErr != nil {
		fmt.Println("等待请求数限制失败:", acquireErr.Error())
		abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
		return false
	}
	if waitErr == nil {
		return true
	}
	if errors.Is(waitErr, context.Canceled) || errors.Is(waitErr, context.DeadlineExceeded) {
		c.Abort()
		return false
	}
	if errors.Is(waitErr, service.ErrRateWaitTimeout) {
		c.Header("Retry-After", strconv.Itoa(setting.RateLimitWaitTimeoutSeconds))
		abortWithOpenAiMessage(c, http.StatusTooManyRequests, modelRateLimitBusyMessage)
		return false
	}

	abortWithOpenAiMessage(c, http.StatusTooManyRequests, modelRateLimitBusyMessage)
	return false
}

func acquireModelRateLimitWaitingSlot(ctx context.Context, userId string, maxWaiting int) (func(), bool, error) {
	if maxWaiting <= 0 {
		return func() {}, false, nil
	}
	if common.RedisEnabled && common.RDB != nil {
		return acquireRedisModelRateLimitWaitingSlot(ctx, userId, maxWaiting)
	}
	release, ok := acquireMemoryModelRateLimitWaitingSlot(userId, maxWaiting)
	return release, ok, nil
}

func acquireRedisModelRateLimitWaitingSlot(ctx context.Context, userId string, maxWaiting int) (func(), bool, error) {
	key := fmt.Sprintf("rateLimit:waiting:%s", userId)
	count, err := common.RDB.Incr(ctx, key).Result()
	if err != nil {
		return nil, false, err
	}
	common.RDB.Expire(ctx, key, modelRateLimitWaitingTTL())
	if count > int64(maxWaiting) {
		_, _ = common.RDB.Decr(ctx, key).Result()
		return func() {}, false, nil
	}

	var once sync.Once
	release := func() {
		once.Do(func() {
			count, err := common.RDB.Decr(context.Background(), key).Result()
			if err == nil && count <= 0 {
				_ = common.RDB.Del(context.Background(), key).Err()
			}
		})
	}
	return release, true, nil
}

func acquireMemoryModelRateLimitWaitingSlot(userId string, maxWaiting int) (func(), bool) {
	value, _ := modelRateLimitWaitingCounters.LoadOrStore(userId, &modelRateLimitWaitingCount{})
	counter := value.(*modelRateLimitWaitingCount)
	count := counter.value.Add(1)
	if count > int64(maxWaiting) {
		if counter.value.Add(-1) <= 0 {
			modelRateLimitWaitingCounters.Delete(userId)
		}
		return func() {}, false
	}

	var released atomic.Bool
	return func() {
		if released.CompareAndSwap(false, true) && counter.value.Add(-1) <= 0 {
			modelRateLimitWaitingCounters.Delete(userId)
		}
	}, true
}

func modelRateLimitWaitingTTL() time.Duration {
	ttl := time.Duration(setting.RateLimitWaitTimeoutSeconds)*time.Second + time.Minute
	if ttl < time.Minute {
		return time.Minute
	}
	return ttl
}

// Redis限流处理器
func redisRateLimitHandler(duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		rdb := common.RDB

		// 1. 检查成功请求数限制
		successKey := fmt.Sprintf("rateLimit:%s:%s", ModelRequestRateLimitSuccessCountMark, userId)
		totalKey := fmt.Sprintf("rateLimit:%s", userId)
		tb := limiter.New(c.Request.Context(), rdb)
		tryAcquire := func(ctx context.Context) (bool, error) {
			allowed, err := checkRedisRateLimit(ctx, rdb, successKey, successMaxCount, duration)
			if err != nil || !allowed {
				return allowed, err
			}
			if totalMaxCount == 0 {
				return true, nil
			}
			return tb.Allow(
				ctx,
				totalKey,
				limiter.WithCapacity(int64(totalMaxCount)*duration),
				limiter.WithRate(int64(totalMaxCount)),
				limiter.WithRequested(duration),
			)
		}

		allowed, err := tryAcquire(c.Request.Context())
		if err != nil {
			fmt.Println("检查成功请求数限制失败:", err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			if !waitForModelRateLimitSlot(c, userId, tryAcquire) {
				return
			}
		}

		// 4. 处理请求
		c.Next()

		// 5. 如果请求成功，记录成功请求
		if c.Writer.Status() < 400 {
			recordRedisRequest(context.Background(), rdb, successKey, successMaxCount)
		}
	}
}

// 内存限流处理器
func memoryRateLimitHandler(duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)

	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		totalKey := ModelRequestRateLimitCountMark + userId
		successKey := ModelRequestRateLimitSuccessCountMark + userId

		tryAcquire := func(context.Context) (bool, error) {
			if !inMemoryRateLimiter.Allow(successKey, successMaxCount, duration) {
				return false, nil
			}
			if totalMaxCount > 0 && !inMemoryRateLimiter.Request(totalKey, totalMaxCount, duration) {
				return false, nil
			}
			return true, nil
		}

		allowed, err := tryAcquire(c.Request.Context())
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			if !waitForModelRateLimitSlot(c, userId, tryAcquire) {
				return
			}
		}

		// 3. 处理请求
		c.Next()

		// 4. 如果请求成功，记录到实际的成功请求计数中
		if c.Writer.Status() < 400 {
			inMemoryRateLimiter.Request(successKey, successMaxCount, duration)
		}
	}
}

// ModelRequestRateLimit 模型请求限流中间件
func ModelRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 在每个请求时检查是否启用限流
		if !setting.ModelRequestRateLimitEnabled {
			c.Next()
			return
		}

		// 计算限流参数
		duration := int64(setting.ModelRequestRateLimitDurationMinutes * 60)
		totalMaxCount := setting.ModelRequestRateLimitCount
		successMaxCount := setting.ModelRequestRateLimitSuccessCount

		// User level group owns RPM limits; token/provider group must not affect them.
		group := common.GetContextKeyString(c, constant.ContextKeyUserGroup)

		//获取分组的限流配置
		groupTotalCount, groupSuccessCount, found := setting.GetGroupRateLimit(group)
		if found {
			totalMaxCount = groupTotalCount
			successMaxCount = groupSuccessCount
		}

		// 根据存储类型选择并执行限流处理器
		if common.RedisEnabled {
			redisRateLimitHandler(duration, totalMaxCount, successMaxCount)(c)
		} else {
			memoryRateLimitHandler(duration, totalMaxCount, successMaxCount)(c)
		}
	}
}
