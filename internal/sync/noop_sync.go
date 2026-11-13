package sync

import (
	"context"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/domain"
	"github.com/frendyardiansyah31/rfid-middleware/internal/queue"
)

type NoopSync struct{}

func NewNoopSync() *NoopSync {
	return &NoopSync{}
}

func (n *NoopSync) Save(ctx context.Context, event queue.Event) error {
	// do nothing
	return nil
}

func (n *NoopSync) PullAll(ctx context.Context) ([]RFIDStatus, error) {
	return []RFIDStatus{}, nil
}

func (n *NoopSync) PullDelta(ctx context.Context, since time.Time) ([]domain.RFIDSnapshot, error) {
	return []domain.RFIDSnapshot{}, nil
}

func (n *NoopSync) CountRemote(ctx context.Context) (int, error) {
	return 0, nil
}
