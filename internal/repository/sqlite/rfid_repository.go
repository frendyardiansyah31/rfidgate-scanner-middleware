package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/domain"
	"github.com/frendyardiansyah31/rfid-middleware/internal/repository"
)

type sqliteRFIDRepository struct {
	db *sql.DB
	tx *sql.Tx
}

func NewRFIDRepository(db *sql.DB) (repository.RFIDRepository, error) {
	// Ensure schema exists
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS rfid_item (
			rfid TEXT PRIMARY KEY,
			item_code TEXT NOT NULL,
			title TEXT,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to init rfid_item schema: %w", err)
	}

	return &sqliteRFIDRepository{db: db}, nil
}

func (r *sqliteRFIDRepository) UpsertStatus(ctx context.Context, snap domain.RFIDSnapshot) error {
	query := `
		INSERT INTO rfid_status (rfid, status)
		VALUES (?, ?)
		ON CONFLICT(rfid) DO UPDATE SET
			status = excluded.status,
			updated_at = CURRENT_TIMESTAMP
	`
	return r.exec(ctx, query, snap.RFID, snap.Status)
}

func (r *sqliteRFIDRepository) UpsertItem(ctx context.Context, snap domain.RFIDSnapshot) error {
	query := `
		INSERT INTO rfid_item (rfid, item_code, title, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(rfid) DO UPDATE SET
			item_code = excluded.item_code,
			title = excluded.title,
			updated_at = CURRENT_TIMESTAMP
	`
	return r.exec(ctx, query, snap.RFID, snap.ItemCode, snap.Title)
}

func (r *sqliteRFIDRepository) WithTx(
	ctx context.Context,
	fn func(repo repository.RFIDRepository) error,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	txRepo := &sqliteRFIDRepository{
		db: r.db,
		tx: tx,
	}

	if err := fn(txRepo); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

func (r *sqliteRFIDRepository) LastUpdated(ctx context.Context) (time.Time, error) {
	// Use the older of the two tables' max timestamps so that if rfid_item is
	// behind (e.g. newly added table with no data yet), a full backfill is
	// triggered automatically. After backfill both tables stay in sync.
	query := `
		SELECT IFNULL(MIN(
			(SELECT IFNULL(MAX(updated_at), '1970-01-01 00:00:00') FROM rfid_status),
			(SELECT IFNULL(MAX(updated_at), '1970-01-01 00:00:00') FROM rfid_item)
		), '1970-01-01 00:00:00')
	`
	row := r.queryRow(ctx, query)
	var ts string
	if err := row.Scan(&ts); err != nil {
		return time.Time{}, err
	}
	return time.Parse("2006-01-02 15:04:05", ts)
}

// Helpers

func (r *sqliteRFIDRepository) exec(ctx context.Context, query string, args ...interface{}) error {
	if r.tx != nil {
		_, err := r.tx.ExecContext(ctx, query, args...)
		return err
	}
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *sqliteRFIDRepository) queryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	if r.tx != nil {
		return r.tx.QueryRowContext(ctx, query, args...)
	}
	return r.db.QueryRowContext(ctx, query, args...)
}
