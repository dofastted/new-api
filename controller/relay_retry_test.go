package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldRetrySkipsClientCanceledErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		err  *types.NewAPIError
	}{
		{
			name: "wrapped context canceled",
			err: types.NewErrorWithStatusCode(
				fmt.Errorf("request context done: %w", context.Canceled),
				types.ErrorCodeBadResponse,
				http.StatusInternalServerError,
			),
		},
		{
			name: "client gone stream marker",
			err: types.NewErrorWithStatusCode(
				fmt.Errorf("stream ended: reason=client_gone end_error=%q", context.Canceled.Error()),
				types.ErrorCodeBadResponse,
				http.StatusInternalServerError,
			),
		},
		{
			name: "channel-coded cancellation",
			err: types.NewErrorWithStatusCode(
				fmt.Errorf("request context done: %w", context.Canceled),
				types.ErrorCodeChannelInvalidKey,
				http.StatusInternalServerError,
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

			assert.False(t, shouldRetry(ctx, tt.err, 3))
		})
	}
}

func TestShouldRetryDoesNotFallbackForTooManyRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	orig := operation_setting.AutomaticRetryStatusCodeRanges
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = orig })
	require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString("429,500"))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	err := types.NewErrorWithStatusCode(
		fmt.Errorf("upstream rate limited"),
		types.ErrorCodeBadResponse,
		http.StatusTooManyRequests,
	)

	assert.False(t, shouldRetry(ctx, err, 3))
}

func TestShouldContinueAfterUpstreamRateLimitUsesWaitBudget(t *testing.T) {
	restoreRelayChannelRateLimitTestState(t)
	ctx := newRelayRateLimitTestContext()
	err := types.NewErrorWithStatusCode(
		fmt.Errorf("upstream rate limited"),
		types.ErrorCodeBadResponse,
		http.StatusTooManyRequests,
	)

	assert.True(t, shouldContinueAfterUpstreamRateLimit(ctx, err))
	assert.Empty(t, ctx.Writer.Header().Get("Retry-After"))
}

func TestShouldContinueAfterUpstreamRateLimitDisabledWithoutSpinning(t *testing.T) {
	restoreRelayChannelRateLimitTestState(t)
	setting.ChannelRateLimitCooldownSeconds = 0
	ctx := newRelayRateLimitTestContext()
	err := types.NewErrorWithStatusCode(
		fmt.Errorf("upstream rate limited"),
		types.ErrorCodeBadResponse,
		http.StatusTooManyRequests,
	)

	assert.False(t, shouldContinueAfterUpstreamRateLimit(ctx, err))
}

func TestShouldContinueAfterTaskUpstreamRateLimitSkipsLocalErrors(t *testing.T) {
	restoreRelayChannelRateLimitTestState(t)
	ctx := newRelayRateLimitTestContext()
	taskErr := &dto.TaskError{StatusCode: http.StatusTooManyRequests, LocalError: true}

	assert.False(t, shouldContinueAfterTaskUpstreamRateLimit(ctx, taskErr))

	taskErr.LocalError = false
	assert.True(t, shouldContinueAfterTaskUpstreamRateLimit(ctx, taskErr))
}

func TestShouldRetryAllowsNonTooManyRequestsRetryableStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	orig := operation_setting.AutomaticRetryStatusCodeRanges
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = orig })
	require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString("500"))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	err := types.NewErrorWithStatusCode(
		fmt.Errorf("upstream failed"),
		types.ErrorCodeBadResponse,
		http.StatusInternalServerError,
	)

	assert.True(t, shouldRetry(ctx, err, 1))
}

func TestShouldRetrySkipsClaudeCliAccessDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	orig := operation_setting.AutomaticRetryStatusCodeRanges
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = orig })
	require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString("403,500"))
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	err := types.NewOpenAIError(
		errors.New("This API endpoint is only accessible via the official Claude CLI"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusForbidden,
	)

	assert.False(t, shouldRetry(ctx, err, 3))
	assert.False(t, isRetryableChannelFailure(err))
}

