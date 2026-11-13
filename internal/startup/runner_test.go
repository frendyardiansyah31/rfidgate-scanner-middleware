package startup_test

import (
	"context"
	"time"

	_ "modernc.org/sqlite"

	"github.com/frendyardiansyah31/rfid-middleware/internal/queue"
	"github.com/frendyardiansyah31/rfid-middleware/internal/sync"

	_ "github.com/go-sql-driver/mysql"
)

type FakeSync struct {
	pullAll   []sync.RFIDStatus
	pullDelta []sync.RFIDStatus
	pushed    []queue.Event
}

func (f *FakeSync) PullAll(ctx context.Context) ([]sync.RFIDStatus, error) {
	return f.pullAll, nil
}

func (f *FakeSync) PullDelta(ctx context.Context, since time.Time) ([]sync.RFIDStatus, error) {
	return f.pullDelta, nil
}

func (f *FakeSync) Push(ctx context.Context, e queue.Event) error {
	f.pushed = append(f.pushed, e)
	return nil
}

func (f *FakeSync) Save(ctx context.Context, event queue.Event) error {
	return nil
}

func (f *FakeSync) CountRemote(ctx context.Context) (int, error) {
	return 1, nil
}

// func TestStartupRunner_Run(t *testing.T) {
// 	ctx := context.Background()

// 	// ===== temp sqlite file =====
// 	tmp, err := os.CreateTemp("", "startup-*.db")
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	defer os.Remove(tmp.Name())

// 	// ===== SINGLE sqlite DB =====
// 	db, err := sql.Open("sqlite", tmp.Name())
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	defer db.Close()

// 	// ===== sqlite cache (PAKAI DB YANG SAMA) =====
// 	sqliteCache, err := cache.NewSQLiteCacheFromDB(db)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	memCache, _ := cache.NewMemoryCache(100)
// 	compositeCache := cache.NewCompositeCache(memCache, sqliteCache)

// 	// ===== offline store (DB YANG SAMA) =====
// 	offlineStore := sync.NewSQLiteOfflineStore(db)

// 	// seed offline event
// 	_, err = db.Exec(`
// 		INSERT INTO offline_events (rfid, status, created_at)
// 		VALUES ('RFID-OFFLINE', 1, datetime('now'))
// 	`)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	// ===== fake slims =====
// 	fakeSync := &FakeSync{
// 		pullAll: []sync.RFIDStatus{
// 			{RFID: "RFID-1", Status: cache.StatusLoaned},
// 		},
// 		pullDelta: []sync.RFIDStatus{
// 			{RFID: "RFID-2", Status: cache.StatusNotLoaned},
// 		},
// 	}

// 	log, _ := logger.New("debug")

// 	runner := startup.NewRunner(
// 		compositeCache,
// 		fakeSync,
// 		offlineStore,
// 		log,
// 	)

// 	// ===== run startup =====
// 	runner.Run(ctx)

// 	// ===== DEBUG (sekarang HARUS ADA ROW) =====
// 	rows, err := db.Query(`SELECT rfid, status FROM rfid_status`)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	defer rows.Close()
// 	for rows.Next() {
// 		var r string
// 		var s int
// 		rows.Scan(&r, &s)
// 		t.Log("SQLITE ROW:", r, s)
// 	}

// 	// ===== ASSERTS =====

// 	if !sqliteCache.Exists("RFID-1") {
// 		t.Fatal("RFID-1 not found in sqlite")
// 	}
// 	if !sqliteCache.Exists("RFID-2") {
// 		t.Fatal("RFID-2 not found in sqlite")
// 	}

// 	if _, ok := memCache.Get("RFID-1"); !ok {
// 		t.Fatal("memory cache not warmed")
// 	}

// 	if len(fakeSync.pushed) != 0 {
// 		t.Fatalf("expected 0 offline push, got %d", len(fakeSync.pushed))
// 	}
// }
