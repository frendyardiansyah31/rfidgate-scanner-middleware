package cache

import "time"

type CompositeCache struct {
	memory Cache
	sqlite *SQLiteCache
}

func NewCompositeCache(
	memory Cache,
	sqlite *SQLiteCache,
) *CompositeCache {
	return &CompositeCache{
		memory: memory,
		sqlite: sqlite,
	}
}

func (c *CompositeCache) Get(rfid string) (Status, bool) {
	// 1️⃣ Try memory
	if status, ok := c.memory.Get(rfid); ok {
		return status, true
	}

	// 2️⃣ Fallback to sqlite
	status, ok := c.sqlite.Get(rfid)
	if !ok {
		return StatusUnknown, false
	}

	// 3️⃣ Warm-up memory
	_ = c.memory.Set(rfid, status)

	return status, true
}

// func (c *CompositeCache) Set(rfid string, status Status) error {
// 	// write-through
// 	if err := c.memory.Set(rfid, status); err != nil {
// 		return err
// 	}
// 	return c.sqlite.Set(rfid, status)
// }
func (c *CompositeCache) Set(rfid string, status Status) error {
	if err := c.sqlite.Set(rfid, status); err != nil {
		return err
	}
	if err := c.memory.Set(rfid, status); err != nil {
		return err
	}
	return nil
}

func (c *CompositeCache) Close() error {
	_ = c.memory.Close()
	return c.sqlite.Close()
}

func (c *CompositeCache) SQLiteEmpty() bool {
	return c.sqlite.IsEmpty()
}

func (c *CompositeCache) LastUpdated() (time.Time, error) {
	return c.sqlite.LastUpdated()
}

func (c *CompositeCache) SetSQLite(rfid string, status Status) error {
	return c.sqlite.Set(rfid, status)
}

func (c *CompositeCache) SetMemory(rfid string, status Status) error {
	return c.memory.Set(rfid, status)
}

func (c *CompositeCache) AllFromSQLite() (map[string]Status, error) {
	return c.sqlite.LoadAll()
}

func (c *CompositeCache) SQLiteCount() (int, error) {
	return c.sqlite.Count()
}

// ClearMemory empties the in-memory tier if it supports it (SQLite is untouched).
func (c *CompositeCache) ClearMemory() {
	if m, ok := c.memory.(interface{ Clear() }); ok {
		m.Clear()
	}
}
