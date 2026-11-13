# Code Quality Assessment — RFID Middleware

**Reviewer perspective:** Senior Software Engineer + Tech Architect  
**Date:** 2026-05-17  
**Project:** golang-rfid-middleware

---

## Scorecard

| Dimensi | Score | Grade |
|---|---|---|
| **Project Structure & Architecture** | 7.5 / 10 | B+ |
| **Code Readability** | 5.5 / 10 | C+ |
| **Error Handling** | 5.0 / 10 | C |
| **Abstraction & Interfaces** | 7.0 / 10 | B |
| **Concurrency Safety** | 6.0 / 10 | B- |
| **Configuration Management** | 4.0 / 10 | D+ |
| **Testing** | 6.5 / 10 | B- |
| **Logging & Observability** | 5.5 / 10 | C+ |
| **Security** | 3.5 / 10 | D |
| **Dependency Management** | 7.5 / 10 | B+ |
| **Overall** | **5.8 / 10** | **C+** |

---

## Detail per Dimensi

### 1. Project Structure — 7.5/10

Yang bagus: Layer separation jelas — `api/`, `cache/`, `decision/`, `sync/`, `queue/`, `repository/`. Dependency flow relatif clean, tidak ada circular import.

Yang kurang:
- `cmd/` directory ada tapi kosong — orphaned
- `nssm.exe` binary di-commit ke repo (bukan tempat binary dalam VCS)
- Sync logic bocor ke `main.go` (harusnya di use-case layer)
- Tidak ada `usecase/` atau `service/` layer yang memisahkan business orchestration

---

### 2. Code Readability — 5.5/10

Yang bagus: Naming cukup deskriptif (`CompositeCache`, `EngineImpl`, `RFIDSnapshot`). Fungsi mayoritas < 80 baris.

Yang kurang:
- `fmt.Println` debug masih di production code: `internal/decision/engine_impl.go:105,138`, `internal/queue/queue.go:43`
- Magic numbers: hardcoded `5*time.Second`, `1*time.Second`, `10*time.Second` tersebar di code
- File `zap_logger.go` dan `repository_impl.go` berisi blok kode yang di-comment out panjang — dead code harusnya dihapus
- Status mapping opaque: `0=Loaned, 1=NotLoaned` vs `0=Allow, 1=Alarm` — tanpa named constant yang jelas

---

### 3. Error Handling — 5.0/10

Yang bagus: Context timeout konsisten, retry dengan exponential backoff ada di `retry.go`.

Yang kurang:
- Panic recovery di `internal/decision/engine_impl.go:131-133` **menelan semua error secara diam-diam** — ini red flag besar
- Banyak `_ =` (ignored errors) di `composite.go`, `offline_sqlite.go`, `sync_rfid.go`
- Hampir tidak ada `fmt.Errorf("context: %w", err)` — error kehilangan stack context
- `log.Fatal` di `main.go:31,46` — tidak ada graceful degradation saat startup

---

### 4. Abstraction & Interfaces — 7.0/10

Yang bagus: Interface-driven untuk semua komponen utama. `cache/interface.go`, `decision.Engine`, `sync.SlimsSync`, `repository.RFIDRepository` — semua testable.

Yang kurang:
- `CompositeCache` menggunakan concrete `*SQLiteCache` bukan interface — melanggar prinsip yang sudah dibangun sendiri
- `cache.Status` return bare `int` bukan typed constant — abstraksi bocor
- Tidak ada factory/constructor pattern yang konsisten — DI dilakukan manual di `main.go` tapi tidak seragam

---

### 5. Concurrency Safety — 6.0/10

Yang bagus: Semaphore pattern untuk MySQL worker slots, channel-based communication, context cancellation propagation.

Yang kurang:
- History insert di `internal/decision/engine_impl.go:150-156` pakai `go func()` tanpa menunggu — tidak ada jaminan persist sebelum keputusan berikutnya
- Event queue drop silent saat overflow — tidak ada backpressure mechanism
- Goroutine leak risk: ticker loop di `main.go:141` tanpa `select` yang proper
- Belum pernah di-run dengan `-race` detector

---

### 6. Configuration Management — 4.0/10

Yang bagus: Viper dengan layered config (YAML → env vars → defaults). Defaults di-set untuk semua key.

Yang kurang (critical):
- **Credentials MySQL plaintext di `config.yaml:16`** — DSN dengan password hardcoded dan kemungkinan ter-commit ke repo
- TTL cache hardcoded di `internal/cache/memory.go:35`: `24*time.Hour` tidak bisa dikonfigurasi
- Sync interval `10*time.Second` hardcoded di `main.go:140`
- Tidak ada separation antara config file development vs production

---

### 7. Testing — 6.5/10

Yang bagus: Table-driven tests, property-based testing dengan `gopter`, test untuk panic safety. Coverage di decision engine cukup solid.

Yang kurang:
- **Zero test untuk HTTP handlers** — seluruh layer API tidak ter-cover
- Tidak ada integration test untuk full pipeline: sync → cache → decision → response
- Tidak ada test untuk history repository
- Build/test pipeline tidak terlihat ada (`Makefile` atau CI config tidak ditemukan)

---

### 8. Logging & Observability — 5.5/10

Yang bagus: Zap dengan rotation (50MB, 10 backup, 14 hari), JSON + console output, caller tracking.

Yang kurang:
- `fmt.Println` di production menbypass logger sepenuhnya
- Tidak ada **correlation ID** / request tracing — sulit debug issue di production
- Tidak ada metrics endpoint (Prometheus/expvar) — blind saat monitoring
- `GetRecent` di history repository tampaknya tidak digunakan oleh siapapun

---

### 9. Security — 3.5/10

Critical issues:
- **Credentials di config.yaml yang kemungkinan ter-commit** — immediate action required
- **No authentication/authorization** pada HTTP API — siapapun bisa hit endpoint RFID check
- Input validation RFID hanya cek empty string — tidak ada length limit, charset validation, atau rate limiting
- Tidak ada HTTPS/TLS configuration

Yang aman: Parameterized query konsisten dipakai — tidak ada SQL injection risk.

---

### 10. Dependency Management — 7.5/10

Yang bagus: Dependensi minimal dan well-maintained. Go version terbaru. Tidak ada bloat.

Catatan: `cenkalti/backoff` ada di `go.sum` tapi tidak di-import langsung — kemungkinan transitive dependency yang tidak perlu.

---

## Summary Untuk Architect

### Strengths (pertahankan)
- Interface-driven architecture yang solid
- Two-tier cache design (Ristretto + SQLite) tepat untuk use case
- Retry + backoff pattern yang benar
- Decision engine test coverage layak

### Critical Fixes (lakukan sekarang)
- Hapus credentials dari `config.yaml` → pindah ke env vars + `.env.example`
- Tambah API authentication minimal (API key header)
- Hapus semua `fmt.Println` dari production code
- Hapus dead code (commented-out blocks)

### Next Sprint
- Tambah correlation ID di request context
- Wrap semua error dengan context (`%w`)
- Test HTTP handler layer
- Pisahkan use-case orchestration dari `main.go`
- Run `go test -race` di CI

---

**Kesimpulan:** Project ini menunjukkan pemahaman arsitektur yang cukup baik untuk level junior-mid — interface design dan cache strategy adalah buktinya. Tapi ada beberapa pola yang tidak konsisten dan security gap yang perlu dibenahi sebelum layak disebut production-grade. Dengan 2-3 sprint focused refactoring, score ini bisa naik ke **7.5/10**.
