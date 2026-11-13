package startup

import (
	"context"

	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
)

func (r *Runner) flushOffline(ctx context.Context) {
	events, err := r.offline.LoadAll(ctx)
	if err != nil {
		r.log.Error("offline load failed", logger.F("error", err))
		return
	}

	if len(events) == 0 {
		r.log.Info("no offline events to flush")
		return
	}

	success := 0

	for _, e := range events {
		select {
		case <-ctx.Done():
			r.log.Warn("offline flush aborted by context")
			return
		default:
			if err := r.sync.Save(ctx, e); err != nil {
				r.log.Error("offline sync failed",
					logger.F("rfid", e.RFID),
					logger.F("error", err),
				)
				continue
			}

			_ = r.offline.Delete(ctx, e)
			success++
		}
	}

	r.log.Info("offline flush completed",
		logger.F("total", len(events)),
		logger.F("success", success),
	)
}
