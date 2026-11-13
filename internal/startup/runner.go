package startup

import (
	"context"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
	"github.com/frendyardiansyah31/rfid-middleware/internal/sync"
	"github.com/frendyardiansyah31/rfid-middleware/internal/usecase"
)

type Runner struct {
	cache   *cache.CompositeCache
	sync    sync.SlimsSync
	offline sync.OfflineStore
	useCase *usecase.SyncRFIDUseCase
	log     logger.Logger
}

func NewRunner(
	cache *cache.CompositeCache,
	sync sync.SlimsSync,
	offline sync.OfflineStore,
	useCase *usecase.SyncRFIDUseCase,
	log logger.Logger,
) *Runner {
	return &Runner{
		cache:   cache,
		sync:    sync,
		offline: offline,
		useCase: useCase,
		log:     log,
	}
}

// func (r *Runner) Run(ctx context.Context) error {
// 	start := time.Now()
// 	r.log.Info("startup recovery started")

// 	count, err := r.sync.PullAll(ctx)
// 	if err != nil {
// 		r.log.Error("initial pull failed", logger.F("error", err))
// 		return err
// 	}
// 	r.log.Info("initial pull completed", logger.F("count", count))

// 	r.warmCache(ctx)
// 	r.initialPull(ctx)
// 	r.log.Info(
// 		"initial pull completed",
// 		logger.F("count", count),
// 		logger.F("duration", time.Since(start)),
// 	)
// 	r.deltaSync(ctx)
// 	r.flushOffline(ctx)

//		r.log.Info("startup recovery completed")
//		return nil
//	}

// dari gemini
// func (r *Runner) Run(ctx context.Context) error {
// 	r.log.Info("startup recovery started")

// 	start := time.Now()
// 	// r.warmCache(ctx)
// 	initialMemoryCount := r.warmCacheReturningCount(ctx)
// 	logDuration(r.log, "warm cache", start)

// 	// ini dihapus karena sudah dihandle di deltaSync, isinya pun sama.
// 	// start = time.Now()
// 	// r.initialPull(ctx)
// 	// logDuration(r.log, "initial pull", start)

// 	start = time.Now()
// 	r.deltaSync(ctx)
// 	logDuration(r.log, "delta sync", start)

// 	start = time.Now()
// 	r.flushOffline(ctx)
// 	logDuration(r.log, "offline flush", start)

// 	finalCount, _ := r.cache.SQLiteCount()

// 	r.auditData(ctx, finalCount, initialMemoryCount)

// 	r.log.Info("startup recovery completed")

// 	return nil
// }

func logDuration(log logger.Logger, msg string, start time.Time) {
	log.Info(msg, logger.F("duration", time.Since(start)))
}

func (r *Runner) RunAsync(ctx context.Context) {
	go func() {
		if err := r.Run(ctx); err != nil {
			r.log.Error("startup recovery failed", logger.F("error", err))
		}
	}()
}

// func (r *Runner) auditData(ctx context.Context, memCount int) {
// 	// 1. Cek jumlah di SQLite
// 	sqliteCount, err := r.cache.SQLiteCount() // Nanti kita update CompositeCache sedikit
// 	if err != nil {
// 		r.log.Error("audit failed: sqlite error", logger.F("error", err))
// 		return
// 	}

// 	// 2. Cek jumlah di SLiMS (Remote)
// 	slimsCount, err := r.sync.CountRemote(ctx)
// 	if err != nil {
// 		r.log.Warn("audit warning: could not connect to slims to verify count", logger.F("error", err))
// 		// Jangan return, tetap log apa yang kita punya
// 		slimsCount = -1 // Tandai sebagai error
// 	}

// 	// 3. Bandingkan dan Log
// 	loggerParams := []logger.Field{
// 		logger.F("memory_count", memCount),
// 		logger.F("sqlite_count", sqliteCount),
// 		logger.F("slims_count", slimsCount),
// 	}

