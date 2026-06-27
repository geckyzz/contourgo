-- +goose Up
-- SQL in this section is executed when the migration is applied.

-- Table to store seen tweet IDs for twitter/nitter monitoring
CREATE TABLE IF NOT EXISTS twitter_tweets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account TEXT NOT NULL,
    tweet_id TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    link TEXT NOT NULL DEFAULT '',
    published_at INTEGER NOT NULL DEFAULT 0,
    seen_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(account, tweet_id)
);

CREATE INDEX IF NOT EXISTS idx_twitter_tweets_account ON twitter_tweets(account);

-- +goose Down
-- SQL in this section is executed when the migration is rolled back.
DROP TABLE IF EXISTS twitter_tweets;
