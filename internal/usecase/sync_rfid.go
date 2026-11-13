package usecase

import (
	"context"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
	"github.com/frendyardiansyah31/rfid-middleware/internal/repository"
	"github.com/frendyardiansyah31/rfid-middleware/internal/sync"
)

type SyncRFIDUseCase struct {
	mysqlSync sync.SlimsSync
	repo      repository.RFIDRepository
	// Optional: Access to memory cache to update it immediately
	// If the application reads from Repository/SQLite directly, this might not be needed.
	// But as per current investigation, CompositeCache is used.
	// We should update it if possible.
	memCache *cache.CompositeCache // Using concrete type as per main.go usage, or interface
	log      logger.Logger
}

func NewSyncRFIDUseCase(
	mysqlSync sync.SlimsSync,
	repo repository.RFIDRepository,
	memCache *cache.CompositeCache,
	log logger.Logger,
) *SyncRFIDUseCase {
	return &SyncRFIDUseCase{
		mysqlSync: mysqlSync,
		repo:      repo,
		memCache:  memCache,
		log:       log,
	}
}

func (u *SyncRFIDUseCase) Run(ctx context.Context) error {
	// 1. Get Last Sync Time
	lastSync, err := u.repo.LastUpdated(ctx)
	if err != nil {
		u.log.Error("failed to get last updated time", logger.F("error", err))
		return err
	}

	u.log.Info("starting rfid sync", logger.F("since", lastSync))

	// 2. Pull Delta from SLiMS
	snapshots, err := u.mysqlSync.PullDelta(ctx, lastSync)
	if err != nil {
		u.log.Error("pull delta failed", logger.F("error", err))
		return err
	}

	if len(snapshots) == 0 {
		u.log.Info("no new data to sync")
		return nil
	}

	// 3. Persist and Update Cache in Transaction
	err = u.repo.WithTx(ctx, func(txRepo repository.RFIDRepository) error {
		for _, snap := range snapshots {
			// Persist Status (Transactional)
			if err := txRepo.UpsertStatus(ctx, snap); err != nil {
				return err
			}

			// Persist Item Metadata (Slow changing)
			if err := txRepo.UpsertItem(ctx, snap); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		u.log.Error("failed to persist rfid data", logger.F("error", err))
		return err
	}

	// 4. Update Memory Cache (Outside Transaction)
	// We want the app to see the changes immediately
	count := 0
	for _, snap := range snapshots {
		// Adapting domain.Status to cache.Status
		// Assumption: 0 = Loaned, 1 = Available (Check api/handler_rfid.go or logs for enum mapping)
		// logic in mysql_sync.go:
		// Loaned = 0
		// Available = 1
		// cache.Status:
		// Check internal/cache/interface.go or similar
		// Assuming direct mapping or logic needed.
		// Wait, mysql_sync.go used to map to cache.Status enum.
		// PullDelta now returns domain struct which has Status int.
		// I should map it back to cache.Status

		var status cache.Status
		switch snap.Status {
		case 1:
			status = cache.StatusNotLoaned // Available
		case 0:
			status = cache.StatusLoaned // Loaned
		default:
			status = cache.StatusUnknown
		}

		if u.memCache != nil {
			_ = u.memCache.SetMemory(snap.RFID, status)
		}
		count++
	}

	u.log.Info("sync completed", logger.F("processed", count))
	return nil
}
