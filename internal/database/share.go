package database

import (
	"database/sql"
	"errors"
	"time"
)

// ShareSnapshot 是一条只读分享快照：整个会话或单条问答的消息副本。
// payload 为 JSON 文本（server 侧定义的结构），读取时原样返回。
type ShareSnapshot struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"` // "answer" | "session"
	Title     string    `json:"title"`
	Payload   string    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}

// ErrShareNotFound 表示分享 ID 不存在。
var ErrShareNotFound = errors.New("share not found")

// CreateShare 写入一条分享快照。
func (db *DB) CreateShare(s *ShareSnapshot) error {
	_, err := db.conn.Exec(
		`INSERT INTO shares (id, kind, title, payload) VALUES (?, ?, ?, ?)`,
		s.ID, s.Kind, s.Title, s.Payload,
	)
	return err
}

// GetShare 按 ID 读取分享快照。
func (db *DB) GetShare(id string) (*ShareSnapshot, error) {
	var s ShareSnapshot
	var title sql.NullString
	err := db.conn.QueryRow(
		`SELECT id, kind, title, payload, created_at FROM shares WHERE id = ?`, id,
	).Scan(&s.ID, &s.Kind, &title, &s.Payload, &s.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrShareNotFound
	}
	if err != nil {
		return nil, err
	}
	s.Title = title.String
	return &s, nil
}
