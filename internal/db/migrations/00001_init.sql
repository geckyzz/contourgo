-- +goose Up
-- SQL in this section is executed when the migration is applied.
CREATE TABLE IF NOT EXISTS torrents (
    service TEXT,
    torrent_id TEXT,
    title TEXT,
    comment_count INTEGER,
    last_scraped_at DATETIME,
    PRIMARY KEY (service, torrent_id)
);

CREATE TABLE IF NOT EXISTS comments (
    service TEXT,
    torrent_id TEXT,
    comment_id TEXT,
    username TEXT,
    message TEXT,
    timestamp INTEGER,
    position INTEGER,
    user_role TEXT,
    avatar_url TEXT,
    PRIMARY KEY (service, torrent_id, comment_id)
);

-- +goose Down
-- SQL in this section is executed when the migration is rolled back.
DROP TABLE IF EXISTS comments;
DROP TABLE IF EXISTS torrents;
