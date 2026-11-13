Yang dibutuhkan C# agar bisa kirim ke Koha sendiri

Hanya gate.ini di samping RFD1861Demo.exe:
KohaBaseUrl=https://<host-koha>/api/v1/contrib/rfid_gate
KohaApiKey=<key khusus gate>
Selama dua baris itu terisi dan Koha terjangkau dari PC gate, alurnya berjalan sendiri: middleware mati, lalu Koha langsung. Tidak ada proses lain yang perlu hidup.

Yang hilang tanpa watchdog

┌──────────────────────────────────┬───────────────────────────────────┐
│              Fitur               │          Tanpa watchdog           │
├──────────────────────────────────┼───────────────────────────────────┤
│ Keputusan gate (alarm atau       │ tetap jalan                       │
│ boleh)                           │                                   │
├──────────────────────────────────┼───────────────────────────────────┤
│ Fallback Koha langsung           │ tetap jalan                       │
├──────────────────────────────────┼───────────────────────────────────┤
│ Nada peringatan saat fail-open   │ tetap jalan (ada di Form1)        │
├──────────────────────────────────┼───────────────────────────────────┤
│ Berkas gate-status.txt           │ tetap ditulis, tapi tidak ada     │
│                                  │ yang membacanya                   │
├──────────────────────────────────┼───────────────────────────────────┤
│ Notifikasi Telegram dan kotak    │ tidak ada                         │
│ peringatan layar                 │                                   │
├──────────────────────────────────┼───────────────────────────────────┤
│ Restart otomatis middleware yang │ tidak ada                         │
│  hang (heal.ps1)                 │                                   │
├──────────────────────────────────┼───────────────────────────────────┤
│ Restart middleware yang crash    │ tetap jalan (itu fitur service    │
│                                  │ Windows, bukan watchdog)          │
└──────────────────────────────────┴───────────────────────────────────┘

Jadi risikonya bukan salah keputusan, tapi tidak ada yang memberi tahu staf kalau gate sedang fail-open.

Kalau middleware tidak dipasang sama sekali

Ini juga bisa jalan, dengan beberapa catatan jujur:
- Setiap 10 detik ada satu scan yang tertahan sekitar 1 detik, karena C# tetap mencoba middleware dulu (breaker membukanya lagi). Tidak ada opsi untuk melewatinya sekarang.
- Tidak ada mirror offline. Kalau Koha mati, langsung fail-open.
- Tidak ada riwayat scan lokal dan dashboard, karena itu dikelola middleware. Riwayat tetap ada di Koha (/gate/history), tapi hanya untuk scan yang dikenali.
- Semua keputusan bergantung pada satu panggilan HTTPS ke Koha per scan.

Kalau kamu memang ingin mode "C# langsung ke Koha saja" tanpa middleware, saya bisa tambahkan opsi di gate.ini (misalnya MiddlewareUrl= dikosongkan berarti dilewati). Itu mengubah kode di GateDecisionClient.cs, jadi saya tanyakan dulu: mau ditambahkan?