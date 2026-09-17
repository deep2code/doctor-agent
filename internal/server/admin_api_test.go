package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/doctor-agent/internal/agent"
	"github.com/doctor-agent/internal/auth"
	"github.com/doctor-agent/internal/config"
	"github.com/doctor-agent/internal/database"
	"github.com/doctor-agent/internal/knowledge"
	"github.com/doctor-agent/internal/session"
)

// TestAdminKnowledgeAPI verifies the admin console flow: admin login via
// Basic auth, upload a medical knowledge dataset, and per-dataset stats.
func TestAdminKnowledgeAPI(t *testing.T) {
	os.Setenv("MARIA_DB_HOST", "127.0.0.1")
	os.Setenv("MARIA_DB_PORT", "3307")
	os.Setenv("MARIA_DB_USER", "root")
	os.Setenv("MARIA_DB_PASSWORD", "")
	os.Setenv("MARIA_DB_APP_DB", "doctor_agent_test_admin")
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

	// Create the admin user (admin / admin123) like createInitialAdmin does.
	authSvc := auth.NewService(db)
	if existing, _ := db.GetUserByUsername("admin"); existing != nil {
		_ = db.DeleteUser(existing.ID)
	}
	if _, err := authSvc.AdminCreateUser(&auth.AdminCreateUserInput{
		Username: "admin",
		Password: "admin123",
		IsAdmin:  true,
	}, &database.User{ID: "bootstrap", Username: "bootstrap", IsAdmin: true}); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	ag, err := agent.New(cfg)
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	ag.SetSessionStore(session.NewDBStore(db))

	s := NewWithDB(cfg, ag, authSvc, db)
	basic := "Basic " + base64Std("admin:admin123")

	// 1. Stats without auth → 401
	w := httptest.NewRecorder()
	s.handleAdminKnowledgeStats(w, httptest.NewRequest(http.MethodGet, "/admin/knowledge/stats", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("stats without auth = %d, want 401", w.Code)
	}

	// 2. Stats with auth → ok, empty or partial
	req := httptest.NewRequest(http.MethodGet, "/admin/knowledge/stats", nil)
	req.Header.Set("Authorization", basic)
	w = httptest.NewRecorder()
	s.handleAdminKnowledgeStats(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("stats with auth = %d body=%s", w.Code, w.Body.String())
	}

	// 3. Upload a small medical dataset
	medicalJSON := `[{"id":"test-hp","condition_zh":"幽门螺杆菌感染测试条目","category":"消化内科",
	  "summary":"测试用医学条目，验证管理后台上传。",
	  "keywords":["幽门螺杆菌","测试"],"citations":[]}]`
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "hp_infection.json")
	fw.Write([]byte(medicalJSON))
	mw.Close()

	req = httptest.NewRequest(http.MethodPost, "/admin/knowledge", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", basic)
	w = httptest.NewRecorder()
	s.handleAdminKnowledge(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("upload = %d body=%s", w.Code, w.Body.String())
	}
	var up struct {
		Dataset string `json:"dataset"`
		Rows    int    `json:"rows"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &up); err != nil {
		t.Fatalf("upload unmarshal: %v", err)
	}
	if up.Dataset != "medical" || up.Rows != 1 {
		t.Fatalf("upload result = %+v, want medical/1", up)
	}

	// 4. Stats now show the medical dataset
	req = httptest.NewRequest(http.MethodGet, "/admin/knowledge/stats", nil)
	req.Header.Set("Authorization", basic)
	w = httptest.NewRecorder()
	s.handleAdminKnowledgeStats(w, req)
	var st struct {
		Stats map[string]int `json:"stats"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("stats unmarshal: %v", err)
	}
	if st.Stats["medical"] != 1 {
		t.Fatalf("medical count = %d, want 1 (stats=%v)", st.Stats["medical"], st.Stats)
	}

	// 5. Verify the store actually serves the uploaded entry.
	store, err := knowledge.Load()
	if err != nil {
		t.Fatalf("knowledge.Load: %v", err)
	}
	e := store.GetMedicalByID("test-hp")
	if e == nil || e.ConditionZH != "幽门螺杆菌感染测试条目" {
		t.Fatalf("uploaded entry not found in store: %+v", e)
	}

	// 6. Invalid file rejected
	var buf2 bytes.Buffer
	mw2 := multipart.NewWriter(&buf2)
	fw2, _ := mw2.CreateFormFile("file", "random_stuff.json")
	fw2.Write([]byte(`{"foo":"bar"}`))
	mw2.Close()
	req = httptest.NewRequest(http.MethodPost, "/admin/knowledge", &buf2)
	req.Header.Set("Content-Type", mw2.FormDataContentType())
	req.Header.Set("Authorization", basic)
	w = httptest.NewRecorder()
	s.handleAdminKnowledge(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid upload = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
	fmt.Println("OK admin knowledge API passed")
}

func base64Std(s string) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	b := []byte(s)
	var out []byte
	for i := 0; i < len(b); i += 3 {
		var chunk [3]byte
		n := copy(chunk[:], b[i:])
		out = append(out, chars[chunk[0]>>2])
		out = append(out, chars[(chunk[0]&0x3)<<4|chunk[1]>>4])
		if n > 1 {
			out = append(out, chars[(chunk[1]&0xf)<<2|chunk[2]>>6])
		} else {
			out = append(out, '=')
		}
		if n > 2 {
			out = append(out, chars[chunk[2]&0x3f])
		} else {
			out = append(out, '=')
		}
	}
	return string(out)
}

var _ = io.Discard
