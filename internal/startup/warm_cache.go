package startup

import (
	"context"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
)

func WarmMemoryCache(
	mem cache.Cache,
	sqlite *cache.SQLiteCache,
	log logger.Logger,
) error {

	items, err := sqlite.LoadAll()
	if err != nil {
		log.Error("failed to load sqlite cache", logger.F("error", err))
		return err
	}

	for rfid, status := range items {
		_ = mem.Set(rfid, status)
	}

	log.Info("memory cache warmed", logger.F("count", len(items)))
	return nil
}

func (r *Runner) warmCache(ctx context.Context) {
	items, err := r.cache.AllFromSQLite()
	if err != nil {
		r.log.Error("warm cache failed", logger.F("error", err))
		return
	}

	for rfid, status := range items {
		_ = r.cache.SetMemory(rfid, status)
	}

	r.log.Info("memory cache warmed", logger.F("count", len(items)))

}
