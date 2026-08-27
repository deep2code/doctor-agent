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
		// Audit log table (管理员操作记录)
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			admin_id VARCHAR(64) NOT NULL,
			admin_username VARCHAR(255) NOT NULL,
			action VARCHAR(64) NOT NULL,
			target_type VARCHAR(64),
			target_id VARCHAR(255),
			details TEXT,
			ip_address VARCHAR(64),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// System config table (系统配置)
		`CREATE TABLE IF NOT EXISTS system_config (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			config_key VARCHAR(128) UNIQUE NOT NULL,
			config_value TEXT NOT NULL,
			description TEXT,
			updated_by VARCHAR(64),
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		)`,
		// Knowledge versions table (知识库版本)
		`CREATE TABLE IF NOT EXISTS knowledge_versions (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			dataset VARCHAR(128) NOT NULL,
			version INT NOT NULL DEFAULT 1,
			entry_count INT NOT NULL DEFAULT 0,
			checksum VARCHAR(64),
			created_by VARCHAR(64),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_dataset_version (dataset, version)
		)`,
		// API usage stats table (API 调用统计)
		`CREATE TABLE IF NOT EXISTS api_stats (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			endpoint VARCHAR(128) NOT NULL,
			method VARCHAR(16) NOT NULL,
			status_code INT,
			response_time_ms INT,
			ip_address VARCHAR(64),
			user_agent TEXT,
			error_message TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Indexes (CREATE INDEX IF NOT EXISTS is MariaDB-only; MySQL 8+ rejects it.
		// Tolerate "duplicate key name" so both engines migrate cleanly.)
		`CREATE INDEX idx_sessions_user_id ON sessions(user_id)`,
		`CREATE INDEX idx_messages_session_id ON messages(session_id)`,
		`CREATE INDEX idx_feedback_session_id ON feedback(session_id)`,
		`CREATE INDEX idx_feedback_rating ON feedback(rating)`,
		`CREATE INDEX idx_audit_logs_admin_id ON audit_logs(admin_id)`,
		`CREATE INDEX idx_audit_logs_action ON audit_logs(action)`,
		`CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at)`,
		`CREATE INDEX idx_system_config_key ON system_config(config_key)`,
		`CREATE INDEX idx_knowledge_versions_dataset ON knowledge_versions(dataset)`,
		`CREATE INDEX idx_api_stats_endpoint ON api_stats(endpoint)`,
		`CREATE INDEX idx_api_stats_created_at ON api_stats(created_at)`,
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

// --- Audit log operations ---

// AuditLogRecord represents an audit log entry.
type AuditLogRecord struct {
	ID           int64     `json:"id"`
	AdminID      string    `json:"admin_id"`
	AdminUsername string    `json:"admin_username"`
	Action       string    `json:"action"`
	TargetType   string    `json:"target_type,omitempty"`
	TargetID     string    `json:"target_id,omitempty"`
	Details      string    `json:"details,omitempty"`
	IPAddress    string    `json:"ip_address,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// AddAuditLog records an admin action.
func (db *DB) AddAuditLog(log *AuditLogRecord) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(
		`INSERT INTO audit_logs (admin_id, admin_username, action, target_type, target_id, details, ip_address) 
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		log.AdminID, log.AdminUsername, log.Action, log.TargetType, log.TargetID, log.Details, log.IPAddress,
	)
	return err
}

// ListAuditLogs lists audit logs with pagination.
func (db *DB) ListAuditLogs(limit, offset int) ([]AuditLogRecord, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	rows, err := db.conn.Query(
		`SELECT id, admin_id, admin_username, action, COALESCE(target_type,''), COALESCE(target_id,''), 
		 COALESCE(details,''), COALESCE(ip_address,''), created_at 
		 FROM audit_logs ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("Failed to close rows", "error", err)
		}
	}()

	var logs []AuditLogRecord
	for rows.Next() {
		var l AuditLogRecord
		if err := rows.Scan(&l.ID, &l.AdminID, &l.AdminUsername, &l.Action, &l.TargetType, &l.TargetID, &l.Details, &l.IPAddress, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, nil
}

// GetAuditLogsCount returns total count of audit logs.
func (db *DB) GetAuditLogsCount() (int, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var count int
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&count)
	return count, err
}

// --- System config operations ---

