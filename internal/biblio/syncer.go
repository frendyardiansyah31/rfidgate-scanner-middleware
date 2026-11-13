package biblio

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
	"github.com/frendyardiansyah31/rfid-middleware/internal/safego"
)

type SyncStatus struct {
	LastSyncAt   *time.Time `json:"last_sync_at"`
	TotalRecords int        `json:"total_records"`
	LastError    *string    `json:"last_error"`
	IsSyncing    bool       `json:"is_syncing"`
}

// StateRepo is a Repository that can also persist sync state and apply a batch
// atomically. The delta mode needs it; SQLiteRepository implements it.
type StateRepo interface {
	Repository
	State(key string) (string, bool)
	ApplyBatch(entries []Entry, prune bool, state map[string]string) error
}

// DeltaConfig tunes the delta mode. Zero values take the defaults.
type DeltaConfig struct {
	// DeltaEvery is how often a sync runs. Default 10 minutes (titles and covers
	// change rarely, and the Koha delta query is the heavier of the two syncs).
	DeltaEvery time.Duration
	// FullEvery is the maximum age of the last full sync before the next one is a
	// full sync, which also prunes deleted items. Default 24 hours.
	FullEvery time.Duration
	// Overlap is subtracted from the stored cursor so late-visible changes are
	// fetched again (harmless: upserts are idempotent). Default 15 minutes.
	Overlap time.Duration
	// MinKeepRatio rejects a full sync that returns fewer than this fraction of
	// the rows already stored, so a bad response cannot wipe the table. Default 0.5.
	MinKeepRatio float64
}

func (c *DeltaConfig) setDefaults() {
	if c.DeltaEvery <= 0 {
		c.DeltaEvery = 10 * time.Minute
	}
	if c.FullEvery <= 0 {
		c.FullEvery = 24 * time.Hour
	}
	if c.Overlap <= 0 {
		c.Overlap = 15 * time.Minute
	}
	if c.MinKeepRatio <= 0 {
		c.MinKeepRatio = 0.5
	}
}

// kohaTimeFormat is the format of the source's server_time / since parameter.
const kohaTimeFormat = "2006-01-02 15:04:05"

type Syncer struct {
	repo     Repository
	log      logger.Logger
	mu       sync.Mutex
	statusMu sync.RWMutex
	status   SyncStatus

	delta     DeltaFetcher
	stateRepo StateRepo
	cfg       DeltaConfig
	now       func() time.Time
}

// NewDeltaSyncer builds a syncer that keeps the table in step with a
// delta-capable source: delta syncs every DeltaEvery, a full (pruning) sync when
// the last one is older than FullEvery.
func NewDeltaSyncer(src DeltaFetcher, repo StateRepo, log logger.Logger, cfg DeltaConfig) *Syncer {
	cfg.setDefaults()
	return &Syncer{delta: src, stateRepo: repo, repo: repo, log: log, cfg: cfg, now: time.Now}
}

func (s *Syncer) Status() SyncStatus {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	return s.status
}

func (s *Syncer) enabled() bool { return s.delta != nil }

// Start runs an initial sync and then syncs on the DeltaConfig schedule
// (delta every DeltaEvery, full when the last full sync is older than FullEvery).
func (s *Syncer) Start(ctx context.Context) {
	if !s.enabled() {
		s.log.Warn("biblio sync disabled: no source available")
		return
	}
	s.startDelta(ctx)
}

func (s *Syncer) startDelta(ctx context.Context) {
	s.log.Info("biblio delta sync scheduled",
		logger.F("delta_every", s.cfg.DeltaEvery.String()),
		logger.F("full_every", s.cfg.FullEvery.String()),
	)
	go func() {
		_ = s.sync(ctx, false)
		t := time.NewTicker(s.cfg.DeltaEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = s.sync(ctx, false)
			}
		}
	}()
}

// RunOnce executes a sync now. Returns immediately if another sync is running.
// A manual run in delta mode is always a FULL sync (the admin asked for a
// refresh); the schedule decides between delta and full on its own.
func (s *Syncer) RunOnce(ctx context.Context) error {
	if !s.enabled() {
		return nil
	}
	return s.sync(ctx, true)
}

