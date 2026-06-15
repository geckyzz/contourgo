-- +goose Up
ALTER TABLE announcement_queue ADD COLUMN mentions_disabled BOOLEAN DEFAULT FALSE;

-- +goose Down
-- SQLite doesn't support dropping columns.
