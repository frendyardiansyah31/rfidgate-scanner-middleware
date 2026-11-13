package startup

import (
	"context"

	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
)

// func InitialPullIfNeeded(
// 	ctx context.Context,
// 	sqlite *cache.SQLiteCache,
// 	mem cache.Cache,
// 	syncer sync.SlimsSync,
// 	log logger.Logger,
// ) error {

// 	if !sqlite.IsEmpty() {
// 		return nil
// 	}

// 	log.Warn("sqlite empty, performing full pull from slims")

// 	items, err := syncer.PullAll(ctx)
// 	if err != nil {
// 		log.Error("initial pull failed", logger.F("error", err))
// 		return err
// 	}

// 	for _, item := range items {
// 		_ = sqlite.Set(item.RFID, item.Status)
// 		_ = mem.Set(item.RFID, item.Status)
// 	}

// 	log.Info("initial pull completed", logger.F("count", len(items)))
// 	return nil
// }

// func (r *Runner) initialPull(ctx context.Context) {
// 	items, err := r.sync.PullAll(ctx)
// 	if err != nil {
// 		r.log.Error("initial pull failed", logger.F("error", err))
// 		return
// 	}

// 	for _, it := range items {
// 		if err := r.cache.Set(it.RFID, it.Status); err != nil {
// 			r.log.Error("cache set failed", logger.F("error", err))
// 		}
// 	}

//		r.log.Info(
//			"initial pull completed",
//			logger.F("count", len(items)),
//		)
//	}
func (r *Runner) initialPull(ctx context.Context) {
	err := r.useCase.Run(ctx)
	if err != nil {
		r.log.Error("initial pull failed", logger.F("error", err))
	}
}
