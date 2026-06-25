package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
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
