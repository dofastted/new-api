package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelRateLimitWaitingCounter(t *testing.T) {
	restoreModelRateLimitTestState(t)

	release, ok, err := acquireModelRateLimitWaitingSlot(context.Background(), "user-1", 1)
	require.NoError(t, err)
	require.True(t, ok)

	_, ok, err = acquireModelRateLimitWaitingSlot(context.Background(), "user-1", 1)
	require.NoError(t, err)
	assert.False(t, ok)

	release()

	release, ok, err = acquireModelRateLimitWaitingSlot(context.Background(), "user-1", 1)
	require.NoError(t, err)
	require.True(t, ok)
	release()
}

func TestModelRequestRateLimitMiddlewareWaitsAndAllows(t *testing.T) {
	restoreModelRateLimitTestState(t)
	setModelRateLimitTestSettings()
	seedSuccessfulRequestLimit(t)

	waitCalled := false
	modelRateLimitWaitForSlot = func(ctx context.Context, tryAcquire func() bool, budget time.Duration) error {
		waitCalled = true
		assert.False(t, tryAcquire())
		resetMemoryRateLimiter()
		assert.True(t, tryAcquire())
		return nil
	}

	recorder := performModelRateLimitRequest(t)

	assert.True(t, waitCalled)
	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestModelRequestRateLimitMiddlewareTimeoutReturnsRetryAfter(t *testing.T) {
	restoreModelRateLimitTestState(t)
	setModelRateLimitTestSettings()
	seedSuccessfulRequestLimit(t)

	modelRateLimitWaitForSlot = func(ctx context.Context, tryAcquire func() bool, budget time.Duration) error {
		return service.RateWaitTimeoutError{Budget: budget}
	}

	recorder := performModelRateLimitRequest(t)

	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.Equal(t, "60", recorder.Header().Get("Retry-After"))
	assert.Contains(t, recorder.Body.String(), modelRateLimitBusyMessage)
}

func TestModelRequestRateLimitMiddlewareSwitchOffReturnsImmediate429(t *testing.T) {
	restoreModelRateLimitTestState(t)
	setModelRateLimitTestSettings()
	setting.ModelRequestRateLimitWaitEnabled = false
	seedSuccessfulRequestLimit(t)

	waitCalled := false
	modelRateLimitWaitForSlot = func(ctx context.Context, tryAcquire func() bool, budget time.Duration) error {
		waitCalled = true
		return nil
	}

	recorder := performModelRateLimitRequest(t)

	assert.False(t, waitCalled)
	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.Empty(t, recorder.Header().Get("Retry-After"))
	assert.Contains(t, recorder.Body.String(), modelRateLimitBusyMessage)
}

func performModelRateLimitRequest(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 1)
		c.Next()
	})
	router.Use(ModelRequestRateLimit())
	router.GET("/v1/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	router.ServeHTTP(recorder, req)
	return recorder
}

func setModelRateLimitTestSettings() {
	setting.ModelRequestRateLimitEnabled = true
	setting.ModelRequestRateLimitWaitEnabled = true
	setting.RateLimitWaitTimeoutSeconds = 60
	setting.RateLimitMaxWaitingPerUser = 10
	setting.ModelRequestRateLimitDurationMinutes = 1
	setting.ModelRequestRateLimitCount = 0
	setting.ModelRequestRateLimitSuccessCount = 1
	setting.ModelRequestRateLimitGroup = map[string][2]int{}
}

func seedSuccessfulRequestLimit(t *testing.T) {
	t.Helper()

	resetMemoryRateLimiter()
	require.True(t, inMemoryRateLimiter.Request(ModelRequestRateLimitSuccessCountMark+"1", 1, 60))
}

func resetMemoryRateLimiter() {
	inMemoryRateLimiter = common.InMemoryRateLimiter{}
	inMemoryRateLimiter.Init(time.Minute)
}

func restoreModelRateLimitTestState(t *testing.T) {
	t.Helper()

	origRedisEnabled := common.RedisEnabled
	origRDB := common.RDB
	origModelRequestRateLimitEnabled := setting.ModelRequestRateLimitEnabled
	origModelRequestRateLimitWaitEnabled := setting.ModelRequestRateLimitWaitEnabled
	origRateLimitWaitTimeoutSeconds := setting.RateLimitWaitTimeoutSeconds
	origRateLimitMaxWaitingPerUser := setting.RateLimitMaxWaitingPerUser
	origDuration := setting.ModelRequestRateLimitDurationMinutes
	origCount := setting.ModelRequestRateLimitCount
	origSuccessCount := setting.ModelRequestRateLimitSuccessCount
	origGroup := setting.ModelRequestRateLimitGroup
	origWaitForSlot := modelRateLimitWaitForSlot

	common.RedisEnabled = false
	common.RDB = nil
	resetMemoryRateLimiter()
	modelRateLimitWaitingCounters.Range(func(key, value any) bool {
		modelRateLimitWaitingCounters.Delete(key)
		return true
	})

	t.Cleanup(func() {
		common.RedisEnabled = origRedisEnabled
		common.RDB = origRDB
		setting.ModelRequestRateLimitEnabled = origModelRequestRateLimitEnabled
		setting.ModelRequestRateLimitWaitEnabled = origModelRequestRateLimitWaitEnabled
		setting.RateLimitWaitTimeoutSeconds = origRateLimitWaitTimeoutSeconds
		setting.RateLimitMaxWaitingPerUser = origRateLimitMaxWaitingPerUser
		setting.ModelRequestRateLimitDurationMinutes = origDuration
		setting.ModelRequestRateLimitCount = origCount
		setting.ModelRequestRateLimitSuccessCount = origSuccessCount
		setting.ModelRequestRateLimitGroup = origGroup
		modelRateLimitWaitForSlot = origWaitForSlot
		resetMemoryRateLimiter()
		modelRateLimitWaitingCounters.Range(func(key, value any) bool {
			modelRateLimitWaitingCounters.Delete(key)
			return true
		})
	})
}
