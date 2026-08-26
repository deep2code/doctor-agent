package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
)

// DB wraps the MariaDB database connection.
type DB struct {
	conn *sql.DB
	mu   sync.RWMutex
}

// Config holds database configuration.
type Config struct {
	DSN string // Go MySQL driver DSN for the application database
}

// New creates a new database connection and initializes tables.
func New(cfg Config) (*DB, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("database DSN is required")
	}

	conn, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Test connection
	if err := conn.Ping(); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			slog.Warn("Failed to close database connection after ping error", "error", closeErr)
		}
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	db := &DB{conn: conn}

	// Initialize tables
	if err := db.migrate(); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			slog.Warn("Failed to close database connection after migrate error", "error", closeErr)
		}
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	slog.Info("Database initialized", "dsn", cfg.DSN)
	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.conn.Close()
}

// migrate creates all required tables.
func (db *DB) migrate() error {
	queries := []string{
		// Users table
		`CREATE TABLE IF NOT EXISTS users (
			id VARCHAR(64) PRIMARY KEY,
			username VARCHAR(255) UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			nickname TEXT,
			phone VARCHAR(64),
			email VARCHAR(255),
			is_admin BOOLEAN DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_login_at DATETIME
		)`,
		// Sessions table
		`CREATE TABLE IF NOT EXISTS sessions (
			id VARCHAR(64) PRIMARY KEY,
			user_id VARCHAR(64),
			title TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		// Messages table
		`CREATE TABLE IF NOT EXISTS messages (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			session_id VARCHAR(64) NOT NULL,
			role VARCHAR(32) NOT NULL,
			content TEXT NOT NULL,
			tool_calls TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
		)`,
		// Feedback table
		`CREATE TABLE IF NOT EXISTS feedback (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			session_id VARCHAR(64),
			message_id VARCHAR(64),
			rating VARCHAR(8) NOT NULL CHECK(rating IN ('up', 'down')),
			comment TEXT,
			user_id VARCHAR(64),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Indexes (CREATE INDEX IF NOT EXISTS is MariaDB-only; MySQL 8+ rejects it.
		// Tolerate "duplicate key name" so both engines migrate cleanly.)
		`CREATE INDEX idx_sessions_user_id ON sessions(user_id)`,
		`CREATE INDEX idx_messages_session_id ON messages(session_id)`,
		`CREATE INDEX idx_feedback_session_id ON feedback(session_id)`,
		`CREATE INDEX idx_feedback_rating ON feedback(rating)`,
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			// Ignore duplicate-index errors on re-runs (error 1061: Duplicate key name).
			if isDuplicateIndexError(err) {
				continue
			}
			return fmt.Errorf("executing migration: %w", err)
		}
	}

	return nil
}

// isDuplicateIndexError reports whether err is MySQL/MariaDB error 1061
// (ER_DUP_KEYNAME, "Duplicate key name") — raised when CREATE INDEX targets an
// index that already exists.
func isDuplicateIndexError(err error) bool {
	if err == nil {
		return false
	}
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number == 1061
	}
	return false
}

// Health checks database connectivity.
func (db *DB) Health() error {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.conn.Ping()
}

// Stats returns database statistics.
func (db *DB) Stats() sql.DBStats {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.conn.Stats()
}

// --- User operations ---

// User represents a user record.
type User struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	Nickname     string     `json:"nickname"`
	Phone        string     `json:"phone,omitempty"`
	Email        string     `json:"email,omitempty"`
	IsAdmin      bool       `json:"is_admin"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

// CreateUser creates a new user.
func (db *DB) CreateUser(user *User) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(
		`INSERT INTO users (id, username, password_hash, nickname, phone, email, is_admin) 
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		user.ID, user.Username, user.PasswordHash, user.Nickname, user.Phone, user.Email, user.IsAdmin,
	)
	return err
}

