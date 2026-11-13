package biblio

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Fetcher is the source of biblio data for the syncer.
type Fetcher interface {
	FetchAll(ctx context.Context) ([]Entry, error)
}

type MySQLFetcher struct {
	db *sql.DB
}

func NewMySQLFetcher(db *sql.DB) *MySQLFetcher {
	return &MySQLFetcher{db: db}
}

func (f *MySQLFetcher) FetchAll(ctx context.Context) ([]Entry, error) {
	rows, err := f.db.QueryContext(ctx, `
		SELECT i.item_code, b.biblio_id, b.title, b.image
		FROM biblio b
		JOIN item i ON b.biblio_id = i.biblio_id
		WHERE b.image IS NOT NULL AND b.image != ''
	`)
	if err != nil {
		return nil, fmt.Errorf("query biblio: %w", err)
	}
	defer rows.Close()

	now := time.Now()
	var entries []Entry
	for rows.Next() {
		var e Entry
		var biblioID int
		if err := rows.Scan(&e.ItemCode, &biblioID, &e.Title, &e.ImageFilename); err != nil {
			return nil, err
		}
		e.BiblioID = fmt.Sprintf("%d", biblioID)
		e.LastSynced = now
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
