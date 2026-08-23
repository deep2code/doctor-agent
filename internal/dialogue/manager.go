package dialogue

import (
	"strings"
	"sync"
)

// Manager manages multi-turn dialogue state and context.
type Manager struct {
	mu             sync.RWMutex
	sessions       map[string]*DialogueState
	intentRecognizer *IntentRecognizer
}

// NewManager creates a new dialogue manager.
func NewManager() *Manager {
	return &Manager{
		sessions:         make(map[string]*DialogueState),
		intentRecognizer: NewIntentRecognizer(),
	}
}

// GetOrCreate gets or creates a dialogue state for a session.
func (m *Manager) GetOrCreate(sessionID string) *DialogueState {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, ok := m.sessions[sessionID]; ok {
		return state
	}

	state := NewDialogueState()
	m.sessions[sessionID] = state
	return state
}

// ProcessMessage processes a user message and updates dialogue state.
func (m *Manager) ProcessMessage(sessionID, message string) *DialogueState {
	state := m.GetOrCreate(sessionID)
	state.IncrementTurn()

	// Recognize intent
	intent := m.intentRecognizer.Recognize(message)
	state.SetIntent(intent)

	// Extract entities and fill slots
	m.extractEntities(state, message)

	// State transition based on intent and context
	m.transitionState(state, message)

	return state
}

// extractEntities extracts entities from the message and fills slots.
func (m *Manager) extractEntities(state *DialogueState, message string) {
	// Extract symptoms
	symptoms := []string{"头痛", "发烧", "咳嗽", "恶心", "呕吐", "腹泻", "腹痛", "胸闷", "心慌", "失眠", "乏力"}
	for _, symptom := range symptoms {
		if strings.Contains(message, symptom) {
			state.FillSlot("symptom", symptom)
		}
	}

	// Extract body parts
	bodyParts := []string{"头", "胸", "腹", "腰", "背", "腿", "手", "脚", "眼", "耳", "鼻", "喉"}
	for _, part := range bodyParts {
		if strings.Contains(message, part) {
			state.FillSlot("body_part", part)
		}
	}

	// Extract duration patterns
	if strings.Contains(message, "今天") || strings.Contains(message, "刚才") {
		state.FillSlot("duration", "急性")
	} else if strings.Contains(message, "几天") || strings.Contains(message, "一周") {
		state.FillSlot("duration", "亚急性")
	} else if strings.Contains(message, "很久") || strings.Contains(message, "长期") || strings.Contains(message, "经常") {
		state.FillSlot("duration", "慢性")
	}
}

// transitionState transitions the dialogue state based on intent and context.
func (m *Manager) transitionState(state *DialogueState, message string) {
	switch state.CurrentIntent {
	case IntentGreeting:
		state.TransitionTo(StateResponding)
		state.NeedsFollowUp = false

	case IntentSymptom:
		if !state.AllRequiredFilled() {
			state.TransitionTo(StateGathering)
			state.NeedsFollowUp = true
		} else {
			state.TransitionTo(StateAnalyzing)
			state.NeedsFollowUp = false
		}

	case IntentDrug:
		state.AddSlot("drug_name", true)
		if state.Slots["drug_name"] != nil && !state.Slots["drug_name"].Filled {
			state.TransitionTo(StateGathering)
			state.NeedsFollowUp = true
		} else {
			state.TransitionTo(StateAnalyzing)
			state.NeedsFollowUp = false
		}

	case IntentEmergency:
		state.TransitionTo(StateResponding)
		state.NeedsFollowUp = false

	default:
		state.TransitionTo(StateResponding)
		state.NeedsFollowUp = false
	}
}

// GetFollowUpQuestion returns a follow-up question if needed.
func (m *Manager) GetFollowUpQuestion(state *DialogueState) string {
	if !state.NeedsFollowUp {
		return ""
	}

	if state.CurrentIntent == IntentSymptom {
		if state.Slots["symptom"] == nil || !state.Slots["symptom"].Filled {
			return "请问您有什么症状？"
		}
		if state.Slots["duration"] == nil || !state.Slots["duration"].Filled {
			return "这个症状持续多久了？"
		}
		if state.Slots["body_part"] == nil || !state.Slots["body_part"].Filled {
			return "请问是身体哪个部位不舒服？"
		}
	}

	if state.CurrentIntent == IntentDrug {
		if state.Slots["drug_name"] == nil || !state.Slots["drug_name"].Filled {
			return "请问您想了解哪种药物？"
		}
	}

	return ""
}

// RemoveSession removes a session's dialogue state.
func (m *Manager) RemoveSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
}
