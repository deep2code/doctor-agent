package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/doctor-agent/internal/agent"
	"github.com/doctor-agent/internal/database"
	"github.com/doctor-agent/internal/config"
	"github.com/doctor-agent/internal/session"
)

// TestSessionAPIEndToEnd exercises the /sessions REST API against a real
// MariaDB/MySQL instance (started locally on port 3307 for this test).
// Skip if that instance is not reachable.
func TestSessionAPIEndToEnd(t *testing.T) {
	// Point the app DB at the local scratch MySQL started for manual tests.
	os.Setenv("MARIA_DB_HOST", "127.0.0.1")
	os.Setenv("MARIA_DB_PORT", "3307")
	os.Setenv("MARIA_DB_USER", "root")
	os.Setenv("MARIA_DB_PASSWORD", "")
	os.Setenv("MARIA_DB_APP_DB", "doctor_agent_test_sess")
	os.Setenv("MARIA_DB_KNOWLEDGE_DB", "doctor_knowledge")
	os.Setenv("LLM_PROVIDER", "anthropic")
	os.Setenv("ANTHROPIC_API_KEY", "test-key")

	cfg := config.Load()
	if err := cfg.EnsureAppDB(); err != nil {
		t.Skipf("cannot create app db: %v", err)
	}
	if err := cfg.EnsureKnowledgeDB(); err != nil {
		t.Skipf("cannot create knowledge db: %v", err)
	}
	db, err := database.New(database.Config{DSN: cfg.AppDBDSN()})
	if err != nil {
		t.Skipf("no local MySQL: %v", err)
	}
	defer db.Close()

	// Fresh start: clear sessions table.
	_ = db.DeleteSession("conv-e2e-001")
	_ = db.DeleteSession("conv-e2e-002")

	// Build a dedicated agent bound to the scratch DB env above (do NOT reuse
	// sharedAgent, whose once-cached instance points at the default 3306).
	ag, err := agent.New(cfg)
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	ag.SetSessionStore(session.NewDBStore(db))

	s := NewWithDB(baseCfg(), ag, nil, db)
	doReq := func(method, path string) (*httptest.ResponseRecorder, string) {
		var r *http.Request
		if method == http.MethodDelete {
			r = httptest.NewRequest(method, path, nil)
		} else {
			r = httptest.NewRequest(method, path, nil)
		}
		w := httptest.NewRecorder()
		if strings.HasPrefix(path, "/sessions/") {
			s.handleSessionByID(w, r)
		} else {
			s.handleSessions(w, r)
		}
		body, _ := io.ReadAll(w.Result().Body)
		return w, string(body)
	}

	// Empty list initially
	w, body := doReq(http.MethodGet, "/sessions")
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d", w.Code)
	}
	if !strings.Contains(body, `"sessions":[]`) && !strings.Contains(body, `"sessions": [`) {
		t.Fatalf("expected empty list, got %s", body)
	}

	// Persist a session directly (simulating chat completion)
	sess := session.New("conv-e2e-001")
	sess.AddUserMessage("我一喝牛奶就拉肚子")
	sess.AddAssistantMessage("可能是乳糖不耐受")
	if err := db.CreateSession(&database.SessionRecord{ID: "conv-e2e-001", Title: "我一喝牛奶就拉肚子"}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := db.AddMessage(&database.MessageRecord{SessionID: "conv-e2e-001", Role: "user", Content: "我一喝牛奶就拉肚子"}); err != nil {
		t.Fatalf("add msg: %v", err)
	}
	if err := db.AddMessage(&database.MessageRecord{SessionID: "conv-e2e-001", Role: "assistant", Content: "可能是乳糖不耐受"}); err != nil {
		t.Fatalf("add msg2: %v", err)
	}

	// List contains it
	_, body = doReq(http.MethodGet, "/sessions")
	if !strings.Contains(body, "conv-e2e-001") || !strings.Contains(body, "我一喝牛奶就拉肚子") {
		t.Fatalf("list missing session: %s", body)
	}

	// Get messages
	w, body = doReq(http.MethodGet, "/sessions/conv-e2e-001")
	if w.Code != http.StatusOK {
		t.Fatalf("get status = %d body=%s", w.Code, body)
	}
	var got struct {
		ID       string `json:"id"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	if got.ID != "conv-e2e-001" || len(got.Messages) != 2 || got.Messages[1].Content != "可能是乳糖不耐受" {
		t.Fatalf("unexpected get response: %s", body)
	}

	// Invalid id rejected
	w, _ = doReq(http.MethodGet, "/sessions/..%2f..%2fetc%2fpasswd")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid id status = %d, want 400", w.Code)
	}

	// Delete
	w, _ = doReq(http.MethodDelete, "/sessions/conv-e2e-001")
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", w.Code)
	}
	_, body = doReq(http.MethodGet, "/sessions")
	if strings.Contains(body, "conv-e2e-001") {
		t.Fatalf("session still listed after delete: %s", body)
	}
}
