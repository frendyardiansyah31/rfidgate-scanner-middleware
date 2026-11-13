package decision

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
)

type fakeCache struct {
	data map[string]cache.Status
}

func newFakeCache() *fakeCache {
	return &fakeCache{
		data: make(map[string]cache.Status),
	}
}

func (f *fakeCache) Get(rfid string) (cache.Status, bool) {
	val, ok := f.data[rfid]
	return val, ok
}

func (f *fakeCache) Set(rfid string, status cache.Status) error {
	f.data[rfid] = status
	return nil
}

func (f *fakeCache) Close() error {
	return nil
}

func TestCheckRFID_CacheMiss_ShouldAlarm(t *testing.T) {
	cacheRepo := newFakeCache()
	engine := NewEngine(cacheRepo, nil, 1*time.Second)

	result := engine.CheckRFID(context.Background(), "RFID12345")

	// Implementation returns Allow for unknown RFID
	if result.Decision != DecisionAllow {
		t.Fatalf("expected allow, got %d", result.Decision)
	}

	// rfid_status is ground truth from SLiMS sync — CheckRFID must never write to it
	if _, ok := cacheRepo.Get("RFID12345"); ok {
		t.Fatalf("cache must stay untouched for unknown rfid, but an entry was created")
	}
}

func TestCheckRFID_Loaned_ShouldAllow(t *testing.T) {
	cacheRepo := newFakeCache()
	cacheRepo.Set("RFID123", cache.StatusLoaned)

	engine := NewEngine(cacheRepo, nil, 1*time.Second)

	result := engine.CheckRFID(context.Background(), "RFID123")

	if result.Decision != DecisionAllow {
		t.Fatalf("expected allow, got %d", result.Decision)
	}

	// Ground truth must stay exactly as seeded — CheckRFID is read-only against the cache
	if status, _ := cacheRepo.Get("RFID123"); status != cache.StatusLoaned {
		t.Fatalf("expected cache to remain StatusLoaned, got %v", status)
	}
}

func TestCheckRFID_Returned_ShouldAlarm(t *testing.T) {
	cacheRepo := newFakeCache()
	cacheRepo.Set("RFID123", cache.StatusNotLoaned)

	engine := NewEngine(cacheRepo, nil, 1*time.Second)

	result := engine.CheckRFID(context.Background(), "RFID123")

	if result.Decision != DecisionAlarm {
		t.Fatalf("expected alarm, got %d", result.Decision)
	}
}

func TestEngine_CheckRFID_TableDriven(t *testing.T) {
	cacheRepo := newFakeCache()
	engine := NewEngine(cacheRepo, nil, 1*time.Second)

	tests := []struct {
		name       string
		setupCache func()
		rfid       string
		want       int
	}{
		{
			name:       "Cache miss -> allow",
			setupCache: func() {},
			rfid:       "RFID001",
			want:       DecisionAllow,
		},

		{
			name: "Loaned -> allow",
			setupCache: func() {
				cacheRepo.Set("RFID002", cache.StatusLoaned)
			},
			rfid:       "RFID002",
			want:       DecisionAllow,
		},
		{
			name: "Not loaned -> alarm",
			setupCache: func() {
				cacheRepo.Set("RFID003", cache.StatusNotLoaned)
			},
			rfid:       "RFID003",
			want:       DecisionAlarm,
		},
		{
			name: "Unknown status in cache -> alarm",
			setupCache: func() {
				cacheRepo.Set("RFID004", cache.StatusUnknown)
			},
			rfid:       "RFID004",
			want:       DecisionAlarm,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// setup cache
			tt.setupCache()

			cacheBefore, hadEntry := cacheRepo.Get(tt.rfid)

			got := engine.CheckRFID(context.Background(), tt.rfid)
			if got.Decision != tt.want {
				t.Fatalf("got %d, want %d", got.Decision, tt.want)
			}

			// Ground truth (rfid_status) must never be mutated by a decision
			cacheAfter, hasEntry := cacheRepo.Get(tt.rfid)
			if hasEntry != hadEntry || cacheAfter != cacheBefore {
				t.Fatalf("cache must stay untouched: before=(%v,%v) after=(%v,%v)",
					cacheBefore, hadEntry, cacheAfter, hasEntry)
			}
		})
	}
}

