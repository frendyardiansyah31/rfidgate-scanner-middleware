package mirror

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
	"github.com/frendyardiansyah31/rfid-middleware/internal/koha"
	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
)

type nopLog struct{}

func (nopLog) Debug(string, ...logger.Field) {}
func (nopLog) Info(string, ...logger.Field)  {}
func (nopLog) Warn(string, ...logger.Field)  {}
func (nopLog) Error(string, ...logger.Field) {}
func (nopLog) Fatal(string, ...logger.Field) {}

type fakeSource struct {
	res   koha.StatusResult
	err   error
	calls []string // the `since` of every call
}

func (f *fakeSource) FetchStatus(_ context.Context, since string) (koha.StatusResult, error) {
	f.calls = append(f.calls, since)
	return f.res, f.err
}

func rec(norm, item string, st cache.Status) koha.StatusRecord {
	return koha.StatusRecord{RFIDNorm: norm, ItemCode: item, Status: st}
}

func newMirror(t *testing.T, src Source) (*Mirror, *cache.CompositeCache) {
	t.Helper()
	mem, err := cache.NewMemoryCache(10000)
	if err != nil {
		t.Fatal(err)
	}
	sq, err := cache.NewSQLiteCache(filepath.Join(t.TempDir(), "edge.db"))
	if err != nil {
		t.Fatal(err)
	}
	c := cache.NewCompositeCache(mem, sq)
	t.Cleanup(func() { _ = c.Close() })

	m, err := New(sq.DB(), c, src, nopLog{}, Config{})
	if err != nil {
		t.Fatal(err)
	}
	return m, c
}

func TestFullSync_FillsSQLiteItemsAndState(t *testing.T) {
	src := &fakeSource{res: koha.StatusResult{
		ServerTime: "2026-11-11 22:00:00", TotalRFID: 2,
		Records: []koha.StatusRecord{rec("AAA", "B1", cache.StatusLoaned), rec("BBB", "B2", cache.StatusNotLoaned)},
	}}
	m, c := newMirror(t, src)

	if err := m.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(src.calls) != 1 || src.calls[0] != "" {
		t.Fatalf("first sync must be a FULL listing (empty since), got %v", src.calls)
	}
	if got, ok := c.Get("AAA"); !ok || got != cache.StatusLoaned {
		t.Fatalf("AAA: got (%v,%v)", got, ok)
	}
	if got, ok := c.Get("BBB"); !ok || got != cache.StatusNotLoaned {
		t.Fatalf("BBB: got (%v,%v)", got, ok)
	}
	var item string
	if err := m.db.QueryRow(`SELECT item_code FROM rfid_item WHERE rfid='AAA'`).Scan(&item); err != nil || item != "B1" {
		t.Fatalf("rfid_item AAA: %q %v", item, err)
	}
	if v, _ := m.state("cursor"); v != "2026-11-11 22:00:00" {
		t.Fatalf("cursor = %q", v)
	}
	if _, ok := m.lastFullAt(); !ok {
		t.Fatal("last_full_at not stored")
	}
}

