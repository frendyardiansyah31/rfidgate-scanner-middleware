-- Fase 1: kolom normalisasi SN untuk Koha RfidGateApi. Idempotent (MariaDB 10.5+).
-- Dipakai oleh Gate.pm: WHERE r.rfid_norm = <tag tanpa 'SN'>.
-- Jika kolom tidak ada, Gate.pm otomatis memakai query REPLACE() lama (lebih lambat).
ALTER TABLE item_rfid
  ADD COLUMN IF NOT EXISTS rfid_norm VARCHAR(64) AS (REPLACE(rfid,'SN','')) STORED,
  ADD INDEX IF NOT EXISTS idx_rfid_norm (rfid_norm);

-- Cek duplikat sebelum dipakai di produksi (harus 0 baris):
-- SELECT REPLACE(rfid,'SN','') n, COUNT(*) FROM item_rfid GROUP BY n HAVING COUNT(*)>1;

-- Rollback (kolom aman dibiarkan; Gate.pm fallback sendiri):
-- ALTER TABLE item_rfid DROP INDEX idx_rfid_norm, DROP COLUMN rfid_norm;