// SystemConfigRecord represents a system configuration entry.
type SystemConfigRecord struct {
	ID          int64     `json:"id"`
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Description string    `json:"description,omitempty"`
	UpdatedBy   string    `json:"updated_by,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GetSystemConfig retrieves a config value by key.
func (db *DB) GetSystemConfig(key string) (string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var value string
	err := db.conn.QueryRow(`SELECT config_value FROM system_config WHERE config_key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SetSystemConfig sets or updates a config value.
func (db *DB) SetSystemConfig(key, value, description, updatedBy string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(
		`INSERT INTO system_config (config_key, config_value, description, updated_by) 
		 VALUES (?, ?, ?, ?) 
		 ON DUPLICATE KEY UPDATE config_value = VALUES(config_value), description = VALUES(description), updated_by = VALUES(updated_by)`,
		key, value, description, updatedBy,
	)
	return err
}

// ListSystemConfigs lists all system configs.
func (db *DB) ListSystemConfigs() ([]SystemConfigRecord, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.conn.Query(
		`SELECT id, config_key, config_value, COALESCE(description,''), COALESCE(updated_by,''), updated_at 
		 FROM system_config ORDER BY config_key`,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("Failed to close rows", "error", err)
		}
	}()

	var configs []SystemConfigRecord
	for rows.Next() {
		var c SystemConfigRecord
		if err := rows.Scan(&c.ID, &c.Key, &c.Value, &c.Description, &c.UpdatedAt); err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}
	return configs, nil
}

// DeleteSystemConfig deletes a config by key.
func (db *DB) DeleteSystemConfig(key string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(`DELETE FROM system_config WHERE config_key = ?`, key)
	return err
}

// --- Knowledge version operations ---

