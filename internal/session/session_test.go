package session

import (
	"fmt"
	"sync"
	"sync/atomic"
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

// TestAddEmptyAssistantMessageRejected verifies that an assistant message with
// empty content is NOT stored. An empty assistant message would be serialized
// as {"role":"assistant"} (content and tool_calls both omitted via omitempty),
// which OpenAI-compatible endpoints reject with HTTP 400
// "content or tool_calls must be set" on the subsequent turn.
func TestAddEmptyAssistantMessageRejected(t *testing.T) {
	s := New("s-empty")
	s.AddUserMessage("你好")
	s.AddAssistantMessage("") // must be rejected

	msgs := s.GetMessages()
	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d, want 1 (empty assistant message must not be stored)", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("msgs[0].Role = %q, want user", msgs[0].Role)
	}
	if s.TurnCount() != 0 {
		t.Errorf("TurnCount = %d, want 0", s.TurnCount())
	}

	// A subsequent non-empty assistant message is stored normally.
	s.AddAssistantMessage("你好，有什么可以帮您？")
	if got := len(s.GetMessages()); got != 2 {
		t.Fatalf("after valid assistant len(msgs) = %d, want 2", got)
	}
}

// TestClaimOwner is the access-control core of /chat, /sessions and /share: the
// conversation id is caller-supplied, so ownership has to be decided here.
func TestClaimOwner(t *testing.T) {
	s := New("conv-owned")
	if s.Owner() != "" {
		t.Fatalf("Owner = %q, want empty for a fresh session", s.Owner())
	}

	// First claimant binds the conversation.
	if !s.ClaimOwner("user-a") {
		t.Fatal("first claim by user-a refused")
	}
	if s.Owner() != "user-a" {
		t.Fatalf("Owner = %q, want user-a", s.Owner())
	}
	if !s.ClaimOwner("user-a") {
		t.Error("owner re-claiming its own session refused")
	}
	// Nobody else gets in, and the stored owner never changes.
	for _, other := range []string{"user-b", ""} {
		if s.ClaimOwner(other) {
			t.Errorf("claim by %q accepted on a session owned by user-a", other)
		}
		if s.Owner() != "user-a" {
			t.Fatalf("Owner = %q after a failed claim, want user-a", s.Owner())
		}
	}

	// An empty owner means "unclaimed", not "owned by the anonymous bucket": a
	// conversation that starts before login can be claimed later by the user
	// who continues it (see the owner backfill in DBStore.Save). That is safe
	// only because ids are unguessable — /chat's conversation_id is a
	// credential, so it must come from crypto-random bytes on the client.
	anon := New("conv-anon")
	if !anon.ClaimOwner("") {
		t.Error("anonymous claim on an unclaimed session refused")
	}
	if anon.Owner() != "" {
		t.Fatalf("Owner = %q, want still unclaimed", anon.Owner())
	}
	if !anon.ClaimOwner("user-a") {
		t.Error("user-a could not claim an unclaimed session")
	}
	if anon.ClaimOwner("") {
		t.Error("anonymous caller kept access after user-a claimed the session")
	}
}

// TestClaimOwnerConcurrentFirstClaim pins the atomicity claim: binding and the
// ownership check share one write lock, so exactly one of two racing
// first-writers may proceed.
func TestClaimOwnerConcurrentFirstClaim(t *testing.T) {
	const goroutines = 32
	s := New("conv-race")
	var winners int64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			if s.ClaimOwner(fmt.Sprintf("user-%d", i)) {
				atomic.AddInt64(&winners, 1)
			}
		}(i)
	}
	wg.Wait()

	if winners != 1 {
		t.Errorf("winners = %d, want exactly 1 (owner: %q)", winners, s.Owner())
	}
}
