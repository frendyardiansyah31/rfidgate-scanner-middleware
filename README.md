# RFID Edge Service Middleware

## Overview
The **RFID Edge Service Middleware** is a local Go-based API designed to act as an intermediary between RFID Gate Hardware (C#) and a Remote Server (Koha, via the RfidGateApi plugin) as well as Frontend Dashboards (Svelte/Web UI). 

This middleware ensures that the RFID scanning process remains fast, reliable, and capable of functioning even when the connection to Koha is lost (Offline Mode). It handles RFID validation, logging, and an offline mirror of loan status that is kept in step with Koha in the background.

## Key Features
- **Offline Support:** Uses a local SQLite database to cache valid RFID tags and store scan events when offline.
- **Fast Validation:** Validates RFID scans in milliseconds using an in-memory and SQLite caching layer.
- **Koha Mirror:** Every scan is looked up in Koha first; a local mirror of loan status (delta sync every minute, full sync every few hours) answers when Koha is unreachable. Titles and covers are mirrored the same way.
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
│   ├── sync/                 # Event pipeline
│   │   ├── event_sink.go          # EventSink interface (where published scan events go).
│   │   ├── noop_sync.go           # NoopSink: discards events (Koha needs no write-back).
│   │   ├── offilne_schema.sql     # Database schema for the offline event queue.
│   │   ├── offline_sqlite.go      # Implementation of offline storage in SQLite.
│   │   ├── offline_store.go       # Interfaces for offline storage.
│   │   ├── sqlite_history_repository.go # Adapters for history storage.
│   │   └── worker.go              # Background worker consuming the event queue.
│
├── .gitignore                # Git ignore rules.
├── apispec.yaml              # OpenAPI 3.0.3 specification for the REST API endpoints.
├── config.koha.example.yaml  # Config template (no secrets). Copy to config.yaml and fill in koha_api.*.
├── go.mod / go.sum           # Go module dependency tracking files.
└── main.go                   # Main entry point. Wires up dependencies and starts the HTTP server.
```

## How It Works
1. **Startup**: The application loads the configuration, initializes the `zap` logger and the local SQLite database, and starts the Koha clients.
2. **Mirror sync**: `mirror` warms the in-memory cache from SQLite, then syncs loan status from Koha (`GET /rfid/status`, delta and periodic full). The `biblio` syncer mirrors titles and covers (`GET /rfid/biblio`).
3. **RFID Scan**: A hardware reader hits `/rfid/check`. The `decision.Engine` asks Koha first (short timeout, no retry). If Koha is unreachable it falls back to the in-memory/SQLite mirror. A loaned item is allowed, a not-loaned item raises the alarm, and an unknown tag (wallet, clothes) is allowed with no trace.
4. **History**: A known tag's scan is saved to the local SQLite `gate_logs` table (for the dashboard) and pushed to the live SSE stream.
5. **Event queue**: The decision is also published to an in-memory queue consumed by a worker. Koha needs nothing pushed back, so the sink discards the events today.

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

The `deploy\` folder is recreated on every run and is git-ignored. `config.yaml` contains the Koha API key, so treat the folder as sensitive. Copy it to the target PC (e.g. `C:\rfid-middleware`) and run `install.bat` as Administrator. See `DEPLOYMENT_GUIDE.md` for details.

### 2. Configure

Edit `config.yaml` before installing the service:

```yaml
server:
  address: "127.0.0.1"
  port: 3000

sqlite:
  path: "edge.db"

koha_api:
  base_url: "https://<koha-host>/api/v1/contrib/rfid_gate"
  api_key: "<middleware API key>"

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
