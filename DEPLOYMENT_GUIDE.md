# Deployment Guide — RFID Edge Service Middleware (Windows Service via kardianos/service)

Panduan install binary hasil build di PC baru (gate PC), menjalankannya sebagai Windows Service native lewat `kardianos/service` (bukan NSSM), dan cara membaca lognya.

## 1. Build

Di PC dev (atau langsung di PC target kalau ada Go toolchain terpasang):

```powershell
go build -o rfid-middleware.exe .
```

> Pastikan build dari kode yang sudah memuat fix `internal/logger/zap_logger.go` + `main.go`
> (logger sekarang menerima `exeDir` supaya folder `logs/` selalu dianchor ke folder exe,
> bukan ke current working directory proses). Tanpa fix ini, service native kardianos
> (CWD default = `C:\Windows\System32`) akan menulis log ke `C:\Windows\System32\logs\`,
> bukan ke folder instalasi.

### Build + susun folder deploy otomatis

Script `build-deploy.ps1` (root repo) menjalankan `go build` lalu mengumpulkan semua file
yang dibutuhkan ke folder `deploy\`:

```powershell
.\build-deploy.ps1                    # build + susun deploy\
.\build-deploy.ps1 -IncludeWatchdog   # sertakan script watchdog (tanpa state.json / watchdog.log)
.\build-deploy.ps1 -IncludeDb         # sertakan edge.db yang ada sekarang
.\build-deploy.ps1 -SkipBuild         # pakai rfid-middleware.exe yang sudah ada
```

Isi `deploy\`: `rfid-middleware.exe`, `config.yaml`, `install.bat`, `uninstall.bat`,
`DEPLOYMENT_GUIDE.md`, dan `web\dist\`. Folder dibuat ulang setiap script dijalankan dan
masuk `.gitignore`. Script berhenti dengan error kalau file wajib tidak ada atau build gagal.

Catatan:
- `go build` gagal menimpa `rfid-middleware.exe` kalau prosesnya sedang jalan — hentikan dulu
  atau pakai `-SkipBuild`.
- `config.yaml` berisi password MySQL di `mysql_dsn`; jaga folder `deploy\` baik-baik dan
  periksa isinya sebelum dibawa ke PC target.

Setelah itu copy seluruh isi `deploy\` ke PC target (langkah 2) dan lanjut ke langkah 3.

## 2. Copy ke PC target

Satu folder tetap berisi:

- `rfid-middleware.exe`
- `config.yaml` (lihat langkah 3)
- `install.bat` / `uninstall.bat` — wrapper otomatis untuk register/unregister service
  (stop proses lama dulu, tunggu sampai benar-benar mati, baru install/start — lihat langkah 4 & 7)
- `web\dist\` — halaman frontend dashboard (copy folder `web\dist` dari repo, **bukan** seluruh
  folder `web\`; `web\src` dan `web\tools` hanya untuk build CSS di PC dev). Lihat langkah 3
  untuk setting `web.dir`.
- `edge.db` — opsional, tidak wajib dibawa. Dibuat otomatis oleh `modernc.org/sqlite`
  saat pertama kali jalan, lalu di-backfill otomatis dari SLiMS MySQL (full pull ~1-2 menit
  di background, service tetap melayani request selama proses ini).

Struktur folder deploy:

```
C:\rfid-middleware\
├── rfid-middleware.exe
├── config.yaml
├── install.bat
├── uninstall.bat
├── edge.db              (dibuat otomatis kalau tidak dibawa)
├── logs\                (dibuat otomatis)
└── web\
    └── dist\
        ├── index.html
        ├── app.css
        ├── js\
        └── assets\
```

Taruh di folder tetap, misal `C:\rfid-middleware\`. Hindari Desktop/Downloads/folder yang
kena sinkronisasi OneDrive — bisa bikin permission/lock file aneh untuk SQLite.

> Dashboard juga sudah di-embed ke dalam `rfid-middleware.exe` saat `go build` (`web/embed.go`).
> Jadi tanpa folder `web\dist` pun dashboard tetap tampil, memakai salinan dari saat build.
> Folder `web\dist` di sisi exe dipakai kalau `web.dir` diisi: file dibaca dari disk, jadi
> tampilan bisa diubah (edit lalu refresh) tanpa build ulang exe.

## 3. Konfigurasi `config.yaml`

```yaml
server:
  address: "127.0.0.1"
  port: 3000        # Harus sama dengan URL di gate C# (Form1.cs: http://127.0.0.1:3000/rfid/check).
                     # Kalau muncul "bind: Only one usage of each socket address...",
                     # berarti port 3000 sudah dipakai proses lain (mis. Ingress Server) di mesin itu.

cache:
  memory_max_items: 100000
  memory_ttl_hours: 24

