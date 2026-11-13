package cache

import (
	"time"

	"github.com/dgraph-io/ristretto"
)

type MemoryCache struct {
	cache *ristretto.Cache
}

func NewMemoryCache(maxItems int64) (*MemoryCache, error) {
	c, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: maxItems * 10,
		MaxCost:     maxItems,
		BufferItems: 64,
	})
	if err != nil {
		return nil, err
	}

	return &MemoryCache{cache: c}, nil
}

func (m *MemoryCache) Get(rfid string) (Status, bool) {
	val, ok := m.cache.Get(rfid)
	if !ok {
		return StatusUnknown, false
	}
	return val.(Status), true
}

func (m *MemoryCache) Set(rfid string, status Status) error {
	m.cache.SetWithTTL(rfid, status, 1, 24*time.Hour)
	return nil
}

func (m *MemoryCache) Close() error {
	m.cache.Close()
	return nil
}

// Clear drops every entry. Used after a full mirror sync so a tag that was
// deleted in Koha cannot keep answering from memory until its TTL expires.
func (m *MemoryCache) Clear() {
	m.cache.Clear()
}
