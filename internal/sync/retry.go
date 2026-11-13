package sync

import (
	"context"
	"time"
)

type RetryPolicy struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

func Retry(
	ctx context.Context,
	policy RetryPolicy,
	fn func() error,
) error {

	delay := policy.InitialDelay
	if delay <= 0 {
		delay = 500 * time.Millisecond
	}

	max := policy.MaxDelay
	if max <= 0 {
		max = 30 * time.Second
	}

	for {
		err := fn()
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		delay *= 2
		if delay > max {
			delay = max
		}
	}
}
