package sync

import (
	"context"

	"github.com/frendyardiansyah31/rfid-middleware/internal/queue"
)

// EventSink receives every gate decision the engine publishes to the event queue.
// There is no remote system to push to any more (Koha is read-only for the gate:
// lookups and the offline mirror only), so the only implementation is NoopSink.
// Kept as an interface so a future "send scan history to Koha" sink can plug in.
type EventSink interface {
	Save(ctx context.Context, event queue.Event) error
}
