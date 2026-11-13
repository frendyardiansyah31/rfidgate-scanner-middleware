# RFID Edge Service Middleware

## Overview
The **RFID Edge Service Middleware** is a local Go-based API designed to act as an intermediary between RFID Gate Hardware (C#) and a Remote Server (e.g., SLiMS via MySQL) as well as Frontend Dashboards (Svelte/Web UI). 

This middleware ensures that the RFID scanning process remains fast, reliable, and capable of functioning even when the connection to the main SLiMS database is lost (Offline Mode). It handles RFID validation, logging, queuing, background synchronization, and caching.

## Key Features
- **Offline Support:** Uses a local SQLite database to cache valid RFID tags and store scan events when offline.
- **Fast Validation:** Validates RFID scans in milliseconds using an in-memory and SQLite caching layer.
- **Background Synchronization:** Automatically synchronizes offline scan events to the central SLiMS MySQL database when the connection is restored.
- **Decision Engine:** Determines whether an RFID tag should trigger an alarm based on validation rules.
- **OpenAPI Compliant:** Exposes a documented REST API for hardware integration and dashboard monitoring.

## Project Structure & File Purposes

Below is the complete structure of the project and the purpose of each directory/file:

```text
c:\frendy-project\golang-rfid-middleware
│
├── cmd/
│   └── edge-service/         # Holds executable building scripts/configs if any.
│
├── internal/                 # Core application logic (not exposed as a library)
│   ├── api/                  # HTTP Delivery Layer
│   │   ├── dto.go                 # Data Transfer Objects for request/response bodies.
│   │   ├── handler_dashboard.go   # API endpoints for frontend dashboard (logs, status).
│   │   ├── handler_rfid.go        # Core API endpoint for RFID hardware validation.
│   │   └── http.go                # HTTP server setup and routing.
│   │
│   ├── cache/                # Caching Layer (Memory + SQLite)
│   │   ├── composite.go           # Combines multiple cache layers (Memory -> SQLite).
│   │   ├── interface.go           # Cache interface definitions.
│   │   ├── memory.go              # Fast in-memory cache implementation.
│   │   ├── sqlite.go              # Persistent local cache using SQLite.
│   │   ├── sqlite_config.go       # Configurations for SQLite connection.
│   │   └── sqlite_schema.sql      # Schema initialization for the local SQLite cache.
│   │
│   ├── config/               # Application Configuration
│   │   ├── config.go              # Go struct definitions and config loader logic.
│   │   └── config.yaml            # Main application configuration file (DB, ports, etc).
│   │
│   ├── decision/             # Business Logic / Rules Engine
│   │   ├── engine.go              # Interface for the decision engine.
│   │   ├── engine_impl.go         # Implementation: evaluates if RFID scan triggers alarm.
│   │   ├── mapper.go              # Maps logic outputs to API responses.
│   │   ├── reason.go              # Constants for decision reasons (e.g., LOAN_VALID, UNKNOWN).
│   │   ├── result.go              # Output structure of a decision.
│   │   ├── rule.go                # Interface/structs for individual validation rules.
│   │   └── *_test.go              # Unit tests for decision logic (including race conditions).
│   │
│   ├── domain/               # Domain Models
│   │   └── rfid.go                # Core entity definition for an RFID tag.
│   │
│   ├── helper/               # Utility functions
│   │   └── configure_sqlite.go    # Helper for generic SQLite configurations (PRAGMA).
│   │
│   ├── history/              # Local Scan History Logging
│   │   ├── model_history.go       # Data model for a historical scan log.
│   │   ├── repository.go          # Interface for history storage.
│   │   ├── repository_impl.go     # Base repository logic.
│   │   └── sqlite.go              # SQLite implementation of scan history storage.
│   │
│   ├── http/                 # HTTP Utilities
│   │   └── response/              
│   │       ├── response.go        # Standard API envelope response structures.
│   │       └── writer.go          # JSON response writer helpers.
│   │
│   ├── logger/               # Logging System
│   │   ├── logger.go              # Logging interface definitions.
│   │   └── zap_logger.go          # Implementation using Uber's Zap logger.
│   │
│   ├── queue/                # Event Queuing
│   │   └── event_queue.go         # In-memory queue for offline events waiting to be synced.
│   │
│   ├── repository/           # Data Access Layer
│   │   ├── rfid_repository.go     # Repository interface for RFID data.
│   │   └── sqlite/                
│   │       └── rfid_repository.go # SQLite specific RFID data queries.
│   │
│   ├── startup/              # Application Bootstrapping & Background Tasks
│   │   ├── delta_sync.go          # Periodic sync of latest updates from SLiMS.
│   │   ├── flush_offline.go       # Sends locally cached events to main server on startup.
│   │   ├── initial_pull.go        # Initial data download from SLiMS DB.
│   │   ├── offline_flush.go       # Dedicated logic for flushing offline queue.
│   │   ├── runner.go              # Orchestrates startup sync and periodic sync jobs.
│   │   ├── runner_test.go         # Tests for startup jobs.
│   │   └── warm_cache.go          # Pre-loads SQLite data into in-memory cache at boot.
│   │
│   ├── sync/                 # Synchronization Engine (Edge to Cloud/Main DB)
│   │   ├── constant.go            # Sync constants.
│   │   ├── health.go              # Database connectivity checks.
│   │   ├── mysql_state.go         # MySQL specific connection state tracking.
│   │   ├── mysql_sync.go          # Logic to write events to the SLiMS MySQL database.
│   │   ├── noop_sync.go           # Dummy sync for standalone/testing mode.
│   │   ├── offilne_schema.sql     # Database schema for the offline event queue.
│   │   ├── offline_sqlite.go      # Implementation of offline storage in SQLite.
│   │   ├── offline_store.go       # Interfaces for offline storage.
│   │   ├── retry.go               # Retry logic mechanism.
│   │   ├── retrying_sync.go       # Wrapper to add retry capabilities to sync operations.
│   │   ├── slims_pull.go          # Fetches delta/updates from SLiMS MySQL.
│   │   ├── slims_sync.go          # Interfaces defining SLiMS sync operations.
│   │   ├── sqlite_history_repository.go # Adapters for history sync.
│   │   └── worker.go              # Background worker thread consuming event queue to sync.
│   │
│   └── usecase/              # Application Use Cases
│       └── sync_rfid.go           # Orchestrates the process of syncing RFID data.
│
├── .gitignore                # Git ignore rules.
├── apispec.yaml              # OpenAPI 3.0.3 specification for the REST API endpoints.
├── config.yaml               # Runtime configuration parameters.
├── go.mod / go.sum           # Go module dependency tracking files.
└── main.go                   # Main entry point. Wires up dependencies and starts the HTTP server.
```

## How It Works
1. **Startup**: The application loads the configuration, initializes the `zap` logger, connects to local SQLite, and attempts to connect to the remote SLiMS MySQL server.
2. **Warmup & Sync**: `startup.Runner` executes. It runs a delta pull from SLiMS, loads RFID tags into the local SQLite cache, warms up the fast in-memory cache, and flushes any pending offline logs to the central server.
3. **RFID Scan**: A hardware reader hits `/rfid/check`. The `decision.Engine` checks the in-memory/SQLite cache. If the tag is valid, access is allowed. Otherwise, it commands the hardware to trigger an alarm.
4. **History & Queue**: The scan event is saved to the local SQLite `history` table (for the dashboard) and placed into an in-memory `queue`.
5. **Worker**: A background worker picks up the event from the queue and sends it to the central SLiMS database. If the network is down, the event goes into the `offline_store` (SQLite) and is retried later.

---

## Installation & Running (Windows 11)

### Prerequisites
- Go 1.21+ (for building from source)
- **PowerShell or Command Prompt run as Administrator** (required for service install/uninstall)

### 1. Build

```powershell
go build -o rfid-middleware.exe .
```

Place the resulting `rfid-middleware.exe` together with `config.yaml` and `edge.db` (if it already exists) in the same folder, for example:

```
C:\rfid-middleware\
├── rfid-middleware.exe
├── config.yaml
└── edge.db          ← created automatically on first run
```

> The executable always resolves `config.yaml` and `edge.db` relative to its own directory, so the service works correctly regardless of the current working directory.

#### Build a ready-to-copy deployment folder

`build-deploy.ps1` builds the binary and assembles everything needed for deployment into `.\deploy`:

```powershell
.\build-deploy.ps1                    # go build, then assemble deploy\
.\build-deploy.ps1 -IncludeWatchdog   # also include the watchdog scripts
.\build-deploy.ps1 -IncludeDb         # also ship the current edge.db
.\build-deploy.ps1 -SkipBuild         # reuse the existing rfid-middleware.exe
```

```
deploy\
├── rfid-middleware.exe
├── config.yaml
├── install.bat
├── uninstall.bat
├── DEPLOYMENT_GUIDE.md
├── web\dist\            ← dashboard frontend
└── watchdog\            ← only with -IncludeWatchdog
```

The `deploy\` folder is recreated on every run and is git-ignored. `config.yaml` contains the MySQL DSN, so treat the folder as sensitive. Copy it to the target PC (e.g. `C:\rfid-middleware`) and run `install.bat` as Administrator. See `DEPLOYMENT_GUIDE.md` for details.

### 2. Configure

Edit `config.yaml` before installing the service:

```yaml
server:
  address: "127.0.0.1"
  port: 3000

sqlite:
  path: "edge.db"

slims:
  mysql_dsn: "user:password@tcp(host:3306)/dbname?parseTime=true"
  enabled: true

log:
  level: "info"
```

### 3. Install as a Windows Service

Open **PowerShell as Administrator**, then run:

```powershell
cd C:\rfid-middleware
.\rfid-middleware.exe install
```

Expected output:
```
service install: OK
```

The service is registered under the name **RFIDMiddleware** and display name **RFID Edge Service Middleware**.

### 4. Start / Stop / Restart the Service

```powershell
# Start
.\rfid-middleware.exe start

# Stop
.\rfid-middleware.exe stop

# Restart
.\rfid-middleware.exe restart
```

Alternatively, use the Windows Services panel (`services.msc`) or `sc.exe`:

```powershell
sc.exe start RFIDMiddleware
sc.exe stop  RFIDMiddleware
```

### 5. Uninstall the Service

```powershell
.\rfid-middleware.exe stop
.\rfid-middleware.exe uninstall
```

### 6. Run Interactively (without installing as a service)

Simply double-click the `.exe` or run it directly from a terminal — no argument needed:

```powershell
.\rfid-middleware.exe
```

Press `Ctrl+C` to stop. This mode is useful for testing and development.

### 7. Verify the Service is Running

```powershell
sc.exe query RFIDMiddleware
```

Or check the API health endpoint:

```powershell
curl http://127.0.0.1:3000/health
```

### Service Management Summary

| Command | What it does |
|---|---|
| `rfid-middleware.exe install` | Register as a Windows service (requires Admin) |
| `rfid-middleware.exe uninstall` | Remove the Windows service (requires Admin) |
| `rfid-middleware.exe start` | Start the service via SCM |
| `rfid-middleware.exe stop` | Stop the service gracefully |
| `rfid-middleware.exe restart` | Restart the service |
| `rfid-middleware.exe` *(no args)* | Run interactively (Ctrl+C to stop) |

### Auto-start on Boot

After `install`, the service startup type is set to **Automatic** by default, so it starts automatically when Windows boots. To change it:

```powershell
sc.exe config RFIDMiddleware start= demand   # manual start
sc.exe config RFIDMiddleware start= auto     # automatic (default)
```
