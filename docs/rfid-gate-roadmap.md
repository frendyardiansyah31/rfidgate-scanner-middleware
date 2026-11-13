# Roadmap Perbaikan RFID Gate (per Fase + Checklist)

Dasar: [rfid-gate-flow-audit-and-plan.md](rfid-gate-flow-audit-and-plan.md).
Cara pakai: centang `[x]` saat selesai. Satu fase dianggap selesai bila semua kriteria "Selesai bila" terpenuhi.

## Keputusan yang sudah diambil

| # | Keputusan |
|---|---|
| 1 | Bila middleware dan Koha sama-sama mati: **fail-open dengan peringatan** (tetap Allow, bunyi peringatan beda dari alarm, tulis log, notifikasi staf). |
| 2 | Tag tidak dikenal: **tidak dicatat** (tidak di log Koha, tidak di `gate_logs`, tidak dikirim ke queue), seperti versi lama. |
| 3 | **SLiMS deprecated.** Backend hanya Koha. Jalur SLiMS dihapus, bukan diperbaiki (masalah 4 jadi tidak relevan). |
| 4 | Arsitektur: **Go middleware tetap pengambil keputusan utama.** C# hanya ditambah fallback tipis. Full C# + SQLite tidak dipilih (gate .NET 2.0, kegagalan menular, kode decompile). |
| 5 | Notifikasi staf lewat **Telegram** (bot). |
| 6 | Data biblio disinkronkan dari **endpoint baru di Koha** ke SQLite, dengan field seminimal mungkin. |
| 7 | Fase opsional (file cache C#, modernisasi gate) **dihapus** dari roadmap; tidak perlu saat ini. |

## Urutan rilis

DB/Koha dulu, lalu Go, lalu C#. Setiap langkah bisa di-rollback dengan menonaktifkan kode barunya. Kolom DB baru tidak merusak kode lama.

```
Fase 0 -> Fase 1 (Koha) -> Fase 2 (Go) -> Fase 3A (mirror status) -> Fase 3B (biblio) -> Fase 4 (ketahanan) -> Fase 5 (C#) -> Fase 6 (bersih-bersih)
```

---

## Fase 0 - Persiapan dan baseline

**Goal:** tahu kondisi data produksi dan punya titik kembali sebelum mengubah apa pun.

- [x] Buat branch kerja dan commit kondisi sekarang: branch `chore/gate-koha-roadmap`, commit `c5cf95a` (tanpa `edge.db`).
- [x] Backup `edge.db` dan tabel RFID `koha_dev` ke `C:\frendy-project\backups\golang-rfid-middleware\2026-11-11\` (`edge.db` terverifikasi checksum, `koha_dev_rfid_tables.sql` berisi `item_rfid` dan `plugin_rfid_gate_log`).
- [ ] Backup DB Koha **produksi** (belum ada akses dari sini; lakukan sebelum Fase 1 menyentuh produksi).
- [x] Hitung tag di **dev**: total 3, berawalan `SN` 2, tanpa `SN` 1, `SN` di tengah 0.
- [ ] Hitung hal yang sama di **produksi** Koha.
- [x] Duplikat setelah normalisasi di **dev**: 0.
- [ ] Duplikat setelah normalisasi di **produksi** (harus 0; bila tidak, putuskan cara menanganinya sebelum Fase 1).
- [x] Baseline `/rfid/check` Koha **dev** sebelum perubahan (30 panggilan, 3 baris `item_rfid`): tag dikenal p50 12 ms / p95 18 ms; tag tidak dikenal p50 11 ms / p95 15 ms. Panggilan pertama setelah idle 0,9-2,2 detik (lihat catatan di bawah). Baseline produksi menyusul.
- [x] `go test ./...` hijau sebelum mulai (paket biblio, cache, decision, koha, startup lulus; paket lain tidak punya tes).

Catatan Fase 0:
- **Cold start plack:** Starman berjalan dengan `--max-requests 50 --workers 2`, jadi worker di-recycle tiap 50 request. Panggilan pertama setelah idle atau recycle memakan 0,9-2,2 detik, jauh di atas batas remote 400 ms di Go. Saat itu Go jatuh ke mirror lokal. Perlu dipertimbangkan di Fase 3/4: naikkan `--max-requests`, tambah worker, atau keep-warm lewat ping `/health`.
- Middleware tidak sedang berjalan di mesin ini (port 3000 dan 9000 tidak merespons), jadi tidak ada baseline sisi Go.
- Tag dev hanya 3 baris, jadi angka dev tidak mewakili produksi.
- `config.yaml`, `deploy/config.yaml`, dan `internal/config/config.yaml` berisi `mysql_dsn` SLiMS lengkap dengan kredensial dan sudah tercatat di git. Karena SLiMS deprecated, hapus dan rotasi kredensialnya di Fase 6.
- `edge.db` tetap tercatat di git dan berubah tiap middleware jalan. Pertimbangkan `git rm --cached` dan masukkan ke `.gitignore`.

**Selesai bila:** angka baseline tercatat, backup ada, tidak ada duplikat normalisasi (atau sudah ada rencana menanganinya).

---

## Fase 1 - Plugin Koha: normalisasi cepat dan log bersih

**Goal:** satu panggilan ke Koha cukup untuk semua format tag, cepat, dan tidak mengotori log.
File: `/var/lib/koha/uiii-lib/plugins/Koha/Plugin/Com/Library/RfidGateApi/REST/V1/Gate.pm` (di WSL Debian12).

Status: **selesai di Koha dev** (2026-11-11); produksi menyusul. Plugin sudah di-rebuild sebagai `RfidGateApi-1.0.4.kpz` (aturan `plugins/CLAUDE.md` Koha: naik versi tiap rebuild, uji instansiasi dua kali, restart plack). Migrasi `rfid_norm` kini jalan otomatis dari hook `install`/`upgrade`, jadi cukup upload `.kpz` di produksi. Salinan ada di `koha-plugin/` (`RfidGateApi-1.0.4.kpz`, `RfidGateApi.pm`, `Gate.pm`, `phase1_migration.sql` sebagai referensi manual). Backup `Gate.pm` asli di dev: `Gate.pm.bak-20261111`.

- [x] Tambah kolom turunan `rfid_norm` (STORED, indexed) di `item_rfid`. Migrasi idempotent dan rollback ada di `phase1_migration.sql`. Diukur di skala 34.713 baris (tabel sementara): query lama `REPLACE()` median 37 ms, kolom + index median 0,6 ms.
- [x] Normalisasi input di Perl (`s/SN//g`, sama dengan `REPLACE`), lookup `rfid_norm = input_norm`. Bila kolom hilang (mis. `InventoryRfid.pm` membuat ulang tabel), `Gate.pm` otomatis memakai query `REPLACE()` lama; cek ulang tiap 60 detik.
- [x] Fallback ke barcode dipertahankan.
- [x] Tag tidak dikenal tidak lagi di-insert ke `plugin_rfid_gate_log` (terbukti: 6 panggilan tak dikenal, 0 baris baru).
- [-] ~~Insert log setelah response terkirim~~ **Dibatalkan:** satu `INSERT` ringan (kurang dari 1 ms) dan Starman/PSGI tidak memberi keuntungan nyata dari menunda; risiko lebih besar dari manfaat.
- [ ] Buat API key khusus gate untuk fallback C# (Fase 5). Dibuat lewat halaman konfigurasi plugin (satu key per baris), bukan lewat skrip, supaya nilainya tidak lewat log. **Dikerjakan oleh Anda saat Fase 5.**
- [x] Tes 4 kombinasi SN lewat curl di dev: DB `SN` kirim `SN`, DB `SN` kirim tanpa `SN`, DB polos kirim polos, DB polos kirim `SN`+polos. Semua benar dengan satu panggilan. Tag tidak dikenal: `ITEM_NOT_FOUND`. Tag sedang dipinjam (baris uji sementara, sudah dihapus): `BORROWED` untuk kedua bentuk.
- [x] Waktu respons dev setelah perubahan: tag dikenal p50 11 ms / p95 14 ms; tidak dikenal p50 9 ms / p95 13 ms (tidak lebih lambat dari baseline).
- [ ] Ukur ulang dan terapkan di **produksi** (bukan dev): backup, cek duplikat normalisasi, upload `RfidGateApi-1.0.4.kpz` lewat Koha (migrasi otomatis), restart plack, lalu ukur.
- [x] Pembersihan dev: baris `plugin_rfid_gate_log` buatan tes dihapus (kembali 16 baris), `item_rfid` tetap 3 baris.

**Selesai bila (dev: terpenuhi):** keempat kombinasi SN menghasilkan hasil benar dengan satu panggilan, tag tidak dikenal tidak menambah baris log, dan waktu respons tidak lebih lambat dari baseline.

---

## Fase 2 - Go: satu panggilan, tag tidak dikenal dibuang

**Goal:** jalur scan di Go sederhana, cepat, dan bersih dari efek samping yang salah.

Status: **selesai** (2026-11-11), diuji ujung ke ujung terhadap Koha dev.

- [x] `engine_impl.go`: `remoteLookup` memanggil Koha **sekali** dengan tag mentah; retry `"SN"+rfid` dihapus (server Koha sudah menormalisasi).
- [x] `applyUnknown` tidak lagi memanggil `applyDecision`: tidak publish ke queue, tidak insert `gate_logs`. Tetap Allow (0), reason `UNKNOWN_RFID`, signal false.
- [x] Butir "pisahkan `ToCacheStatus`" **tidak perlu**: masalahnya (Allow dibaca sebagai "loaned" untuk tag tidak dikenal) hilang karena tag tidak dikenal tidak lagi lewat `applyDecision`. `ToCacheStatus` kini hanya dipakai untuk keputusan atas buku yang dikenal.
- [x] `recover` ditambahkan di goroutine `history.Insert`.
- [x] Tes diperbarui (`engine_test.go`, `engine_remote_test.go`, `engine_property_extended_test.go`) dan ditambah: tag tidak dikenal tidak meninggalkan jejak di queue maupun history, Koha dipanggil tepat sekali dengan tag mentah, tag dikenal tetap tercatat, panic di history tidak mematikan proses.
- [x] Format JSON ke C# tidak berubah: `"decision":<int>` tetap, diverifikasi dari body asli.
- [x] `go vet ./...` bersih dan `go test ./...` hijau. `-race` tidak bisa dijalankan di mesin ini (butuh cgo/gcc); jalankan di mesin build bila tersedia.
- [x] Uji ujung ke ujung (middleware asli, config sementara di luar repo, Koha dev): tag SN tidak dipinjam menghasilkan decision 1 + signal true; tag yang sama tanpa `SN` menghasilkan decision 1; tag tidak dikenal menghasilkan decision 0 `UNKNOWN_RFID`. Log plugin Koha bertambah 2 (dua scan dikenal, nol untuk tidak dikenal). `gate_logs` middleware hanya berisi dua scan dikenal.

Catatan:
- Jalur lokal (saat Koha down) masih mencoba kunci `"SN"+rfid` ke cache. Itu sengaja dibiarkan sampai Fase 3 mengganti kunci mirror dengan bentuk ternormalisasi.
- `ToCacheStatus` memetakan Allow ke `StatusLoaned` dan Alarm ke `StatusNotLoaned`. Ini untuk event queue; untuk keputusan Alarm karena timeout (bukan "tidak dipinjam") statusnya tetap tercatat sebagai NotLoaned. Perlu ditinjau bila queue dipakai untuk pelaporan.

**Selesai bila:** tes hijau, scan tag dikenal tetap menghasilkan log dan SSE, scan tag tidak dikenal tidak meninggalkan jejak di `gate_logs` maupun Koha.

---

## Fase 3A - Mirror status offline terisi di mode Koha

**Goal:** saat Koha down atau lambat, keputusan tetap benar dari SQLite lokal (menutup masalah 1: sekarang mirror kosong lalu fail-open).
Bagian dari Fase 3 yang dipecah dua. Fase 3B (biblio) tidak bergantung pada 3A dan bisa dikerjakan terpisah.

Status: **selesai di Koha dev** (2026-11-11); butir produksi dan cold start masih terbuka.

**Plugin Koha** (`RfidGateApi-1.0.5.kpz`, endpoint `GET /rfid/status`)
- [x] Endpoint bulk dan delta: `since` (delta) atau tanpa `since` (full), isi `rfid_norm` + `item_code` + status (`BORROWED`/`NOT_BORROWED`), paginasi keyset (`after`, `limit` maksimal 5000), `server_time` dari jam Koha, `total_rfid` untuk audit, `X-API-Key`. Tidak dicatat di `plugin_rfid_gate_log`.
- [x] Delta memakai `items.timestamp` (ber-index, ikut berubah saat pinjam/kembali/perpanjang/edit) ditambah `item_rfid.assigned_at`. Diverifikasi dari data dev: pinjam 4 dari 4, kembali 29 dari 30. **Satu anomali** (returndate 141 detik lebih baru dari `items.timestamp`), sehingga delta tidak 100% presisi. Mitigasi di Go: jendela tumpang tindih 15 menit, dan sync penuh tiap 6 jam.
- [x] Keterbatasan yang diketahui: item atau tag yang **dihapus** di Koha tidak terlihat oleh delta; baru hilang dari mirror pada sync penuh berikutnya (maksimal 6 jam).
- [x] Tes beban di skala 34.713 baris (data sintetis, sudah dihapus): sync penuh 7 halaman, 3,2 MB, 1,7 detik; delta 15-21 ms, 231 byte.

**Go** (paket baru `internal/mirror`, `internal/rfidtag`, `internal/koha/status.go`)
- [x] Pull baru untuk Koha menggantikan `NoopSync` di mode Koha; mengisi `rfid_status` dan `rfid_item` (judul tidak ditimpa, milik sync biblio 3B). Sync pertama selalu penuh; berikutnya delta tiap 60 detik; penuh lagi bila terakhir lebih dari 6 jam. Interval bisa diatur lewat `koha_api.sync_interval_seconds`, `full_sync_hours`, `overlap_minutes`.
- [x] Kunci mirror ternormalisasi lewat satu fungsi (`rfidtag.Normalize`, sama dengan `REPLACE(...,'SN','')`) dipakai engine, enricher, dan mirror. Fallback `"SN"+rfid` di jalur lokal dan di enricher dihapus.
- [x] Migrasi data lama: tidak perlu skrip terpisah. Sync penuh pertama menghapus baris lama yang kuncinya berawalan `SN` (hasil sync SLiMS). Diuji dengan salinan `edge.db` produksi lama (40.198 baris): sesudah sync penuh tersisa 3 baris sesuai Koha dev.
- [x] Pengaman sync penuh: ditolak (mirror tidak diubah) bila hasil kosong, kurang dari 50% isi mirror, atau tidak cocok dengan `total_rfid` Koha. Baris lama berawalan `SN` tidak dihitung agar tidak menghalangi sync pertama.
- [x] Startup: memori dihangatkan dari SQLite, lalu sync langsung jalan. Setelah sync penuh, memori dikosongkan dan diisi ulang agar tag yang sudah dihapus tidak menjawab dari memori.
- [x] Audit selisih jumlah mirror vs Koha: dicatat sebagai peringatan bila selisih lebih dari 1% (minimal 10).

**Ketahanan terkait**
- [ ] Mitigasi cold start plack (temuan Fase 0: worker dingin 0,9-2,2 detik, melebihi batas remote 400 ms): naikkan `--max-requests`, tambah worker, atau keep-warm lewat ping `/health`. Belum dikerjakan; dampaknya sekarang lebih kecil karena mirror sudah menjadi cadangan yang benar.

**Tes** (otomatis: `internal/mirror`, `internal/koha`, `internal/rfidtag`; manual: middleware asli ke Koha dev)
- [x] Failover: Koha dimatikan, tag tidak dipinjam tetap Alarm (`NOT_LOAN`) dari mirror dalam sekitar 17 ms, tag dipinjam Allow (`LOAN_VALID`), tag tidak dikenal Allow. Bentuk tag dengan atau tanpa `SN` sama-sama benar.
- [x] Pulih: Koha dinyalakan lagi, keputusan kembali dari Koha tanpa intervensi.
- [x] Delta: tag baru yang dipetakan ke item yang sedang dipinjam muncul di mirror dalam satu siklus sync (interval 3 detik di uji), lalu terjawab `LOAN_VALID` saat Koha mati.
- [ ] Uji dengan data produksi: unggah `RfidGateApi-1.0.5.kpz` ke Koha produksi, backup, cek duplikat normalisasi, ukur waktu sync penuh di skala sebenarnya.
- [ ] `go test -race` (butuh cgo/gcc; tidak tersedia di mesin ini).

Catatan:
- Jika Koha mengembalikan jauh lebih sedikit tag daripada mirror yang sudah berformat ternormalisasi (misalnya Koha direset), sync penuh ditolak dengan sengaja. Operator perlu mengosongkan `rfid_status` untuk menerima daftar baru.
- Kolom `item_rfid.assigned_at` tidak ber-index, sehingga cabang kedua delta memindai `item_rfid` (17 ms di 34 ribu baris). Tidak perlu diubah sekarang.

**Selesai bila:** dengan Koha dimatikan, tag dipinjam menghasilkan Allow dan tag tidak dipinjam menghasilkan Alarm dari mirror; setelah Koha hidup lagi, mirror ikut ter-update dan audit selisih jumlah mendekati 0. (Dev: terpenuhi.)

---

## Fase 3B - Sync biblio dari Koha ke SQLite

**Goal:** judul, cover, dan metadata ringkas di dashboard/SSE terisi dari Koha tanpa SLiMS (pengganti `biblio.Syncer` yang membaca MySQL SLiMS), dengan delta sehingga tidak upsert 47 ribu record tiap jam.

Status: **selesai di Koha dev** (2026-11-11); butir produksi dan frontend Tauri terbuka.

**Temuan yang menentukan desain**
- Cover Koha disimpan sebagai blob di `cover_images` (dev: 1.764 cover dari 49.654 biblio, satu per biblio), disajikan OPAC lewat `opac-image.pl`, bukan sebagai nama file seperti SLiMS.
- Untuk biblio **tanpa** cover, `opac-image.pl` membalas GIF kosong 43 byte, jadi referensi cover hanya diisi bila cover ada.
- `biblio.title` Koha berakhiran tanda ISBD (`"Tafsir ruh al-bayan /"`); dibersihkan di plugin.
- Frontend (web tertanam dan Tauri) membangun URL cover dengan path SLiMS (`/lib/minigalnano/createthumb.php?...`).

**Plugin Koha** (`RfidGateApi-1.0.6.kpz`, endpoint `GET /rfid/biblio`)
- [x] Field minimal, hanya untuk item ber-RFID yang punya barcode: `item_code`, `biblio_id` (biblionumber), `title` (tanda ISBD akhir dibuang), `has_cover`. Cover itu sendiri tidak dikirim.
- [x] Kontrak sama dengan `/rfid/status`: tanpa `since` = penuh, dengan `since` = delta (`items.timestamp`, `biblio.timestamp`, `cover_images.timestamp`, atau tag baru), paginasi keyset (`after` = nomor item, `limit` maksimal 5000), `server_time`, `total`, `X-API-Key`.
- [x] Tes di skala 34.713 baris (sintetis, sudah dihapus): jumlah record dan `has_cover` (1.986) **sama persis** dengan hitungan SQL, tidak ada judul yang masih berakhiran tanda ISBD. Pull penuh 3,3 detik (4 MB); delta sekitar 200 ms (memindai `biblio.timestamp`, lebih berat dari status), jadi sisi Go polling tiap 10 menit.

**Go**
- [x] `koha.FetchBiblio` (klien paging, berbagi helper `getJSON` dengan `FetchStatus`) dan `biblio.KohaFetcher` menggantikan `NewMySQLFetcher` di mode Koha. Item tanpa barcode dilewati.
- [x] `item_biblio_map.image_filename` berisi **biblionumber** hanya bila cover ada; kosong bila tidak ada (dashboard menampilkan placeholder).
- [x] Delta: sync pertama penuh; berikutnya delta tiap 10 menit (`koha_api.biblio_sync_interval_minutes`) dengan tumpang tindih 15 menit; penuh lagi tiap 24 jam (`biblio_full_sync_hours`) yang memangkas item yang sudah hilang. Admin trigger manual = sync penuh. Entri dan cursor ditulis dalam satu transaksi (tabel baru `biblio_state`).
- [x] Pengaman sync penuh sama seperti mirror: ditolak bila kosong, di bawah 50% isi tabel, atau tidak cocok dengan total Koha.
- [x] Judul tetap UTF-8 utuh (diakritik Arab terverifikasi di log scan).

**Frontend**
- [x] Web tertanam (`web/dist/js/format.js`) dan aplikasi Tauri (`BookScanCard.tsx`): path cover diganti ke `/cgi-bin/koha/opac-image.pl?thumbnail=1&biblionumber=` (thumbnail) dan `?biblionumber=` (besar). Perubahan Tauri hanya 2 konstanta dan **belum di-commit** di repo itu (ada perubahan lain milik Anda di sana).
- [ ] **Perlu konfirmasi:** `CATALOGUE_BASE` masih `https://catalogue.uiii.ac.id`. Path baru hanya berfungsi bila host itu melayani **OPAC Koha**. Jika OPAC Koha produksi ada di host lain, ubah konstanta ini di kedua frontend.

**Tes** (otomatis: `internal/biblio`, `internal/koha`; manual: middleware asli ke Koha dev)
- [x] Ujung ke ujung: tag pada item yang biblionya punya cover menghasilkan log scan dengan judul, `biblio_id`, dan `image_filename` = biblionumber; URL cover itu dibalas 200 `image/png` (thumbnail 55 KB, besar 183 KB). Tag pada item tanpa cover: judul ada, `image_filename` kosong. Tag yang ditambahkan **sesudah** middleware start terambil oleh delta (status dan biblio) tanpa restart.
- [ ] Data lama di `item_biblio_map` hasil SLiMS: dipangkas oleh sync penuh pertama (kuncinya `item_code`/barcode); diuji di dev, bukan dengan data produksi.
- [ ] Uji produksi: unggah `RfidGateApi-1.0.6.kpz`, ukur waktu sync penuh di skala sebenarnya (~47 ribu item), cek cover di dashboard dengan host OPAC produksi.

**Selesai bila:** data biblio terisi dari Koha tanpa koneksi ke SLiMS, sync berikutnya hanya mengambil perubahan, dan dashboard menampilkan judul serta cover seperti sebelumnya. (Dev: terpenuhi; menunggu konfirmasi host OPAC untuk produksi.)

---
## Fase 4 - Ketahanan middleware

**Goal:** middleware jarang mati, dan bila mati cepat hidup lagi serta staf tahu.

Status: **kode dan skrip selesai, diuji di mesin dev tanpa hak Administrator**. Yang butuh Administrator (memasang service dan task) dan token Telegram asli belum diuji; skrip verifikasinya sudah disiapkan.

**Tiga lapis**
| Lapis | Menangani | Berkas |
|---|---|---|
| Pemulihan service Windows | proses mati/crash | `install.bat` |
| `heal.ps1` (task `RFIDGateHeal`, SYSTEM) | proses hidup tapi hang | `watchdog/heal.ps1` |
| `watchdog.ps1` (task `RFIDGateWatchdog`, user) | memberi tahu staf (layar + Telegram) | `watchdog/watchdog.ps1` |

**Pemulihan service**
- [x] `install.bat` mengatur `start= auto`, restart 5 detik (kegagalan ke-1 dan ke-2) lalu 30 detik (ke-3 dan seterusnya, sebagai jeda agar middleware yang tidak bisa start tidak di-restart terus), reset 24 jam, dan `failureflag 1` (pulihkan juga bila keluar dengan kode error). Dipilih lewat `sc` karena opsi `kardianos/service` v1.2.4 hanya mendukung satu aksi dan tidak menyetel flag itu (dibaca dari kode modulnya).
- [ ] **Jalankan di PC dengan Administrator:** `install.bat`, lalu `watchdog\verify-service-recovery.ps1` (cek konfigurasi, mematikan proses paksa, ukur waktu hingga `/health` menjawab). Service `RFIDMiddleware` di mesin dev ini berstatus Stopped, start type Demand, tanpa aksi pemulihan, dan sengaja tidak saya ubah.
- [x] Perilaku port 3000 dipakai proses lain (diuji): instance kedua keluar dengan kode 1 dan pesan jelas `cannot listen on port (already in use?)`. Sebagai service ini berarti restart berulang (5 detik, 5 detik, lalu tiap 30 detik) sampai portnya bebas; `[DOWN]` Telegram masuk setelah 2 menit.

**Hang (proses hidup, `/health` tidak menjawab)**
- [x] `watchdog/heal.ps1`: bila service Running tetapi `/health` gagal 3 menit berturut-turut, service di-restart. Aturan aman: tidak menyentuh service yang Stopped (dimatikan sengaja untuk perawatan), maksimal 3 restart per jam, dan memberi tahu Telegram sekali bila batas tercapai.
- [x] Diuji dengan `-DryRun` terhadap service yang Running (Spooler): 3 kali "would restart", percobaan ke-4 ditolak karena batas; terhadap service Stopped (`RFIDMiddleware`) dibiarkan.
- [ ] Uji restart sungguhan di PC dengan Administrator (membekukan proses lalu melihat `RFIDGateHeal` me-restart-nya).

**Telegram**
- [x] `watchdog/watchdog-common.ps1` (`Send-Telegram`) dan `watchdog/telegram.example.json`. Token dan chat id hanya di `telegram.json` (di-gitignore) atau variabel lingkungan; tidak pernah ditulis ke log (diuji: token tidak ada di log, termasuk saat pengiriman gagal). Tanpa `telegram.json`, Telegram nonaktif.
- [x] Pesan: `[DOWN]` (middleware, reader, dashboard), `[WARN]` (Koha tidak terjangkau lebih dari 10 menit), `[PULIH]`, `[HEAL]`. Nama mesin ikut di setiap pesan. Pengingat tiap 15 menit (Koha 30 menit), tidak spam. Pesan `[PULIH]` hanya dikirim bila `[DOWN]` sebelumnya terkirim.
- [x] Telegram dikirim **sebelum** kotak peringatan layar (kotak itu memblokir sampai diklik).
- [x] Middleware baru dinotifikasi setelah 2 menit (`$GoDownThresholdMinutes`) supaya restart otomatis tidak memicu alarm palsu.
- [x] Pemeriksaan Koha: `watchdog.ps1` sebelumnya hanya memeriksa `slims_mysql` (bernilai `disabled` di mode Koha), jadi Koha mati tidak pernah terdeteksi. Kini memeriksa `slims_api` (keterjangkauan Koha).
- [x] Uji ujung ke ujung dengan middleware asli dan server Telegram palsu: middleware mati (tidak ada pesan dalam masa tenggang 2 menit, lalu `[DOWN]`, tanpa pengingat berulang), hidup lagi (`[PULIH]`), Koha dimatikan (tidak ada pesan sebelum 10 menit, lalu `[WARN]`), Koha hidup (`[PULIH]`), dan gangguan Telegram tidak menjatuhkan watchdog.
- [ ] **Perlu dari Anda:** bot token dan chat id grup staf, untuk uji kirim ke Telegram sungguhan (buat bot lewat `@BotFather`; langkah lengkap di `DEPLOYMENT_GUIDE.md` bagian 8).
- [ ] Mode "semua mati" dari gate C# memicu pesan yang sama: dikerjakan di Fase 5 (C# menulis berkas yang dibaca watchdog, atau memanggil Telegram langsung).

**Panic di goroutine**
- [x] Paket `internal/safego`. `recover` kini ada di sync mirror, sync biblio (panic menjadi error dan `IsSyncing` tetap di-reset), worker antrean, lookup cache di engine, dan insert history. Ada tes untuk tiap kasus. Panic di handler HTTP sudah ditangani `net/http`.

**Dokumentasi dan paket deploy**
- [x] `register-watchdog-task.ps1` mendaftarkan `RFIDGateWatchdog` (user) dan, bila dijalankan sebagai Administrator, `RFIDGateHeal` (SYSTEM).
- [x] `build-deploy.ps1 -IncludeWatchdog` ikut menyalin `watchdog-common.ps1`, `heal.ps1`, `verify-service-recovery.ps1`, dan `telegram.example.json`.
- [x] `DEPLOYMENT_GUIDE.md` bagian 8 (tiga lapis, pemasangan, langkah bot Telegram, arti tiap pesan).
- [ ] Daftarkan kedua task di PC gate dan pastikan `RFIDGateWatchdog` berjalan di sesi pengguna yang login (syarat agar kotak peringatan tampil).

**Selesai bila:** proses dimatikan paksa kembali hidup otomatis dalam waktu yang disepakati (misalnya kurang dari 30 detik) dan staf menerima notifikasi bila down lebih lama. (Pemberitahuan: terpenuhi di uji dengan server Telegram palsu. Waktu pulih: menunggu `verify-service-recovery.ps1` di PC dengan Administrator.)

---

## Fase 5 - C#: fallback tipis dan fail-open dengan peringatan

**Goal:** gate tetap memberi keputusan benar bila middleware mati, dan tidak diam bila semuanya mati.

Status: **kode selesai dan teruji di CLR 2.0 (runtime yang sama dengan gate); belum dijalankan di aplikasi gate dengan reader asli** (butuh perangkat keras dan build Visual Studio).

Berkas ada di `gate-csharp/` (sumber asli di `C:\RFID_Gate\...` bukan repo git; backup di `C:\frendy-project\backups\rfid-gate-csharp\2026-11-11\`).

**Alur keputusan per scan** (`GateDecisionClient.cs`)
1. Middleware lokal (`POST /rfid/check`, timeout 1 detik).
2. Bila gagal dan `gate.ini` berisi `KohaBaseUrl` + `KohaApiKey`: Koha langsung (timeout 1,5 detik). `BORROWED` menjadi 0 (boleh), `NOT_BORROWED` menjadi 1 (**alarm**), `ITEM_NOT_FOUND` menjadi 0.
3. Bila keduanya gagal: **fail-open** (keputusan 0, tanpa alarm), nada peringatan `SystemSounds.Exclamation` (beda dari alarm, maksimal sekali per 10 detik), catatan di log gate, dan berkas status untuk watchdog.

**Yang dikerjakan**
- [x] Timeout pendek untuk middleware dan Koha (dapat diatur di `gate.ini`).
- [x] Circuit breaker **per sumber** (default 10 detik): setelah gagal, sumber itu dilewati, lalu dicoba lagi sekali. Saat keduanya dilewati, scan langsung fail-open tanpa menunggu timeout.
- [x] Fallback Koha langsung dengan `X-API-Key` khusus gate; terjemahan keputusan sama dengan gate versi lama.
- [x] Perbaikan bug lama: `CallGoMiddleware` mengembalikan string kosong saat gagal dan `ParseDecisionFromJson` mengubahnya jadi keputusan 0, jadi middleware mati tampak seperti "boleh" tanpa jejak. Kini kegagalan (error transport, timeout, status bukan 200, tidak ada keputusan, atau nilai selain 0/1) selalu dianggap **gagal** dan memicu fallback. Parser memakai regex yang toleran spasi.
- [x] HTTPS/TLS: flag TLS 1.2 (`(SecurityProtocolType)3072`) diset sekali di konstruktor statis. Diverifikasi dari kelas itu sendiri di CLR 2.0: handshake ke `catalogue.uiii.ac.id` berhasil (tanpa flag, `SendFailure`).
- [x] API key dibaca dari `gate.ini` (bukan hardcode) dan tidak pernah ditulis ke log. Contoh: `gate-csharp/gate.ini.example`.
- [x] `Form1.checklabel` memakai kelas baru (`gate-csharp/Form1.cs.diff`; perubahan hanya itu). Hasil patch berhasil dikompilasi.
- [x] Berkas status `%ProgramData%\RFIDGate\gate-status.txt` (`mode=normal|koha-direct|fail-open`, `since`, `updated`), ditulis saat mode berubah dan, selama terdegradasi, tiap 20 detik.
- [x] `watchdog.ps1` pemeriksaan ke-4: bila `fail-open` dan `updated` dalam 3 menit terakhir, kirim `[DOWN] Gate Tanpa Pengecekan` (layar + Telegram), `[PULIH]` setelah normal. Berkas yang basi (aplikasi gate ditutup saat gangguan) tidak memicu alarm. `state.json` lama tanpa entri `gate` tetap terbaca.

**Hasil uji**
- [x] 42 tes otomatis di CLR 2.0 dengan server HTTP palsu: middleware normal, jawaban buruk (HTTP 500, tanpa keputusan, nilai 7) ditangani sebagai kegagalan, ketiga status Koha, Koha 401, keduanya mati, middleware menggantung (fallback dalam 303 ms dengan timeout 300 ms), breaker kedua sumber (dicoba lagi setelah waktunya, dipakai lagi setelah pulih), escape JSON, berkas pengaturan, berkas status.
- [x] Uji langsung klien C# (CLR 2.0) terhadap middleware Go asli dan Koha dev, dengan komponen dimatikan bertahap: `Middleware` lalu `KohaDirect` (middleware dimatikan) lalu `None`/fail-open (Koha ikut dimatikan) lalu `Middleware` lagi (middleware hidup, menjawab dari mirror walau Koha masih mati). Latensi: middleware median 13 ms, Koha langsung median 58 ms, fail-open median 0 ms (breaker). Berkas status berakhir `mode=normal`.
- [x] Pengujian watchdog dengan berkas status yang ditulis oleh kelas C# asli (6 skenario, termasuk basi dan upgrade `state.json`).

**Temuan latensi (penting untuk operasional)**
- Di Windows, koneksi ke port loopback yang **tertutup** baru dilaporkan "ditolak" setelah ~2 detik (diukur: 2,03-2,06 detik di CLR 2.0). Karena itu yang membatasi adalah timeout klien, bukan penolakan koneksi.
- Akibatnya scan **pertama** saat gangguan mulai menunggu sekitar timeout middleware + timeout Koha (diukur 1,0-1,6 detik); sesudah itu scan langsung selama jendela breaker, dan satu scan "pengintai" tiap 10 detik menanggung biaya itu lagi selama gangguan masih berlangsung. Pada awalnya breaker hanya melindungi middleware, sehingga tiap scan saat Koha mati menunggu 1,5 detik; ditemukan di uji langsung dan diperbaiki dengan breaker kedua.
- Sisi Go tidak menunjukkan masalah serupa: dengan Koha mati, scan lewat middleware tetap bermedian 13 ms (hanya scan pertama setelah middleware baru hidup yang ~1 detik karena start dingin).

**Belum dikerjakan / perlu Anda**
- [ ] Bangun ulang aplikasi gate lewat Visual Studio dan uji dengan reader asli (alarm untuk buku tidak dipinjam, nada peringatan saat fail-open).
- [ ] Ulangi uji TLS di **PC gate sebenarnya** (OS dan patch .NET bisa berbeda dari mesin dev).
- [ ] Buat **API key khusus gate** di halaman konfigurasi plugin Koha (satu key per baris), isi `gate.ini` di PC gate. Tanpa key ini fallback Koha tidak aktif dan gate langsung fail-open bila middleware mati.
- [ ] Putuskan host Koha untuk `KohaBaseUrl` di produksi (sama dengan pertanyaan host OPAC di Fase 3B).
- [ ] Atur nada peringatan bila `SystemSounds.Exclamation` kurang terdengar di PC gate (speaker/volume).

**Selesai bila:** ketiga skenario (normal, middleware mati, semua mati) berperilaku sesuai keputusan 1 di PC gate nyata, dengan alarm hanya untuk buku tidak dipinjam. (Perilaku: terpenuhi di uji CLR 2.0; menunggu uji di PC gate nyata.)

---

## Fase 6 - Bersih-bersih SLiMS dan dokumentasi

**Goal:** kode dan dokumen hanya mencerminkan Koha (keputusan 3).

- [ ] Hapus jalur SLiMS: `internal/sync/remote_lookup.go` (`MySQLRemoteLookup`), `PullDelta`/`PullAll` untuk SLiMS, query `log_bukukeluar_test`, konfigurasi `slims` yang tidak dipakai.
- [ ] Sederhanakan `main.go`: hilangkan cabang `useKoha` vs SLiMS.
- [ ] Hapus `biblio.NewMySQLFetcher` dan konfigurasi `slims_api`/`slims.mysql_dsn` setelah fetcher Koha (Fase 3) berjalan.
- [ ] Hapus kode mati: `dto.go` `RFIDCheckResponse`, blok komentar di `handler_rfid.go`, `history/repository_impl.go` kosong.
- [ ] Update `CLAUDE.md`: backend Koha, alur scan baru, daftar masalah (hapus yang sudah selesai atau usang).
- [ ] Update `README.md` dan `DEPLOYMENT_GUIDE.md`.
- [ ] Update memory `.claude/memory/rfid-gate-flow.md` dengan alur terbaru.
- [ ] Jalankan seluruh tes dan build `rfid-middleware.exe`.

**Selesai bila:** tidak ada referensi SLiMS yang masih dipakai di kode aktif, tes hijau, dokumen akurat.

---

## Ringkasan pelacakan

| Fase | Fokus | Komponen | Status |
|---|---|---|---|
| 0 | Baseline dan backup | semua | [x] dev, [ ] produksi |
| 1 | Normalisasi cepat, log bersih | Koha plugin | [x] dev, [ ] produksi |
| 2 | Satu panggilan, tag tidak dikenal dibuang | Go | [x] |
| 3A | Mirror status offline terisi | Koha plugin + Go | [x] dev, [ ] produksi |
| 3B | Sync biblio dari Koha | Koha plugin + Go + frontend | [x] dev, [ ] produksi |
| 4 | Ketahanan middleware + Telegram | Windows Service, watchdog | [x] kode, [ ] uji admin + token |
| 5 | Fallback dan peringatan | C# gate + watchdog | [x] kode + uji CLR 2.0, [ ] PC gate nyata |
| 6 | Bersih-bersih SLiMS, dokumen | Go + docs | [ ] |
