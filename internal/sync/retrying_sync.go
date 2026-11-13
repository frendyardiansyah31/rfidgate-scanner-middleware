package sync

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/frendyardiansyah31/rfid-middleware/internal/domain"
	"github.com/frendyardiansyah31/rfid-middleware/internal/queue"
)

type RetryingSync struct {
	inner  SlimsSync
	online bool
}

func NewRetryingSync(inner SlimsSync) *RetryingSync {
	return &RetryingSync{
		inner:  inner,
		online: true,
	}
}

func (r *RetryingSync) IsOnline() bool {
	return r.online
}

func (r *RetryingSync) Save(ctx context.Context, event queue.Event) error {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 1 * time.Second
	bo.MaxInterval = 30 * time.Second

	_, err := backoff.Retry(
		ctx,
		func() (struct{}, error) {
			err := r.inner.Save(ctx, event)
			if err != nil {
				r.online = false
				return struct{}{}, err
			}

			r.online = true
			return struct{}{}, nil
		},
		backoff.WithBackOff(bo),
	)

	return err
}

func (r *RetryingSync) PullDelta(
	ctx context.Context,
	since time.Time,
) ([]domain.RFIDSnapshot, error) {

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 2 * time.Second
	bo.MaxInterval = 1 * time.Minute

	result, err := backoff.Retry(
		ctx,
		func() ([]domain.RFIDSnapshot, error) {
			rows, err := r.inner.PullDelta(ctx, since)
			if err != nil {
				r.online = false
				return nil, err
			}

			r.online = true
			return rows, nil
		},
		backoff.WithBackOff(bo),
	)

	return result, err
}

func (r *RetryingSync) CountRemote(ctx context.Context) (int, error) {
	return r.inner.CountRemote(ctx)
}
