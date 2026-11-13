package api

import (
	"database/sql"
	"io/fs"
	"net/http"

	"github.com/frendyardiansyah31/rfid-middleware/internal/biblio"
	"github.com/frendyardiansyah31/rfid-middleware/internal/decision"
	"github.com/frendyardiansyah31/rfid-middleware/internal/history"
	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
)

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func NewHTTPServer(
	addr string,
	engine decision.Engine,
	history history.Repository,
	logger logger.Logger,
	broker *SSEBroker,
	syncer *biblio.Syncer,
	sqliteDB *sql.DB,
	mysqlDB *sql.DB,
	libraryHealthURL string,
	webDir string,
	webFS fs.FS,
) *http.Server {

	handler := NewRFIDHandler(engine, logger)
	dashboard := NewDashboardHandler(history, broker)
	admin := NewAdminHandler(syncer)
	health := NewHealthHandler(sqliteDB, mysqlDB, syncer, logger, libraryHealthURL)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", corsMiddleware(health.Check))
	mux.HandleFunc("/rfid/check", corsMiddleware(handler.CheckRFID))
	mux.HandleFunc("/rfid/logs", corsMiddleware(dashboard.GetLogs))
	mux.HandleFunc("/rfid/logs/stream", corsMiddleware(dashboard.StreamLogs))
	mux.HandleFunc("/admin/sync", corsMiddleware(admin.TriggerSync))
	mux.HandleFunc("/admin/sync/status", corsMiddleware(admin.SyncStatus))
	mux.Handle("/", staticHandler(webDir, webFS))

	return &http.Server{
		Addr:    addr,
		Handler: mux,
	}
}
