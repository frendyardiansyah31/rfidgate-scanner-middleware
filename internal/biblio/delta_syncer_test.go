package biblio

import (
	"database/sql"
	"context"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/frendyardiansyah31/rfid-middleware/internal/koha"
	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
)

type nopLog struct{}

func (nopLog) Debug(string, ...logger.Field) {}
func (nopLog) Info(string, ...logger.Field)  {}
func (nopLog) Warn(string, ...logger.Field)  {}
func (nopLog) Error(string, ...logger.Field) {}
func (nopLog) Fatal(string, ...logger.Field) {}

type fakeDelta struct {
	batch Batch
	err   error
	calls []string // `since` of every call
}

func (f *fakeDelta) FetchSince(_ context.Context, since string) (Batch, error) {
	f.calls = append(f.calls, since)
	return f.batch, f.err
}

func entry(code, biblio, title, img string) Entry {
	return Entry{ItemCode: code, BiblioID: biblio, Title: title, ImageFilename: img, LastSynced: time.Now()}
}

func newDelta(t *testing.T, src DeltaFetcher) (*Syncer, *SQLiteRepository) {
	t.Helper()
	db, err := openMemDB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewSQLiteRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	s := NewDeltaSyncer(src, repo, nopLog{}, DeltaConfig{})
	return s, repo
}

func TestDeltaSyncer_FirstRunIsFull_ThenDeltaWithOverlap(t *testing.T) {
	src := &fakeDelta{batch: Batch{ServerTime: "2026-11-11 22:00:00", Total: 2,
		Entries: []Entry{entry("B1", "10", "Book One", "10"), entry("B2", "20", "Book Two", "")}}}
	s, repo := newDelta(t, src)
	ctx := context.Background()

	if err := s.sync(ctx, false); err != nil {
		t.Fatal(err)
	}
	if src.calls[0] != "" {
		t.Fatalf("first sync must be FULL (empty since), got %q", src.calls[0])
	}
	if e, ok, _ := repo.Lookup("B1"); !ok || e.Title != "Book One" || e.ImageFilename != "10" || e.BiblioID != "10" {
		t.Fatalf("B1 not stored right: %+v ok=%v", e, ok)
	}

	// Title edited in Koha: the delta returns only that item.
	src.batch = Batch{ServerTime: "2026-11-11 22:10:00", Total: 2, Entries: []Entry{entry("B1", "10", "Book One (2nd ed)", "10")}}
	if err := s.sync(ctx, false); err != nil {
		t.Fatal(err)
	}
	if want := "2026-11-11 21:45:00"; src.calls[1] != want { // 22:00:00 - 15 min
		t.Fatalf("delta since = %q, want %q (cursor minus overlap)", src.calls[1], want)
	}
	if e, _, _ := repo.Lookup("B1"); e.Title != "Book One (2nd ed)" {
		t.Fatalf("delta did not update the title: %+v", e)
	}
	if e, ok, _ := repo.Lookup("B2"); !ok || e.Title != "Book Two" {
		t.Fatalf("untouched row must survive a delta: %+v ok=%v", e, ok)
	}
	if v, _ := repo.State("cursor"); v != "2026-11-11 22:10:00" {
		t.Fatalf("cursor must advance, got %q", v)
	}
}

func TestDeltaSyncer_FullPrunesDeletedItems(t *testing.T) {
	src := &fakeDelta{batch: Batch{ServerTime: "t", Total: 2,
		Entries: []Entry{entry("B1", "10", "One", ""), entry("B2", "20", "Two", "")}}}
	s, repo := newDelta(t, src)
	if err := s.sync(context.Background(), true); err != nil {
		t.Fatal(err)
	}

	src.batch = Batch{ServerTime: "t2", Total: 1, Entries: []Entry{entry("B1", "10", "One", "")}}
	if err := s.sync(context.Background(), true); err != nil { // manual run = full
		t.Fatal(err)
	}
	if _, ok, _ := repo.Lookup("B2"); ok {
		t.Fatal("B2 vanished from Koha: a full sync must prune it")
	}
	if _, ok, _ := repo.Lookup("B1"); !ok {
		t.Fatal("B1 must survive")
	}
}

func TestDeltaSyncer_RejectsSuspiciousFullListing_KeepsTable(t *testing.T) {
	var es []Entry
	for _, c := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"} {
		es = append(es, entry(c, "1", "T"+c, ""))
	}
	src := &fakeDelta{batch: Batch{ServerTime: "t", Total: 10, Entries: es}}
	s, repo := newDelta(t, src)
	if err := s.sync(context.Background(), true); err != nil {
		t.Fatal(err)
	}

	cases := map[string]Batch{
		"empty":                      {ServerTime: "t", Total: 0},
		"far fewer than stored":      {ServerTime: "t", Total: 3, Entries: es[:3]},
		"does not match own total":   {ServerTime: "t", Total: 500, Entries: es},
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			src.batch = b
			if err := s.sync(context.Background(), true); err == nil {
				t.Fatal("want the full sync rejected")
			}
			if _, ok, _ := repo.Lookup("E"); !ok {
				t.Fatal("a rejected full sync must leave the table untouched")
			}
		})
	}
}

