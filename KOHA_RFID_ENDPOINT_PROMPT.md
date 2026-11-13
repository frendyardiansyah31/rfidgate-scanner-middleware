# Tugas: Cek & Buat Endpoint RFID Gate di Koha Dev Server (WSL Debian)

## Konteks

Kami sedang migrasi backend perpustakaan dari **SLiMS** ke **Koha**. Ada Go middleware RFID gate (`golang-rfid-middleware`, di Windows) yang saat ini cek status pinjam buku ke SLiMS. Di SLiMS, kebutuhan ini dilayani oleh plugin custom REST API. Kamu bertugas memastikan **Koha dev server di WSL Debian ini** punya endpoint yang setara. Kalau belum ada, buatkan.

Referensi implementasi SLiMS (baca langsung dari WSL):

```
/mnt/c/laragon/www/slims-dev/plugins/custom_rest_api/
  custom_rest_api.plugin.php        # daftar route
  controllers/RfidController.php    # POST /rfid/check   ← PALING PENTING
  controllers/HealthController.php  # GET  /health
  controllers/LogGateController.php # GET  /gate/history
  loggate_apispec.yaml              # spec OpenAPI /gate/history
```

Baca semua file itu dulu sebelum mulai. Ringkasan kontraknya ada di bawah; kalau ada yang beda antara ringkasan ini dan kode aslinya, **kode asli yang benar**.

## Kontrak endpoint yang harus disamakan (dari SLiMS)

### 1. `POST /rfid/check` (wajib)

- Auth: header `X-API-Key: <key>` atau `Authorization: Bearer <key>`, dicocokkan ke daftar API key yang dikonfigurasi. Kalau tidak ada key yang dikonfigurasi: tolak (fail closed).
- Request: `{"rfid_tag": "SN0038029C1301943003F6F36A"}`
- Resolusi item: cocokkan tag ke field RFID item. Prefix `SN` di-strip di kedua sisi (SLiMS: `REPLACE(label_code,'SN','') = REPLACE(?,'SN','')`), dengan fallback cocok ke kode item (`item_code`, padanannya di Koha kemungkinan `barcode`).
- "Sedang dipinjam" = ada loan aktif (SLiMS: `is_lent = 1 AND is_return = 0`). Di Koha padanannya kemungkinan ada baris di tabel `issues` untuk `itemnumber` tersebut (atau `items.onloan IS NOT NULL`). Verifikasi mana yang benar di skema Koha ini.
- Response sukses (HTTP 200):
  - `{"status": "BORROWED", "rfid_tag": "<tag>"}`
  - `{"status": "NOT_BORROWED", "rfid_tag": "<tag>"}`
  - `{"status": "ITEM_NOT_FOUND", "rfid_tag": "<tag>"}`
- Response error: `{"status": "UNAUTHORIZED"|"INVALID_REQUEST"|"INTERNAL_ERROR", "message": "..."}` dengan HTTP 401 / 400 / 500.
- Side effect: catat setiap scan ke tabel log gate (`tgl`, `label_code` = tag mentah yang di-scan, `sts`: `0` = BORROWED/Permitted, `1` = selain itu/Not Permitted). Ini **best-effort**: kalau insert log gagal, response check tetap jalan normal.
- Tag di log harus di-mask (4 karakter awal + 4 akhir saja).

### 2. `GET /health` (wajib)

- Tanpa auth. Read-only, tanpa side effect.
- DB OK: HTTP 200 `{"status":"ok"}`. DB gagal: HTTP 503 `{"status":"error"}`.

### 3. `GET /gate/history` (wajib, setelah 1 dan 2 jalan)

- Query params: `page` (default 1), `limit` (default 20, clamp 1–200), `itemCode`, `title` (multi-kata, AND LIKE per kata), `status` (0/1), `startDate`, `untilDate` (format `Y-m-d`; kalau invalid pakai default `2000-01-01` dan hari ini).
- Response: `{"error": false, "message": "Gate history retrieved", "data": {"total", "page", "limit", "total_pages", "records": [{"date", "item_code", "label_code", "title", "status", "status_label"}]}}`, dengan `status_label` = `Permitted` / `Not Permitted`.
- Di SLiMS sumber datanya view `vw_log_gate` (join log gate + item + biblio). Di Koha: join tabel log gate milik plugin dengan `items` + `biblio`.

`/biblio/all` dan `/visitor/*` **di luar scope**. Jangan dibuat.

## Langkah kerja

### Fase 1: Investigasi (read-only, jangan ubah apa pun dulu)