// GetUser retrieves a user by ID.
func (db *DB) GetUser(id string) (*User, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	user := &User{}
	err := db.conn.QueryRow(
		`SELECT id, username, password_hash, nickname, phone, email, is_admin, created_at, updated_at, last_login_at 
		 FROM users WHERE id = ?`, id,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Nickname, &user.Phone, &user.Email,
		&user.IsAdmin, &user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return user, err
}

// GetUserByUsername retrieves a user by username.
func (db *DB) GetUserByUsername(username string) (*User, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	user := &User{}
	err := db.conn.QueryRow(
		`SELECT id, username, password_hash, nickname, phone, email, is_admin, created_at, updated_at, last_login_at 
		 FROM users WHERE username = ?`, username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Nickname, &user.Phone, &user.Email,
		&user.IsAdmin, &user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return user, err
}

// UpdateUserLastLogin updates the last login timestamp.
func (db *DB) UpdateUserLastLogin(id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(
		`UPDATE users SET last_login_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id,
	)
	return err
}

// DeleteUser deletes a user by ID.
func (db *DB) DeleteUser(id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

// --- Session operations ---

// SessionRecord represents a session record.
type SessionRecord struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id,omitempty"`
	Title     string    `json:"title,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateSession creates a new session.
func (db *DB) CreateSession(session *SessionRecord) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Anonymous chat-UI sessions have no user: store NULL so the FK to
	// users(id) is not violated (an empty string would fail).
	userID := any(nil)
	if session.UserID != "" {
		userID = session.UserID
	}
	_, err := db.conn.Exec(
		`INSERT INTO sessions (id, user_id, title) VALUES (?, ?, ?)`,
		session.ID, userID, session.Title,
	)
	return err
}

// GetSession retrieves a session by ID.
func (db *DB) GetSession(id string) (*SessionRecord, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	session := &SessionRecord{}
	err := db.conn.QueryRow(
		`SELECT id, COALESCE(user_id,''), title, created_at, updated_at FROM sessions WHERE id = ?`, id,
	).Scan(&session.ID, &session.UserID, &session.Title, &session.CreatedAt, &session.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return session, err
}

// UpdateSessionTitle updates the session title.
func (db *DB) UpdateSessionTitle(id, title string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(
		`UPDATE sessions SET title = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, title, id,
	)
	return err
}

// ListUserSessions lists all sessions for a user.
func (db *DB) ListUserSessions(userID string, limit int) ([]SessionRecord, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	rows, err := db.conn.Query(
		`SELECT id, COALESCE(user_id,''), title, created_at, updated_at 
		 FROM sessions WHERE user_id = ? ORDER BY updated_at DESC LIMIT ?`, userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("Failed to close rows", "error", err)
		}
	}()

	var sessions []SessionRecord
	for rows.Next() {
		var s SessionRecord
		if err := rows.Scan(&s.ID, &s.UserID, &s.Title, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// ListAllSessions lists all sessions (anonymous chat UI), most recently
// updated first, capped at limit.
func (db *DB) ListAllSessions(limit int) ([]SessionRecord, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	rows, err := db.conn.Query(
		`SELECT id, COALESCE(user_id,''), title, created_at, updated_at 
		 FROM sessions ORDER BY updated_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("Failed to close rows", "error", err)
		}
	}()

	var sessions []SessionRecord
	for rows.Next() {
		var s SessionRecord
		if err := rows.Scan(&s.ID, &s.UserID, &s.Title, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// DeleteSession deletes a session and its messages.
func (db *DB) DeleteSession(id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// --- Message operations ---

// MessageRecord represents a message record.
type MessageRecord struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	ToolCalls string    `json:"tool_calls,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AddMessage adds a message to a session.
func (db *DB) AddMessage(msg *MessageRecord) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	result, err := db.conn.Exec(
		`INSERT INTO messages (session_id, role, content, tool_calls) VALUES (?, ?, ?, ?)`,
		msg.SessionID, msg.Role, msg.Content, msg.ToolCalls,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	msg.ID = id

	// Update session timestamp
	_, _ = db.conn.Exec(`UPDATE sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, msg.SessionID)
	return nil
}

// GetSessionMessages retrieves all messages for a session.
func (db *DB) GetSessionMessages(sessionID string) ([]MessageRecord, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.conn.Query(
		`SELECT id, session_id, role, content, tool_calls, created_at 
		 FROM messages WHERE session_id = ? ORDER BY created_at ASC`, sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("Failed to close rows", "error", err)
		}
	}()

	var messages []MessageRecord
	for rows.Next() {
		var m MessageRecord
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.ToolCalls, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, nil
}

// --- Feedback operations ---

// FeedbackRecord represents a feedback record.
type FeedbackRecord struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id,omitempty"`
	MessageID string    `json:"message_id,omitempty"`
	Rating    string    `json:"rating"`
	Comment   string    `json:"comment,omitempty"`
	UserID    string    `json:"user_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AddFeedback adds user feedback.
func (db *DB) AddFeedback(feedback *FeedbackRecord) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	result, err := db.conn.Exec(
		`INSERT INTO feedback (session_id, message_id, rating, comment, user_id) VALUES (?, ?, ?, ?, ?)`,
		feedback.SessionID, feedback.MessageID, feedback.Rating, feedback.Comment, feedback.UserID,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	feedback.ID = id
	return nil
}

// GetFeedbackStats returns feedback statistics.
func (db *DB) GetFeedbackStats() (up int, down int, err error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	err = db.conn.QueryRow(`SELECT COUNT(*) FROM feedback WHERE rating = 'up'`).Scan(&up)
	if err != nil {
		return
	}
	err = db.conn.QueryRow(`SELECT COUNT(*) FROM feedback WHERE rating = 'down'`).Scan(&down)
	return
}
