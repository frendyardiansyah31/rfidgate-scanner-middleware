package sync

import (
	"context"

	"github.com/frendyardiansyah31/rfid-middleware/internal/queue"
)

// NoopSink discards events.
type NoopSink struct{}

func NewNoopSink() *NoopSink {
	return &NoopSink{}
}

func (n *NoopSink) Save(ctx context.Context, event queue.Event) error {
	return nil
}
