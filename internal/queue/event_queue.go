package queue

import (
	"fmt"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
)

type Event struct {
	RFID   string
	Status cache.Status
	Time   time.Time
}

type EventQueue interface {
	Publish(rfid string, status cache.Status)
	Consume() <-chan Event
	Pending() int
}

type InMemoryQueue struct {
	ch        chan Event
	droppedAt time.Time
}

func NewInMemoryQueue(size int) *InMemoryQueue {
	return &InMemoryQueue{
		ch: make(chan Event, size),
	}
}

func (q *InMemoryQueue) Publish(rfid string, status cache.Status) {
	select {
	case q.ch <- Event{RFID: rfid, Status: status}:
	default:
		// drop event if queue full (never block decision)
		// rate-limited log (1x per 5 detik)
		now := time.Now()
		if now.Sub(q.droppedAt) > 5*time.Second {
			q.droppedAt = now
			// ⚠️ jangan block, jangan panic
			fmt.Println("[WARN] event queue full, dropping RFID events")
		}
	}
}

func (q *InMemoryQueue) Consume() <-chan Event {
	return q.ch
}

func (q *InMemoryQueue) Pending() int {
	return len(q.ch)
}
