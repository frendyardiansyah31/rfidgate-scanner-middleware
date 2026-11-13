// Package mirror keeps the gate's local SQLite copy of Koha loan status
// (rfid_status, rfid_item) up to date, so a scan can still be decided when Koha
// is down or slow.
//
// Two kinds of sync:
//
//   - delta: only items changed since the last successful sync (cheap, every minute)
//   - full : the complete listing, which also PRUNES rows Koha no longer has. The
//     delta cannot see deleted items/tags, so a full sync runs at startup (when
//     the mirror is new or old) and every FullEvery after that.
//
// Keys are normalised tags (rfidtag.Normalize), the same form the Koha plugin
// returns and the engine looks up.
package mirror

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
	"github.com/frendyardiansyah31/rfid-middleware/internal/koha"
	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
	"github.com/frendyardiansyah31/rfid-middleware/internal/safego"
)

// Source is where status comes from (the Koha plugin client; a fake in tests).
type Source interface {
	FetchStatus(ctx context.Context, since string) (koha.StatusResult, error)
}

// kohaTimeFormat is the format of Koha's server_time / since parameter.
const kohaTimeFormat = "2006-01-02 15:04:05"

type Config struct {
	// DeltaEvery is how often a sync runs. Default 1 minute.
	DeltaEvery time.Duration
	// FullEvery is the maximum age of the last full sync before the next one is a
	// full sync. Default 6 hours.
	FullEvery time.Duration
	// Overlap is subtracted from the stored cursor so changes that were not yet
	// visible (or whose items.timestamp lags the real event, as seen on a few
	// returns in Koha dev) are fetched again. Re-fetching is harmless (upserts
	// are idempotent). Default 15 minutes.
	Overlap time.Duration
	// MinKeepRatio protects against wiping the mirror on a bad full listing: a
	// full sync that returns fewer than this fraction of the existing rows is
	// rejected. Default 0.5.
	MinKeepRatio float64
}

func (c *Config) setDefaults() {
	if c.DeltaEvery <= 0 {
		c.DeltaEvery = time.Minute
	}
	if c.FullEvery <= 0 {
		c.FullEvery = 6 * time.Hour
	}
	if c.Overlap <= 0 {
		c.Overlap = 15 * time.Minute
	}
	if c.MinKeepRatio <= 0 {
		c.MinKeepRatio = 0.5
	}
}

type Mirror struct {
	db    *sql.DB
	cache *cache.CompositeCache
	src   Source
	log   logger.Logger
	cfg   Config
	now   func() time.Time
}