1. Identifikasi jenis instalasi Koha: paket Debian (`koha-common`, cek `koha-list`, `/etc/koha/sites/*/koha-conf.xml`), koha-testing-docker, atau git checkout. Catat versi Koha.
2. Cari nama instance, cara akses DB (`koha-mysql <instance>`), URL OPAC/staff/API, dan apakah REST API aktif (syspref `RESTBasicAuth`, `RESTOAuth2ClientCredentials`, `RESTPublicAPI`).
3. Cek apakah endpoint RFID sudah ada:
   - plugin yang terpasang (`plugins_dir` di koha-conf.xml, tabel `plugin_data` / `plugin_methods`);
   - route plugin di `/api/v1/contrib/...` (cek spec di `GET /api/v1/.html` atau `/api/v1/`);
   - grep `rfid`, `gate`, `label_code` di direktori plugin dan source Koha.
4. Cari **di field mana tag RFID disimpan** di Koha (paling krusial). Cek `items.barcode`, field item lain, atau subfield MARC 952 lain. Ambil sampel data. Kalau data item di dev server kosong atau tidak jelas, **berhenti dan tanya saya**, jangan menebak.
5. Laporkan temuan Fase 1 secara ringkas sebelum lanjut ke Fase 2: versi, instance, field RFID, endpoint yang sudah ada.

### Fase 2: Buat (hanya kalau endpoint belum ada)

- Implementasikan sebagai **Koha plugin** (`Koha::Plugin::...`, pakai `api_routes` + `api_namespace`, controller Mojolicious, spec OpenAPI JSON). Route akan berada di `/api/v1/contrib/<namespace>/rfid/check`, dst. **Jangan modifikasi source Koha core.**
- Buat tabel log gate milik plugin di method `install()` (nama tabel ikut konvensi plugin Koha), lengkap dengan `uninstall()`.
- API key disimpan di konfigurasi plugin (`store_data` / halaman `configure`), bukan di-hardcode di kode.
- Auth: Koha REST punya mekanisme auth sendiri (Basic/OAuth2). Verifikasi apakah route plugin tanpa `x-koha-authorization` bisa diakses anonim di versi ini. Pilih desain yang membuat Go middleware cukup kirim `X-API-Key` seperti di SLiMS. Kalau tidak memungkinkan, jelaskan opsinya dan tanya saya.
- Logika "sedang dipinjam" harus memakai sumber yang sama dengan sirkulasi Koha (`issues` / `Koha::Checkouts` / `Koha::Items`), jangan definisi baru.
- Pakai query parameterized atau ORM Koha (`Koha::Items->search`, dsb.). Jangan interpolasi string ke SQL.
- Ikuti gaya kode plugin Koha yang umum (lihat kpz plugin contoh `koha-plugin-kitchen-sink` sebagai acuan struktur kalau perlu).

### Fase 3: Verifikasi

Tes dengan `curl` dari dalam WSL:

1. `GET /health` → 200 `{"status":"ok"}`
2. `POST /rfid/check` tanpa key → 401; body bukan JSON → 400; `rfid_tag` kosong → 400
3. Tag item yang sedang dipinjam → `BORROWED`; tag item tersedia → `NOT_BORROWED`; tag ngawur → `ITEM_NOT_FOUND`
4. Tag dengan dan tanpa prefix `SN` → hasil sama
5. Setelah scan, baris log bertambah, lalu muncul di `GET /gate/history` dengan filter `itemCode`, `status`, dan tanggal

Kalau data dev tidak ada item yang sedang dipinjam, buat checkout test lewat staff interface atau API Koha, lalu sebutkan di laporan apa yang kamu buat.

## Batasan

- Ini dev server, tapi tetap: **jangan hapus atau ubah data item/biblio/patron yang sudah ada**. Data test yang kamu buat, catat semua.
- Jangan sentuh `golang-rfid-middleware` atau dashboard Electron. Tugas ini hanya sisi Koha.
- Jangan restart service selain yang perlu untuk reload plugin (mis. `koha-plack --restart <instance>`), dan sebutkan kalau kamu melakukannya.

## Output akhir yang saya minta

1. Temuan Fase 1: versi Koha, instance, field RFID, endpoint yang sudah ada atau belum.
2. Daftar file plugin yang dibuat beserta path-nya.
3. **Tabel mapping endpoint final** SLiMS → Koha: method, URL lengkap, header auth, contoh request dan response. Ini yang akan dipakai untuk update Go middleware.
4. Hasil curl tiap skenario di Fase 3 (lulus/gagal, respons singkat).
5. Hal yang belum beres atau butuh keputusan saya.