sqlite:
  path: "edge.db"    # relatif terhadap folder config.yaml, otomatis di-resolve absolute

queue:
  size: 1000

slims:
  mysql_dsn: "user:pass@tcp(host:3306)/db?parseTime=true"
  enabled: true       # set false kalau PC target belum/tidak punya akses ke MySQL SLiMS —
                       # service tetap jalan standalone/offline mode

log:
  level: "info"

slims_api:
  base_url: "https://catalogue.uiii.ac.id"
  token: ""
  sync_interval_hours: 1

web:
  dir: 'C:\rfid-middleware\web\dist'   # path ABSOLUT ke folder frontend dashboard.
                                        # Kosongkan ("") untuk memakai dashboard yang di-embed di exe.
```

> **Wajib path absolut untuk `web.dir`.** Berbeda dengan `sqlite.path`, nilai `web.dir` tidak
> di-resolve ke folder exe. Saat jalan sebagai service, CWD proses adalah `C:\Windows\System32`,
> jadi path relatif seperti `web/dist` akan mencari folder di tempat yang salah. Pakai tanda
> kutip tunggal di YAML supaya backslash tidak diinterpretasi sebagai escape.

Override tanpa edit file juga bisa lewat environment variable prefix `RFID_`
(Viper `AutomaticEnv` aktif), contoh: `RFID_SERVER_PORT=3000`.

## 4. Install & start service

**Cara cepat (disarankan):** klik kanan `install.bat` → **Run as administrator**.
Script ini otomatis: stop service/proses lama kalau ada (tunggu sampai proses benar-benar
mati, force-kill kalau macet), unregister service lama, `rfid-middleware.exe install`,
`net start RFIDMiddleware`, lalu cek status `RUNNING`. Log proses install tersimpan di
`install_log.txt` satu folder dengan exe.

**Cara manual:** jalankan CMD/PowerShell **as Administrator** (wajib untuk register service Windows):

```powershell
cd C:\rfid-middleware
.\rfid-middleware.exe install
.\rfid-middleware.exe start
```

Output `service start: OK` menandakan sukses.

Startup type default kardianos biasanya Manual. Kalau mau auto-start saat boot:

```powershell
Set-Service -Name RFIDMiddleware -StartupType Automatic
```

## 5. Verifikasi

```powershell
Get-Service RFIDMiddleware
curl http://127.0.0.1:3000/health
```

Response sehat:

```json
{"success":true,"code":"OK","message":"health check","data":{"status":"ok","sqlite":"ok","slims_mysql":"ok",...}}
```

Dashboard: buka `http://127.0.0.1:3000/` di browser PC gate. Kalau halaman kosong/404 dan
`web.dir` diisi, cek bahwa `index.html` benar-benar ada di folder yang ditunjuk.

## 6. Lokasi log & cara baca error

Log ada di **`C:\rfid-middleware\logs\rfid-middleware.log`** (folder exe + `logs/`),
format JSON satu baris per event, ditulis lewat Zap + rotasi lumberjack
(50MB/file, simpan 10 backup, retensi 14 hari, di-gzip).

Cek error/fatal terbaru:

```powershell
Get-Content .\logs\rfid-middleware.log -Tail 50 | Select-String '"level":"(error|fatal)"'
```

Level yang ada: `debug` (kalau `log.level: debug`), `info`, `warn`, `error`, `fatal`.

Data sync juga tercatat di log ini, contoh entri yang dicari:

- `"msg":"sync completed"` (field `processed`) — jumlah RFID/rfid_item yang di-delta-sync
- `"msg":"biblio sync completed"` (field `upserted`, `total_in_db`) — hasil full sync biblio dari SLiMS

## 7. Stop / uninstall

**Cara cepat:** klik kanan `uninstall.bat` → **Run as administrator**. Stop service,
unregister dari SCM, tunggu proses mati (force-kill kalau macet). `rfid-middleware.exe`,
`config.yaml`, `edge.db` **tidak dihapus** — cuma service-nya yang dicopot. Log di
`uninstall_log.txt`.

**Cara manual:**

```powershell
.\rfid-middleware.exe stop
.\rfid-middleware.exe uninstall
```

## 8. Ketahanan dan notifikasi Telegram

Tiga lapis supaya middleware jarang mati, cepat hidup lagi, dan staf tahu:

| Lapis | Menangani | Dipasang oleh |
|---|---|---|
| Pemulihan service Windows | proses **mati/crash** | `install.bat` (otomatis) |
| `heal.ps1` (task `RFIDGateHeal`, SYSTEM) | proses **hidup tapi hang** (`/health` tidak menjawab) | `register-watchdog-task.ps1` (admin) |
| `watchdog.ps1` (task `RFIDGateWatchdog`, user) | memberi tahu staf: layar + Telegram | `register-watchdog-task.ps1` |