func TestDeltaSyncer_SourceDown_KeepsTable_AndReportsError(t *testing.T) {
	src := &fakeDelta{batch: Batch{ServerTime: "t", Total: 1, Entries: []Entry{entry("B1", "10", "One", "")}}}
	s, repo := newDelta(t, src)
	if err := s.sync(context.Background(), true); err != nil {
		t.Fatal(err)
	}

	src.err = errors.New("connection refused")
	if err := s.sync(context.Background(), false); err == nil {
		t.Fatal("want the error surfaced")
	}
	if _, ok, _ := repo.Lookup("B1"); !ok {
		t.Fatal("table must keep serving while the source is down")
	}
	if st := s.Status(); st.LastError == nil {
		t.Fatal("status must expose the last error")
	}
}

func TestDeltaSyncer_FullAgainWhenLastFullIsOld(t *testing.T) {
	src := &fakeDelta{batch: Batch{ServerTime: "2026-11-11 22:00:00", Total: 1, Entries: []Entry{entry("B1", "10", "One", "")}}}
	s, _ := newDelta(t, src)
	base := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)

	s.now = func() time.Time { return base }
	_ = s.sync(context.Background(), false) // full
	s.now = func() time.Time { return base.Add(20 * time.Hour) }
	_ = s.sync(context.Background(), false) // delta (20h < 24h)
	s.now = func() time.Time { return base.Add(25 * time.Hour) }
	_ = s.sync(context.Background(), false) // full again

	if len(src.calls) != 3 || src.calls[0] != "" || src.calls[1] == "" || src.calls[2] != "" {
		t.Fatalf("want full, delta, full; since values: %q", src.calls)
	}
}

func TestDeltaSyncer_PersistsStateAtomicallyWithEntries(t *testing.T) {
	src := &fakeDelta{batch: Batch{ServerTime: "t", Total: 1, Entries: []Entry{entry("B1", "10", "One", "")}}}
	s, repo := newDelta(t, src)
	_ = s.sync(context.Background(), true)
	if _, ok := repo.State("last_full_at"); !ok {
		t.Fatal("last_full_at missing")
	}
	if v, _ := repo.State("cursor"); v != "t" {
		t.Fatalf("cursor = %q", v)
	}
}

// --- Koha adapter ---------------------------------------------------------

type fakeKohaSrc struct{ res koha.BiblioResult }

func (f fakeKohaSrc) FetchBiblio(context.Context, string) (koha.BiblioResult, error) {
	return f.res, nil
}

func TestKohaFetcher_MapsCoverToBiblionumber_OnlyWhenCoverExists(t *testing.T) {
	f := NewKohaFetcher(fakeKohaSrc{res: koha.BiblioResult{ServerTime: "t", Total: 3, Records: []koha.BiblioRecord{
		{ItemCode: "B1", BiblioID: "42", Title: "With cover", HasCover: true},
		{ItemCode: "B2", BiblioID: "43", Title: "No cover", HasCover: false},
		{ItemCode: "", BiblioID: "44", Title: "No barcode", HasCover: true},
	}}})
	b, err := f.FetchSince(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Entries) != 2 {
		t.Fatalf("an item without a barcode cannot be scanned and must be skipped, got %d entries", len(b.Entries))
	}
	if b.Entries[0].ImageFilename != "42" || b.Entries[0].BiblioID != "42" {
		t.Fatalf("cover ref must be the biblionumber: %+v", b.Entries[0])
	}
	if b.Entries[1].ImageFilename != "" {
		t.Fatalf("no cover must give an EMPTY reference (the OPAC serves a blank gif otherwise): %+v", b.Entries[1])
	}
	if b.ServerTime != "t" || b.Total != 3 {
		t.Fatalf("server time / total must pass through: %+v", b)
	}
}


func openMemDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one connection so the in-memory DB (and TEMP tables) are shared
	return db, nil
}

type panicDelta struct{}

func (panicDelta) FetchSince(context.Context, string) (Batch, error) { panic("bad json") }

func TestDeltaSyncer_PanicInSource_BecomesError_NotCrash(t *testing.T) {
	s, _ := newDelta(t, panicDelta{})
	err := s.sync(context.Background(), true)
	if err == nil {
		t.Fatal("a panic must be reported as an error, not swallowed silently")
	}
	if st := s.Status(); st.IsSyncing {
		t.Fatal("IsSyncing must be reset even after a panic")
	}
}
