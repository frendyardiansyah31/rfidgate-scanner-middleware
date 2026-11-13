CREATE TABLE IF NOT EXISTS offline_events (
    rfid TEXT NOT NULL,
    status INTEGER NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_offline_events_created
ON offline_events(created_at);
