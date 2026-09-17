package session

import (
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/database"
	"github.com/doctor-agent/internal/llm"
)

// DBStore persists sessions to the MariaDB application database.
type DBStore struct {
	db *database.DB
}

// NewDBStore creates a new database-backed session store.
func NewDBStore(db *database.DB) *DBStore {
	return &DBStore{db: db}
}

// Save persists a session to the database.
func (s *DBStore) Save(sess *Session) error {
	sess.mu.RLock()
	messages := make([]llm.Message, len(sess.Messages))
	copy(messages, sess.Messages)
	sess.mu.RUnlock()

	// Check if session exists
	existing, err := s.db.GetSession(sess.ID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}

	if existing == nil {
		// Create new session, deriving the title from the first user message.
		err = s.db.CreateSession(&database.SessionRecord{
			ID:     sess.ID,
			Title:  titleOf(messages),
			UserID: "",
		})
		if err != nil {
			return fmt.Errorf("creating session: %w", err)
		}
	} else if t := titleOf(messages); t != "" && existing.Title != t {
		// Keep the stored title fresh (first user message usually set at creation,
		// but a session created without messages gets its title here).
		_ = s.db.UpdateSessionTitle(sess.ID, t)
	}

	// Get existing message count
	existingMsgs, err := s.db.GetSessionMessages(sess.ID)
	if err != nil {
		return fmt.Errorf("getting messages: %w", err)
	}

	// Add new messages
	if len(messages) > len(existingMsgs) {
		for i := len(existingMsgs); i < len(messages); i++ {
			msg := messages[i]
			err = s.db.AddMessage(&database.MessageRecord{
				SessionID: sess.ID,
				Role:      msg.Role,
				Content:   msg.Content,
			})
			if err != nil {
				return fmt.Errorf("adding message: %w", err)
			}
		}
	}

	return nil
}

// Load reads a session from the database.
func (s *DBStore) Load(id string) (*Session, error) {
	record, err := s.db.GetSession(id)
	if err != nil {
		return nil, fmt.Errorf("getting session: %w", err)
	}
	if record == nil {
		return nil, nil
	}

	// Get messages
	messages, err := s.db.GetSessionMessages(id)
	if err != nil {
		return nil, fmt.Errorf("getting messages: %w", err)
	}

	// Convert to llm.Message format
	sess := &Session{
		ID:             record.ID,
		ContextSummary: record.Title,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	}

	for _, msg := range messages {
		sess.Messages = append(sess.Messages, llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return sess, nil
}

// List returns all persisted session IDs, most recently updated first.
func (s *DBStore) List() ([]string, error) {
	recs, err := s.db.ListAllSessions(200)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	ids := make([]string, 0, len(recs))
	for _, r := range recs {
		ids = append(ids, r.ID)
	}
	return ids, nil
}

// Delete removes a session from the database.
func (s *DBStore) Delete(id string) error {
	return s.db.DeleteSession(id)
}

// Ensure DBStore has the same interface as FileStore
var _ Store = (*DBStore)(nil)

// Store is the interface for session persistence.
type Store interface {
	Save(session *Session) error
	Load(id string) (*Session, error)
	List() ([]string, error)
	Delete(id string) error
}

// titleOf derives a conversation title from the first user message.
func titleOf(messages []llm.Message) string {
	for _, m := range messages {
		if m.Role == "user" {
			t := strings.TrimSpace(m.Content)
			if t == "" {
				continue
			}
			// Keep titles short for the sidebar.
			runes := []rune(t)
			if len(runes) > 24 {
				t = string(runes[:24]) + "…"
			}
			return t
		}
	}
	return ""
}
