package api

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/biblio"
	"github.com/frendyardiansyah31/rfid-middleware/internal/http/response"
	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
)

type HealthHandler struct {
	sqliteDB     *sql.DB
	mysqlDB      *sql.DB // nil when SLiMS sync is disabled (cfg.Slims.Enabled = false)
	syncer       *biblio.Syncer
	log          logger.Logger
	healthURL    string // full library app probe URL (SLiMS ?p=api/health or Koha rfid_gate/health); "" = disabled
	httpClient   *http.Client
}

func NewHealthHandler(sqliteDB *sql.DB, mysqlDB *sql.DB, syncer *biblio.Syncer, log logger.Logger, healthURL string) *HealthHandler {
	return &HealthHandler{
		sqliteDB:     sqliteDB,
		mysqlDB:      mysqlDB,
		syncer:       syncer,
		log:          log,
		healthURL:    healthURL,
		httpClient:   &http.Client{Timeout: 3 * time.Second},
	}
}

type healthReport struct {
	Status                   string  `json:"status"` // "ok" | "degraded"
	SQLite                   string  `json:"sqlite"`
	SlimsMySQL               string  `json:"slims_mysql"`
	SlimsAPI                 string  `json:"slims_api"` // library app reachability (SLiMS or Koha, per library.backend); key kept for dashboard compatibility
	BiblioLastSyncMinutesAgo *int    `json:"biblio_last_sync_minutes_ago"`
	BiblioLastSyncError      *string `json:"biblio_last_sync_error"`
}

func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	report := healthReport{Status: "ok"}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.sqliteDB.PingContext(ctx); err != nil {
		report.SQLite = "error: " + err.Error()
		report.Status = "degraded"
		h.log.Error("health check: sqlite unreachable", logger.F("error", err))
	} else {
		report.SQLite = "ok"
	}

	if h.mysqlDB == nil {
		report.SlimsMySQL = "disabled"
	} else {
		mysqlCtx, mysqlCancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer mysqlCancel()

		if err := h.mysqlDB.PingContext(mysqlCtx); err != nil {
			report.SlimsMySQL = "unreachable: " + err.Error()
			report.Status = "degraded"
			h.log.Warn("health check: slims mysql unreachable", logger.F("error", err))
		} else {
			report.SlimsMySQL = "ok"
		}
	}

	if h.healthURL == "" {
		report.SlimsAPI = "disabled"
	} else {
		apiCtx, apiCancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer apiCancel()

		req, err := http.NewRequestWithContext(apiCtx, http.MethodGet, h.healthURL, nil)
		if err != nil {
			report.SlimsAPI = "error: " + err.Error()
			report.Status = "degraded"
		} else if resp, err := h.httpClient.Do(req); err != nil {
			report.SlimsAPI = "unreachable: " + err.Error()
			report.Status = "degraded"
			h.log.Warn("health check: slims api unreachable", logger.F("error", err))
		} else {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				report.SlimsAPI = "ok"
			} else {
				report.SlimsAPI = "unreachable: unexpected status " + resp.Status
				report.Status = "degraded"
			}
		}
	}

	if h.syncer != nil {
		status := h.syncer.Status()
		if status.LastSyncAt != nil {
			minutesAgo := int(time.Since(*status.LastSyncAt).Minutes())
			report.BiblioLastSyncMinutesAgo = &minutesAgo
		}
		report.BiblioLastSyncError = status.LastError
	}

	response.OK(w, "health check", report)
}
