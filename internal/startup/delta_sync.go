package startup

import (
	"context"

	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
)

// func DeltaSync(
// 	ctx context.Context,
// 	sqlite *cache.SQLiteCache,
// 	mem cache.Cache,
// 	syncer sync.SlimsSync,
// 	log logger.Logger,
// ) error {

// 	last, err := sqlite.LastUpdated()
// 	if err != nil {
// 		return err
// 	}

// 	items, err := syncer.PullDelta(ctx, last)
// 	if err != nil {
// 		log.Error("delta pull failed", logger.F("error", err))
// 		return err
// 	}

// 	for _, item := range items {
// 		_ = sqlite.Set(item.RFID, item.Status)
// 		_ = mem.Set(item.RFID, item.Status)
// 	}

// 	log.Info("delta sync completed", logger.F("count", len(items)))
// 	return nil
// }

// func (r *Runner) deltaSync(ctx context.Context) {
// 	last, err := r.cache.LastUpdated()
// 	if err != nil {
// 		r.log.Error("delta sync skipped", logger.F("error", err))
// 		return
// 	}

// 	items, err := r.sync.PullDelta(ctx, last)
// 	if err != nil {
// 		r.log.Error("delta sync failed", logger.F("error", err))
// 		return
// 	}

// 	for _, it := range items {
// 		_ = r.cache.Set(it.RFID, it.Status)
// 	}

//		r.log.Info("delta sync completed", logger.F("count", len(items)))
//	}
// func (r *Runner) deltaSync(ctx context.Context) {
// 	lastStr, _ := r.offline.GetState(ctx, sync.SyncKeyLastUpdate)

// 	lastTime := time.Unix(0, 0)
// 	if lastStr != "" {
// 		if t, err := time.Parse(time.RFC3339, lastStr); err == nil {
// 			lastTime = t
// 		}
// 	}

// 	r.log.Info("delta sync start",
// 		logger.F("since", lastTime),
// 	)

// 	rows, err := r.sync.PullDelta(ctx, lastTime)
// 	if err != nil {
// 		r.log.Error("delta sync failed", logger.F("error", err))
// 		return
// 	}

// 	for _, rfid := range rows {
// 		r.cache.Set(rfid.RFID, rfid.Status)

// 		if rfid.UpdatedAt.After(lastTime) {
// 			lastTime = rfid.UpdatedAt
// 		}
// 	}

// 	_ = r.offline.SetState(
// 		ctx,
// 		sync.SyncKeyLastUpdate,
// 		lastTime.Format(time.RFC3339),
// 	)
// }

func (r *Runner) deltaSync(ctx context.Context) {
	err := r.useCase.Run(ctx)
	if err != nil {
		r.log.Error("delta sync failed", logger.F("error", err))
	}
}