func TestDeltaSync_UsesCursorMinusOverlap_AndUpserts(t *testing.T) {
	src := &fakeSource{res: koha.StatusResult{
		ServerTime: "2026-11-11 22:00:00", TotalRFID: 2,
		Records: []koha.StatusRecord{rec("AAA", "B1", cache.StatusLoaned), rec("BBB", "B2", cache.StatusNotLoaned)},
	}}
	m, c := newMirror(t, src)
	if err := m.SyncOnce(context.Background()); err != nil { // full
		t.Fatal(err)
	}

	// Koha now reports AAA returned (changed), delta server time moves on.
	src.res = koha.StatusResult{
		ServerTime: "2026-11-11 22:01:00", TotalRFID: 2,
		Records: []koha.StatusRecord{rec("AAA", "B1", cache.StatusNotLoaned)},
	}
	if err := m.SyncOnce(context.Background()); err != nil { // delta
		t.Fatal(err)
	}
	if want := "2026-11-11 21:45:00"; src.calls[1] != want { // 22:00:00 - 15 min
		t.Fatalf("delta since = %q, want %q (cursor minus 15 min overlap)", src.calls[1], want)
	}
	if got, _ := c.Get("AAA"); got != cache.StatusNotLoaned {
		t.Fatalf("AAA must flip to NotLoaned after delta, got %v", got)
	}
	if got, _ := c.Get("BBB"); got != cache.StatusNotLoaned {
		t.Fatalf("BBB untouched by delta, got %v", got)
	}
	if v, _ := m.state("cursor"); v != "2026-11-11 22:01:00" {
		t.Fatalf("cursor must advance to the latest server_time, got %q", v)
	}
}

func TestFullSync_PrunesRowsKohaNoLongerHas_AndLegacyKeys(t *testing.T) {
	src := &fakeSource{res: koha.StatusResult{
		ServerTime: "t", TotalRFID: 2,
		Records: []koha.StatusRecord{rec("AAA", "B1", cache.StatusLoaned), rec("BBB", "B2", cache.StatusLoaned)},
	}}
	m, c := newMirror(t, src)

	// legacy rows from the old SLiMS sync: raw, SN-prefixed keys
	if err := c.SetSQLite("SNOLD1", cache.StatusLoaned); err != nil {
		t.Fatal(err)
	}
	_ = c.SetMemory("SNOLD1", cache.StatusLoaned)

	if err := m.FullSync(context.Background()); err != nil {
		t.Fatal(err)
	}

	// BBB is deleted in Koha: the next full listing no longer has it.
	src.res = koha.StatusResult{ServerTime: "t2", TotalRFID: 1,
		Records: []koha.StatusRecord{rec("AAA", "B1", cache.StatusLoaned)}}
	if err := m.FullSync(context.Background()); err != nil {
		t.Fatal(err)
	}

	if _, ok := c.Get("BBB"); ok {
		t.Fatal("BBB must be pruned from sqlite AND memory after it vanished from Koha")
	}
	if _, ok := c.Get("SNOLD1"); ok {
		t.Fatal("legacy SN-keyed row must be pruned by the first full sync")
	}
	if _, ok := c.Get("AAA"); !ok {
		t.Fatal("AAA must survive")
	}
	var n int
	_ = m.db.QueryRow(`SELECT COUNT(*) FROM rfid_item`).Scan(&n)
	if n != 1 {
		t.Fatalf("rfid_item must be pruned too, has %d rows", n)
	}
}

func TestFullSync_RejectsSuspiciousListings_AndKeepsMirror(t *testing.T) {
	seed := []koha.StatusRecord{}
	for _, k := range []string{"A1", "A2", "A3", "A4", "A5", "A6", "A7", "A8", "A9", "A0"} {
		seed = append(seed, rec(k, "I"+k, cache.StatusLoaned))
	}
	src := &fakeSource{res: koha.StatusResult{ServerTime: "t", TotalRFID: 10, Records: seed}}
	m, c := newMirror(t, src)
	if err := m.FullSync(context.Background()); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		res  koha.StatusResult
	}{
		{"empty listing", koha.StatusResult{ServerTime: "t", TotalRFID: 0}},
		{"far fewer than the mirror (below 50%)", koha.StatusResult{ServerTime: "t", TotalRFID: 3, Records: seed[:3]}},
		{"does not match Koha's own total", koha.StatusResult{ServerTime: "t", TotalRFID: 500, Records: seed}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src.res = tc.res
			if err := m.FullSync(context.Background()); err == nil {
				t.Fatal("want the full sync to be rejected")
			}
			if _, ok := c.Get("A5"); !ok {
				t.Fatal("a rejected full sync must leave the mirror untouched")
			}
		})
	}
}

