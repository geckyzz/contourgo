package db

import (
	"database/sql"
	"embed"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

type DB struct {
	Conn *sql.DB
	mu   sync.Mutex
}

func InitDB(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	if err := goose.SetDialect("sqlite3"); err != nil {
		conn.Close()
		return nil, err
	}

	goose.SetBaseFS(embedMigrations)
	if err := goose.Up(conn, "migrations"); err != nil {
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
	err := db.Conn.QueryRow("SELECT comment_count FROM torrents WHERE service = ? AND (torrent_id = ? OR torrent_id LIKE '%.' || ?)", service, torrentID, torrentID).
		Scan(&count)
	if err == sql.ErrNoRows {
		return 0, false
	}
	if err != nil {
		return 0, false
	}
	return count, true
}

type dbConn interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func (db *DB) WithTx(fn func(*sql.Tx) error) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.Conn.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

func updateTorrent(
	conn dbConn,
	service, torrentID, title string,
	count int,
	uploadedAt int64,
	uploader string,
) error {
	_, err := conn.Exec(`
		INSERT INTO torrents (service, torrent_id, title, comment_count, uploaded_at, uploader, last_scraped_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(service, torrent_id) DO UPDATE SET
			title = excluded.title,
			comment_count = excluded.comment_count,
			uploaded_at = COALESCE(NULLIF(excluded.uploaded_at, 0), torrents.uploaded_at),
			uploader = COALESCE(NULLIF(excluded.uploader, ''), torrents.uploader),
			last_scraped_at = excluded.last_scraped_at
	`, service, torrentID, title, count, uploadedAt, uploader, time.Now())
	return err
}

func (db *DB) UpdateTorrent(
	service, torrentID, title string,
	count int,
	uploadedAt int64,
	uploader string,
) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return updateTorrent(db.Conn, service, torrentID, title, count, uploadedAt, uploader)
}

func (db *DB) UpdateTorrentTx(
	tx *sql.Tx,
	service, torrentID, title string,
	count int,
	uploadedAt int64,
	uploader string,
) error {
	return updateTorrent(tx, service, torrentID, title, count, uploadedAt, uploader)
}

func (db *DB) IsCommentStored(service, torrentID, commentID string) bool {
	db.mu.Lock()
	defer db.mu.Unlock()
	var exists int
	err := db.Conn.QueryRow(`
		SELECT 1 FROM comments 
		WHERE service = ? AND (torrent_id = ? OR torrent_id LIKE '%.' || ?) AND comment_id = ?
	`, service, torrentID, torrentID, commentID).Scan(&exists)
	return err == nil
}

func storeComment(
	conn dbConn,
	service, torrentID, commentID, username, message string,
	timestamp int64,
	position int,
	role, avatarURL, parentID, parentMessage string,
) error {
	_, err := conn.Exec(`
		INSERT OR IGNORE INTO comments (service, torrent_id, comment_id, username, message, timestamp, position, user_role, avatar_url, parent_id, parent_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, service, torrentID, commentID, username, message, timestamp, position, role, avatarURL, parentID, parentMessage)
	return err
}

func (db *DB) StoreComment(
	service, torrentID, commentID, username, message string,
	timestamp int64,
	position int,
	role, avatarURL, parentID, parentMessage string,
) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return storeComment(
		db.Conn,
		service,
		torrentID,
		commentID,
		username,
		message,
		timestamp,
		position,
		role,
		avatarURL,
		parentID,
		parentMessage,
	)
}

func (db *DB) StoreCommentTx(
	tx *sql.Tx,
	service, torrentID, commentID, username, message string,
	timestamp int64,
	position int,
	role, avatarURL, parentID, parentMessage string,
) error {
	return storeComment(
		tx,
		service,
		torrentID,
		commentID,
		username,
		message,
		timestamp,
		position,
		role,
		avatarURL,
		parentID,
		parentMessage,
	)
}

func (db *DB) GetLatestCommentByUsersBeforePosition(
	service, torrentID string,
	usernames []string,
	beforePos int,
) (Comment, bool) {
	if len(usernames) == 0 {
		return Comment{}, false
	}
	db.mu.Lock()
	defer db.mu.Unlock()

	placeholders := make([]string, len(usernames))
	args := make([]any, 0, 4+len(usernames))
	args = append(args, service, torrentID, torrentID)
	for i, u := range usernames {
		placeholders[i] = "LOWER(?)"
		args = append(args, strings.ToLower(u))
	}
	args = append(args, beforePos)

	query := fmt.Sprintf(`
		SELECT service, torrent_id, comment_id, username, message, timestamp, position, 
		       COALESCE(user_role, ''), COALESCE(avatar_url, ''), COALESCE(parent_id, ''), COALESCE(parent_message, '')
		FROM comments 
		WHERE service = ? 
		  AND (torrent_id = ? OR torrent_id LIKE '%%.' || ?)
		  AND LOWER(username) IN (%s)
		  AND position < ?
		ORDER BY position DESC
		LIMIT 1
	`, strings.Join(placeholders, ", "))

	var c Comment
	err := db.Conn.QueryRow(query, args...).Scan(
		&c.Service, &c.TorrentID, &c.CommentID, &c.Username, &c.Message, &c.Timestamp, &c.Position,
		&c.UserRole, &c.AvatarURL, &c.ParentID, &c.ParentMessage,
	)
	if err != nil {
		return Comment{}, false
	}
	return c, true
}

func (db *DB) GetComment(service, torrentID, commentID string) (Comment, bool) {
	db.mu.Lock()
	defer db.mu.Unlock()
	var c Comment
	err := db.Conn.QueryRow(`
		SELECT service, torrent_id, comment_id, username, message, timestamp, position, 
		       COALESCE(user_role, ''), COALESCE(avatar_url, ''), COALESCE(parent_id, ''), COALESCE(parent_message, '')
		FROM comments 
		WHERE service = ? AND (torrent_id = ? OR torrent_id LIKE '%.' || ?) AND comment_id = ?
	`, service, torrentID, torrentID, commentID).Scan(&c.Service, &c.TorrentID, &c.CommentID, &c.Username, &c.Message, &c.Timestamp, &c.Position, &c.UserRole, &c.AvatarURL, &c.ParentID, &c.ParentMessage)
	if err == sql.ErrNoRows {
		return c, false
	}
	if err != nil {
		return c, false
	}
	return c, true
}

func (db *DB) GetTorrent(service, torrentID string) (Torrent, bool) {
	db.mu.Lock()
	defer db.mu.Unlock()
	var t Torrent
	err := db.Conn.QueryRow(`
		SELECT service, torrent_id, title, comment_count, COALESCE(uploaded_at, 0), COALESCE(uploader, ''), last_scraped_at
		FROM torrents
		WHERE service = ? AND (torrent_id = ? OR torrent_id LIKE '%.' || ?)
	`, service, torrentID, torrentID).Scan(&t.Service, &t.TorrentID, &t.Title, &t.CommentCount, &t.UploadedAt, &t.Uploader, &t.LastScrapedAt)
	if err == sql.ErrNoRows {
		return t, false
	}
	if err != nil {
		return t, false
	}
	return t, true
}

type Torrent struct {
	Service       string
	TorrentID     string
	Title         string
	CommentCount  int
	UploadedAt    int64
	Uploader      string
	LastScrapedAt time.Time
}

type Comment struct {
	Service       string
	TorrentID     string
	CommentID     string
	Username      string
	Message       string
	Timestamp     int64
	Position      int
	UserRole      string
	AvatarURL     string
	ParentID      string
	ParentMessage string
}

type QueuedAnnouncement struct {
	ID              int
	Service         string
	ChannelID       string
	TorrentID       string
	CommentID       string
	AuthorIconURL   string
	ShowCommentID   bool
	ResolveImage    bool
	RetryCount      int
	LastError       string
	CreatedAt       time.Time
	MentionsDisable bool

	// Joined data

	Torrent Torrent
	Comment Comment
}

func (db *DB) EnqueueAnnouncement(
	service, channelID, torrentID, commentID, authorIconURL string,
	showCommentID, resolveImage bool,
	mentionsDisable bool,
) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.Conn.Exec(`
		INSERT INTO announcement_queue (service, channel_id, torrent_id, comment_id, author_icon_url, show_comment_id, resolve_image, mentions_disabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, service, channelID, torrentID, commentID, authorIconURL, showCommentID, resolveImage, mentionsDisable)
	return err
}

func (db *DB) GetPendingAnnouncements() ([]QueuedAnnouncement, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	rows, err := db.Conn.Query(`
		SELECT 
			q.id, q.service, q.channel_id, q.torrent_id, q.comment_id, COALESCE(q.author_icon_url, ''), q.show_comment_id, q.resolve_image, 
			COALESCE(q.retry_count, 0), COALESCE(q.last_error, ''), q.created_at, COALESCE(q.mentions_disabled, FALSE),
			COALESCE(t.title, ''), COALESCE(t.uploaded_at, 0), COALESCE(t.uploader, ''), COALESCE(t.comment_count, 0), t.last_scraped_at,
			COALESCE(c.username, ''), COALESCE(c.message, ''), COALESCE(c.timestamp, 0), COALESCE(c.position, 0), 
			COALESCE(c.user_role, ''), COALESCE(c.avatar_url, ''), COALESCE(c.parent_id, ''), COALESCE(c.parent_message, '')
		FROM announcement_queue q
		LEFT JOIN torrents t ON q.service = t.service AND q.torrent_id = t.torrent_id
		LEFT JOIN comments c ON q.service = c.service AND q.torrent_id = c.torrent_id AND q.comment_id = c.comment_id
		WHERE q.posted_at IS NULL AND q.retry_count < 5
		ORDER BY q.service ASC, t.uploaded_at ASC, c.timestamp ASC, q.comment_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var announcements []QueuedAnnouncement
	for rows.Next() {
		var a QueuedAnnouncement
		var lastScrapedAt sql.NullTime
		if err := rows.Scan(
			&a.ID,
			&a.Service,
			&a.ChannelID,
			&a.TorrentID,
			&a.CommentID,
			&a.AuthorIconURL,
			&a.ShowCommentID,
			&a.ResolveImage,
			&a.RetryCount,
			&a.LastError,
			&a.CreatedAt,
			&a.MentionsDisable,
			&a.Torrent.Title,
			&a.Torrent.UploadedAt,
			&a.Torrent.Uploader,
			&a.Torrent.CommentCount,
			&lastScrapedAt,
			&a.Comment.Username,
			&a.Comment.Message,
			&a.Comment.Timestamp,
			&a.Comment.Position,
			&a.Comment.UserRole,
			&a.Comment.AvatarURL,
			&a.Comment.ParentID,
			&a.Comment.ParentMessage,
		); err != nil {
			return nil, err
		}

		if lastScrapedAt.Valid {
			a.Torrent.LastScrapedAt = lastScrapedAt.Time
		}

		a.Torrent.Service = a.Service
		a.Torrent.TorrentID = a.TorrentID
		a.Comment.Service = a.Service
		a.Comment.TorrentID = a.TorrentID
		a.Comment.CommentID = a.CommentID
		announcements = append(announcements, a)
	}
	return announcements, nil
}

func (db *DB) FailAnnouncement(id int, errMsg string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.Conn.Exec(
		"UPDATE announcement_queue SET retry_count = retry_count + 1, last_error = ? WHERE id = ?",
		errMsg,
		id,
	)
	return err
}

func (db *DB) MarkAnnouncementPosted(id int) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.Conn.Exec(
		"UPDATE announcement_queue SET posted_at = ?, retry_count = 0, last_error = NULL WHERE id = ?",
		time.Now(),
		id,
	)
	return err
}

func (db *DB) GetLatestTorrents(limit int) ([]Torrent, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	rows, err := db.Conn.Query(
		"SELECT service, torrent_id, title, comment_count, last_scraped_at FROM torrents ORDER BY last_scraped_at DESC LIMIT ?",
		limit,
	)
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
	rows, err := db.Conn.Query(`
		SELECT service, torrent_id, comment_id, username, message, timestamp, position, 
		       COALESCE(user_role, ''), COALESCE(avatar_url, ''), COALESCE(parent_id, ''), COALESCE(parent_message, '') 
		FROM comments 
		ORDER BY timestamp DESC 
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.Service, &c.TorrentID, &c.CommentID, &c.Username, &c.Message, &c.Timestamp, &c.Position, &c.UserRole, &c.AvatarURL, &c.ParentID, &c.ParentMessage); err != nil {
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
