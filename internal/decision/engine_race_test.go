package decision

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
	"github.com/frendyardiansyah31/rfid-middleware/internal/queue"
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

type raceSafeQueue struct {
	mu     sync.Mutex
	events []queue.Event
}

func newRaceSafeQueue() *raceSafeQueue {
	return &raceSafeQueue{}
}

func (q *raceSafeQueue) Publish(rfid string, status cache.Status) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.events = append(q.events, queue.Event{
		RFID:   rfid,
		Status: status,
	})
}

func (q *raceSafeQueue) Consume() <-chan queue.Event {
	return nil
}

func (q *raceSafeQueue) Count() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.events)
}

func (q *raceSafeQueue) Pending() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.events)
}

func TestDecisionEngine_RaceCondition(t *testing.T) {
	cacheRepo := newRaceSafeCache()
	queueRepo := newRaceSafeQueue()

	engine := NewEngine(cacheRepo, queueRepo, nil, 1*time.Second)

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

	if queueRepo.Count() != goroutines {
		t.Fatalf("expected %d events, got %d", goroutines, queueRepo.Count())
	}
}
