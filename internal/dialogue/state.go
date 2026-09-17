package dialogue

// State represents the dialogue state.
type State string

const (
	StateIdle       State = "idle"        // 空闲状态
	StateGathering  State = "gathering"   // 信息收集中
	StateAnalyzing  State = "analyzing"   // 分析中
	StateResponding State = "responding"  // 回复中
	StateFollowUp   State = "follow_up"   // 追问中
	StateClosing    State = "closing"     // 结束中
)

// Slot represents an information slot to fill.
type Slot struct {
	Name     string
	Value    string
	Required bool
	Filled   bool
}

// DialogueState manages the current state of a conversation.
type DialogueState struct {
	CurrentState    State
	CurrentIntent   Intent
	Slots           map[string]*Slot
	TurnCount       int
	NeedsFollowUp   bool
	ContextSummary  string
}

// NewDialogueState creates a new dialogue state.
func NewDialogueState() *DialogueState {
	return &DialogueState{
		CurrentState: StateIdle,
		Slots:        make(map[string]*Slot),
	}
}

// TransitionTo moves to a new state.
func (ds *DialogueState) TransitionTo(newState State) {
	ds.CurrentState = newState
}

// SetIntent sets the current intent.
func (ds *DialogueState) SetIntent(intent Intent) {
	ds.CurrentIntent = intent
}

// AddSlot adds a slot to track.
func (ds *DialogueState) AddSlot(name string, required bool) {
	ds.Slots[name] = &Slot{Name: name, Required: required}
}

// FillSlot fills a slot with a value.
func (ds *DialogueState) FillSlot(name, value string) {
	if slot, ok := ds.Slots[name]; ok {
		slot.Value = value
		slot.Filled = true
	}
}

// AllRequiredFilled checks if all required slots are filled.
func (ds *DialogueState) AllRequiredFilled() bool {
	for _, slot := range ds.Slots {
		if slot.Required && !slot.Filled {
			return false
		}
	}
	return true
}

// IncrementTurn increments the turn counter.
func (ds *DialogueState) IncrementTurn() {
	ds.TurnCount++
}

// ShouldClose determines if the conversation should end.
func (ds *DialogueState) ShouldClose() bool {
	return ds.TurnCount > 20 || ds.CurrentState == StateClosing
}
