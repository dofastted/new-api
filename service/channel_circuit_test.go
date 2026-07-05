package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyChannelCircuitFailure(t *testing.T) {
	highLoadSettings := dto.ChannelSettings{
		CircuitBreaker: &dto.ChannelCircuitBreakerSettings{
			Enabled:          true,
			FailureThreshold: 1,
			OpenSeconds:      300,
			Rules: []dto.ChannelCircuitBreakerRule{
				{
					Name:        "claude_high_load",
					Class:       ChannelCircuitClassHighLoadTemporarilyUnavailable,
					StatusCodes: []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable},
				},
			},
		},
	}
	disabledSettings := dto.ChannelSettings{
		CircuitBreaker: &dto.ChannelCircuitBreakerSettings{
			Enabled:          false,
			FailureThreshold: 1,
			OpenSeconds:      300,
			Rules: []dto.ChannelCircuitBreakerRule{
				{
					Name:        "claude_high_load",
					Class:       ChannelCircuitClassHighLoadTemporarilyUnavailable,
					StatusCodes: []int{http.StatusInternalServerError},
				},
			},
		},
	}

	tests := []struct {
		name            string
		settings        dto.ChannelSettings
		err             *types.NewAPIError
		wantRecord      bool
		wantCategory    string
		wantPolicy      bool
		wantPolicyName  string
		wantThreshold   int
		wantOpenSeconds int
	}{
		{
			name:            "status 500 maps configured rule to high load circuit policy",
			settings:        highLoadSettings,
			err:             circuitStatusError(http.StatusInternalServerError),
			wantRecord:      true,
			wantCategory:    ChannelCircuitClassHighLoadTemporarilyUnavailable,
			wantPolicy:      true,
			wantPolicyName:  "claude_high_load",
			wantThreshold:   1,
			wantOpenSeconds: 300,
		},
		{
			name:            "status 502 maps configured rule to high load circuit policy",
			settings:        highLoadSettings,
			err:             circuitStatusError(http.StatusBadGateway),
			wantRecord:      true,
			wantCategory:    ChannelCircuitClassHighLoadTemporarilyUnavailable,
			wantPolicy:      true,
			wantPolicyName:  "claude_high_load",
			wantThreshold:   1,
			wantOpenSeconds: 300,
		},
		{
			name:            "status 503 maps configured rule to high load circuit policy",
			settings:        highLoadSettings,
			err:             circuitStatusError(http.StatusServiceUnavailable),
			wantRecord:      true,
			wantCategory:    ChannelCircuitClassHighLoadTemporarilyUnavailable,
			wantPolicy:      true,
			wantPolicyName:  "claude_high_load",
			wantThreshold:   1,
			wantOpenSeconds: 300,
		},
		{
			name:       "status 429 is not recorded even when retryable-looking upstream error arrives",
			settings:   highLoadSettings,
			err:        circuitStatusError(http.StatusTooManyRequests),
			wantRecord: false,
		},
		{
			name:       "configured breaker ignores unmatched retryable status",
			settings:   highLoadSettings,
			err:        circuitStatusError(http.StatusGatewayTimeout),
			wantRecord: false,
		},
		{
			name:       "disabled circuit breaker does not record configured match",
			settings:   disabledSettings,
			err:        circuitStatusError(http.StatusInternalServerError),
			wantRecord: false,
		},
		{
			name:         "missing circuit setting records retryable 500 under error code category",
			settings:     dto.ChannelSettings{},
			err:          circuitStatusError(http.StatusInternalServerError),
			wantRecord:   true,
			wantCategory: string(types.ErrorCodeBadResponseStatusCode),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := ClassifyChannelCircuitFailure(tt.settings, tt.err)

			assert.Equal(t, tt.wantRecord, decision.ShouldRecord)
			if !tt.wantRecord {
				assert.Empty(t, decision.Category)
				return
			}
			require.NotEmpty(t, decision.Category)
			assert.Equal(t, tt.wantCategory, decision.Category)
			if tt.wantPolicy {
				assert.Equal(t, tt.wantPolicyName, decision.Policy.Name)
				assert.Equal(t, tt.wantThreshold, decision.Policy.FailureThreshold)
				assert.Equal(t, tt.wantOpenSeconds, decision.Policy.OpenSeconds)
			}
		})
	}
}

func TestShouldRecordChannelCircuitFailure(t *testing.T) {
	tests := []struct {
		name string
		err  *types.NewAPIError
		want bool
	}{
		{
			name: "records retryable upstream 500",
			err:  circuitStatusError(http.StatusInternalServerError),
			want: true,
		},
		{
			name: "does not record client cancellation",
			err:  types.NewErrorWithStatusCode(context.Canceled, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError),
			want: false,
		},
		{
			name: "does not record skip retry errors",
			err: types.NewErrorWithStatusCode(
				errors.New("local access policy denied request"),
				types.ErrorCodeAccessDenied,
				http.StatusForbidden,
				types.ErrOptionWithSkipRetry(),
			),
			want: false,
		},
		{
			name: "does not record official Claude CLI access denied 403",
			err: types.NewOpenAIError(
				errors.New("This API endpoint is only accessible via the official Claude CLI"),
				types.ErrorCodeBadResponseStatusCode,
				http.StatusForbidden,
			),
			want: false,
		},
		{
			name: "does not record upstream 429",
			err:  circuitStatusError(http.StatusTooManyRequests),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ShouldRecordChannelCircuitFailure(tt.err))
		})
	}
}

func circuitStatusError(statusCode int) *types.NewAPIError {
	return types.NewOpenAIError(
		errors.New(http.StatusText(statusCode)),
		types.ErrorCodeBadResponseStatusCode,
		statusCode,
	)
}
