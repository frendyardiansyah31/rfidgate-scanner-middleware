package api

import (
	"sync"

	"github.com/frendyardiansyah31/rfid-middleware/internal/history"
)

type SSEBroker struct {
	mu      sync.Mutex
	clients map[chan history.HistoryLog]struct{}
}

func NewSSEBroker() *SSEBroker {
	return &SSEBroker{clients: make(map[chan history.HistoryLog]struct{})}
}

func (b *SSEBroker) Notify(log history.HistoryLog) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- log:
		default:
		}
	}
}

func (b *SSEBroker) subscribe() chan history.HistoryLog {
	ch := make(chan history.HistoryLog, 8)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *SSEBroker) unsubscribe(ch chan history.HistoryLog) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
}