// KnowledgeVersionRecord represents a knowledge version entry.
type KnowledgeVersionRecord struct {
	ID         int64     `json:"id"`
	Dataset    string    `json:"dataset"`
	Version    int       `json:"version"`
	EntryCount int       `json:"entry_count"`
	Checksum   string    `json:"checksum,omitempty"`
	CreatedBy  string    `json:"created_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// AddKnowledgeVersion records a new knowledge version.
func (db *DB) AddKnowledgeVersion(record *KnowledgeVersionRecord) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(
		`INSERT INTO knowledge_versions (dataset, version, entry_count, checksum, created_by) 
		 VALUES (?, ?, ?, ?, ?)`,
		record.Dataset, record.Version, record.EntryCount, record.Checksum, record.CreatedBy,
	)
	return err
}

// ListKnowledgeVersions lists versions for a dataset.
func (db *DB) ListKnowledgeVersions(dataset string) ([]KnowledgeVersionRecord, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.conn.Query(
		`SELECT id, dataset, version, entry_count, COALESCE(checksum,''), COALESCE(created_by,''), created_at 
		 FROM knowledge_versions WHERE dataset = ? ORDER BY version DESC`, dataset,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("Failed to close rows", "error", err)
		}
	}()

	var versions []KnowledgeVersionRecord
	for rows.Next() {
		var v KnowledgeVersionRecord
		if err := rows.Scan(&v.ID, &v.Dataset, &v.Version, &v.EntryCount, &v.Checksum, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, nil
}

// GetLatestKnowledgeVersion returns the latest version for a dataset.
func (db *DB) GetLatestKnowledgeVersion(dataset string) (*KnowledgeVersionRecord, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	v := &KnowledgeVersionRecord{}
	err := db.conn.QueryRow(
		`SELECT id, dataset, version, entry_count, COALESCE(checksum,''), COALESCE(created_by,''), created_at 
		 FROM knowledge_versions WHERE dataset = ? ORDER BY version DESC LIMIT 1`, dataset,
	).Scan(&v.ID, &v.Dataset, &v.Version, &v.EntryCount, &v.Checksum, &v.CreatedBy, &v.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return v, err
}

// ListAllKnowledgeDatasets returns all datasets with version counts.
func (db *DB) ListAllKnowledgeDatasets() (map[string]int, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.conn.Query(
		`SELECT dataset, COUNT(*) as version_count FROM knowledge_versions GROUP BY dataset ORDER BY dataset`,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("Failed to close rows", "error", err)
		}
	}()

	result := make(map[string]int)
	for rows.Next() {
		var dataset string
		var count int
		if err := rows.Scan(&dataset, &count); err != nil {
			return nil, err
		}
		result[dataset] = count
	}
	return result, nil
}

// --- API stats operations ---

// APIStatsRecord represents an API usage statistic entry.
type APIStatsRecord struct {
	ID             int64     `json:"id"`
	Endpoint       string    `json:"endpoint"`
	Method         string    `json:"method"`
	StatusCode     int       `json:"status_code,omitempty"`
	ResponseTimeMs int       `json:"response_time_ms,omitempty"`
	IPAddress      string    `json:"ip_address,omitempty"`
	UserAgent      string    `json:"user_agent,omitempty"`
	ErrorMessage   string    `json:"error_message,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// AddAPIStats records an API call statistic.
func (db *DB) AddAPIStats(stats *APIStatsRecord) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(
		`INSERT INTO api_stats (endpoint, method, status_code, response_time_ms, ip_address, user_agent, error_message) 
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		stats.Endpoint, stats.Method, stats.StatusCode, stats.ResponseTimeMs, stats.IPAddress, stats.UserAgent, stats.ErrorMessage,
	)
	return err
}

// GetAPIStatsSummary returns API usage summary (top endpoints, avg response time, error rate).
func (db *DB) GetAPIStatsSummary(hours int) (map[string]any, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if hours <= 0 {
		hours = 24
	}

	summary := make(map[string]any)

	// Total requests
	var totalRequests int
	err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM api_stats WHERE created_at >= DATE_SUB(NOW(), INTERVAL ? HOUR)`, hours,
	).Scan(&totalRequests)
	if err != nil {
		return nil, err
	}
	summary["total_requests"] = totalRequests

	// Average response time
	var avgResponseTime float64
	err = db.conn.QueryRow(
		`SELECT COALESCE(AVG(response_time_ms), 0) FROM api_stats WHERE created_at >= DATE_SUB(NOW(), INTERVAL ? HOUR) AND response_time_ms > 0`, hours,
	).Scan(&avgResponseTime)
	if err != nil {
		return nil, err
	}
	summary["avg_response_time_ms"] = int(avgResponseTime)

	// Error count (status >= 400)
	var errorCount int
	err = db.conn.QueryRow(
		`SELECT COUNT(*) FROM api_stats WHERE created_at >= DATE_SUB(NOW(), INTERVAL ? HOUR) AND status_code >= 400`, hours,
	).Scan(&errorCount)
	if err != nil {
		return nil, err
	}
	summary["error_count"] = errorCount
	if totalRequests > 0 {
		summary["error_rate"] = float64(errorCount) / float64(totalRequests) * 100
	} else {
		summary["error_rate"] = 0.0
	}

	// Top endpoints
	rows, err := db.conn.Query(
		`SELECT endpoint, COUNT(*) as count, AVG(response_time_ms) as avg_time 
		 FROM api_stats WHERE created_at >= DATE_SUB(NOW(), INTERVAL ? HOUR) 
		 GROUP BY endpoint ORDER BY count DESC LIMIT 10`, hours,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("Failed to close rows", "error", err)
		}
	}()

	type endpointStats struct {
		Endpoint string  `json:"endpoint"`
		Count    int     `json:"count"`
		AvgTime  float64 `json:"avg_time"`
	}
	var endpoints []endpointStats
	for rows.Next() {
		var e endpointStats
		if err := rows.Scan(&e.Endpoint, &e.Count, &e.AvgTime); err != nil {
			return nil, err
		}
		endpoints = append(endpoints, e)
	}
	summary["top_endpoints"] = endpoints

	// Requests per hour (last 24 hours)
	rows2, err := db.conn.Query(
		`SELECT DATE_FORMAT(created_at, '%Y-%m-%d %H:00:00') as hour, COUNT(*) as count 
		 FROM api_stats WHERE created_at >= DATE_SUB(NOW(), INTERVAL 24 HOUR) 
		 GROUP BY hour ORDER BY hour`,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows2.Close(); err != nil {
			slog.Warn("Failed to close rows", "error", err)
		}
	}()

	type hourlyStats struct {
		Hour  string `json:"hour"`
		Count int    `json:"count"`
	}
	var hourly []hourlyStats
	for rows2.Next() {
		var h hourlyStats
		if err := rows2.Scan(&h.Hour, &h.Count); err != nil {
			return nil, err
		}
		hourly = append(hourly, h)
	}
	summary["hourly_requests"] = hourly

	return summary, nil
}

