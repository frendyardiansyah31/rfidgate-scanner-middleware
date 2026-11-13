package history

// import (
// 	"database/sql"
// 	"time"
// )

// // Implementasi SQLite
// type SQLiteRepository struct {
// 	db *sql.DB
// }

// // Constructor
// func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
// 	repo := &SQLiteRepository{db: db}
// 	_ = repo.initSchema() // Auto-create table saat init
// 	return repo
// }

// // initSchema: Membuat tabel gate_logs jika belum ada
// func (r *SQLiteRepository) initSchema() error {
// 	query := `
// 	CREATE TABLE IF NOT EXISTS gate_logs (
// 		id INTEGER PRIMARY KEY AUTOINCREMENT,
// 		rfid TEXT NOT NULL,
// 		status INTEGER NOT NULL,
// 		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
// 	);
// 	`
// 	_, err := r.db.Exec(query)
// 	return err
// }

// // Insert: Menyimpan log ke database
// func (r *SQLiteRepository) Insert(h HistoryLog) error {
// 	// Kita biarkan SQLite yang mengisi created_at via DEFAULT CURRENT_TIMESTAMP
// 	// agar lebih cepat dan konsisten dengan waktu server DB.
// 	query := `INSERT INTO gate_logs (rfid, status) VALUES (?, ?)`

// 	_, err := r.db.Exec(query, h.RFID, h.Status)
// 	return err
// }

// // GetRecent: Mengambil data terakhir (untuk Dashboard)
// func (r *SQLiteRepository) GetRecent(limit int) ([]HistoryLog, error) {
// 	rows, err := r.db.Query(`
// 		SELECT id, rfid, status, created_at
// 		FROM gate_logs
// 		ORDER BY created_at DESC
// 		LIMIT ?`, limit)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer rows.Close()

// 	var logs []HistoryLog
// 	for rows.Next() {
// 		var l HistoryLog
// 		var tsStr string

// 		if err := rows.Scan(&l.ID, &l.RFID, &l.Status, &tsStr); err != nil {
// 			continue
// 		}

// 		// Parsing waktu SQLite string ke time.Time Go
// 		// SQLite default format: "2006-01-02 15:04:05"
// 		t, err := time.Parse("2006-01-02 15:04:05", tsStr)
// 		if err == nil {
// 			l.CreatedAt = t
// 		} else {
// 			// Fallback jika format timezone berbeda (misal ada Z)
// 			l.CreatedAt, _ = time.Parse(time.RFC3339, tsStr)
// 		}

// 		logs = append(logs, l)
// 	}
// 	return logs, nil
// }
