package sync

import (
	"context"
	"database/sql"

	"github.com/frendyardiansyah31/rfid-middleware/internal/history"
	"github.com/frendyardiansyah31/rfid-middleware/internal/queue"
)

type SQLiteOfflineStore struct {
	db *sql.DB
}

func NewSQLiteOfflineStore(db *sql.DB) *SQLiteOfflineStore {
	schema := `
	CREATE TABLE IF NOT EXISTS offline_events (
		rfid TEXT NOT NULL,
		status INTEGER NOT NULL,
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_offline_events_created
	ON offline_events(created_at);

	CREATE TABLE IF NOT EXISTS sync_state (
		key TEXT PRIMARY KEY,
		value TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, _ = db.Exec(schema)

	return &SQLiteOfflineStore{db: db}
}

func (s *SQLiteOfflineStore) Save(ctx context.Context, event queue.Event) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO offline_events (rfid, status, created_at)
		VALUES (?, ?, ?)
	`,
		event.RFID,
		int(event.Status),
		event.Time,
	)
	return err
}

func (s *SQLiteOfflineStore) LoadAll(ctx context.Context) ([]queue.Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT rfid, status, created_at
		FROM offline_events
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []queue.Event
	for rows.Next() {
		var e queue.Event
		if err := rows.Scan(&e.RFID, &e.Status, &e.Time); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

func (s *SQLiteOfflineStore) Delete(ctx context.Context, e queue.Event) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM offline_events WHERE rfid = ? AND created_at = ?`,
		e.RFID, e.Time,
	)
	return err
}

func (s *SQLiteOfflineStore) GetState(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM sync_state WHERE key = ?`,
		key,
	).Scan(&value)

	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (s *SQLiteOfflineStore) SetState(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sync_state (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key)
		DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`, key, value)

	return err
}

func (s *SQLiteOfflineStore) GetRecent(limit int) ([]history.HistoryLog, error) {
	rows, err := s.db.Query(`
		SELECT id, rfid, status, created_at
		FROM gate_logs
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []history.HistoryLog
	for rows.Next() {
		var h history.HistoryLog
		if err := rows.Scan(
			&h.ID,
			&h.RFID,
			&h.Status,
			&h.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, h)
	}
	return result, nil
}

func (s *SQLiteOfflineStore) Insert(ctx context.Context, h history.HistoryLog) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gate_logs (id, rfid, status, created_at)
		VALUES (?, ?, ?, ?)
	`,
		h.ID,
		h.RFID,
		h.Status,
		h.CreatedAt,
	)
	return err
}
