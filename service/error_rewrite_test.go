package service

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRewriteUserFacingErrorRuleMatching(t *testing.T) {
	restoreErrorRewriteSettings(t)
	operation_setting.ErrorRewriteEnabled = true
	operation_setting.ErrorRewriteRules = []operation_setting.ErrorRewriteRule{
		{
			Name:        "disabled-first",
			StatusCodes: "500-599",
			Keywords:    []string{"upstream"},
			Message:     "disabled message",
			StatusCode:  http.StatusBadGateway,
			Enabled:     false,
		},
		{
			Name:        "range-keyword-first",
			StatusCodes: "500-599",
			Keywords:    []string{"UPSTREAM"},
			Message:     "range keyword message",
			StatusCode:  http.StatusServiceUnavailable,
			Enabled:     true,
		},
		{
			Name:        "generic-later",
			StatusCodes: "500-599",
			Message:     "generic range message",
			StatusCode:  http.StatusBadGateway,
			Enabled:     true,
		},
	}

	err := newRewriteTestError("Upstream service temporarily unavailable", http.StatusBadGateway)
	rewritten := RewriteUserFacingError(err)

	require.NotSame(t, err, rewritten)
	assert.Equal(t, http.StatusServiceUnavailable, rewritten.StatusCode)
	assert.Equal(t, "range keyword message", rewritten.Error())
	assert.Equal(t, types.ErrorCode("server_error"), rewritten.GetErrorCode())
	assert.Equal(t, "server_error", rewritten.ToOpenAIError().Type)
}

func TestRewriteUserFacingErrorNoMatchPassesThrough(t *testing.T) {
	restoreErrorRewriteSettings(t)
	operation_setting.ErrorRewriteEnabled = true
	operation_setting.ErrorRewriteRules = []operation_setting.ErrorRewriteRule{
		{
			Name:        "keyword-required",
			StatusCodes: "500-599",
			Keywords:    []string{"specific provider failure"},
			Message:     "rewritten",
			StatusCode:  http.StatusServiceUnavailable,
			Enabled:     true,
		},
	}

	err := newRewriteTestError("ordinary server error", http.StatusInternalServerError)
	rewritten := RewriteUserFacingError(err)

	assert.Same(t, err, rewritten)
	assert.Equal(t, "ordinary server error", rewritten.Error())
}

func TestRewriteUserFacingErrorSwitchOffPassesThrough(t *testing.T) {
	restoreErrorRewriteSettings(t)
	operation_setting.ErrorRewriteEnabled = false
	operation_setting.ErrorRewriteRules = cloneRewriteTestRules(operation_setting.DefaultErrorRewriteRules)

	err := newRewriteTestError("status_code=429, Upstream rate limit exceeded, please retry later", http.StatusTooManyRequests)
	rewritten := RewriteUserFacingError(err)

	assert.Same(t, err, rewritten)
	assert.Equal(t, "status_code=429, Upstream rate limit exceeded, please retry later", rewritten.Error())
}

