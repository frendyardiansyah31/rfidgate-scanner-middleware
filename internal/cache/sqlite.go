package cache

import (
	"database/sql"
	"time"

	_ "embed"

	_ "modernc.org/sqlite"
)

//go:embed sqlite_schema.sql
var sqliteSchema string

type SQLiteCache struct {
	db *sql.DB
}

func NewSQLiteCache(dbPath string) (*SQLiteCache, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	ConfigureSQLite(db)
	if _, err := db.Exec(sqliteSchema); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteCache{db: db}, nil
}

func NewSQLiteCacheFromDB(db *sql.DB) (*SQLiteCache, error) {
	c := &SQLiteCache{db: db}

	ConfigureSQLite(db)
	if err := c.initSchema(); err != nil {
		return nil, err
	}

	return c, nil
}

func (s *SQLiteCache) Get(rfid string) (Status, bool) {
	var status int
	err := s.db.QueryRow(
		`SELECT status FROM rfid_status WHERE rfid = ?`,
		rfid,
	).Scan(&status)

	if err != nil {
		return StatusUnknown, false
	}

	return Status(status), true
}

func (s *SQLiteCache) Set(rfid string, status Status) error {
	_, err := s.db.Exec(`
		INSERT INTO rfid_status (rfid, status)
		VALUES (?, ?)
		ON CONFLICT(rfid) DO UPDATE SET
			status = excluded.status,
			updated_at = CURRENT_TIMESTAMP
	`, rfid, status)

	return err
}

func (s *SQLiteCache) Close() error {
	s.db.Close()
	return nil
}

func (s *SQLiteCache) LoadAll() (map[string]Status, error) {
	rows, err := s.db.Query(`SELECT rfid, status FROM rfid_status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]Status)
	for rows.Next() {
		var rfid string
		var status int
		if err := rows.Scan(&rfid, &status); err != nil {
			return nil, err
		}
		result[rfid] = Status(status)
	}
	return result, nil
}

func (s *SQLiteCache) IsEmpty() bool {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM rfid_status`).Scan(&count)
	if err != nil {
		return true
	}
	return count == 0
}

func (s *SQLiteCache) LastUpdated() (time.Time, error) {
	row := s.db.QueryRow(`
		SELECT IFNULL(MAX(updated_at), '1970-01-01 00:00:00')
		FROM rfid_status
	`)

	var ts string
	if err := row.Scan(&ts); err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}

	t, err := time.Parse("2006-01-02 15:04:05", ts)
	if err != nil {
		return time.Time{}, err
	}

	return t, nil
}

func (c *SQLiteCache) DB() *sql.DB {
	return c.db
}

func (s *SQLiteCache) Exists(rfid string) bool {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(1) FROM rfid_status WHERE rfid = ?`,
		rfid,
	).Scan(&count)

	if err != nil {
		return false
	}
	return count > 0
}

func (c *SQLiteCache) initSchema() error {
	_, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS rfid_status (
			rfid TEXT PRIMARY KEY,
			status INTEGER NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)
	return err
}

func (s *SQLiteCache) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM rfid_status`).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}
