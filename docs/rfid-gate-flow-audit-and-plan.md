# Audit Alur RFID Gate dan Rencana Perbaikan

Tanggal: 2026-11-11
Cakupan: gate C# (`RFD1861Demo/Form1.cs`) -> middleware Go -> Koha / SLiMS.
Status: hasil pembacaan kode dan pengecekan DB `koha_dev` (hanya `SELECT`/`EXPLAIN`). Belum ada kode yang diubah.

## 1. Kesimpulan alur

Alur utama sama dengan versi lama dan arah keputusan tidak terbalik.

| Tahap | Lama (C# langsung ke MySQL) | Sekarang (Go) | Hasil |
|---|---|---|---|
| Scan tag | `checklabel` | `POST /rfid/check` | sama |
| Dipinjam | `Logbuku(...,0)`, tanpa alarm | `decideFromStatus`: Loaned -> `DecisionAllow` (0) | sama |
| Tidak dipinjam | `playaudio()`, sts 1 | `DecisionAlarm` (1); C# alarm bila `decision==1` | sama |
| Tag tidak ada | return diam-diam | `applyUnknown`: Allow (0) | sama (tanpa alarm) |

Pemeriksaan tanda (apakah ada yang terbalik):
- `cache.StatusLoaned=0`, `NotLoaned=1`. `remoteStatusQuery` dan `PullDelta` memakai 0 = loaned, 1 = available. Konsisten.
- `usecase.Run` memetakan 0 ke Loaned dan 1 ke NotLoaned. Benar.
- `PullAll` memakai kode 1 dan 2 lalu memetakannya benar ke enum.
- JSON ke C# berbentuk `"decision":0,"reason":...` (key map di-sort Go). Parser string-search C# cocok.

## 2. Temuan di Koha (WSL Debian12, localhost:8001, DB `koha_dev`)

- Plugin: `/var/lib/koha/uiii-lib/plugins/Koha/Plugin/Com/Library/RfidGateApi/REST/V1/Gate.pm`.
- Endpoint Koha sudah menormalisasi `SN` di kedua sisi (`REPLACE(r.rfid,'SN','')`), sama dengan versi lama. Masalah 3 untuk jalur Koha sudah beres di server.
- Koha mengecek tabel `issues` (hanya pinjaman aktif), jadi masalah 4 tidak ada di jalur Koha. Di dev tidak ada item dengan lebih dari 1 baris `issues`.
- Data dev: `items` 34.713, `item_rfid` 3, `issues` 4, `plugin_rfid_gate_log` 16. Dari 3 tag, 2 berawalan `SN` (26 karakter) dan 1 tidak (9 karakter). Format campur, jadi normalisasi memang perlu.
- Angka waktu query (5-18 ms) tidak mewakili produksi karena tabel dev sangat kecil.
- `REPLACE()` pada kolom `rfid` membuat index tidak terpakai (scan seluruh tabel). Aman di dev, lambat bila `item_rfid` besar.
- Go memanggil Koha dua kali untuk tag yang tidak ketemu (mentah, lalu `"SN"+rfid`). Server sudah menormalisasi, jadi panggilan kedua sia-sia dan tercatat ganda di `plugin_rfid_gate_log`.
- Log Koha untuk tag tidak dikenal memakai `status=1` (Not Permitted), padahal gate memberi Allow. Semantik log terbalik dari kenyataan.
- Insert log berjalan sinkron sebelum response dikirim, menambah latensi di jalur scan.

## 3. Daftar masalah (urut keparahan)

1. **Mode Koha: mirror lokal tidak pernah terisi** (`main.go:110`). `cfg.Slims.Enabled && !useKoha` bernilai false, jadi `NoopSync` aktif; runner, delta sync, dan warm `rfid_status` tidak jalan. Saat Koha down atau lebih dari 400 ms, fallback ke SQLite kosong lalu Unknown lalu Allow. Buku tidak dipinjam lolos tanpa alarm.
2. **Fail-open di sisi gate.** Middleware mati, HTTP 400/500, atau response kosong membuat C# memberi decision 0 (Allow), tanpa alarm.
3. **Normalisasi `SN` tidak setara versi lama (jalur SLiMS dan mirror lokal).** Lama: `replace(...,'SN','')` di kedua sisi. Sekarang: coba tag mentah lalu `"SN"+rfid`. Gate kirim `SN123` sedangkan DB menyimpan `123` menghasilkan Unknown lalu Allow.
4. **Aturan "dipinjam" lebih longgar (jalur SLiMS).** Lama: ada baris loan mana pun dengan `is_lent=1 AND is_return=0`. Sekarang: hanya loan dengan `last_update` terbaru (`remote_lookup.go:27`, `PullDelta`). Loan lama yang masih terbuka plus loan baru yang sudah return menghasilkan alarm palsu atau lolos palsu. Dua loan dengan `last_update` sama memilih sembarang (`LIMIT 1`).
5. **Tag tidak dikenal ikut dicatat sebagai "dipinjam"** (`engine_impl.go:154`, `mapper.go`). `applyUnknown` memanggil `applyDecision(Allow)`, `ToCacheStatus` memetakannya ke `StatusLoaned`, sehingga `log_bukukeluar_test` menerima sts=0 untuk dompet/baju dan dashboard menampilkannya "Permitted" dengan judul kosong. Lama: tidak dicatat.
6. **Kecil:** tag tidak ketemu memanggil remote 2 kali dalam deadline 400 ms; `dto.go` `RFIDCheckResponse` tidak dipakai (kode mati); banyak blok komentar di `handler_rfid.go`; daftar masalah di CLAUDE.md soal `fmt.Println` sudah usang.

## 4. Rencana

### A. Samakan normalisasi SN (masalah 3 dan latensi)
Target: satu panggilan, satu query, normalisasi dilakukan sekali.

Koha (plugin):
1. Tambah kolom turunan `rfid_norm` di `item_rfid` (nilai `REPLACE(rfid,'SN','')`) dengan index. Lookup memakai `=` biasa.
2. Normalisasi input di Perl dengan aturan yang sama, cari `rfid_norm = input_norm`.
3. Pertahankan fallback barcode.
4. Catat log scan setelah response terkirim (tidak menahan jawaban ke gate).
5. Tag tidak dikenal: jangan dicatat, atau beri status sendiri (perlu keputusan).

Go:
1. Jalur Koha: panggil sekali dengan tag mentah, hapus retry `"SN"+rfid`.
2. Jalur SLiMS: query dengan normalisasi dua sisi seperti C# lama; tambah kolom ternormalisasi bila lambat.
3. Mirror lokal: simpan kunci ternormalisasi di `rfid_status`; engine cukup satu `Get`, tanpa fallback SN. Re-key data lama dan cek tabrakan dulu (dev: 0 duplikat).

### B. Samakan aturan "sedang dipinjam" (masalah 4)
- SLiMS: ganti `MAX(last_update)` dengan `EXISTS (loan is_lent=1 AND is_return=0)` di `remoteStatusQuery` dan `PullDelta` sekaligus.
- Koha: tidak perlu diubah.
- Verifikasi sebelum deploy: di data produksi hitung item dengan loan terbuka lebih dari satu dan bandingkan hasil aturan lama vs baru. Selisih harus 0 atau bisa dijelaskan.

### C. Isi mirror di mode Koha (masalah 1)
Tambah endpoint bulk di plugin Koha (status per `since`) dan panggil dari Go untuk mengisi `rfid_status`. Kerjakan bersama A karena kunci mirror ikut berubah.

### D. Bila middleware mati (masalah 2)

| Opsi | Kelebihan | Kekurangan |
|---|---|---|
| 1. C# hit DB Koha langsung | cepat | Kredensial DB di PC gate, rawan injection, terikat skema. Desain lama yang sengaja ditinggalkan. **Tolak.** |
| 2. C# panggil Koha API langsung | kecil dan aman | Logika keputusan ada di dua tempat; tidak jalan bila Koha juga down. |
| 3. Hanya middleware | satu tempat logika | Tanpa cadangan bila middleware mati. |

Rekomendasi: gabungan, middleware tetap jalur utama, tiga lapis:
1. **Cegah mati.** Windows Service restart otomatis saat gagal; `watchdog.ps1` aktif dan memberi notifikasi ke staf; tambah `recover` di goroutine (mis. `history.Insert`) karena panic di goroutine mematikan proses.
2. **Cadangan di C#** (hanya saat middleware mati). C# coba middleware dengan timeout pendek; bila gagal panggil Koha `/rfid/check` langsung dengan `X-API-Key` khusus gate (key terpisah, bisa dicabut). Terjemahan: BORROWED -> 0, NOT_BORROWED -> 1 (alarm), tidak ditemukan -> 0. Tambah circuit breaker: setelah gagal, lewati middleware selama N detik agar tiap scan tidak menunggu timeout.
3. **Bila keduanya mati: fail-open tapi tidak diam.** Fail-alarm membuat dompet/baju ikut alarm dan gate tidak bisa dipakai. Tetap Allow, tapi bunyikan nada peringatan "sistem down" yang berbeda dari alarm, tulis ke file log, dan kirim notifikasi ke staf lewat watchdog. Ini kebijakan bisnis dan perlu persetujuan.

### E. Lain-lain
- Tag tidak dikenal (masalah 5): jangan kirim ke queue dan `gate_logs`, seperti versi lama.
- Tes: tabel kasus SN (gate `SN+hex` vs DB `hex`, kebalikannya, keduanya `SN`, keduanya tanpa `SN`), loan ganda, tag tidak dikenal, Koha timeout, middleware mati lalu fallback C#.
- Urutan rilis: DB (kolom baru, belum dipakai) -> Koha plugin -> Go -> C#. Rollback tiap langkah dengan menonaktifkan kode barunya; kolom baru tidak merusak kode lama.

## 5. Keputusan yang masih terbuka

1. Setuju kebijakan "fail-open dengan peringatan" bila middleware dan Koha sama-sama mati?
2. Log Koha untuk tag tidak dikenal: dibuang atau diberi status sendiri?
3. Jalur SLiMS masih dipakai di produksi, atau semua sudah pindah ke Koha? Ini menentukan prioritas langkah B.
