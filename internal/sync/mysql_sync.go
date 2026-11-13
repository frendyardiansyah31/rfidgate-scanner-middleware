package sync

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/domain"
	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
	"github.com/frendyardiansyah31/rfid-middleware/internal/queue"
)

type DBState int

const (
	DBStarting DBState = iota
	DBConnected
	DBDisconnected
	DBReconnecting
)

type MySQLSlimsSync struct {
	db        *sql.DB
	mysqlDown uint32
	log       logger.Logger

	mu    sync.Mutex
	state DBState
}

func NewMySQLSync(db *sql.DB, log logger.Logger) *MySQLSlimsSync {
	return &MySQLSlimsSync{db: db, log: log, state: DBStarting}
}

func (s *MySQLSlimsSync) Save(
	ctx context.Context,
	event queue.Event,
) error {

	return Retry(ctx, RetryPolicy{
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     10 * time.Second,
	}, func() error {

		_, err := s.db.ExecContext(ctx, `
									INSERT INTO log_bukukeluar_test (tgl, label_code, sts)
									VALUES (?, ?, ?)
									`,
			time.Now().Format("2006-01-02 15:04:05"),
			event.RFID,
			int(event.Status),
		)

		if err != nil {
			return err
		}

		s.log.Info("event pushed to slims",
			logger.F("rfid", event.RFID),
			logger.F("status", event.Status),
		)

		return nil
	})
}

func (s *MySQLSlimsSync) PullDelta(
	ctx context.Context,
	since time.Time,
) ([]domain.RFIDSnapshot, error) {

	var result []domain.RFIDSnapshot

	err := Retry(ctx, RetryPolicy{
		InitialDelay: 60 * time.Second,
		MaxDelay:     180 * time.Second,
	}, func() error {
		query := `
            SELECT
				i.label_code AS rfid,
				i.item_code,
				b.title,
				CASE
					WHEN l.is_lent = 1 AND l.is_return = 0 THEN 0 -- Loaned
					ELSE 1 -- Available
				END AS status,
				-- Ambil tanggal paling baru dari 3 kemungkinan: Tgl Buat, Tgl Edit, Tgl Pinjam
				GREATEST(
					COALESCE(i.input_date, '1970-01-01 00:00:00'),
					COALESCE(i.last_update, '1970-01-01 00:00:00'),
					COALESCE(l.last_update, '1970-01-01 00:00:00')
				) AS updated_at
			FROM item i
			LEFT JOIN biblio b ON b.biblio_id = i.biblio_id
			LEFT JOIN loan l
				ON l.item_code = i.item_code
				AND l.last_update = (
					SELECT MAX(l2.last_update)
					FROM loan l2
					WHERE l2.item_code = i.item_code
				)
			WHERE
				i.label_code IS NOT NULL AND i.label_code != ''
				AND (
					COALESCE(i.input_date, '1970-01-01') > ?
					OR COALESCE(i.last_update, '1970-01-01') > ?
					OR COALESCE(l.last_update, '1970-01-01') > ?
				)
        	`

		rows, err := s.db.QueryContext(ctx, query, since, since, since)

		if err != nil {
			s.markDown(err)
			return err
		}

		defer rows.Close()
		result = result[:0] // Reset slice

		count := 0
		lastLogTime := time.Now()

		for rows.Next() {
			var r domain.RFIDSnapshot
			var ts string
			var title sql.NullString

			if err := rows.Scan(&r.RFID, &r.ItemCode, &title, &r.Status, &ts); err != nil {
				s.markDown(err)
				return err
			}

			if title.Valid {
				r.Title = title.String
			}

			// Parsing waktu yang lebih toleran (kadang SLiMS pakai format beda)
			t, err := time.Parse("2006-01-02 15:04:05", ts)
			if err != nil {
				// Fallback jika format waktu error, pakai waktu sekarang
				t = time.Now()
			}
			r.UpdatedAt = t
			result = append(result, r)

			// 🔧 LOG PROGRESS DOWNLOAD (Setiap 5.000 item atau 3 detik)
			count++
			if count%5000 == 0 || time.Since(lastLogTime) > 3*time.Second {
				s.log.Info("downloading data from slims...",
					logger.F("fetched", count),
				)
				lastLogTime = time.Now()
			}
		}

		// Log final jumlah download
		s.log.Info("download from slims finished", logger.F("total_fetched", count))

		s.markUp()
		return nil
	})

	return result, err
}

func (s *MySQLSlimsSync) CountRemote(ctx context.Context) (int, error) {
	var count int
	// Hitung item yang punya kode RFID (label_code tidak null)
	err := s.db.QueryRowContext(ctx, `
        SELECT COUNT(*) 
        FROM item 
        WHERE label_code IS NOT NULL AND label_code != ''
    `).Scan(&count)

	if err != nil {
		return 0, err
	}
	return count, nil
}
