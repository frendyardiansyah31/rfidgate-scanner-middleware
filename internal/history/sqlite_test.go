package history

import (
	"database/sql"
	"encoding/json"
	"testing"

	_ "modernc.org/sqlite"
)

func newRepo(t *testing.T) *SQLiteRepository {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return NewSQLiteRepository(db)
}

// Regression: a fresh edge.db has no scans yet. GetRecent returned a nil slice, which
// JSON-encodes as null, and the dashboards then failed with
// "Invalid API response: data is not an array".
func TestGetRecent_EmptyTable_IsEmptyArrayNotNull(t *testing.T) {
	logs, err := newRepo(t).GetRecent(50)
	if err != nil {
		t.Fatal(err)
	}
	if logs == nil {
		t.Fatal("GetRecent must return a non-nil slice for an empty table")
	}
	b, _ := json.Marshal(logs)
	if string(b) != "[]" {
		t.Fatalf("empty history must encode as [], got %s", b)
	}
}

func TestGetRecent_ReturnsInsertedRows_NewestFirst(t *testing.T) {
	r := newRepo(t)
	for _, rfid := range []string{"A", "B"} {
		if err := r.Insert(&HistoryLog{RFID: rfid, Status: 1}); err != nil {
			t.Fatal(err)
		}
	}
	logs, err := r.GetRecent(50)
	if err != nil || len(logs) != 2 {
		t.Fatalf("want 2 rows, got %d err=%v", len(logs), err)
	}
}