// 	if slimsCount != -1 {
// 		if sqliteCount == slimsCount {
// 			r.log.Info("✅ DATA INTEGRITY MATCHED", loggerParams...)
// 		} else {
// 			diff := slimsCount - sqliteCount
// 			r.log.Warn("⚠️ DATA DISCREPANCY DETECTED",
// 				append(loggerParams, logger.F("missing_in_local", diff))...,
// 			)
// 		}
// 	} else {
// 		r.log.Info("local data status", loggerParams...)
// 	}
// }

func (r *Runner) Run(ctx context.Context) error {
	r.log.Info("startup recovery started")

	start := time.Now()

	// 1. Warm Cache (Ambil jumlah awal)
	// Variabel ini hanya untuk debug awal, bukan hasil akhir
	initialMemoryCount := r.warmCacheReturningCount(ctx)
	logDuration(r.log, "warm cache", start)

	start = time.Now()
	r.initialPull(ctx)
	logDuration(r.log, "initial pull", start)

	start = time.Now()
	r.deltaSync(ctx) // <-- Di sini 31rb data masuk
	logDuration(r.log, "delta sync", start)

	start = time.Now()
	r.flushOffline(ctx)
	logDuration(r.log, "offline flush", start)

	// ==========================================
	// 🔥 PERBAIKAN AUDIT
	// ==========================================

	// Update Memory Count dengan data terbaru dari SQLite
	// Karena Delta Sync sudah menyimpan ke SQLite & Memory,
	// maka jumlah di SQLite mencerminkan jumlah Memory yang seharusnya.
	finalCount, _ := r.cache.SQLiteCount()

	// Masukkan finalCount sebagai representasi memory & sqlite (karena sudah sinkron)
	r.auditData(ctx, finalCount, initialMemoryCount)

	r.log.Info("startup recovery completed")

	return nil
}

// Update fungsi Audit untuk menerima 2 parameter
func (r *Runner) auditData(ctx context.Context, currentCount int, initialCount int) {
	// 1. Cek jumlah di SLiMS (Remote)
	slimsCount, err := r.sync.CountRemote(ctx)
	if err != nil {
		r.log.Warn("audit warning: could not connect to slims", logger.F("error", err))
		slimsCount = -1
	}

	// 2. Logika Deteksi Duplikat
	// Jika SLiMS punya lebih banyak dari Lokal, kemungkinan ada RFID ganda di SLiMS
	// yang di-merge menjadi satu di Lokal.

	loggerParams := []logger.Field{
		logger.F("local_count", currentCount),  // SQLite & Memory (Synced)
		logger.F("slims_count", slimsCount),    // MySQL SLiMS
		logger.F("initial_load", initialCount), // Sebelum Sync
	}

	if slimsCount != -1 {
		if currentCount == slimsCount {
			r.log.Info("✅ DATA INTEGRITY MATCHED", loggerParams...)
		} else {
			diff := slimsCount - currentCount // positif = kurang di lokal, negatif = lebih di lokal (record yatim)
			extra := fields(loggerParams,
				logger.F("diff_slims_minus_local", diff),
			)
			if diff > 0 {
				extra = fields(extra, logger.F("diagnosis", "local is missing records — delta sync may have gaps"))
			} else {
				extra = fields(extra, logger.F("diagnosis", "local has orphan records — items removed/nulled in SLiMS but never deleted locally"))
			}
			r.log.Warn("⚠️ DATA DISCREPANCY DETECTED", extra...)
		}
	} else {
		r.log.Info("local data status", loggerParams...)
	}
}

func fields(base []logger.Field, extra ...logger.Field) []logger.Field {
	return append(base, extra...)
}

func (r *Runner) warmCacheReturningCount(ctx context.Context) int {
	items, err := r.cache.AllFromSQLite()
	if err != nil {
		r.log.Error("warm cache failed", logger.F("error", err))
		return 0
	}

	for rfid, status := range items {
		_ = r.cache.SetMemory(rfid, status)
	}

	// Return jumlah item yang berhasil masuk memory
	return len(items)
}

func (r *Runner) PerformDeltaSync(ctx context.Context) error {
	// Kita panggil fungsi internal yang sudah ada
	// Karena Anda bilang initialPull di-comment, deltaSync ini yang jadi andalan
	r.deltaSync(ctx)
	return nil
}
