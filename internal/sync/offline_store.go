package sync

import (
	"context"

	"github.com/frendyardiansyah31/rfid-middleware/internal/queue"
)

type OfflineStore interface {
	Save(ctx context.Context, event queue.Event) error
	LoadAll(ctx context.Context) ([]queue.Event, error)
	Delete(ctx context.Context, event queue.Event) error
	GetState(ctx context.Context, key string) (string, error)
	SetState(ctx context.Context, key, value string) error
}
