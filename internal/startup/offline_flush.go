package startup

import (
	"context"

	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
	"github.com/frendyardiansyah31/rfid-middleware/internal/queue"
	"github.com/frendyardiansyah31/rfid-middleware/internal/sync"
)

func FlushOfflineEvents(
	ctx context.Context,
	queue queue.EventQueue,
	syncer sync.SlimsSync,
	log logger.Logger,
) {

	for queue.Pending() > 0 {
		select {
		case <-ctx.Done():
			return

		case ev := <-queue.Consume():
			if err := syncer.Save(ctx, ev); err != nil {
				log.Error("failed resend event", logger.F("error", err))
				queue.Publish(ev.RFID, ev.Status)
				return
			}
		}
	}

	log.Info("offline events flushed")
}