// ListAPIStats lists API stats with pagination.
func (db *DB) ListAPIStats(limit, offset int) ([]APIStatsRecord, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	rows, err := db.conn.Query(
		`SELECT id, endpoint, method, status_code, response_time_ms, COALESCE(ip_address,''), 
		 COALESCE(user_agent,''), COALESCE(error_message,''), created_at 
		 FROM api_stats ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("Failed to close rows", "error", err)
		}
	}()

	var stats []APIStatsRecord
	for rows.Next() {
		var s APIStatsRecord
		if err := rows.Scan(&s.ID, &s.Endpoint, &s.Method, &s.StatusCode, &s.ResponseTimeMs, &s.IPAddress, &s.UserAgent, &s.ErrorMessage, &s.CreatedAt); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, nil
}

// --- User behavior analysis ---

// GetUserBehaviorStats returns user behavior statistics.
func (db *DB) GetUserBehaviorStats() (map[string]any, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	summary := make(map[string]any)

	// Total sessions
	var totalSessions int
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&totalSessions)
	if err != nil {
		return nil, err
	}
	summary["total_sessions"] = totalSessions

	// Total messages
	var totalMessages int
	err = db.conn.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&totalMessages)
	if err != nil {
		return nil, err
	}
	summary["total_messages"] = totalMessages

	// Average messages per session
	var avgMessages float64
	if totalSessions > 0 {
		avgMessages = float64(totalMessages) / float64(totalSessions)
	}
	summary["avg_messages_per_session"] = avgMessages

	// Top session titles (common questions)
	rows, err := db.conn.Query(
		`SELECT title, COUNT(*) as count FROM sessions 
		 WHERE title IS NOT NULL AND title != '' 
		 GROUP BY title ORDER BY count DESC LIMIT 20`,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("Failed to close rows", "error", err)
		}
	}()

	type titleStats struct {
		Title string `json:"title"`
		Count int    `json:"count"`
	}
	var titles []titleStats
	for rows.Next() {
		var t titleStats
		if err := rows.Scan(&t.Title, &t.Count); err != nil {
			return nil, err
		}
		titles = append(titles, t)
	}
	summary["top_questions"] = titles

	// Sessions by hour of day
	rows2, err := db.conn.Query(
		`SELECT HOUR(created_at) as hour, COUNT(*) as count 
		 FROM sessions GROUP BY hour ORDER BY hour`,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows2.Close(); err != nil {
			slog.Warn("Failed to close rows", "error", err)
		}
	}()

	type hourlySessions struct {
		Hour  int `json:"hour"`
		Count int `json:"count"`
	}
	var hourly []hourlySessions
	for rows2.Next() {
		var h hourlySessions
		if err := rows2.Scan(&h.Hour, &h.Count); err != nil {
			return nil, err
		}
		hourly = append(hourly, h)
	}
	summary["sessions_by_hour"] = hourly

	// Active users (last 7 days)
	var activeUsers int
	err = db.conn.QueryRow(
		`SELECT COUNT(DISTINCT user_id) FROM sessions WHERE created_at >= DATE_SUB(NOW(), INTERVAL 7 DAY) AND user_id IS NOT NULL`,
	).Scan(&activeUsers)
	if err != nil {
		return nil, err
	}
	summary["active_users_7d"] = activeUsers

	return summary, nil
}

// --- Feedback with details ---

// GetFeedbackWithDetails returns feedback with session details.
func (db *DB) GetFeedbackWithDetails(limit, offset int) ([]map[string]any, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	rows, err := db.conn.Query(
		`SELECT f.id, f.session_id, f.message_id, f.rating, COALESCE(f.comment,''), 
		 COALESCE(f.user_id,''), f.created_at, COALESCE(s.title,'') as session_title
		 FROM feedback f 
		 LEFT JOIN sessions s ON f.session_id = s.id
		 ORDER BY f.created_at DESC LIMIT ? OFFSET ?`, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("Failed to close rows", "error", err)
		}
	}()

	var results []map[string]any
	for rows.Next() {
		var id int64
		var sessionID, messageID, rating, comment, userID, sessionTitle string
		var createdAt time.Time
		if err := rows.Scan(&id, &sessionID, &messageID, &rating, &comment, &userID, &createdAt, &sessionTitle); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{
			"id":            id,
			"session_id":    sessionID,
			"message_id":    messageID,
			"rating":        rating,
			"comment":       comment,
			"user_id":       userID,
			"created_at":    createdAt,
			"session_title": sessionTitle,
		})
	}
	return results, nil
}

// GetFeedbackStatsByPeriod returns feedback stats grouped by period.
func (db *DB) GetFeedbackStatsByPeriod(period string) ([]map[string]any, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var dateFormat string
	switch period {
	case "day":
		dateFormat = "%Y-%m-%d"
	case "week":
		dateFormat = "%Y-%u"
	case "month":
		dateFormat = "%Y-%m"
	default:
		dateFormat = "%Y-%m-%d"
	}

	rows, err := db.conn.Query(
		fmt.Sprintf(`SELECT DATE_FORMAT(created_at, '%s') as period, 
		 SUM(CASE WHEN rating = 'up' THEN 1 ELSE 0 END) as up_count,
		 SUM(CASE WHEN rating = 'down' THEN 1 ELSE 0 END) as down_count,
		 COUNT(*) as total
		 FROM feedback GROUP BY period ORDER BY period DESC LIMIT 30`, dateFormat),
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("Failed to close rows", "error", err)
		}
	}()

	var results []map[string]any
	for rows.Next() {
		var period string
		var upCount, downCount, total int
		if err := rows.Scan(&period, &upCount, &downCount, &total); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{
			"period":     period,
			"up_count":   upCount,
			"down_count": downCount,
			"total":      total,
		})
	}
	return results, nil
}
