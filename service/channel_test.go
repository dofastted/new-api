package service

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/stretchr/testify/assert"
)

func TestShouldDisableChannelOn429(t *testing.T) {
	orig := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = orig })

	tests := []struct {
		name string
		err  *types.NewAPIError
		want bool
	}{
		{
			name: "quota exhausted keyword with 429 must disable",
			err: types.NewErrorWithStatusCode(
				fmt.Errorf("You exceeded your current quota, please check your plan and billing details."),
				types.ErrorCodeBadResponse,
				http.StatusTooManyRequests,
			),
			want: true,
		},
		{
			name: "plain rate limit 429 must not disable",
			err: types.NewErrorWithStatusCode(
				fmt.Errorf("Upstream rate limit exceeded, please retry later"),
				types.ErrorCodeBadResponse,
				http.StatusTooManyRequests,
			),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ShouldDisableChannel(tt.err))
		})
	}
}
