CREATE TABLE IF NOT EXISTS rfid_status (
    rfid TEXT PRIMARY KEY,
    status INTEGER NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_rfid_status_status
ON rfid_status(status);