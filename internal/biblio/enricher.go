package biblio

import (
	"database/sql"

	"github.com/frendyardiansyah31/rfid-middleware/internal/history"
	"github.com/frendyardiansyah31/rfid-middleware/internal/rfidtag"
)

// SQLiteEnricher implements history.Enricher.
// It resolves rfid → item_code (via rfid_item) then item_code → biblio data (via item_biblio_map).
type SQLiteEnricher struct {
	db   *sql.DB
	repo Repository
}

func NewSQLiteEnricher(db *sql.DB, repo Repository) *SQLiteEnricher {
	return &SQLiteEnricher{db: db, repo: repo}
}

func (e *SQLiteEnricher) Enrich(log history.HistoryLog) history.HistoryLog {
	// rfid_item is keyed by the normalised tag (no "SN"), so one lookup covers
	// both the prefixed and the plain form of the scanned tag.
	var itemCode, itemTitle string
	_ = e.db.QueryRow(
		`SELECT IFNULL(item_code,''), IFNULL(title,'') FROM rfid_item WHERE rfid = ?`, rfidtag.Normalize(log.RFID),
	).Scan(&itemCode, &itemTitle)

	log.ItemCode = itemCode

	if itemCode == "" {
		log.Title = "Unknown"
		return log
	}

	entry, found, _ := e.repo.Lookup(itemCode)
	if !found {
		// item_biblio_map has no row yet: fall back to the title that
		// rfid_item already carries from the SLiMS pull.
		if itemTitle != "" {
			log.Title = itemTitle
		} else {
			log.Title = "Unknown"
		}
		return log
	}

	log.BiblioID = entry.BiblioID
	log.Title = entry.Title
	log.ImageFilename = entry.ImageFilename
	return log
}