func TestProcessChannelErrorRecordsHighLoadChannelCircuitTrace(t *testing.T) {
	restoreRelayCircuitTestState(t, 42001)
	ctx := newRelayCircuitTestContext(42001, highLoadCircuitSettings(http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable))
	err := types.NewOpenAIError(
		errors.New("upstream overloaded"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusServiceUnavailable,
	)

	processChannelError(ctx, *types.NewChannelError(42001, constant.ChannelTypeAnthropic, "claude-sub", false, "", false), err)

	status := service.GetChannelCircuitStatus(42001)
	require.Equal(t, model.ChannelCircuitOpen, status.State)
	assert.Equal(t, service.ChannelCircuitClassHighLoadTemporarilyUnavailable, status.LastCategory)
	assert.Equal(t, 1, status.FailureCount)
	assert.Greater(t, status.NextAttemptUnix, time.Now().Unix())

	chain := service.GetChannelChain(ctx)
	require.Len(t, chain, 1)
	entry := chain[0]
	assert.Equal(t, 42001, entry.ChannelId)
	assert.Equal(t, constant.ChannelTypeAnthropic, entry.ChannelType)
	assert.Equal(t, service.ChannelChainReasonFailure, entry.Reason)
	assert.Equal(t, string(model.ChannelCircuitOpen), entry.CircuitState)
	assert.Equal(t, service.ChannelCircuitClassHighLoadTemporarilyUnavailable, entry.CircuitClass)
	assert.Equal(t, status.NextAttemptUnix, entry.CircuitOpenUntil)
	assert.Equal(t, "same_group_retry", entry.FallbackCandidate)
}

func TestProcessChannelErrorDoesNotOpenCircuitForOfficialClaudeCli403(t *testing.T) {
	restoreRelayCircuitTestState(t, 42002)
	ctx := newRelayCircuitTestContext(42002, highLoadCircuitSettings(http.StatusForbidden))
	err := types.NewOpenAIError(
		errors.New("This API endpoint is only accessible via the official Claude CLI"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusForbidden,
	)

	processChannelError(ctx, *types.NewChannelError(42002, constant.ChannelTypeAnthropic, "claude-sub", false, "", false), err)

	status := service.GetChannelCircuitStatus(42002)
	assert.Equal(t, model.ChannelCircuitClosed, status.State)
	assert.Equal(t, 0, status.FailureCount)

	chain := service.GetChannelChain(ctx)
	require.Len(t, chain, 1)
	assert.Equal(t, service.ChannelChainCircuitStateClosed, chain[0].CircuitState)
	assert.Empty(t, chain[0].CircuitClass)
	assert.Zero(t, chain[0].CircuitOpenUntil)
}

func TestProcessChannelErrorDoesNotOpenCircuitForClientCancellation(t *testing.T) {
	restoreRelayCircuitTestState(t, 42003)
	ctx := newRelayCircuitTestContext(42003, highLoadCircuitSettings(http.StatusServiceUnavailable))
	err := types.NewErrorWithStatusCode(
		fmt.Errorf("request context done: %w", context.Canceled),
		types.ErrorCodeBadResponse,
		http.StatusServiceUnavailable,
	)

	processChannelError(ctx, *types.NewChannelError(42003, constant.ChannelTypeAnthropic, "claude-sub", false, "", false), err)

	status := service.GetChannelCircuitStatus(42003)
	assert.Equal(t, model.ChannelCircuitClosed, status.State)
	assert.Equal(t, 0, status.FailureCount)

	chain := service.GetChannelChain(ctx)
	require.Len(t, chain, 1)
	assert.Equal(t, service.ChannelChainCircuitStateClosed, chain[0].CircuitState)
	assert.Empty(t, chain[0].CircuitClass)
	assert.Zero(t, chain[0].CircuitOpenUntil)
}

func TestProcessChannelErrorDoesNotOpenCircuitForSkipRetry(t *testing.T) {
	restoreRelayCircuitTestState(t, 42004)
	ctx := newRelayCircuitTestContext(42004, highLoadCircuitSettings(http.StatusServiceUnavailable))
	err := types.NewErrorWithStatusCode(
		errors.New("local access policy denied request"),
		types.ErrorCodeAccessDenied,
		http.StatusServiceUnavailable,
		types.ErrOptionWithSkipRetry(),
	)

	processChannelError(ctx, *types.NewChannelError(42004, constant.ChannelTypeAnthropic, "claude-sub", false, "", false), err)

	status := service.GetChannelCircuitStatus(42004)
	assert.Equal(t, model.ChannelCircuitClosed, status.State)
	assert.Equal(t, 0, status.FailureCount)

	chain := service.GetChannelChain(ctx)
	require.Len(t, chain, 1)
	assert.Equal(t, service.ChannelChainCircuitStateClosed, chain[0].CircuitState)
	assert.Empty(t, chain[0].CircuitClass)
	assert.Zero(t, chain[0].CircuitOpenUntil)
}

func TestPrepareTooManyRequestsRetrySelectsAlternateEnabledKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyChannelMultiKeyIndex, 0)
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "key-a")
	channel := multiKeyRetryTestChannel()
	tried := make(map[int]struct{})

	var slept []time.Duration
	origSleep := tooManyRequestsRetrySleep
	tooManyRequestsRetrySleep = func(delay time.Duration) {
		slept = append(slept, delay)
	}
	t.Cleanup(func() { tooManyRequestsRetrySleep = origSleep })

	require.True(t, prepareTooManyRequestsRetry(ctx, channel, "gpt-test", tried, 0))

	assert.Equal(t, []time.Duration{time.Second}, slept)
	assert.Contains(t, tried, 0)
	assert.Equal(t, "key-b", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
	assert.Equal(t, 1, common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex))
	assert.Equal(t, map[int]int{0: common.ChannelStatusEnabled, 1: common.ChannelStatusEnabled}, channel.ChannelInfo.MultiKeyStatusList)
	assert.Empty(t, channel.ChannelInfo.MultiKeyDisabledReason)
}

