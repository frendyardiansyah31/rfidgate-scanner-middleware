package biblio

import (
	"database/sql"
	"time"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) (*SQLiteRepository, error) {
	r := &SQLiteRepository{db: db}
	return r, r.initSchema()
}

func (r *SQLiteRepository) initSchema() error {
	_, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS item_biblio_map (
			item_code      TEXT PRIMARY KEY,
			biblio_id      TEXT NOT NULL,
			title          TEXT,
			image_filename TEXT,
			last_synced    DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_ibm_biblio_id ON item_biblio_map(biblio_id);
		CREATE TABLE IF NOT EXISTS biblio_state (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`)
	return err
}

func (r *SQLiteRepository) Upsert(e Entry) error {
	_, err := r.db.Exec(`
		INSERT INTO item_biblio_map (item_code, biblio_id, title, image_filename, last_synced)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(item_code) DO UPDATE SET
			biblio_id      = excluded.biblio_id,
			title          = excluded.title,
			image_filename = excluded.image_filename,
			last_synced    = excluded.last_synced
	`, e.ItemCode, e.BiblioID, e.Title, e.ImageFilename,
		e.LastSynced.Format("2006-01-02 15:04:05"))
	return err
}

func (r *SQLiteRepository) Lookup(itemCode string) (Entry, bool, error) {
	var e Entry
	var ts string
	err := r.db.QueryRow(`
		SELECT item_code, biblio_id, IFNULL(title,''), IFNULL(image_filename,''), last_synced
		FROM item_biblio_map WHERE item_code = ?
	`, itemCode).Scan(&e.ItemCode, &e.BiblioID, &e.Title, &e.ImageFilename, &ts)
	if err == sql.ErrNoRows {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, err
	}
	e.LastSynced, _ = time.Parse("2006-01-02 15:04:05", ts)
	return e, true, nil
}

func (r *SQLiteRepository) Count() (int, error) {
	var n int
	return n, r.db.QueryRow(`SELECT COUNT(*) FROM item_biblio_map`).Scan(&n)
}

// State returns a stored sync-state value (cursor, last_full_at).
func (r *SQLiteRepository) State(key string) (string, bool) {
	var v string
	if err := r.db.QueryRow(`SELECT value FROM biblio_state WHERE key = ?`, key).Scan(&v); err != nil {
		return "", false
	}
	return v, true
}

// ApplyBatch writes entries and the given sync state in ONE transaction, so the
// stored cursor never claims more than what was actually written. With prune,
// every row not in entries is deleted (a FULL sync: the delta cannot see items
// Koha has deleted).
func (r *SQLiteRepository) ApplyBatch(entries []Entry, prune bool, state map[string]string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	up, err := tx.Prepare(`
		INSERT INTO item_biblio_map (item_code, biblio_id, title, image_filename, last_synced)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(item_code) DO UPDATE SET
			biblio_id      = excluded.biblio_id,
			title          = excluded.title,
			image_filename = excluded.image_filename,
			last_synced    = excluded.last_synced`)
	if err != nil {
		return err
	}
	defer up.Close()

	var keep *sql.Stmt
	if prune {
		if _, err := tx.Exec(`CREATE TEMP TABLE biblio_keep (item_code TEXT PRIMARY KEY)`); err != nil {
			return err
		}
		if keep, err = tx.Prepare(`INSERT OR IGNORE INTO biblio_keep (item_code) VALUES (?)`); err != nil {
			return err
		}
		defer keep.Close()
	}

	for _, e := range entries {
		if _, err := up.Exec(e.ItemCode, e.BiblioID, e.Title, e.ImageFilename,
			e.LastSynced.Format("2006-01-02 15:04:05")); err != nil {
			return err
		}
		if prune {
			if _, err := keep.Exec(e.ItemCode); err != nil {
				return err
			}
		}
	}

	if prune {
		if _, err := tx.Exec(`DELETE FROM item_biblio_map WHERE item_code NOT IN (SELECT item_code FROM biblio_keep)`); err != nil {
			return err
		}
		if _, err := tx.Exec(`DROP TABLE biblio_keep`); err != nil {
			return err
		}
	}
	for k, v := range state {
		if _, err := tx.Exec(`INSERT INTO biblio_state (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}