func TestCheckRFID_ContextCancelled(t *testing.T) {
	cacheRepo := newFakeCache()
	engine := NewEngine(cacheRepo, nil, 1*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := engine.CheckRFID(ctx, "RFID-CANCEL")

	if result.Decision != DecisionAlarm {
		t.Fatalf("expected alarm on cancelled context, got %d", result.Decision)
	}

}

func TestCheckRFID_CacheExpired_ShouldAlarm(t *testing.T) {
	cacheRepo := newFakeCache()
	engine := NewEngine(cacheRepo, nil, 100*time.Millisecond)

	cacheRepo.Set("RFID-EXPIRE", cache.StatusLoaned)

	time.Sleep(200 * time.Millisecond)

	result := engine.CheckRFID(context.Background(), "RFID-EXPIRE")

	// Fake cache doesn't have expiry logic, so it returns the item (Loaned) -> Allow
	if result.Decision != DecisionAllow {
		t.Fatalf("expected allow for valid cache item, got %d", result.Decision)
	}
}

func TestCheckRFID_DuplicateBurst(t *testing.T) {
	cacheRepo := newFakeCache()
	cacheRepo.Set("RFID-SPAM", cache.StatusNotLoaned)
	engine := NewEngine(cacheRepo, nil, 1*time.Second)

	for i := 0; i < 10; i++ {
		if got := engine.CheckRFID(context.Background(), "RFID-SPAM"); got.Decision != DecisionAlarm {
			t.Fatalf("scan %d: expected alarm, got %d", i, got.Decision)
		}
	}
}

type errorCache struct{}

func (e *errorCache) Get(string) (cache.Status, bool) {
	return cache.StatusUnknown, false
}
func (e *errorCache) Set(string, cache.Status) error {
	return errors.New("cache write failed")
}
func (e *errorCache) Close() error { return nil }

func TestCheckRFID_CacheSetError(t *testing.T) {
	cacheRepo := &errorCache{}
	engine := NewEngine(cacheRepo, nil, 1*time.Second)

	result := engine.CheckRFID(context.Background(), "RFID-ERR")

	if result.Decision != DecisionAllow {
		t.Fatalf("expected allow when cache fails")
	}
}

func TestCheckRFID_InvalidRFID(t *testing.T) {
	cacheRepo := newFakeCache()
	engine := NewEngine(cacheRepo, nil, 1*time.Second)

	tests := []string{"", "   ", "\n"}

	for _, rfid := range tests {
		result := engine.CheckRFID(context.Background(), rfid)
		if result.Decision != DecisionAllow {
			t.Fatalf("invalid rfid '%s' should allow", rfid)
		}
	}
}

// Regression guard for the cache-poisoning bug: a scan with the raw (non-"SN") rfid used
// to write its decision back into the cache under the raw key via applyDecision. That
// created a permanent duplicate entry which then shadowed the real "SN"-prefixed record
// (the ground truth synced from SLiMS) on every future lookup for that tag, freezing the
// decision at whatever was computed the very first time — even after the real loan status
// changed. applyDecision must never write to the cache at all.
func TestCheckRFID_DoesNotCreateRawKeyEntry_WhenMatchedViaSNFallback(t *testing.T) {
	cacheRepo := newFakeCache()
	engine := NewEngine(cacheRepo, nil, 1*time.Second)

	const plainRFID = "0038029C1301943003F71EB3"
	const snRFID = "SN" + plainRFID

	// The mirror is keyed by the normalised tag (no "SN"), as the Koha sync stores it.
	cacheRepo.Set(plainRFID, cache.StatusNotLoaned)

	// The gate may send either form; both must resolve to the same record.
	for _, scanned := range []string{plainRFID, snRFID} {
		if result := engine.CheckRFID(context.Background(), scanned); result.Decision != DecisionAlarm {
			t.Fatalf("scan %q: expected alarm (record is NotLoaned), got %d", scanned, result.Decision)
		}
	}

	// A scan must never write the scanned form back into the cache.
	if _, ok := cacheRepo.Get(snRFID); ok {
		t.Fatalf("SN-prefixed cache entry must NOT be created — this is exactly the poisoning bug")
	}

	// A second scan must reflect updated ground truth.
	cacheRepo.Set(plainRFID, cache.StatusLoaned) // simulate the mirror sync catching up
	for _, scanned := range []string{plainRFID, snRFID} {
		if result := engine.CheckRFID(context.Background(), scanned); result.Decision != DecisionAllow {
			t.Fatalf("scan %q: expected allow after sync updated the record, got %d", scanned, result.Decision)
		}
	}
}
