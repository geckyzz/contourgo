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

// Tweet represents a single tweet/post fetched from a Nitter RSS feed.
type Tweet struct {
	Account     string
	TweetID     string
	Title       string
	Link        string
	PublishedAt int64
	SeenAt      time.Time
}

// IsTweetSeen returns true if a tweet with the given account+tweetID is already stored.
func (db *DB) IsTweetSeen(account, tweetID string) bool {
	db.mu.Lock()
	defer db.mu.Unlock()
	var exists int
	err := db.Conn.QueryRow(
		`SELECT 1 FROM twitter_tweets WHERE account = ? AND tweet_id = ?`,
		account, tweetID,
	).Scan(&exists)
	return err == nil
}

// StoreTweet persists a seen tweet so it won't be re-announced.
func (db *DB) StoreTweet(account, tweetID, title, link string, publishedAt int64) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.Conn.Exec(
		`INSERT OR IGNORE INTO twitter_tweets (account, tweet_id, title, link, published_at)
		 VALUES (?, ?, ?, ?, ?)`,
		account, tweetID, title, link, publishedAt,
	)
	return err
}

// GetLatestTweets returns the most recent tweets stored in the DB.
func (db *DB) GetLatestTweets(limit int) ([]Tweet, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	rows, err := db.Conn.Query(
		`SELECT account, tweet_id, title, link, published_at, seen_at
		 FROM twitter_tweets
		 ORDER BY seen_at DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tweets []Tweet
	for rows.Next() {
		var t Tweet
		if err := rows.Scan(&t.Account, &t.TweetID, &t.Title, &t.Link, &t.PublishedAt, &t.SeenAt); err != nil {
			return nil, err
		}
		tweets = append(tweets, t)
	}
	return tweets, nil
}

// Donator represents a tracked donor on Discord with their active role subscription.
type Donator struct {
	UserID     string
	Username   string
	TotalUSD   float64
	ExpiryTime int64
	WarnedAt   int64
	UpdatedAt  time.Time
}

// GetDonator retrieves a donator by their user ID.
func (db *DB) GetDonator(userID string) (Donator, bool) {
	db.mu.Lock()
	defer db.mu.Unlock()
	var d Donator
	var updatedAtStr string
	err := db.Conn.QueryRow(
		`SELECT user_id, username, total_usd, expiry_time, warned_at, updated_at
		 FROM donators
		 WHERE user_id = ?`,
		userID,
	).Scan(&d.UserID, &d.Username, &d.TotalUSD, &d.ExpiryTime, &d.WarnedAt, &updatedAtStr)
	if err != nil {
		return d, false
	}
	if t, err := time.Parse("2006-01-02 15:04:05", updatedAtStr); err == nil {
		d.UpdatedAt = t
	} else if t, err := time.Parse(time.RFC3339, updatedAtStr); err == nil {
		d.UpdatedAt = t
	} else {
		d.UpdatedAt = time.Now()
	}
	return d, true
}

// UpdateDonator updates or creates a donator entry.
func (db *DB) UpdateDonator(userID, username string, totalUSD float64, expiryTime int64) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	// When renewing or changing expiry, reset warned_at back to 0
	_, err := db.Conn.Exec(
		`INSERT INTO donators (user_id, username, total_usd, expiry_time, warned_at, updated_at)
		 VALUES (?, ?, ?, ?, 0, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id) DO UPDATE SET
		 	username = excluded.username,
			total_usd = excluded.total_usd,
			expiry_time = excluded.expiry_time,
			warned_at = CASE WHEN expiry_time != donators.expiry_time THEN 0 ELSE donators.warned_at END,
			updated_at = CURRENT_TIMESTAMP`,
		userID, username, totalUSD, expiryTime,
	)
	return err
}

// UpdateDonatorWarned sets the warned_at timestamp for a user.
func (db *DB) UpdateDonatorWarned(userID string, warnedAt int64) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.Conn.Exec(
		`UPDATE donators SET warned_at = ?, updated_at = CURRENT_TIMESTAMP WHERE user_id = ?`,
		warnedAt, userID,
	)
	return err
}

// GetExpiredDonators returns donators whose expiry_time is in the past (non-zero) and haven't expired.
func (db *DB) GetExpiredDonators(now int64) ([]Donator, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	rows, err := db.Conn.Query(
		`SELECT user_id, username, total_usd, expiry_time, warned_at, updated_at
		 FROM donators
		 WHERE expiry_time > 0 AND expiry_time <= ?`,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var donators []Donator
	for rows.Next() {
		var d Donator
		var updatedAtStr string
		if err := rows.Scan(&d.UserID, &d.Username, &d.TotalUSD, &d.ExpiryTime, &d.WarnedAt, &updatedAtStr); err != nil {
			return nil, err
		}
		if t, err := time.Parse("2006-01-02 15:04:05", updatedAtStr); err == nil {
			d.UpdatedAt = t
		} else if t, err := time.Parse(time.RFC3339, updatedAtStr); err == nil {
			d.UpdatedAt = t
		} else {
			d.UpdatedAt = time.Now()
		}
		donators = append(donators, d)
	}
	return donators, nil
}

// GetActiveDonators returns all active donators (expiry_time > now) sorted by expiry time.
func (db *DB) GetActiveDonators(now int64) ([]Donator, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	rows, err := db.Conn.Query(
		`SELECT user_id, username, total_usd, expiry_time, warned_at, updated_at
		 FROM donators
		 WHERE expiry_time > ?
		 ORDER BY expiry_time DESC`,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var donators []Donator
	for rows.Next() {
		var d Donator
		var updatedAtStr string
		if err := rows.Scan(&d.UserID, &d.Username, &d.TotalUSD, &d.ExpiryTime, &d.WarnedAt, &updatedAtStr); err != nil {
			return nil, err
		}
		if t, err := time.Parse("2006-01-02 15:04:05", updatedAtStr); err == nil {
			d.UpdatedAt = t
		} else if t, err := time.Parse(time.RFC3339, updatedAtStr); err == nil {
			d.UpdatedAt = t
		} else {
			d.UpdatedAt = time.Now()
		}
		donators = append(donators, d)
	}
	return donators, nil
}

// DonationLog represents a logged donation entry in the db.
type DonationLog struct {
	ID        int
	UserID    string
	Amount    float64
	Account   string
	Note      string
	CreatedAt time.Time
}

// AddDonationLog stores a detailed log of a contribution.
func (db *DB) AddDonationLog(userID string, amount float64, account, note string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.Conn.Exec(
		`INSERT INTO donation_logs (user_id, amount, account, note, created_at)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		userID, amount, account, note,
	)
	return err
}

// GetDonationLogs retrieves all logs, optionally filtered by user ID.
func (db *DB) GetDonationLogs(userID string) ([]DonationLog, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	var rows *sql.Rows
	var err error
	if userID != "" {
		rows, err = db.Conn.Query(
			`SELECT id, user_id, amount, account, note, created_at
			 FROM donation_logs
			 WHERE user_id = ?
			 ORDER BY created_at DESC`,
			userID,
		)
	} else {
		rows, err = db.Conn.Query(
			`SELECT id, user_id, amount, account, note, created_at
			 FROM donation_logs
			 ORDER BY created_at DESC`,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []DonationLog
	for rows.Next() {
		var l DonationLog
		var createdAtStr string
		if err := rows.Scan(&l.ID, &l.UserID, &l.Amount, &l.Account, &l.Note, &createdAtStr); err != nil {
			return nil, err
		}
		if t, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
			l.CreatedAt = t
		} else if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
			l.CreatedAt = t
		} else {
			l.CreatedAt = time.Now()
		}
		logs = append(logs, l)
	}
	return logs, nil
}
