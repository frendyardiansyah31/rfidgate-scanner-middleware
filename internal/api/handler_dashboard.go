package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/history"
	"github.com/frendyardiansyah31/rfid-middleware/internal/http/response"
)

type DashboardHandler struct {
	history history.Repository
	broker  *SSEBroker
}

func NewDashboardHandler(history history.Repository, broker *SSEBroker) *DashboardHandler {
	return &DashboardHandler{history: history, broker: broker}
}

func (h *DashboardHandler) StreamLogs(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := h.broker.subscribe()
	defer h.broker.unsubscribe(ch)

	// Flush headers now so EventSource.onopen fires before the first scan.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case log, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(log)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (h *DashboardHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := h.history.GetRecent(50)
	if err != nil {
		response.Fail(
			w,
			http.StatusInternalServerError,
			"DB_ERROR",
			"failed to fetch logs",
		)
		return
	}

	response.OK(w, "success", logs)
}