func TestPrepareTooManyRequestsRetryReturnsFalseWithoutAlternateKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name           string
		channel        *model.Channel
		currentIndex   int
		tried          map[int]struct{}
		currentKey     string
		wantContextKey string
	}{
		{
			name: "single key channel",
			channel: &model.Channel{
				Key: "only-key",
			},
			currentIndex:   0,
			currentKey:     "only-key",
			wantContextKey: "only-key",
		},
		{
			name:           "all multi keys already tried",
			channel:        multiKeyRetryTestChannel(),
			currentIndex:   0,
			tried:          map[int]struct{}{1: {}},
			currentKey:     "key-a",
			wantContextKey: "key-a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(ctx, constant.ContextKeyChannelMultiKeyIndex, tt.currentIndex)
			common.SetContextKey(ctx, constant.ContextKeyChannelKey, tt.currentKey)
			if tt.tried == nil {
				tt.tried = make(map[int]struct{})
			}

			sleepCalled := false
			origSleep := tooManyRequestsRetrySleep
			tooManyRequestsRetrySleep = func(time.Duration) {
				sleepCalled = true
			}
			t.Cleanup(func() { tooManyRequestsRetrySleep = origSleep })

			assert.False(t, prepareTooManyRequestsRetry(ctx, tt.channel, "gpt-test", tt.tried, 0))
			assert.False(t, sleepCalled)
			assert.Equal(t, tt.wantContextKey, common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
			_, selectedChannel := ctx.Get(string(constant.ContextKeyChannelId))
			assert.False(t, selectedChannel)
		})
	}
}

func multiKeyRetryTestChannel() *model.Channel {
	return &model.Channel{
		Key: "key-a\nkey-b",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeySize:       2,
			MultiKeyStatusList: map[int]int{0: common.ChannelStatusEnabled, 1: common.ChannelStatusEnabled},
		},
	}
}