// New creates the helper tables it needs (rfid_item, mirror_state); rfid_status
// comes from the cache package's own schema.
func New(db *sql.DB, c *cache.CompositeCache, src Source, log logger.Logger, cfg Config) (*Mirror, error) {
	cfg.setDefaults()
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS rfid_item (
			rfid TEXT PRIMARY KEY,
			item_code TEXT NOT NULL,
			title TEXT,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS mirror_state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`); err != nil {
		return nil, fmt.Errorf("mirror: init schema: %w", err)
	}
	return &Mirror{db: db, cache: c, src: src, log: log, cfg: cfg, now: time.Now}, nil
}

// Start warms the memory cache from SQLite, then syncs immediately and on every
// tick until ctx is cancelled. It returns at once; the loop runs in a goroutine.
func (m *Mirror) Start(ctx context.Context) {
	go func() {
		m.warmMemory()
		m.syncAndLog(ctx)

		t := time.NewTicker(m.cfg.DeltaEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				m.log.Info("mirror sync stopped")
				return
			case <-t.C:
				m.syncAndLog(ctx)
			}
		}
	}()
	m.log.Info("mirror sync started",
		logger.F("delta_every", m.cfg.DeltaEvery.String()),
		logger.F("full_every", m.cfg.FullEvery.String()),
	)
}

func (m *Mirror) syncAndLog(ctx context.Context) {
	defer safego.Recover(m.log, "mirror.sync", nil)
	if err := m.SyncOnce(ctx); err != nil {
		// Koha being unreachable is expected from time to time; the mirror keeps
		// serving whatever it already has.
		m.log.Warn("mirror sync failed", logger.F("error", err))
	}
}

// SyncOnce runs a full sync when the mirror has never had one or the last one is
// older than FullEvery, otherwise a delta sync.
func (m *Mirror) SyncOnce(ctx context.Context) error {
	lastFull, hasFull := m.lastFullAt()
	if !hasFull || m.now().Sub(lastFull) >= m.cfg.FullEvery {
		return m.FullSync(ctx)
	}
	return m.DeltaSync(ctx)
}

// DeltaSync fetches items changed since (cursor - Overlap) and upserts them.
func (m *Mirror) DeltaSync(ctx context.Context) error {
	cursor, ok := m.state("cursor")
	if !ok {
		return m.FullSync(ctx)
	}
	since, err := m.sinceFrom(cursor)
	if err != nil {
		m.log.Warn("mirror: bad stored cursor, doing a full sync", logger.F("cursor", cursor), logger.F("error", err))
		return m.FullSync(ctx)
	}

	res, err := m.src.FetchStatus(ctx, since)
	if err != nil {
		return fmt.Errorf("delta fetch: %w", err)
	}

	if err := m.upsert(ctx, res.Records, false, res.ServerTime); err != nil {
		return err
	}
	for _, r := range res.Records {
		_ = m.cache.SetMemory(r.RFIDNorm, r.Status)
	}
	m.audit(res.TotalRFID)
	if len(res.Records) > 0 {
		m.log.Debug("mirror delta sync", logger.F("changed", len(res.Records)))
	}
	return nil
}

// FullSync replaces the mirror with Koha's complete listing, pruning rows Koha no
// longer has. It refuses (and keeps the mirror untouched) when the listing looks
// wrong: empty, wildly smaller than the mirror, or not matching Koha's own total.
func (m *Mirror) FullSync(ctx context.Context) error {
	res, err := m.src.FetchStatus(ctx, "")
	if err != nil {
		return fmt.Errorf("full fetch: %w", err)
	}

	existing := m.count()
	if err := m.checkFull(len(res.Records), res.TotalRFID, existing); err != nil {
		return err
	}

	if err := m.upsert(ctx, res.Records, true, res.ServerTime); err != nil {
		return err
	}

	// Memory may still hold keys that were just pruned: rebuild it from the result.
	m.cache.ClearMemory()
	for _, r := range res.Records {
		_ = m.cache.SetMemory(r.RFIDNorm, r.Status)
	}
	m.log.Info("mirror full sync", logger.F("records", len(res.Records)), logger.F("was", existing))
	m.audit(res.TotalRFID)
	return nil
}

func (m *Mirror) checkFull(got, total, existing int) error {
	if got == 0 {
		return fmt.Errorf("full sync rejected: Koha returned 0 records")
	}
	// Koha's total is read at the start of the pull and items can change while we
	// page, so allow a small drift (max of 10 or 1%).
	tol := total / 100
	if tol < 10 {
		tol = 10
	}
	if diff := got - total; diff > tol || -diff > tol {
		return fmt.Errorf("full sync rejected: got %d records but Koha reports %d", got, total)
	}
	if existing > 0 && float64(got) < float64(existing)*m.cfg.MinKeepRatio {
		return fmt.Errorf("full sync rejected: got %d records, mirror has %d (below %.0f%%)",
			got, existing, m.cfg.MinKeepRatio*100)
	}
	return nil
}

// upsert writes records in one transaction. When prune is true, every row not in
// records is deleted from rfid_status and rfid_item. The cursor (and, for a full
// sync, last_full_at) is stored in the same transaction, so state never claims
// more than what was actually written.
func (m *Mirror) upsert(ctx context.Context, recs []koha.StatusRecord, prune bool, serverTime string) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO rfid_status (rfid, status) VALUES (?, ?)
		ON CONFLICT(rfid) DO UPDATE SET status = excluded.status, updated_at = CURRENT_TIMESTAMP`)
	if err != nil {
		return err
	}
	defer stStmt.Close()

	// title is owned by the biblio sync, so it is never overwritten here.
	itStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO rfid_item (rfid, item_code) VALUES (?, ?)
		ON CONFLICT(rfid) DO UPDATE SET item_code = excluded.item_code, updated_at = CURRENT_TIMESTAMP`)
	if err != nil {
		return err
	}
	defer itStmt.Close()

	if prune {
		if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE mirror_keep (rfid TEXT PRIMARY KEY)`); err != nil {
			return err
		}
	}
	keepStmt := (*sql.Stmt)(nil)
	if prune {
		keepStmt, err = tx.PrepareContext(ctx, `INSERT OR IGNORE INTO mirror_keep (rfid) VALUES (?)`)
		if err != nil {
			return err
		}
		defer keepStmt.Close()
	}

	for _, r := range recs {
		if _, err := stStmt.ExecContext(ctx, r.RFIDNorm, int(r.Status)); err != nil {
			return err
		}
		if _, err := itStmt.ExecContext(ctx, r.RFIDNorm, r.ItemCode); err != nil {
			return err
		}
		if prune {
			if _, err := keepStmt.ExecContext(ctx, r.RFIDNorm); err != nil {
				return err
			}
		}
	}

	if prune {
		if _, err := tx.ExecContext(ctx, `DELETE FROM rfid_status WHERE rfid NOT IN (SELECT rfid FROM mirror_keep)`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM rfid_item WHERE rfid NOT IN (SELECT rfid FROM mirror_keep)`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DROP TABLE mirror_keep`); err != nil {
			return err
		}
		if err := setState(ctx, tx, "last_full_at", strconv.FormatInt(m.now().Unix(), 10)); err != nil {
			return err
		}
	}
	if serverTime != "" {
		if err := setState(ctx, tx, "cursor", serverTime); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func setState(ctx context.Context, e execer, key, value string) error {
	_, err := e.ExecContext(ctx,
		`INSERT INTO mirror_state (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (m *Mirror) state(key string) (string, bool) {
	var v string
	if err := m.db.QueryRow(`SELECT value FROM mirror_state WHERE key = ?`, key).Scan(&v); err != nil {
		return "", false
	}
	return v, true
}

func (m *Mirror) lastFullAt() (time.Time, bool) {
	v, ok := m.state("last_full_at")
	if !ok {
		return time.Time{}, false
	}
	sec, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(sec, 0), true
}

// sinceFrom turns the stored Koha-local cursor into the `since` to send:
// cursor - Overlap. The cursor is Koha's own clock, so no time zone conversion
// is involved; the arithmetic is done on the naive timestamp.
func (m *Mirror) sinceFrom(cursor string) (string, error) {
	t, err := time.Parse(kohaTimeFormat, cursor)
	if err != nil {
		return "", err
	}
	return t.Add(-m.cfg.Overlap).Format(kohaTimeFormat), nil
}

// count returns the number of mirror rows in normalised form. A normalised key
// never contains "SN", so rows that do are legacy (the old SLiMS sync keyed
// them by the raw label, e.g. "SN0038..."). They are ignored here so they can
// neither block the first Koha full sync nor skew the audit; that full sync
// prunes them.
func (m *Mirror) count() int {
	var n int
	_ = m.db.QueryRow(`SELECT COUNT(*) FROM rfid_status WHERE rfid NOT LIKE '%SN%'`).Scan(&n)
	return n
}

func (m *Mirror) warmMemory() {
	items, err := m.cache.AllFromSQLite()
	if err != nil {
		m.log.Error("mirror: warm memory cache failed", logger.F("error", err))
		return
	}
	for k, st := range items {
		_ = m.cache.SetMemory(k, st)
	}
	m.log.Info("memory cache warmed from sqlite", logger.F("count", len(items)))
}

// audit logs when the mirror and Koha disagree on how many tagged items exist.
func (m *Mirror) audit(kohaTotal int) {
	local := m.count()
	tol := kohaTotal / 100
	if tol < 10 {
		tol = 10
	}
	if d := local - kohaTotal; d > tol || -d > tol {
		m.log.Warn("mirror/koha count discrepancy",
			logger.F("local", local), logger.F("koha", kohaTotal))
	}
}
