package biblio

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/frendyardiansyah31/rfid-middleware/internal/history"
)

type fakeRepo struct{ entries map[string]Entry }

func (f fakeRepo) Upsert(Entry) error { return nil }
func (f fakeRepo) Count() (int, error) { return len(f.entries), nil }
func (f fakeRepo) Lookup(code string) (Entry, bool, error) {
	e, ok := f.entries[code]
	return e, ok, nil
}

func TestEnrich(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE rfid_item (rfid TEXT PRIMARY KEY, item_code TEXT NOT NULL, title TEXT)`); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO rfid_item VALUES ('AAA','C1','Item Title 1')`,
		`INSERT INTO rfid_item VALUES ('BBB','C2','Orphan Title')`,
		`INSERT INTO rfid_item VALUES ('CCC','C3','')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	repo := fakeRepo{entries: map[string]Entry{
		"C1": {ItemCode: "C1", BiblioID: "10", Title: "Biblio Title 1", ImageFilename: "c1.jpg"},
	}}
	en := NewSQLiteEnricher(db, repo)

	tests := []struct {
		name, rfid, wantTitle, wantCode string
	}{
		{"map hit wins over rfid_item title", "AAA", "Biblio Title 1", "C1"},
		{"map miss falls back to rfid_item title", "BBB", "Orphan Title", "C2"},
		{"map miss and empty rfid_item title", "CCC", "Unknown", "C3"},
		{"unknown tag", "ZZZ", "Unknown", ""},
		// rfid_item is keyed by the normalised tag, so the SN-prefixed form of a
		// scan resolves to the same row as the plain form.
		{"SN-prefixed scan resolves to normalised row", "SNAAA", "Biblio Title 1", "C1"},
		{"SN-prefixed scan, map miss", "SNBBB", "Orphan Title", "C2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := en.Enrich(history.HistoryLog{RFID: tc.rfid})
			if got.Title != tc.wantTitle || got.ItemCode != tc.wantCode {
				t.Fatalf("got title=%q code=%q, want title=%q code=%q", got.Title, got.ItemCode, tc.wantTitle, tc.wantCode)
			}
		})
	}
}