func TestRewriteUserFacingErrorSkipsLocalDiagnosticsAndClientCancel(t *testing.T) {
	restoreErrorRewriteSettings(t)
	operation_setting.ErrorRewriteEnabled = true
	operation_setting.ErrorRewriteRules = cloneRewriteTestRules(operation_setting.DefaultErrorRewriteRules)

	tests := []struct {
		name string
		err  *types.NewAPIError
	}{
		{
			name: "count token failed",
			err:  types.NewErrorWithStatusCode(errors.New("status_code=503, No available accounts: no available accounts"), types.ErrorCodeCountTokenFailed, http.StatusServiceUnavailable),
		},
		{
			name: "client canceled",
			err:  types.NewErrorWithStatusCode(errors.New("context canceled"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Same(t, tc.err, RewriteUserFacingError(tc.err))
		})
	}
}

func TestRewriteUserFacingErrorDefaultRules(t *testing.T) {
	restoreErrorRewriteSettings(t)
	operation_setting.ErrorRewriteEnabled = true
	operation_setting.ErrorRewriteRules = cloneRewriteTestRules(operation_setting.DefaultErrorRewriteRules)

	tests := []struct {
		name           string
		message        string
		statusCode     int
		wantMessage    string
		wantStatusCode int
		wantSame       bool
	}{
		{
			name:           "generic upstream 429",
			message:        "status_code=429, Upstream rate limit exceeded, please retry later",
			statusCode:     http.StatusTooManyRequests,
			wantMessage:    "We're experiencing high demand right now. Please retry in a moment.",
			wantStatusCode: http.StatusTooManyRequests,
		},
		{
			name:           "no available accounts",
			message:        "status_code=503, No available accounts: no available accounts",
			statusCode:     http.StatusServiceUnavailable,
			wantMessage:    "The service is temporarily unavailable. Please try again later.",
			wantStatusCode: http.StatusServiceUnavailable,
		},
		{
			name:           "quota limited 429",
			message:        "You exceeded your current quota, please check your plan and billing details.",
			statusCode:     http.StatusTooManyRequests,
			wantMessage:    "The service is temporarily unavailable. Please try again later.",
			wantStatusCode: http.StatusServiceUnavailable,
		},
		{
			name:           "claude cli required",
			message:        "This API endpoint is only accessible via the official Claude CLI",
			statusCode:     http.StatusForbidden,
			wantMessage:    "This model requires the official Claude Code CLI. Please use Claude Code and retry.",
			wantStatusCode: http.StatusForbidden,
		},
		{
			name:           "codex responses endpoint required",
			message:        "codex 客户端限制.请使用 /v1/responses endpoint",
			statusCode:     http.StatusBadRequest,
			wantMessage:    "This model requires the official Codex CLI. Please use Codex (/v1/responses) and retry.",
			wantStatusCode: http.StatusForbidden,
		},
		{
			name:       "bad request parameter passthrough",
			message:    "status_code=400, Missing required parameter: 'tool_choice.type'.",
			statusCode: http.StatusBadRequest,
			wantSame:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := newRewriteTestError(tc.message, tc.statusCode)
			rewritten := RewriteUserFacingError(err)
			if tc.wantSame {
				assert.Same(t, err, rewritten)
				assert.Equal(t, tc.message, rewritten.Error())
				return
			}
			require.NotSame(t, err, rewritten)
			assert.Equal(t, tc.wantStatusCode, rewritten.StatusCode)
			assert.Equal(t, tc.wantMessage, rewritten.Error())
			assert.Equal(t, tc.wantMessage, rewritten.ToOpenAIError().Message)
			assert.Equal(t, tc.wantMessage, rewritten.ToClaudeError().Message)
			assert.NotContains(t, strings.ToLower(rewritten.ToOpenAIError().Message), "upstream")
			assert.NotContains(t, strings.ToLower(rewritten.ToClaudeError().Message), "upstream")
		})
	}
}

func TestRewriteUserFacingTaskError(t *testing.T) {
	restoreErrorRewriteSettings(t)
	operation_setting.ErrorRewriteEnabled = true
	operation_setting.ErrorRewriteRules = cloneRewriteTestRules(operation_setting.DefaultErrorRewriteRules)

	statusCode, code, message := RewriteUserFacingTaskError(
		http.StatusTooManyRequests,
		string(types.ErrorCodeBadResponseStatusCode),
		"status_code=429, Upstream rate limit exceeded, please retry later",
		false,
	)

	assert.Equal(t, http.StatusTooManyRequests, statusCode)
	assert.Equal(t, "rate_limit_exceeded", code)
	assert.Equal(t, "We're experiencing high demand right now. Please retry in a moment.", message)

	statusCode, code, message = RewriteUserFacingTaskError(
		http.StatusTooManyRequests,
		string(types.ErrorCodeBadResponseStatusCode),
		"status_code=429, Upstream rate limit exceeded, please retry later",
		true,
	)

	assert.Equal(t, http.StatusTooManyRequests, statusCode)
	assert.Equal(t, string(types.ErrorCodeBadResponseStatusCode), code)
	assert.Equal(t, "status_code=429, Upstream rate limit exceeded, please retry later", message)
}

func newRewriteTestError(message string, statusCode int) *types.NewAPIError {
	return types.NewOpenAIError(errors.New(message), types.ErrorCodeBadResponseStatusCode, statusCode)
}

func restoreErrorRewriteSettings(t *testing.T) {
	t.Helper()
	oldEnabled := operation_setting.ErrorRewriteEnabled
	oldRules := cloneRewriteTestRules(operation_setting.ErrorRewriteRules)
	t.Cleanup(func() {
		operation_setting.ErrorRewriteEnabled = oldEnabled
		operation_setting.ErrorRewriteRules = oldRules
	})
}

func cloneRewriteTestRules(rules []operation_setting.ErrorRewriteRule) []operation_setting.ErrorRewriteRule {
	cloned := make([]operation_setting.ErrorRewriteRule, len(rules))
	for i, rule := range rules {
		cloned[i] = rule
		cloned[i].Keywords = append([]string(nil), rule.Keywords...)
	}
	return cloned
}
