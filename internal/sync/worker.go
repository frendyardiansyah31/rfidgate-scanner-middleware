package sync

import (
	"context"

	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
	"github.com/frendyardiansyah31/rfid-middleware/internal/queue"
	"github.com/frendyardiansyah31/rfid-middleware/internal/safego"
)

type Worker struct {
	queue   queue.EventQueue
	syncer  SlimsSync
	offline OfflineStore
	logger  logger.Logger

	sem chan struct{} // concurrency limiter
}

func NewWorker(
	eventQueue queue.EventQueue,
	syncer SlimsSync,
	offline OfflineStore,
	logger logger.Logger,
) *Worker {
	return &Worker{
		queue:   eventQueue,
		syncer:  syncer,
		offline: offline,
		logger:  logger,
		sem:     make(chan struct{}, 4), // 🔧 MAX 4 concurrent MySQL sync
	}
}

func (w *Worker) Start(ctx context.Context) {
	go w.run(ctx)
}

func (w *Worker) run(ctx context.Context) {
	w.logger.Info("sync worker started")

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("sync worker stopped")
			return

		case event := <-w.queue.Consume():
			w.sem <- struct{}{} // acquire slot

			go func(ev queue.Event) {
				defer func() { <-w.sem }() // release slot
				w.process(ctx, ev)
			}(event)
		}
	}
}

func (w *Worker) process(ctx context.Context, event queue.Event) {
	defer safego.Recover(w.logger, "sync.worker", nil)
	err := w.syncer.Save(ctx, event)
	if err == nil {
		return // ✅ sukses
	}

	// ❌ gagal total (retry sudah dilakukan di RetryingSync)
	w.logger.Error(
		"sync failed, saving to offline store",
		logger.F("rfid", event.RFID),
		logger.F("error", err),
	)

	if err2 := w.offline.Save(ctx, event); err2 != nil {
		w.logger.Error(
			"failed to save offline event",
			logger.F("rfid", event.RFID),
			logger.F("error", err2),
		)
	}
}
