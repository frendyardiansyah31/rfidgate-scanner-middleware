package history

import (
	"database/sql"
	"time"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	schema := `
		CREATE TABLE IF NOT EXISTS gate_logs (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			rfid           TEXT,
			status         INTEGER,
			created_at     DATETIME,
			item_code      TEXT,
			title          TEXT,
			biblio_id      TEXT,
			image_filename TEXT
		);
	`
	db.Exec(schema)

	// Migrations for existing databases — errors silently ignored (column already exists)
	db.Exec(`ALTER TABLE gate_logs ADD COLUMN item_code TEXT`)
	db.Exec(`ALTER TABLE gate_logs ADD COLUMN title TEXT`)
	db.Exec(`ALTER TABLE gate_logs ADD COLUMN biblio_id TEXT`)
	db.Exec(`ALTER TABLE gate_logs ADD COLUMN image_filename TEXT`)

	return &SQLiteRepository{db: db}
}

func (r *SQLiteRepository) Insert(log *HistoryLog) error {
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	result, err := r.db.Exec(`
		INSERT INTO gate_logs (rfid, status, created_at, item_code, title, biblio_id, image_filename)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		log.RFID,
		log.Status,
		log.CreatedAt.Format(time.RFC3339),
		log.ItemCode,
		log.Title,
		log.BiblioID,
		log.ImageFilename,
	)
	if err != nil {
		return err
	}
	log.ID, _ = result.LastInsertId()
	return nil
}

func (r *SQLiteRepository) GetRecent(limit int) ([]HistoryLog, error) {
	rows, err := r.db.Query(`
		SELECT
			id,
			rfid,
			status,
			created_at,
			IFNULL(item_code, '')      AS item_code,
			IFNULL(title, '')          AS title,
			IFNULL(biblio_id, '')      AS biblio_id,
			IFNULL(image_filename, '') AS image_filename
		FROM gate_logs
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Non-nil on purpose: an empty table must encode as [] and not null. The dashboards
	// reject anything that is not an array, so a fresh edge.db (no scans yet) showed
	// "Failed to load data: Invalid API response: data is not an array".
	logs := make([]HistoryLog, 0)
	for rows.Next() {
		var l HistoryLog
		var ts string
		if err := rows.Scan(
			&l.ID, &l.RFID, &l.Status, &ts,
			&l.ItemCode, &l.Title, &l.BiblioID, &l.ImageFilename,
		); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			// Backward-compat: rows written before this fix used a zone-less layout
			// that always held local wall-clock time, but was mislabeled as UTC on
			// read. Reinterpret them as Local instead of defaulting to UTC.
			t, err = time.ParseInLocation("2006-01-02 15:04:05", ts, time.Local)
			if err != nil {
				t = time.Now()
			}
		}
		l.CreatedAt = t
		logs = append(logs, l)
	}
	return logs, nil
}