**Pemulihan service.** `install.bat` mengatur auto-start dan restart otomatis: kegagalan ke-1 dan ke-2
restart setelah 5 detik, ke-3 dan seterusnya setelah 30 detik (jeda lebih panjang supaya middleware yang
tidak bisa start, misalnya port 3000 dipakai proses lain, tidak di-restart terus-menerus). Verifikasi dari
PowerShell **Administrator**:

```powershell
cd <folder deploy>\watchdog
powershell -ExecutionPolicy Bypass -File .\verify-service-recovery.ps1          # cek konfigurasi + uji crash
powershell -ExecutionPolicy Bypass -File .\verify-service-recovery.ps1 -SkipKill # hanya cek konfigurasi
```

**Telegram.**
1. Buat bot lewat `@BotFather` (perintah `/newbot`), simpan token.
2. Masukkan bot ke grup staf, kirim satu pesan, lalu buka
   `https://api.telegram.org/bot<TOKEN>/getUpdates` dan catat `chat.id` (grup biasanya diawali `-`).
3. Salin `watchdog\telegram.example.json` menjadi `watchdog\telegram.json`, isi `bot_token`, `chat_id`, dan
   `machine_name` (nama PC gate, ikut tampil di tiap pesan). **Jangan** di-commit; file ini sudah di-gitignore.
4. Daftarkan task (sekali, PowerShell **Administrator**):
   `powershell -ExecutionPolicy Bypass -File .\register-watchdog-task.ps1`
5. Uji tanpa menunggu gangguan sungguhan: `Start-ScheduledTask RFIDGateWatchdog`, lalu matikan service
   (`net stop RFIDMiddleware`), tunggu sekitar 3 menit; pesan `[DOWN]` harus masuk, dan `[PULIH]` setelah `net start`.

Perilaku pesan: `[DOWN]` untuk middleware, aplikasi reader, atau dashboard yang mati (middleware baru dinotifikasi
setelah 2 menit supaya restart otomatis tidak memicu alarm palsu); `[WARN]` bila Koha tidak terjangkau lebih dari
10 menit (gate tetap jalan dari mirror lokal); `[PULIH]` saat kembali normal; `[HEAL]` saat `heal.ps1` me-restart
middleware yang hang, atau bila batas 3 restart per jam tercapai. Pengingat diulang tiap 15 menit (Koha tiap 30 menit).
Tanpa `telegram.json`, Telegram otomatis nonaktif dan notifikasi di layar tetap berjalan.

### Aplikasi gate (C#): cadangan bila middleware mati

Aplikasi gate (`RFD1861Demo.exe`) memutuskan tiap scan berurutan: middleware lokal, lalu Koha langsung (bila
dikonfigurasi), lalu **fail-open** dengan nada peringatan bila keduanya tidak menjawab. Kode ada di `gate-csharp/`.

1. Di Koha: buka halaman konfigurasi plugin *RFID Gate API* dan tambahkan **API key khusus gate** (satu key per
   baris; jangan memakai key middleware, agar bisa dicabut sendiri).
2. Di PC gate: salin `gate-csharp\gate.ini.example` menjadi `gate.ini` **di samping `RFD1861Demo.exe`**, lalu isi
   `KohaBaseUrl` dan `KohaApiKey`. Tanpa keduanya, fallback Koha mati dan gate langsung fail-open bila
   middleware mati. `gate.ini` berisi key; jangan di-commit atau dibagikan.
3. Berkas status `%ProgramData%\RFIDGate\gate-status.txt` ditulis otomatis; `watchdog.ps1` membacanya dan
   mengirim `[DOWN] Gate Tanpa Pengecekan` bila gate sedang meloloskan buku tanpa pengecekan.

Waktu yang perlu diketahui: scan pertama saat gangguan dimulai bisa tertahan 1 sampai 2,5 detik (timeout
middleware + Koha), lalu scan berikutnya langsung selama 10 detik (`BreakerSeconds`). Di Windows, koneksi ke
port tertutup baru dilaporkan "ditolak" setelah sekitar 2 detik, jadi timeout yang membatasi.

## Catatan tambahan

- Instalasi ini tidak butuh NSSM — kardianos native `install`/`start` sudah cukup
  setelah fix logger di atas. Deployment lama (PC ini) pakai NSSM karena `AppDirectory`
  NSSM di-set eksplisit ke folder project, jadi log sebelumnya kebetulan tetap benar
  meskipun logger belum di-anchor ke `exeDir`.
- `config.yaml` dan `edge.db` selalu dicari relatif ke folder exe (`config.Load` menambahkan
  `exeDir` sebagai search path Viper), jadi tidak terpengaruh isu CWD di atas — hanya
  file log yang tadinya rawan salah folder.
