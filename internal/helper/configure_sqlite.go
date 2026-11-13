package helper

import "database/sql"

func configureSQLite(db *sql.DB) {
	// WAL = reader tidak keblok writer
	db.Exec(`PRAGMA journal_mode=WAL;`)

	// trade durability vs speed (AMAN untuk cache)
	db.Exec(`PRAGMA synchronous=NORMAL;`)

	// tunggu jika db lagi di-lock (ms)
	db.Exec(`PRAGMA busy_timeout=3000;`)
}