func TestIsRetryableChannelFailureMatchesRetryPolicyAtLimit(t *testing.T) {
	tests := []struct {
		name string
		err  *types.NewAPIError
		want bool
	}{
		{
			name: "retryable upstream 500",
			err: types.NewErrorWithStatusCode(
				fmt.Errorf("upstream failed"),
				types.ErrorCodeBadResponse,
				http.StatusInternalServerError,
			),
			want: true,
		},
		{
			name: "non retryable bad request",
			err: types.NewErrorWithStatusCode(
				fmt.Errorf("bad request"),
				types.ErrorCodeBadResponse,
				http.StatusBadRequest,
			),
			want: false,
		},
		{
			name: "client cancellation",
			err: types.NewErrorWithStatusCode(
				fmt.Errorf("request context done: %w", context.Canceled),
				types.ErrorCodeBadResponse,
				http.StatusInternalServerError,
			),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isRetryableChannelFailure(tt.err))
		})
	}
}

func TestSelectRelayChannelWithRateLimitExcludesLimitedChannel(t *testing.T) {
	restoreRelayChannelRateLimitTestState(t)
	ctx := newRelayRateLimitTestContext()
	retryParam := &service.RetryParam{}
	limited := newRelayRateLimitTestChannel(31001, 0)
	available := newRelayRateLimitTestChannel(31002, 0)
	service.MarkChannelRateLimited(limited.Id, time.Hour)

	channel, err := selectRelayChannelWithRateLimit(ctx, retryParam, func() (*model.Channel, *types.NewAPIError) {
		if _, ok := retryParam.ExcludedChannelIds[limited.Id]; ok {
			return available, nil
		}
		return limited, nil
	})

	require.Nil(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, available.Id, channel.Id)
	assert.Contains(t, retryParam.ExcludedChannelIds, limited.Id)
}

func TestSelectRelayChannelWithRateLimitExcludesRPMFullChannel(t *testing.T) {
	restoreRelayChannelRateLimitTestState(t)
	ctx := newRelayRateLimitTestContext()
	retryParam := &service.RetryParam{}
	limited := newRelayRateLimitTestChannel(31007, 1)
	available := newRelayRateLimitTestChannel(31008, 0)
	require.True(t, service.TryAcquireChannelSlot(limited.Id, limited.GetSetting().RateLimitRPM))

	channel, err := selectRelayChannelWithRateLimit(ctx, retryParam, func() (*model.Channel, *types.NewAPIError) {
		if _, ok := retryParam.ExcludedChannelIds[limited.Id]; ok {
			return available, nil
		}
		return limited, nil
	})

	require.Nil(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, available.Id, channel.Id)
	assert.Contains(t, retryParam.ExcludedChannelIds, limited.Id)
}