func (s *Syncer) sync(ctx context.Context, forceFull bool) (err error) {
	defer safego.Recover(s.log, "biblio.sync", &err)
	return s.syncInner(ctx, forceFull)
}

func (s *Syncer) syncInner(ctx context.Context, forceFull bool) error {
	if !s.mu.TryLock() {
		s.log.Info("biblio sync already running, skipping")
		return nil
	}
	defer s.mu.Unlock()

	s.setStatus(func(st *SyncStatus) { st.IsSyncing = true })
	defer s.setStatus(func(st *SyncStatus) { st.IsSyncing = false })

	start := s.now()
	s.log.Info("biblio sync started")

	upserted, err := s.runDelta(ctx, forceFull)
	elapsed := s.now().Sub(start).Round(time.Millisecond)

	if err != nil {
		msg := err.Error()
		s.setStatus(func(st *SyncStatus) { st.LastError = &msg })
		s.log.Error("biblio sync failed",
			logger.F("elapsed", elapsed.String()),
			logger.F("error", err),
		)
		return err
	}

	total, _ := s.repo.Count()
	now := s.now()
	s.setStatus(func(st *SyncStatus) {
		st.LastSyncAt = &now
		st.TotalRecords = total
		st.LastError = nil
	})
	s.log.Info("biblio sync completed",
		logger.F("upserted", upserted),
		logger.F("total_in_db", total),
		logger.F("elapsed", elapsed.String()),
	)
	return nil
}

// runDelta fetches either everything (full) or what changed since
// cursor - Overlap (delta), and applies it atomically together with the new
// cursor.
func (s *Syncer) runDelta(ctx context.Context, forceFull bool) (int, error) {
	full, since := s.plan(forceFull)

	b, err := s.delta.FetchSince(ctx, since)
	if err != nil {
		return 0, fmt.Errorf("fetch: %w", err)
	}

	state := map[string]string{}
	if b.ServerTime != "" {
		state["cursor"] = b.ServerTime
	}
	if full {
		existing, _ := s.repo.Count()
		if err := s.checkFull(len(b.Entries), b.Total, existing); err != nil {
			return 0, err
		}
		state["last_full_at"] = strconv.FormatInt(s.now().Unix(), 10)
	}

	if err := s.stateRepo.ApplyBatch(b.Entries, full, state); err != nil {
		return 0, fmt.Errorf("apply: %w", err)
	}
	return len(b.Entries), nil
}

// plan decides full vs delta and, for a delta, the `since` to send.
func (s *Syncer) plan(forceFull bool) (full bool, since string) {
	if forceFull {
		return true, ""
	}
	lastFull, ok := s.stateRepo.State("last_full_at")
	if !ok {
		return true, ""
	}
	sec, err := strconv.ParseInt(lastFull, 10, 64)
	if err != nil || s.now().Sub(time.Unix(sec, 0)) >= s.cfg.FullEvery {
		return true, ""
	}
	cursor, ok := s.stateRepo.State("cursor")
	if !ok {
		return true, ""
	}
	t, err := time.Parse(kohaTimeFormat, cursor)
	if err != nil {
		s.log.Warn("biblio: bad stored cursor, doing a full sync", logger.F("cursor", cursor))
		return true, ""
	}
	return false, t.Add(-s.cfg.Overlap).Format(kohaTimeFormat)
}

// checkFull refuses a full listing that looks wrong, leaving the table untouched.
func (s *Syncer) checkFull(got, total, existing int) error {
	if got == 0 {
		return fmt.Errorf("full sync rejected: source returned 0 entries")
	}
	tol := total / 100 // the total is read at the start of the pull; allow small drift
	if tol < 10 {
		tol = 10
	}
	if diff := got - total; diff > tol || -diff > tol {
		return fmt.Errorf("full sync rejected: got %d entries but the source reports %d", got, total)
	}
	if existing > 0 && float64(got) < float64(existing)*s.cfg.MinKeepRatio {
		return fmt.Errorf("full sync rejected: got %d entries, table has %d (below %.0f%%)",
			got, existing, s.cfg.MinKeepRatio*100)
	}
	return nil
}

func (s *Syncer) setStatus(fn func(*SyncStatus)) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	fn(&s.status)
}
