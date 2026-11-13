package api

import (
	"context"
	"net/http"

	"github.com/frendyardiansyah31/rfid-middleware/internal/biblio"
	"github.com/frendyardiansyah31/rfid-middleware/internal/http/response"
)

type AdminHandler struct {
	syncer *biblio.Syncer
}

func NewAdminHandler(syncer *biblio.Syncer) *AdminHandler {
	return &AdminHandler{syncer: syncer}
}

func (h *AdminHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if h.syncer.Status().IsSyncing {
		response.OK(w, "already running", map[string]string{"status": "already_running"})
		return
	}
	go h.syncer.RunOnce(context.Background())
	response.OK(w, "started", map[string]string{"status": "started"})
}

func (h *AdminHandler) SyncStatus(w http.ResponseWriter, r *http.Request) {
	response.OK(w, "ok", h.syncer.Status())
}
