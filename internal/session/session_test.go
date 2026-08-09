package session

import (
	"testing"
)

func TestSessionLifecycle(t *testing.T) {
	s := New("s1")
	if s.TurnCount() != 0 {
		t.Fatalf("TurnCount = %d, want 0", s.TurnCount())
	}

	s.AddUserMessage("我喝了牛奶拉肚子")
	s.AddAssistantMessage("可能是乳糖不耐受")
	if s.TurnCount() != 1 {
		t.Fatalf("TurnCount = %d, want 1", s.TurnCount())
	}

	msgs := s.GetMessages()
	if len(msgs) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "我喝了牛奶拉肚子" {
		t.Errorf("msg[0] = %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "可能是乳糖不耐受" {
		t.Errorf("msg[1] = %+v", msgs[1])
	}

	// TrimHistory keeps the most recent turns.
	for i := 0; i < 6; i++ {
		s.AddUserMessage("u")
		s.AddAssistantMessage("a")
	}
	s.TrimHistory(3) // keep 3 turns = 6 messages
	if got := len(s.GetMessages()); got != 6 {
		t.Fatalf("after trim len = %d, want 6", got)
	}
	if s.TurnCount() != 3 {
		t.Fatalf("TurnCount after trim = %d, want 3", s.TurnCount())
	}

	// Clear resets history but keeps ID.
	s.Clear()
	if s.TurnCount() != 0 {
		t.Fatalf("TurnCount after clear = %d, want 0", s.TurnCount())
	}
	if s.ID != "s1" {
		t.Fatalf("ID = %q after clear, want s1", s.ID)
	}
}

func TestFileStoreRoundTrip(t *testing.T) {
	fs, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	s := New("conv-123")
	s.AddUserMessage("第一个问题")
	s.AddAssistantMessage("第一个回答")
	s.SetPatientContext(&PatientContext{Region: "guangdong", G6PDStatus: "deficient"})
	s.DisclaimerSent = true

	if err := fs.Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	restored, err := fs.Load("conv-123")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if restored == nil {
		t.Fatal("Load returned nil for existing session")
	}
	if restored.ID != s.ID {
		t.Errorf("ID = %q, want %q", restored.ID, s.ID)
	}
	if restored.TurnCount() != 1 {
		t.Errorf("TurnCount = %d, want 1", restored.TurnCount())
	}
	if !restored.DisclaimerSent {
		t.Error("DisclaimerSent not restored")
	}
	msgs := restored.GetMessages()
	if len(msgs) != 2 || msgs[0].Content != "第一个问题" || msgs[1].Content != "第一个回答" {
		t.Errorf("messages not restored: %+v", msgs)
	}
	pc := restored.GetPatientContext()
	if pc == nil || pc.Region != "guangdong" || pc.G6PDStatus != "deficient" {
		t.Errorf("patient context not restored: %+v", pc)
	}

	// List returns the id.
	ids, err := fs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ids) != 1 || ids[0] != "conv-123" {
		t.Errorf("List = %v, want [conv-123]", ids)
	}

	// Missing session loads as nil, nil.
	missing, err := fs.Load("nope")
	if err != nil || missing != nil {
		t.Errorf("Load(missing) = %v, %v; want nil, nil", missing, err)
	}

	// Delete removes it.
	if err := fs.Delete("conv-123"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	gone, _ := fs.Load("conv-123")
	if gone != nil {
		t.Error("session still present after Delete")
	}
}

func TestFileStoreRejectsPathTraversal(t *testing.T) {
	fs, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	for _, bad := range []string{"../evil", "a/b", "..", "a\\b", "../../etc/passwd", "", "x/y"} {
		if ValidID(bad) {
			t.Errorf("ValidID(%q) = true, want false", bad)
		}
		if _, err := fs.Load(bad); err == nil {
			t.Errorf("Load(%q) should error", bad)
		}
		if err := fs.Delete(bad); err == nil {
			t.Errorf("Delete(%q) should error", bad)
		}
	}

	// A valid ID still round-trips.
	s := New("conv-good-1")
	s.AddUserMessage("hi")
	if err := fs.Save(s); err != nil {
		t.Fatalf("Save valid id: %v", err)
	}
}

func TestFileStoreOverwrite(t *testing.T) {
	fs, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	// Saving twice with more turns must fully replace the snapshot.
	s := New("c1")
	s.AddUserMessage("1")
	s.AddAssistantMessage("1a")
	if err := fs.Save(s); err != nil {
		t.Fatalf("first Save: %v", err)
	}

	s.AddUserMessage("2")
	s.AddAssistantMessage("2a")
	if err := fs.Save(s); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	restored, _ := fs.Load("c1")
	if restored.TurnCount() != 2 {
		t.Errorf("TurnCount = %d, want 2 (snapshot must overwrite, not append)", restored.TurnCount())
	}
}
