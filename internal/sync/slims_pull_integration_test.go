//go:build integration

package sync

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
	slimssync "github.com/frendyardiansyah31/rfid-middleware/internal/sync"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

func setupSQLite(t *testing.T) *cache.SQLiteCache {
	t.Helper()

	_ = os.Remove("test_pull.sqlite")

	c, err := cache.NewSQLiteCache("test_pull.sqlite")
	if err != nil {
		t.Fatal(err)
	}

	return c
}

func TestPullAll_ToSQLite(t *testing.T) {
	if os.Getenv("SLIMS_MYSQL_DSN") == "" {
		t.Skip("SLIMS_MYSQL_DSN not set")
	}

	mysqlDB, err := sql.Open("mysql", os.Getenv("SLIMS_MYSQL_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer mysqlDB.Close()

	syncer := slimssync.NewMySQLSync(mysqlDB)
	sqliteCache := setupSQLite(t)
	defer sqliteCache.Close()

	ctx := context.Background()

	items, err := syncer.PullAll(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(items) == 0 {
		t.Fatal("no data pulled from MySQL")
	}

	for _, item := range items {
		if err := sqliteCache.Set(item.RFID, item.Status); err != nil {
			t.Fatal(err)
		}
	}

	// verify one sample
	status, ok := sqliteCache.Get(items[0].RFID)
	if !ok {
		t.Fatal("rfid not found in sqlite")
	}

	t.Logf("integration pull success: total=%d, sample_rfid=%s status=%d",
		len(items), items[0].RFID, status)
}
