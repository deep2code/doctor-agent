package session

import (
	"encoding/json"
	"fmt"
	"log/slog"
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
	state := dbSessionState{DisclaimerSent: sess.DisclaimerSent}
	if sess.PatientContext != nil {
		pc := *sess.PatientContext
		state.PatientContext = &pc
	}
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

	// Rewrite the full transcript instead of appending: in-memory history is
	// trimmed by TrimHistory and wiped by Clear, so the stored rows stop
	// being a prefix of the message list and an incremental append would
	// silently persist nothing after the first trim (and a reload would
	// resurrect cleared history).
	rows := make([]database.MessageRecord, 0, len(messages))
	for _, msg := range messages {
		rows = append(rows, database.MessageRecord{
			SessionID: sess.ID,
			Role:      msg.Role,
			Content:   msg.Content,
		})
	}
	if err := s.db.ReplaceSessionMessages(sess.ID, rows); err != nil {
		return fmt.Errorf("rewriting messages: %w", err)
	}

	// Persist snapshot state (patient context / disclaimer flag) so restored
	// sessions keep allergy, G6PD and thalassemia context.
	if encoded, err := json.Marshal(state); err == nil {
		if err := s.db.SetSessionState(sess.ID, string(encoded)); err != nil {
			slog.Warn("Failed to persist session state", "id", sess.ID, "error", err)
		}
	}

	return nil
}

// dbSessionState is the DBStore snapshot of non-message session state.
type dbSessionState struct {
	PatientContext *PatientContext `json:"patient_context,omitempty"`
	DisclaimerSent bool            `json:"disclaimer_sent,omitempty"`
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

	// Restore snapshot state (patient context / disclaimer flag).
	if record.State != "" {
		var state dbSessionState
		if err := json.Unmarshal([]byte(record.State), &state); err != nil {
			slog.Warn("Failed to parse session state, restoring messages only", "id", id, "error", err)
		} else {
			sess.PatientContext = state.PatientContext
			sess.DisclaimerSent = state.DisclaimerSent
		}
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
	recs, err := s.db.ListAllSessions(200, 0)
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
