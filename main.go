package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/frendyardiansyah31/rfid-middleware/internal/api"
	"github.com/frendyardiansyah31/rfid-middleware/internal/biblio"
	"github.com/frendyardiansyah31/rfid-middleware/internal/cache"
	"github.com/frendyardiansyah31/rfid-middleware/internal/config"
	"github.com/frendyardiansyah31/rfid-middleware/internal/decision"
	"github.com/frendyardiansyah31/rfid-middleware/internal/history"
	"github.com/frendyardiansyah31/rfid-middleware/internal/koha"
	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
	"github.com/frendyardiansyah31/rfid-middleware/internal/mirror"
	"github.com/frendyardiansyah31/rfid-middleware/internal/queue"
	"github.com/frendyardiansyah31/rfid-middleware/internal/repository/sqlite"
	"github.com/frendyardiansyah31/rfid-middleware/internal/serverinfo"
	"github.com/frendyardiansyah31/rfid-middleware/internal/startup"
	"github.com/frendyardiansyah31/rfid-middleware/internal/sync"
	"github.com/frendyardiansyah31/rfid-middleware/internal/usecase"
	"github.com/frendyardiansyah31/rfid-middleware/web"
	_ "github.com/go-sql-driver/mysql"
	"github.com/kardianos/service"
)

const (
	svcName        = "RFIDMiddleware"
	svcDisplayName = "RFID Edge Service Middleware"
	svcDescription = "RFID gate middleware with SLiMS sync and offline support"
)

type program struct {
	cancel context.CancelFunc
	done   chan struct{}
	exeDir string
}

func (p *program) Start(s service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})
	go func() {
		defer close(p.done)
		run(ctx, p.exeDir)
	}()
	return nil
}

func (p *program) Stop(s service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	select {
	case <-p.done:
	case <-time.After(15 * time.Second):
	}
	return nil
}