func TestSelectRelayChannelWithRateLimitWaitsAndSucceeds(t *testing.T) {
	restoreRelayChannelRateLimitTestState(t)
	ctx := newRelayRateLimitTestContext()
	retryParam := &service.RetryParam{}
	limitedA := newRelayRateLimitTestChannel(31003, 0)
	limitedB := newRelayRateLimitTestChannel(31004, 0)
	availableAfterWait := newRelayRateLimitTestChannel(31005, 0)
	service.MarkChannelRateLimited(limitedA.Id, time.Hour)
	service.MarkChannelRateLimited(limitedB.Id, time.Hour)

	released := false
	relayRateWaitForSlot = func(ctx context.Context, tryAcquire func() bool, budget time.Duration) error {
		assert.Equal(t, time.Duration(setting.RateLimitWaitTimeoutSeconds)*time.Second, budget)
		released = true
		require.True(t, tryAcquire())
		return nil
	}

	channel, err := selectRelayChannelWithRateLimit(ctx, retryParam, func() (*model.Channel, *types.NewAPIError) {
		if released {
			return availableAfterWait, nil
		}
		if _, ok := retryParam.ExcludedChannelIds[limitedA.Id]; !ok {
			return limitedA, nil
		}
		if _, ok := retryParam.ExcludedChannelIds[limitedB.Id]; !ok {
			return limitedB, nil
		}
		return nil, types.NewError(errors.New("no available channel"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	})

	require.Nil(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, availableAfterWait.Id, channel.Id)
	assert.True(t, released)
}

func TestSelectRelayChannelWithRateLimitTimeout(t *testing.T) {
	restoreRelayChannelRateLimitTestState(t)
	ctx := newRelayRateLimitTestContext()
	retryParam := &service.RetryParam{}
	limited := newRelayRateLimitTestChannel(31006, 0)
	service.MarkChannelRateLimited(limited.Id, time.Hour)

	relayRateWaitForSlot = func(ctx context.Context, tryAcquire func() bool, budget time.Duration) error {
		assert.False(t, tryAcquire())
		return service.RateWaitTimeoutError{Budget: budget}
	}

	channel, err := selectRelayChannelWithRateLimit(ctx, retryParam, func() (*model.Channel, *types.NewAPIError) {
		if _, ok := retryParam.ExcludedChannelIds[limited.Id]; ok {
			return nil, types.NewError(errors.New("no available channel"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		}
		return limited, nil
	})

	assert.Nil(t, channel)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusTooManyRequests, err.StatusCode)
	assert.Equal(t, "60", ctx.Writer.Header().Get("Retry-After"))
}

func newRelayRateLimitTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return ctx
}

func newRelayRateLimitTestChannel(id int, rpm int) *model.Channel {
	channel := &model.Channel{
		Id:   id,
		Type: constant.ChannelTypeOpenAI,
		Name: fmt.Sprintf("channel-%d", id),
	}
	if rpm > 0 {
		channel.SetSetting(dto.ChannelSettings{RateLimitRPM: rpm})
	}
	return channel
}

func newRelayCircuitTestContext(channelID int, channelSetting dto.ChannelSettings) *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	common.SetContextKey(ctx, constant.ContextKeyChannelId, channelID)
	common.SetContextKey(ctx, constant.ContextKeyChannelName, "claude-sub")
	common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeAnthropic)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "claude-sub")
	common.SetContextKey(ctx, constant.ContextKeyChannelSetting, channelSetting)
	return ctx
}

func highLoadCircuitSettings(statusCodes ...int) dto.ChannelSettings {
	return dto.ChannelSettings{
		CircuitBreaker: &dto.ChannelCircuitBreakerSettings{
			Enabled:          true,
			FailureThreshold: 1,
			OpenSeconds:      300,
			Rules: []dto.ChannelCircuitBreakerRule{
				{
					Name:        "claude_high_load",
					Class:       service.ChannelCircuitClassHighLoadTemporarilyUnavailable,
					StatusCodes: statusCodes,
				},
			},
		},
	}
}

func restoreRelayCircuitTestState(t *testing.T, channelIDs ...int) {
	t.Helper()

	oldRedisEnabled := common.RedisEnabled
	oldRDB := common.RDB
	oldErrorLogEnabled := constant.ErrorLogEnabled
	common.RedisEnabled = false
	common.RDB = nil
	constant.ErrorLogEnabled = false
	for _, channelID := range channelIDs {
		service.ResetChannelCircuit(channelID)
	}

	t.Cleanup(func() {
		for _, channelID := range channelIDs {
			service.ResetChannelCircuit(channelID)
		}
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRDB
		constant.ErrorLogEnabled = oldErrorLogEnabled
	})
}

func restoreRelayChannelRateLimitTestState(t *testing.T) {
	t.Helper()

	origRedisEnabled := common.RedisEnabled
	origRDB := common.RDB
	origWaitForSlot := relayRateWaitForSlot
	origWaitTimeout := setting.RateLimitWaitTimeoutSeconds
	origCooldown := setting.ChannelRateLimitCooldownSeconds

	common.RedisEnabled = false
	common.RDB = nil
	relayRateWaitForSlot = service.WaitForSlot
	setting.RateLimitWaitTimeoutSeconds = 60
	setting.ChannelRateLimitCooldownSeconds = 30

	t.Cleanup(func() {
		common.RedisEnabled = origRedisEnabled
		common.RDB = origRDB
		relayRateWaitForSlot = origWaitForSlot
		setting.RateLimitWaitTimeoutSeconds = origWaitTimeout
		setting.ChannelRateLimitCooldownSeconds = origCooldown
	})
}
