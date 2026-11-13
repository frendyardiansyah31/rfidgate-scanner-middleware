package sync

import (
	"context"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
	"github.com/frendyardiansyah31/rfid-middleware/internal/domain"
	"github.com/frendyardiansyah31/rfid-middleware/internal/queue"
)

type RFIDStatus struct {
	RFID      string
	Status    cache.Status
	UpdatedAt time.Time
}

type SlimsSync interface {
	Save(ctx context.Context, event queue.Event) error
	// PullAll(ctx context.Context) ([]RFIDStatus, error)
	PullDelta(ctx context.Context, since time.Time) ([]domain.RFIDSnapshot, error)
	CountRemote(ctx context.Context) (int, error)
}
