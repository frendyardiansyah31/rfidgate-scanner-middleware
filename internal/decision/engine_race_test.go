package decision

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
)

type raceSafeCache struct {
	mu   sync.RWMutex
	data map[string]cache.Status
}

func newRaceSafeCache() *raceSafeCache {
	return &raceSafeCache{
		data: make(map[string]cache.Status),
	}
}

func (c *raceSafeCache) Get(rfid string) (cache.Status, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.data[rfid]
	return val, ok
}

func (c *raceSafeCache) Set(rfid string, status cache.Status) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[rfid] = status
	return nil
}

func (c *raceSafeCache) Close() error { return nil }

func TestDecisionEngine_RaceCondition(t *testing.T) {
	cacheRepo := newRaceSafeCache()

	engine := NewEngine(cacheRepo, nil, 1*time.Second)

	// preload cache
	cacheRepo.Set("RFID-ALLOW", cache.StatusLoaned)
	cacheRepo.Set("RFID-ALARM", cache.StatusNotLoaned)

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()

			if i%2 == 0 {
				engine.CheckRFID(context.Background(), "RFID-ALLOW")
			} else {
				engine.CheckRFID(context.Background(), "RFID-ALARM")
			}
		}(i)
	}

	wg.Wait()
}
