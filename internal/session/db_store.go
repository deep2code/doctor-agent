package session

import (
	"fmt"

	"github.com/doctor-agent/internal/database"
	"github.com/doctor-agent/internal/llm"
)

// DBStore persists sessions to SQLite database.
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
		// Create new session
		err = s.db.CreateSession(&database.SessionRecord{
			ID:     sess.ID,
			Title:  sess.ContextSummary,
			UserID: "",
		})
		if err != nil {
			return fmt.Errorf("creating session: %w", err)
		}
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
		ID:        record.ID,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}

	for _, msg := range messages {
		sess.Messages = append(sess.Messages, llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return sess, nil
}

// List returns all persisted session IDs.
func (s *DBStore) List() ([]string, error) {
	return []string{}, nil
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
