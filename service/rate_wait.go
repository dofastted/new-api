package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/gin-gonic/gin"
)

var ErrRateWaitTimeout = errors.New("rate limit wait timeout")

type RateWaitTimeoutError struct {
	Budget time.Duration
}

func (e RateWaitTimeoutError) Error() string {
	return fmt.Sprintf("%s after %s", ErrRateWaitTimeout, e.Budget)
}

func (e RateWaitTimeoutError) Is(target error) bool {
	return target == ErrRateWaitTimeout
}

var rateWaitNow = time.Now

var rateWaitSleep = func(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

var rateWaitJitter = func(delay time.Duration) time.Duration {
	if delay <= 0 {
		return delay
	}
	span := delay / 5
	if span <= 0 {
		return delay
	}
	return delay - span + time.Duration(rand.Int63n(int64(span*2)+1))
}

const rateWaitDeadlineContextKey = "_rate_wait_deadline"

func RateWaitBudget(c *gin.Context, timeoutSeconds int) time.Duration {
	if timeoutSeconds <= 0 {
		return 0
	}
	budget := time.Duration(timeoutSeconds) * time.Second
	if c == nil {
		return budget
	}
	now := rateWaitNow()
	if value, ok := c.Get(rateWaitDeadlineContextKey); ok {
		if deadline, ok := value.(time.Time); ok {
			remaining := deadline.Sub(now)
			if remaining < 0 {
				return 0
			}
			return remaining
		}
	}
	deadline := now.Add(budget)
	c.Set(rateWaitDeadlineContextKey, deadline)
	return budget
}

func WaitForSlot(ctx context.Context, tryAcquire func() bool, budget time.Duration) error {
	if tryAcquire == nil {
		return errors.New("tryAcquire is nil")
	}
	if tryAcquire() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if budget <= 0 {
		return RateWaitTimeoutError{Budget: budget}
	}

	start := rateWaitNow()
	deadline := start.Add(budget)
	delay := 500 * time.Millisecond
	const maxDelay = 4 * time.Second

	for {
		remaining := deadline.Sub(rateWaitNow())
		if remaining <= 0 {
			return RateWaitTimeoutError{Budget: budget}
		}

		sleepFor := rateWaitJitter(delay)
		if sleepFor > remaining {
			sleepFor = remaining
		}
		if sleepFor < 0 {
			sleepFor = 0
		}

		if err := ctx.Err(); err != nil {
			return err
		}
		if err := rateWaitSleep(ctx, sleepFor); err != nil {
			return err
		}
		if tryAcquire() {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		if delay < maxDelay {
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
}
