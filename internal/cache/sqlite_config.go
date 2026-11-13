package cache

import "database/sql"

func ConfigureSQLite(db *sql.DB) {
	db.Exec(`PRAGMA journal_mode=WAL;`)
	db.Exec(`PRAGMA synchronous=NORMAL;`)
	db.Exec(`PRAGMA busy_timeout=3000;`)
}