func TestSyncOnce_KohaDown_KeepsServingMirror(t *testing.T) {
	src := &fakeSource{res: koha.StatusResult{ServerTime: "t", TotalRFID: 1,
		Records: []koha.StatusRecord{rec("AAA", "B1", cache.StatusNotLoaned)}}}
	m, c := newMirror(t, src)
	if err := m.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	src.err = errors.New("connection refused")
	if err := m.SyncOnce(context.Background()); err == nil {
		t.Fatal("want the error surfaced to the caller")
	}
	if got, ok := c.Get("AAA"); !ok || got != cache.StatusNotLoaned {
		t.Fatalf("mirror must keep answering while Koha is down, got (%v,%v)", got, ok)
	}
}

func TestSyncOnce_FullSyncAgain_WhenLastFullIsOld(t *testing.T) {
	src := &fakeSource{res: koha.StatusResult{ServerTime: "2026-11-11 22:00:00", TotalRFID: 1,
		Records: []koha.StatusRecord{rec("AAA", "B1", cache.StatusLoaned)}}}
	m, _ := newMirror(t, src)
	base := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return base }

	_ = m.SyncOnce(context.Background()) // full
	m.now = func() time.Time { return base.Add(5 * time.Hour) }
	_ = m.SyncOnce(context.Background()) // delta (5h < 6h)
	m.now = func() time.Time { return base.Add(7 * time.Hour) }
	_ = m.SyncOnce(context.Background()) // full again (7h >= 6h)

	if len(src.calls) != 3 || src.calls[0] != "" || src.calls[1] == "" || src.calls[2] != "" {
		t.Fatalf("want full, delta, full; since values: %q", src.calls)
	}
}

func TestSyncOnce_BadStoredCursor_FallsBackToFull(t *testing.T) {
	src := &fakeSource{res: koha.StatusResult{ServerTime: "2026-11-11 22:00:00", TotalRFID: 1,
		Records: []koha.StatusRecord{rec("AAA", "B1", cache.StatusLoaned)}}}
	m, _ := newMirror(t, src)
	_ = m.SyncOnce(context.Background())
	_ = setState(context.Background(), m.db, "cursor", "garbage")

	if err := m.DeltaSync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if last := src.calls[len(src.calls)-1]; last != "" {
		t.Fatalf("a corrupt cursor must trigger a full listing, since=%q", last)
	}
}

func TestDeltaSync_PreservesTitleOwnedByBiblioSync(t *testing.T) {
	src := &fakeSource{res: koha.StatusResult{ServerTime: "2026-11-11 22:00:00", TotalRFID: 1,
		Records: []koha.StatusRecord{rec("AAA", "B1", cache.StatusLoaned)}}}
	m, _ := newMirror(t, src)
	_ = m.SyncOnce(context.Background())
	if _, err := m.db.Exec(`UPDATE rfid_item SET title='A Book' WHERE rfid='AAA'`); err != nil {
		t.Fatal(err)
	}

	src.res.Records = []koha.StatusRecord{rec("AAA", "B1", cache.StatusNotLoaned)}
	if err := m.DeltaSync(context.Background()); err != nil {
		t.Fatal(err)
	}
	var title string
	_ = m.db.QueryRow(`SELECT title FROM rfid_item WHERE rfid='AAA'`).Scan(&title)
	if title != "A Book" {
		t.Fatalf("title must survive a status delta, got %q", title)
	}
}

type panicSource struct{}

func (panicSource) FetchStatus(context.Context, string) (koha.StatusResult, error) {
	panic("nil map in decoder")
}

// A panic while syncing must not take the process down: it is logged and the
// next tick simply tries again.
func TestSyncAndLog_PanicInSource_IsRecovered(t *testing.T) {
	m, _ := newMirror(t, panicSource{})
	m.syncAndLog(context.Background()) // would crash the test binary without recovery
}
