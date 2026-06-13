-- +goose Up
-- SQL in this section is executed when the migration is applied.

-- 1. Update torrents table
ALTER TABLE torrents ADD COLUMN uploaded_at INTEGER;
ALTER TABLE torrents ADD COLUMN uploader TEXT;

-- 2. Add parent tracking to comments
ALTER TABLE comments ADD COLUMN parent_id TEXT DEFAULT '';
ALTER TABLE comments ADD COLUMN parent_message TEXT DEFAULT '';

-- 3. Create announcement queue table
CREATE TABLE IF NOT EXISTS announcement_queue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    service TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    torrent_id TEXT NOT NULL,
    comment_id TEXT NOT NULL,
    author_icon_url TEXT,
    show_comment_id BOOLEAN DEFAULT FALSE,
    resolve_image BOOLEAN DEFAULT FALSE,
    retry_count INTEGER DEFAULT 0,
    last_error TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    posted_at DATETIME
);

-- +goose Down
-- SQL in this section is executed when the migration is rolled back.
DROP TABLE IF EXISTS announcement_queue;
-- SQLite doesn't support dropping columns, so rollback for torrents/comments would require table recreation
-- which goose doesn't handle automatically via ALTER.