func run(ctx context.Context, exeDir string) {
	cfg, err := config.Load(exeDir)
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	log, err := logger.New(cfg.Log.Level, exeDir)
	if err != nil {
		panic("failed to init logger: " + err.Error())
	}

	log.Info("rfid middleware starting")

	memCache, err := cache.NewMemoryCache(cfg.Cache.MemoryMaxItems)
	if err != nil {
		log.Fatal("failed to init memory cache", logger.F("error", err))
	}

	sqliteCache, err := cache.NewSQLiteCache(cfg.SQLite.Path)
	if err != nil {
		log.Fatal("failed to init sqlite cache", logger.F("error", err))
	}
	offlineStore := sync.NewSQLiteOfflineStore(sqliteCache.DB())
	cacheRepo := cache.NewCompositeCache(memCache, sqliteCache)

	eventQueue := queue.NewInMemoryQueue(cfg.Queue.Size)

	historyRepo := history.NewSQLiteRepository(sqliteCache.DB())

	biblioDB, err := biblio.NewSQLiteRepository(sqliteCache.DB())
	if err != nil {
		log.Fatal("failed to init biblio repo", logger.F("error", err))
	}

	broker := api.NewSSEBroker()
	notifyingRepo := history.NewNotifyingRepository(historyRepo, broker)
	enrichingRepo := history.NewEnrichingRepository(notifyingRepo, biblio.NewSQLiteEnricher(sqliteCache.DB(), biblioDB))

	useKoha := cfg.Library.Backend == "koha"

	var slimsSync sync.SlimsSync
	var db *sql.DB

	if cfg.Slims.Enabled && !useKoha {
		db, err = sql.Open("mysql", cfg.Slims.MySQLDSN)
		if err != nil {
			log.Warn("mysql disabled, fallback to offline mode", logger.F("error", err))
			slimsSync = sync.NewNoopSync()
		} else {
			db.SetMaxOpenConns(25)
			db.SetMaxIdleConns(25)
			db.SetConnMaxLifetime(5 * time.Minute)

			log.Info("mysql client initialized, sync will attempt to connect in background")

			base := sync.NewMySQLSync(db, log)
			slimsSync = sync.NewRetryingSync(base)

			rfidRepo, err := sqlite.NewRFIDRepository(sqliteCache.DB())
			if err != nil {
				log.Fatal("failed to init rfid repo", logger.F("error", err))
			}

			syncUseCase := usecase.NewSyncRFIDUseCase(slimsSync, rfidRepo, cacheRepo, log)

			runner := startup.NewRunner(cacheRepo, slimsSync, offlineStore, syncUseCase, log)

			startupCtx, startupCancel := context.WithTimeout(context.Background(), 5*time.Minute)

			go func() {
				defer startupCancel()
				if err := runner.Run(startupCtx); err != nil {
					log.Error("startup recovery failed", logger.F("error", err))
				}
			}()

			go func() {
				time.Sleep(1 * time.Minute)

				interval := 1 * time.Minute
				ticker := time.NewTicker(interval)
				defer ticker.Stop()

				log.Info("periodic sync started", logger.F("interval", interval))

				for {
					select {
					case <-ctx.Done():
						log.Info("periodic sync stopped")
						return
					case <-ticker.C:
						// Use the run context so an in-flight PullDelta unwinds on
						// shutdown instead of parking in sync.Retry forever.
						if err := runner.PerformDeltaSync(ctx); err != nil {
							log.Error("periodic sync failed", logger.F("error", err))
						}
					}
				}
			}()
		}
	} else {
		log.Info("mysql sync is disabled, running in standalone mode", logger.F("library_backend", cfg.Library.Backend))
		slimsSync = sync.NewNoopSync()
	}

	// Request-time authoritative lookup. When SLiMS MySQL is wired the engine
	// hits it first on every /rfid/check; otherwise it goes straight to the
	// local memory/SQLite mirror.
	var remoteLookup decision.RemoteLookup
	var kohaClient *koha.RemoteLookup
	if useKoha {
		if cfg.KohaAPI.BaseURL == "" || cfg.KohaAPI.APIKey == "" {
			// Not fatal: the gate keeps answering (unknown tags are allowed), but nothing can be
			// looked up or synced until both are set. Say so plainly instead of leaving only the
			// obscure per-request "unsupported protocol scheme" errors.
			log.Warn("koha backend selected but koha_api.base_url and/or koha_api.api_key are empty in config.yaml: no lookups or syncs will work until they are set",
				logger.F("base_url_set", cfg.KohaAPI.BaseURL != ""),
				logger.F("api_key_set", cfg.KohaAPI.APIKey != ""),
			)
		}
		kohaClient = koha.NewRemoteLookup(cfg.KohaAPI.BaseURL, cfg.KohaAPI.APIKey)
		remoteLookup = kohaClient
		log.Info("request-time remote lookup enabled (koha rfid_gate api)", logger.F("base_url", cfg.KohaAPI.BaseURL))

		// Offline mirror: keeps rfid_status/rfid_item in step with Koha so a scan
		// can still be decided when Koha is down or slow.
		mir, err := mirror.New(sqliteCache.DB(), cacheRepo, kohaClient, log, mirror.Config{
			DeltaEvery: time.Duration(cfg.KohaAPI.SyncIntervalSeconds) * time.Second,
			FullEvery:  time.Duration(cfg.KohaAPI.FullSyncHours) * time.Hour,
			Overlap:    time.Duration(cfg.KohaAPI.OverlapMinutes) * time.Minute,
		})
		if err != nil {
			log.Fatal("failed to init mirror", logger.F("error", err))
		}
		mir.Start(ctx)
	} else if db != nil {
		remoteLookup = sync.NewMySQLRemoteLookup(db, log)
		log.Info("request-time remote lookup enabled (slims mysql)")
	} else {
		log.Info("request-time remote lookup disabled, using local mirror only")
	}

	engine := decision.NewEngineWithRemote(
		remoteLookup,
		cacheRepo,
		eventQueue,
		enrichingRepo,
		1*time.Second,
	)

	worker := sync.NewWorker(eventQueue, slimsSync, offlineStore, log)
	worker.Start(ctx)

	var biblioSyncer *biblio.Syncer
	if useKoha {
		// Title and cover metadata from the Koha plugin (GET /rfid/biblio): delta
		// syncs plus a periodic full sync that prunes deleted items.
		biblioSyncer = biblio.NewDeltaSyncer(biblio.NewKohaFetcher(kohaClient), biblioDB, log, biblio.DeltaConfig{
			DeltaEvery: time.Duration(cfg.KohaAPI.BiblioSyncIntervalMinutes) * time.Minute,
			FullEvery:  time.Duration(cfg.KohaAPI.BiblioFullSyncHours) * time.Hour,
			Overlap:    time.Duration(cfg.KohaAPI.OverlapMinutes) * time.Minute,
		})
	} else {
		var biblioFetcher biblio.Fetcher
		if db != nil {
			biblioFetcher = biblio.NewMySQLFetcher(db)
		}
		biblioSyncer = biblio.NewSyncer(biblioFetcher, biblioDB, log)
	}
	biblioSyncer.Start(ctx, cfg.SlimsAPI.SyncIntervalHours)

	// Library app reachability probe shown on the dashboard. JSON key stays
	// "slims_api" for dashboard compatibility regardless of backend.
	libraryHealthURL := ""
	if useKoha {
		if cfg.KohaAPI.BaseURL != "" {
			libraryHealthURL = strings.TrimRight(cfg.KohaAPI.BaseURL, "/") + "/health"
		}
	} else if cfg.SlimsAPI.BaseURL != "" {
		libraryHealthURL = cfg.SlimsAPI.BaseURL + "/?p=api/health"
	}

	server := api.NewHTTPServer(
		fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.Port),
		engine,
		enrichingRepo,
		log,
		broker,
		biblioSyncer,
		sqliteCache.DB(),
		db,
		libraryHealthURL,
		cfg.Web.Dir,
		web.FS(),
	)
	// Bind before announcing, so the banner/file never advertises a port we failed to get.
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		log.Fatal("cannot listen on port (already in use?)",
			logger.F("addr", server.Addr),
			logger.F("error", err),
		)
	}

	logPath := filepath.Join("logs", "rfid-middleware.log")
	if exeDir != "" {
		logPath = filepath.Join(exeDir, logPath)
	}
	info := serverinfo.Info{
		Addr:    server.Addr,
		Backend: cfg.Library.Backend,
		Config:  filepath.Dir(cfg.SQLite.Path), // config.yaml and edge.db live together
		LogFile: logPath,
		Running: true,
	}
	log.Info("http server listening",
		logger.F("addr", server.Addr),
		logger.F("dashboard", info.DashboardURL()),
		logger.F("info_file", filepath.Join(info.Config, serverinfo.FileName)),
	)
	if service.Interactive() {
		serverinfo.PrintBanner(os.Stdout, info)
	}
	if err := serverinfo.WriteFile(info.Config, info); err != nil {
		log.Warn("could not write server info file", logger.F("error", err))
	}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatal("http server error", logger.F("error", err))
		}
	}()

	// Block until context is cancelled (service Stop or Ctrl+C via interactive run)
	<-ctx.Done()
	log.Info("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("http server shutdown error", logger.F("error", err))
	}

	info.Running = false
	_ = serverinfo.WriteFile(info.Config, info)

	time.Sleep(500 * time.Millisecond)

	pendingCount := eventQueue.Pending()
	if pendingCount > 0 {
		log.Info("draining pending events to offline store", logger.F("count", pendingCount))
		for eventQueue.Pending() > 0 {
			select {
			case ev := <-eventQueue.Consume():
				if err := offlineStore.Save(context.Background(), ev); err != nil {
					log.Error("failed to save pending event",
						logger.F("rfid", ev.RFID),
						logger.F("error", err),
					)
				}
			default:
			}
		}
		log.Info("queue drained successfully")
	} else {
		log.Info("no pending events in queue")
	}

	if db != nil {
		log.Info("closing mysql connection")
		_ = db.Close()
	}

	log.Info("closing sqlite connection")
	sqliteCache.Close()

	log.Info("rfid middleware stopped")
}

func main() {
	// Determine the directory of the real executable so config.yaml and edge.db
	// are found when the binary runs as a Windows service (cwd = System32) or
	// from an arbitrary directory. go run is not affected because exeDir is only
	// added as an extra search path, not used as the working directory.
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}

	svcConfig := &service.Config{
		Name:        svcName,
		DisplayName: svcDisplayName,
		Description: svcDescription,
	}

	prg := &program{exeDir: exeDir}
	svc, err := service.New(prg, svcConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create service: %v\n", err)
		os.Exit(1)
	}

	// Handle service control commands: install | uninstall | start | stop | restart
	if len(os.Args) > 1 {
		action := os.Args[1]
		switch action {
		case "install", "uninstall", "start", "stop", "restart":
			if err := service.Control(svc, action); err != nil {
				fmt.Fprintf(os.Stderr, "service %s failed: %v\n", action, err)
				os.Exit(1)
			}
			fmt.Printf("service %s: OK\n", action)
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown command %q\n", action)
			fmt.Fprintf(os.Stderr, "usage: %s [install|uninstall|start|stop|restart]\n", os.Args[0])
			os.Exit(1)
		}
	}

	if err := svc.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "service run error: %v\n", err)
		os.Exit(1)
	}
}
