package db

import (
	"database/sql"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	Conn *sql.DB
	mu   sync.Mutex
}

func InitDB(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// 1. Torrents table
	_, err = conn.Exec(`
		CREATE TABLE IF NOT EXISTS torrents (
			service TEXT,
			torrent_id TEXT,
			title TEXT,
			comment_count INTEGER,
			last_scraped_at DATETIME,
			PRIMARY KEY (service, torrent_id)
		)
	`)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// 2. Comments table
	_, err = conn.Exec(`
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
		)
	`)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &DB{Conn: conn}, nil
}

func (db *DB) Close() error {
	return db.Conn.Close()
}

func (db *DB) GetStoredCommentCount(service, torrentID string) (int, bool) {
	db.mu.Lock()
	defer db.mu.Unlock()
	var count int
	err := db.Conn.QueryRow("SELECT comment_count FROM torrents WHERE service = ? AND torrent_id = ?", service, torrentID).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, false
	}
	if err != nil {
		return 0, false
	}
	return count, true
}

func (db *DB) UpdateTorrent(service, torrentID, title string, count int) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.Conn.Exec(`
		INSERT INTO torrents (service, torrent_id, title, comment_count, last_scraped_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(service, torrent_id) DO UPDATE SET
			title = excluded.title,
			comment_count = excluded.comment_count,
			last_scraped_at = excluded.last_scraped_at
	`, service, torrentID, title, count, time.Now())
	return err
}

func (db *DB) IsCommentStored(service, torrentID, commentID string) bool {
	db.mu.Lock()
	defer db.mu.Unlock()
	var exists int
	err := db.Conn.QueryRow(`
		SELECT 1 FROM comments 
		WHERE service = ? AND torrent_id = ? AND comment_id = ?
	`, service, torrentID, commentID).Scan(&exists)
	return err == nil
}

func (db *DB) StoreComment(service, torrentID, commentID, username, message string, timestamp int64, position int, role, avatarURL string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.Conn.Exec(`
		INSERT OR IGNORE INTO comments (service, torrent_id, comment_id, username, message, timestamp, position, user_role, avatar_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, service, torrentID, commentID, username, message, timestamp, position, role, avatarURL)
	return err
}

type Torrent struct {
	Service       string
	TorrentID     string
	Title         string
	CommentCount  int
	LastScrapedAt time.Time
}

type Comment struct {
	Service   string
	TorrentID string
	CommentID string
	Username  string
	Message   string
	Timestamp int64
	Position  int
	UserRole  string
	AvatarURL string
}

func (db *DB) GetLatestTorrents(limit int) ([]Torrent, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	rows, err := db.Conn.Query("SELECT service, torrent_id, title, comment_count, last_scraped_at FROM torrents ORDER BY last_scraped_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var torrents []Torrent
	for rows.Next() {
		var t Torrent
		if err := rows.Scan(&t.Service, &t.TorrentID, &t.Title, &t.CommentCount, &t.LastScrapedAt); err != nil {
			return nil, err
		}
		torrents = append(torrents, t)
	}
	return torrents, nil
}

func (db *DB) GetLatestComments(limit int) ([]Comment, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	rows, err := db.Conn.Query("SELECT service, torrent_id, comment_id, username, message, timestamp, position, user_role, avatar_url FROM comments ORDER BY timestamp DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.Service, &c.TorrentID, &c.CommentID, &c.Username, &c.Message, &c.Timestamp, &c.Position, &c.UserRole, &c.AvatarURL); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, nil
}

func (db *DB) GetStats() (int, int, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	var torrentsCount, commentsCount int
	err := db.Conn.QueryRow("SELECT COUNT(*) FROM torrents").Scan(&torrentsCount)
	if err != nil {
		return 0, 0, err
	}
	err = db.Conn.QueryRow("SELECT COUNT(*) FROM comments").Scan(&commentsCount)
	if err != nil {
		return 0, 0, err
	}
	return torrentsCount, commentsCount, nil
}
