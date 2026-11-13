package cache

import (
	"path/filepath"
	"testing"
	"time"
)

// Proves the local-tier order inside CompositeCache: a key present only in
// SQLite is still returned on a memory miss, and then warmed into memory.
func TestCompositeCache_MemoryMiss_FallsBackToSQLite_AndWarms(t *testing.T) {
	mem, err := NewMemoryCache(1000)
	if err != nil {
		t.Fatalf("mem cache: %v", err)
	}
	sq, err := NewSQLiteCache(filepath.Join(t.TempDir(), "edge.db"))
	if err != nil {
		t.Fatalf("sqlite cache: %v", err)
	}
	c := NewCompositeCache(mem, sq)
	defer c.Close()

	// Only SQLite has the record — as it would be right after a background sync
	// wrote the mirror but before the entry was ever read.
	if err := c.SetSQLite("TAGX", StatusNotLoaned); err != nil {
		t.Fatalf("seed sqlite: %v", err)
	}

	got, ok := c.Get("TAGX")
	if !ok || got != StatusNotLoaned {
		t.Fatalf("expected SQLite fallback hit, got (%v, %v)", got, ok)
	}

	// Warm-up into memory is asynchronous in ristretto; poll briefly.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if v, ok := mem.Get("TAGX"); ok && v == StatusNotLoaned {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("memory cache was not warmed from SQLite within 2s")
}
