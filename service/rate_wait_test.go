package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitForSlotAcquireScenarios(t *testing.T) {
	tests := []struct {
		name       string
		failures   int
		wantSleeps []time.Duration
	}{
		{
			name:       "immediate success",
			failures:   0,
			wantSleeps: nil,
		},
		{
			name:       "success after backoff",
			failures:   3,
			wantSleeps: []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			restoreRateWaitTestHooks(t)
			now := time.Unix(0, 0)
			rateWaitNow = func() time.Time { return now }
			rateWaitJitter = func(delay time.Duration) time.Duration { return delay }

			var sleeps []time.Duration
			rateWaitSleep = func(ctx context.Context, delay time.Duration) error {
				sleeps = append(sleeps, delay)
				now = now.Add(delay)
				return ctx.Err()
			}

			calls := 0
			err := WaitForSlot(context.Background(), func() bool {
				calls++
				return calls > tc.failures
			}, time.Minute)

			require.NoError(t, err)
			assert.Equal(t, tc.failures+1, calls)
			assert.Equal(t, tc.wantSleeps, sleeps)
		})
	}
}

func TestWaitForSlotTimeout(t *testing.T) {
	restoreRateWaitTestHooks(t)
	now := time.Unix(0, 0)
	rateWaitNow = func() time.Time { return now }
	rateWaitJitter = func(delay time.Duration) time.Duration { return delay }

	var sleeps []time.Duration
	rateWaitSleep = func(ctx context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		now = now.Add(delay)
		return ctx.Err()
	}

	err := WaitForSlot(context.Background(), func() bool { return false }, 750*time.Millisecond)

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRateWaitTimeout))
	assert.Equal(t, []time.Duration{500 * time.Millisecond, 250 * time.Millisecond}, sleeps)
}

func TestWaitForSlotCanceledContext(t *testing.T) {
	restoreRateWaitTestHooks(t)
	rateWaitJitter = func(delay time.Duration) time.Duration { return delay }

	sleepCalled := false
	rateWaitSleep = func(ctx context.Context, delay time.Duration) error {
		sleepCalled = true
		return ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	err := WaitForSlot(ctx, func() bool {
		calls++
		return false
	}, time.Minute)

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls)
	assert.False(t, sleepCalled)
}

func restoreRateWaitTestHooks(t *testing.T) {
	t.Helper()

	origNow := rateWaitNow
	origSleep := rateWaitSleep
	origJitter := rateWaitJitter
	t.Cleanup(func() {
		rateWaitNow = origNow
		rateWaitSleep = origSleep
		rateWaitJitter = origJitter
	})
}
