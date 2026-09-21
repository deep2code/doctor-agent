package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/doctor-agent/internal/agent"
	"github.com/doctor-agent/internal/auth"
	"github.com/doctor-agent/internal/config"
	"github.com/doctor-agent/internal/database"
	"github.com/doctor-agent/internal/session"
)

// newAuthTestServer builds the DB-gated server used by the login + ownership
// tests. It needs the local MariaDB/MySQL named by the MARIA_DB_* environment
// (locally: MARIA_DB_PORT=3307 go test ./...), so it skips when none is
// reachable. It writes sessions/family rows in a scratch app DB and cleans
// them up; the knowledge DB is only read, hence the shared name.
func newAuthTestServer(t *testing.T, mutate func(*config.Config)) (*database.DB, *auth.Service, *Server) {
	t.Helper()
	// t.Setenv, not os.Setenv: these tests run before the rest of the package,
	// and a leaked scratch DB name would point later tests at the wrong schema.
	t.Setenv("MARIA_DB_APP_DB", "doctor_agent_test_auth")
	t.Setenv("MARIA_DB_KNOWLEDGE_DB", "doctor_knowledge")
	t.Setenv("LLM_PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	// These tests fire dozens of requests from one IP through the full stack.
	t.Setenv("RATE_LIMIT", "100000")

	cfg := config.Load()
	if mutate != nil {
		mutate(cfg)
	}
	if err := cfg.EnsureAppDB(); err != nil {
		t.Skipf("cannot create app db: %v", err)
	}
	if err := cfg.EnsureKnowledgeDB(); err != nil {
		t.Skipf("cannot create knowledge db: %v", err)
	}
	db, err := database.New(database.Config{DSN: cfg.AppDBDSN()})
	if err != nil {
		t.Skipf("no local MariaDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	authSvc := auth.NewService(db, "test-auth-secret")
	ag, err := agent.New(cfg)
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	ag.SetSessionStore(session.NewDBStore(db))
	s := NewWithDB(cfg, ag, authSvc, db)
	return db, authSvc, s
}

// bootstrapAdmin is the creator identity AdminCreateUser demands; it is never
// written to the database.
func bootstrapAdmin() *database.User {
	return &database.User{ID: "bootstrap-auth-test", Username: "bootstrap", IsAdmin: true}
}

func createUser(t *testing.T, svc *auth.Service, username string) *database.User {
	t.Helper()
	u, err := svc.AdminCreateUser(&auth.AdminCreateUserInput{
		Username: username,
		Password: "a-good-password",
		Nickname: username,
	}, bootstrapAdmin())
	if err != nil {
		t.Fatalf("creating %s: %v", username, err)
	}
	return u
}

// doReq runs one request through the whole middleware stack. token is sent as
// Authorization: Bearer when non-empty.
func doReq(t *testing.T, s *Server, method, path, token, body string) (int, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	httpReq := httptest.NewRequest(method, path, r)
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, httpReq)
	b, _ := io.ReadAll(w.Result().Body)
	return w.Code, string(b)
}

func jsonQuote(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// loginAs signs in and returns the bearer token the UI would store.
func loginAs(t *testing.T, s *Server, username, password string) string {
	t.Helper()
	code, body := doReq(t, s, http.MethodPost, "/login", "",
		`{"username":`+jsonQuote(username)+`,"password":`+jsonQuote(password)+`}`)
	if code != http.StatusOK {
		t.Fatalf("login %s: status = %d, body = %s", username, code, body)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("login response %s: %v", body, err)
	}
	if out.Token == "" {
		t.Fatalf("login %s returned no token: %s", username, body)
	}
	return out.Token
}

// TestLoginIssuesUsableTokens covers /login and /me: the failure reply must not
// distinguish "no such user" from "wrong password" nor leak the stored
// verifier, and a token must survive the middleware to become an identity.
func TestLoginIssuesUsableTokens(t *testing.T) {
	db, svc, s := newAuthTestServer(t, nil)
	u := createUser(t, svc, "tok-user")
	t.Cleanup(func() { _ = db.DeleteUser(u.ID) })

	// Unknown account and wrong password must be indistinguishable, so the
	// endpoint cannot enumerate usernames.
	codeU, bodyU := doReq(t, s, http.MethodPost, "/login", "", `{"username":"nobody-here","password":"a-good-password"}`)
	codeW, bodyW := doReq(t, s, http.MethodPost, "/login", "", `{"username":`+jsonQuote(u.Username)+`,"password":"wrong-password"}`)
	if codeU != http.StatusUnauthorized || codeW != http.StatusUnauthorized {
		t.Fatalf("bad credentials: unknown=%d wrong=%d, want 401/401", codeU, codeW)
	}
	if bodyU != bodyW {
		t.Errorf("login errors differ (usernames enumerable): %q vs %q", bodyU, bodyW)
	}
	for _, body := range []string{bodyU, bodyW} {
		if strings.Contains(body, "pbkdf2") {
			t.Errorf("login reply leaked a password verifier: %s", body)
		}
	}

	if code, _ := doReq(t, s, http.MethodGet, "/login", "", ""); code != http.StatusMethodNotAllowed {
		t.Errorf("GET /login = %d, want 405", code)
	}

	token := loginAs(t, s, u.Username, "a-good-password")

	if code, body := doReq(t, s, http.MethodGet, "/me", "", ""); code != http.StatusOK {
		t.Fatalf("GET /me without a token = %d, want 200", code)
	} else if !strings.Contains(body, `"user":null`) {
		t.Errorf("anonymous /me = %s, want null user", body)
	}
	code, body := doReq(t, s, http.MethodGet, "/me", token, "")
	if code != http.StatusOK {
		t.Fatalf("GET /me with a token = %d %s", code, body)
	}
	if !strings.Contains(body, u.ID) || !strings.Contains(body, "tok-user") {
		t.Errorf("/me did not identify the caller: %s", body)
	}
	// Only the explicitly listed public fields may reach a client.
	for _, leak := range []string{"pbkdf2", "password", "created_at", "last_login", "token"} {
		if strings.Contains(strings.ToLower(body), leak) {
			t.Errorf("/me leaked %q: %s", leak, body)
		}
	}

	// A tampered token degrades to anonymous — never to another account, and
	// never a 500 that tells the caller the signature check errored.
	forged := token[:len(token)-2] + "AA"
	if code, body := doReq(t, s, http.MethodGet, "/me", forged, ""); code != http.StatusOK || !strings.Contains(body, `"user":null`) {
		t.Errorf("forged token = %d %s, want 200 + null user", code, body)
	}
}

// TestPersonalDataIsolatedByOwner is the regression gate for the cross-user
// leak: /sessions used to list every conversation in the database, and
// conversation_id / family member ids were the only "auth" in front of them.
func TestPersonalDataIsolatedByOwner(t *testing.T) {
	db, svc, s := newAuthTestServer(t, nil)
	a := createUser(t, svc, "iso-user-a")
	b := createUser(t, svc, "iso-user-b")
	tokenA := loginAs(t, s, a.Username, "a-good-password")
	tokenB := loginAs(t, s, b.Username, "a-good-password")

	// Distinct ids that are not substrings of one another, so the body checks
	// below cannot pass or fail by accident.
	const convA, convAnon = "iso-conv-owned", "iso-conv-anon"
	store := session.NewDBStore(db)
	newConv := func(id string) *session.Session {
		sess := session.New(id)
		sess.AddUserMessage("我最近血压有点高怎么办")
		sess.AddAssistantMessage("先记录一周的晨起血压。")
		return sess
	}
	if err := store.Save(newConv(convA)); err != nil {
		t.Fatalf("save %s: %v", convA, err)
	}
	if err := db.SetSessionOwner(convA, a.ID); err != nil {
		t.Fatalf("binding %s to %s: %v", convA, a.ID, err)
	}
	if err := store.Save(newConv(convAnon)); err != nil {
		t.Fatalf("save %s: %v", convAnon, err)
	}

	code, body := doReq(t, s, http.MethodPost, "/family", tokenA,
		`{"name":"妈妈","relation":"母亲","conditions":"高血压","allergies":"青霉素"}`)
	if code != http.StatusCreated {
		t.Fatalf("creating a member = %d %s, want 201", code, body)
	}
	var member database.FamilyMember
	if err := json.Unmarshal([]byte(body), &member); err != nil {
		t.Fatalf("member response %s: %v", body, err)
	}
	if member.UserID != a.ID {
		t.Fatalf("member owner = %q, want %q (the credential picks the owner, not the body)", member.UserID, a.ID)
	}
	memberID := strconv.FormatInt(member.ID, 10)

	t.Cleanup(func() {
		_ = db.DeleteFamilyMember(member.ID, a.ID)
		_ = db.DeleteSession(convA)
		_ = db.DeleteSession(convAnon)
		_ = db.DeleteUser(a.ID)
		_ = db.DeleteUser(b.ID)
	})

	// 1. Listing is per owner.
	code, body = doReq(t, s, http.MethodGet, "/sessions", tokenA, "")
	if code != http.StatusOK {
		t.Fatalf("A's session list = %d %s", code, body)
	}
	if !strings.Contains(body, convA) {
		t.Errorf("A cannot see its own conversation: %s", body)
	}
	if strings.Contains(body, convAnon) {
		t.Errorf("A's list also serves the anonymous bucket: %s", body)
	}
	if code, body = doReq(t, s, http.MethodGet, "/sessions", tokenB, ""); code != http.StatusOK || strings.Contains(body, convA) {
		t.Errorf("B sees A's conversation: %d %s", code, body)
	}
	// No login token keeps the caller in the "" bucket — the single-user
	// behaviour that existed before accounts, not a super-user view.
	code, body = doReq(t, s, http.MethodGet, "/sessions", "", "")
	if code != http.StatusOK {
		t.Fatalf("anonymous session list = %d %s", code, body)
	}
	if !strings.Contains(body, convAnon) {
		t.Errorf("anonymous caller lost its own conversation: %s", body)
	}
	if strings.Contains(body, convA) {
		t.Errorf("anonymous caller sees a logged-in conversation: %s", body)
	}

	// 2. Reading and deleting one conversation by id.
	if code, body = doReq(t, s, http.MethodGet, "/sessions/"+convA, tokenB, ""); code != http.StatusNotFound {
		t.Errorf("B reading A's conversation = %d %s, want 404", code, body)
	}
	if code, body = doReq(t, s, http.MethodGet, "/sessions/"+convA, tokenA, ""); code != http.StatusOK || !strings.Contains(body, "血压") {
		t.Errorf("A reading its own conversation = %d %s, want 200 + transcript", code, body)
	}
	if code, _ = doReq(t, s, http.MethodDelete, "/sessions/"+convA, tokenB, ""); code != http.StatusNotFound {
		t.Errorf("B deleting A's conversation = %d, want 404", code)
	}
	if rec, err := db.GetSession(convA); err != nil || rec == nil {
		t.Errorf("A's conversation disappeared after B's delete attempt: rec=%v err=%v", rec, err)
	}

	// 3. Family profiles — a whole household's conditions, allergies and
	// long-term medication.
	if code, body = doReq(t, s, http.MethodGet, "/family", tokenB, ""); code != http.StatusOK {
		t.Fatalf("B's family list = %d %s", code, body)
	} else if strings.Contains(body, "妈妈") || strings.Contains(body, "青霉素") {
		t.Errorf("B sees A's family profile: %s", body)
	}
	if code, _ = doReq(t, s, http.MethodGet, "/family/"+memberID, tokenB, ""); code != http.StatusNotFound {
		t.Errorf("B reading A's member = %d, want 404", code)
	}
	if code, _ = doReq(t, s, http.MethodPut, "/family/"+memberID, tokenB, `{"name":"改写"}`); code != http.StatusNotFound {
		t.Errorf("B rewriting A's member = %d, want 404", code)
	}
	if m, err := db.GetFamilyMember(member.ID, a.ID); err != nil {
		t.Errorf("re-reading A's member: %v", err)
	} else if m.Name != "妈妈" {
		t.Errorf("B's rewrite landed: name = %q", m.Name)
	}
	if code, _ = doReq(t, s, http.MethodDelete, "/family/"+memberID, tokenB, ""); code != http.StatusNotFound {
		t.Errorf("B deleting A's member = %d, want 404", code)
	}
	if code, body = doReq(t, s, http.MethodPost, "/family", tokenB,
		`{"name":"邻居","user_id":`+jsonQuote(a.ID)+`}`); code != http.StatusCreated {
		t.Errorf("B creating a member = %d %s, want 201", code, body)
	} else {
		var injected database.FamilyMember
		if err := json.Unmarshal([]byte(body), &injected); err != nil {
			t.Fatalf("member response %s: %v", body, err)
		}
		t.Cleanup(func() { _ = db.DeleteFamilyMember(injected.ID, b.ID) })
		if injected.UserID != b.ID {
			t.Errorf("member owner = %q, want %q — the body chose the owner", injected.UserID, b.ID)
		}
		if _, err := db.GetFamilyMember(injected.ID, a.ID); err == nil {
			t.Error("A can read a member B created and tried to donate to A")
		}
	}

	// 4. An anonymous conversation the user continues after signing in follows
	// that user — and leaves the anonymous list, so the next visitor in the
	// shared bucket cannot read it.
	sess, ok := s.agent.ClaimSession(convAnon, a.ID)
	if !ok {
		t.Fatal("A could not continue the conversation it started while signed out")
	}
	if err := store.Save(sess); err != nil {
		t.Fatalf("re-save %s: %v", convAnon, err)
	}
	if _, body = doReq(t, s, http.MethodGet, "/sessions", "", ""); strings.Contains(body, convAnon) {
		t.Errorf("claimed conversation still in the anonymous list: %s", body)
	}
	if _, body = doReq(t, s, http.MethodGet, "/sessions", tokenA, ""); !strings.Contains(body, convAnon) {
		t.Errorf("claimed conversation missing from A's list: %s", body)
	}
	if code, _ = doReq(t, s, http.MethodGet, "/sessions/"+convAnon, tokenB, ""); code != http.StatusNotFound {
		t.Errorf("B reads A's claimed conversation = %d, want 404", code)
	}
}

// TestAPIKeyIsNotAPerson pins the split between the two credentials: API_KEY
// authorizes the deployment, but it must never resolve to a patient's data,
// or every holder of the shared key would read the same medical history.
func TestAPIKeyIsNotAPerson(t *testing.T) {
	db, svc, s := newAuthTestServer(t, func(cfg *config.Config) { cfg.APIKey = "shared-deploy-key" })
	u := createUser(t, svc, "svc-user")
	const conv = "svc-conv"
	if err := db.CreateSession(&database.SessionRecord{ID: conv, Title: "内部会话", UserID: u.ID}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() {
		_ = db.DeleteSession(conv)
		_ = db.DeleteUser(u.ID)
	})

	// API_KEY passes the gate but owns nothing: it sees the anonymous bucket.
	code, body := doReq(t, s, http.MethodGet, "/sessions", "shared-deploy-key", "")
	if code != http.StatusOK {
		t.Fatalf("list with API_KEY = %d %s, want 200", code, body)
	}
	if strings.Contains(body, conv) {
		t.Errorf("the shared deployment key unlocked a user's conversation: %s", body)
	}
	if _, body = doReq(t, s, http.MethodGet, "/sessions", loginAs(t, s, u.Username, "a-good-password"), ""); !strings.Contains(body, conv) {
		t.Errorf("the account itself cannot see its conversation: %s", body)
	}
	// Without any credential the gate closes.
	if code, _ = doReq(t, s, http.MethodGet, "/sessions", "", ""); code != http.StatusUnauthorized {
		t.Errorf("unauthenticated /sessions = %d, want 401", code)
	}
}
