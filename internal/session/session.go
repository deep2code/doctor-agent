package session

import (
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// Session manages a single conversation's state and message history.
type Session struct {
	mu             sync.RWMutex
	ID             string
	Messages       []anthropic.MessageParam
	ContextSummary string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	PatientContext *PatientContext
	DisclaimerSent bool
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
		Messages:  make([]anthropic.MessageParam, 0),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// AddUserMessage appends a user text message to the session.
func (s *Session) AddUserMessage(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Messages = append(s.Messages,
		anthropic.NewUserMessage(anthropic.NewTextBlock(content)))
	s.UpdatedAt = time.Now()
}

// AddAssistantMessage appends an assistant text response to the session.
func (s *Session) AddAssistantMessage(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Messages = append(s.Messages,
		anthropic.NewAssistantMessage(anthropic.NewTextBlock(content)))
	s.UpdatedAt = time.Now()
}

// AddToolUseTurn appends an assistant tool_use block and its tool_result.
func (s *Session) AddToolUseTurn(
	toolBlocks []anthropic.ContentBlockParamUnion,
	toolResults []anthropic.ContentBlockParamUnion,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Messages = append(s.Messages, anthropic.NewAssistantMessage(toolBlocks...))
	s.Messages = append(s.Messages, anthropic.NewUserMessage(toolResults...))
	s.UpdatedAt = time.Now()
}

// GetMessages returns a copy of the current message history.
func (s *Session) GetMessages() []anthropic.MessageParam {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msgs := make([]anthropic.MessageParam, len(s.Messages))
	copy(msgs, s.Messages)
	return msgs
}

// TrimHistory keeps only the most recent N turns.
func (s *Session) TrimHistory(maxTurns int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if maxTurns <= 0 || len(s.Messages) <= maxTurns*2 {
		return
	}

	keep := maxTurns * 2
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
