-- +goose Up
-- SQL in this section is executed when the migration is applied.

CREATE TABLE IF NOT EXISTS donators (
    user_id TEXT PRIMARY KEY,
    username TEXT NOT NULL DEFAULT '',
    total_usd REAL NOT NULL DEFAULT 0.0,
    expiry_time INTEGER NOT NULL DEFAULT 0,
    warned_at INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);


CREATE INDEX IF NOT EXISTS idx_donators_expiry ON donators(expiry_time);

CREATE TABLE IF NOT EXISTS donation_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT NOT NULL,
    amount REAL NOT NULL,
    account TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_donation_logs_user ON donation_logs(user_id);

-- +goose Down
-- SQL in this section is executed when the migration is rolled back.
DROP TABLE IF EXISTS donation_logs;
DROP TABLE IF EXISTS donators;
