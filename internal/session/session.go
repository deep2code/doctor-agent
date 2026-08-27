package session

import (
	"sync"
	"time"

	"github.com/doctor-agent/internal/llm"
)

// Session manages a single conversation's state and message history.
// Fields are JSON-serializable so sessions can be persisted to disk.
type Session struct {
	mu             sync.RWMutex `json:"-"`
	ID             string       `json:"id"`
	Messages       []llm.Message `json:"messages"`
	ContextSummary string       `json:"context_summary,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	PatientContext *PatientContext `json:"patient_context,omitempty"`
	DisclaimerSent bool         `json:"disclaimer_sent"`
}

// PatientContext holds optional structured patient-level context
// that informs risk assessment. This is intentionally high-level;
// no real personally identifiable information is stored.
type PatientContext struct {
	AgeGroup           string   `json:"age_group,omitempty"`
	G6PDStatus         string   `json:"g6pd_status,omitempty"`
	ThalassemiaTrait   string   `json:"thalassemia_trait,omitempty"`
	HBVStatus          string   `json:"hbv_status,omitempty"`
	Region             string   `json:"region,omitempty"`
	KnownAllergies     []string `json:"known_allergies,omitempty"`
	KnownConditions    []string `json:"known_conditions,omitempty"`
	CurrentMedications []string `json:"current_medications,omitempty"`
}

// New creates a new conversation session.
func New(id string) *Session {
	now := time.Now()
	return &Session{
		ID:        id,
		Messages:  make([]llm.Message, 0),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// AddUserMessage appends a user text message to the session.
func (s *Session) AddUserMessage(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Messages = append(s.Messages, llm.Message{Role: "user", Content: content})
	s.UpdatedAt = time.Now()
}

// AddAssistantMessage appends an assistant text response to the session.
func (s *Session) AddAssistantMessage(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Messages = append(s.Messages, llm.Message{Role: "assistant", Content: content})
	s.UpdatedAt = time.Now()
}

// GetMessages returns a copy of the current message history.
func (s *Session) GetMessages() []llm.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msgs := make([]llm.Message, len(s.Messages))
	copy(msgs, s.Messages)
	return msgs
}

// Clear resets the conversation history (keeps ID and patient context).
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = make([]llm.Message, 0)
	s.DisclaimerSent = false
	s.UpdatedAt = time.Now()
}

// TrimHistory keeps only the most recent N turns.
func (s *Session) TrimHistory(maxTurns int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if maxTurns <= 0 || len(s.Messages) <= maxTurns*2 {
		return
	}

	keep := maxTurns * 2
	if keep <= 0 {
		return
	}
	s.Messages = s.Messages[len(s.Messages)-keep:]
}

// SetPatientContext updates the patient context.
func (s *Session) SetPatientContext(pc *PatientContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PatientContext = pc
}

// GetPatientContext returns the current patient context.
func (s *Session) GetPatientContext() *PatientContext {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.PatientContext
}

// TurnCount returns the number of conversation turns.
func (s *Session) TurnCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Messages) / 2
}
