package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/doctor-agent/internal/agent"
	"github.com/doctor-agent/internal/config"
)

// sharedAgent is built once (knowledge base load is heavy); all tests share it.
var (
	agOnce sync.Once
	agInst *agent.Agent
	agErr  error
)

func sharedAgent(t *testing.T) *agent.Agent {
	t.Helper()
	agOnce.Do(func() {
		cfg := baseCfg()
		agInst, agErr = agent.New(cfg)
	})
	if agErr != nil {
		t.Fatalf("agent.New: %v", agErr)
	}
	return agInst
}

// baseCfg returns a deterministic config: anthropic provider with a dummy key
// (never called — tests use the emergency short-circuit path) and safety on.
func baseCfg() *config.Config {
	cfg := config.Load()
	cfg.LLMProvider = "anthropic"
	cfg.AnthropicAPIKey = "test-key"
	cfg.EmergencyEnabled = true
	cfg.ScopeGuardEnabled = true
	cfg.KnowledgeEnabled = true
	cfg.PostVerifyEnabled = true
	return cfg
}

func newTestServer(t *testing.T, mutate func(*config.Config)) *Server {
	t.Helper()
	cfg := baseCfg()
	if mutate != nil {
		mutate(cfg)
	}
	return New(cfg, sharedAgent(t), nil)
}

// newPageTestServer builds a server without the knowledge-base agent: the
// page/crawler handlers never touch it, so these tests run without MariaDB.
func newPageTestServer(t *testing.T, mutate func(*config.Config)) *Server {
	t.Helper()
	cfg := baseCfg()
	if mutate != nil {
		mutate(cfg)
	}
	return New(cfg, nil, nil)
}

// emergencyMessage triggers the L1 short-circuit so no LLM call is made.
const emergencyMessage = "我突然胸口剧痛，喘不上气"

func doRequest(t *testing.T, s *Server, method, path, body, auth, origin string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(rec, req)
	return rec
}

func TestHealthOpen(t *testing.T) {
	s := newTestServer(t, nil)
	rec := doRequest(t, s, http.MethodGet, "/health", "", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", rec.Code)
	}
}

func TestPagesServed(t *testing.T) {
	s := newPageTestServer(t, nil)

	// Root "/" serves the marketing landing page.
	rec := doRequest(t, s, http.MethodGet, "/", "", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	landing := rec.Body.String()
	if !strings.Contains(landing, "医答") || !strings.Contains(landing, "/app") {
		t.Error("landing page is missing expected content (brand or /app link)")
	}

	// "/app" serves the chat web interface (embedded single-file UI).
	app := doRequest(t, s, http.MethodGet, "/app", "", "", "")
	if app.Code != http.StatusOK {
		t.Fatalf("GET /app status = %d, want 200", app.Code)
	}
	appBody := app.Body.String()
	if !strings.Contains(appBody, "医答") || !strings.Contains(appBody, "/chat/stream") {
		t.Error("chat UI is missing expected content (title or stream endpoint)")
	}

	// Unknown paths 404 instead of serving the landing shell.
	if rec := doRequest(t, s, http.MethodGet, "/nope", "", "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("GET /nope = %d, want 404", rec.Code)
	}

	// "/map" serves the disease-map page (interactive SVG + crawlable table).
	mp := doRequest(t, s, http.MethodGet, "/map", "", "", "")
	if mp.Code != http.StatusOK {
		t.Fatalf("GET /map status = %d, want 200", mp.Code)
	}
	mapBody := mp.Body.String()
	if !strings.Contains(mapBody, "高发病地图") || !strings.Contains(mapBody, "广东") {
		t.Error("map page is missing expected content (title or province data)")
	}

	// "/stats" serves the statistics page (tabs + crawlable tables).
	st := doRequest(t, s, http.MethodGet, "/stats", "", "", "")
	if st.Code != http.StatusOK {
		t.Fatalf("GET /stats status = %d, want 200", st.Code)
	}
	statsBody := st.Body.String()
	if !strings.Contains(statsBody, "发病全景") || !strings.Contains(statsBody, "statsTabs") {
		t.Error("stats page is missing expected content (title or tabs)")
	}
	// Shared CSS must be inlined (placeholders resolved), scoped per page.
	checkCSSInlined(t, "landing", landing, ".bm-flipper")
	checkCSSInlined(t, "map", mapBody, ".map-panel")
	checkCSSInlined(t, "stats", statsBody, ".stat-tab")
}

// checkCSSInlined asserts a rendered page has no leftover /*__CSS__*/ style
// placeholder and does carry the CSS classes its interactive section needs.
func checkCSSInlined(t *testing.T, name, body, wantCls string) {
	t.Helper()
	if strings.Contains(body, "__CSS__") || !strings.Contains(body, wantCls) {
		t.Errorf("%s page: CSS not inlined (placeholder left or %q class missing)", name, wantCls)
	}
}

func TestPublicCrawlerFiles(t *testing.T) {
	s := newPageTestServer(t, nil)

	robots := doRequest(t, s, http.MethodGet, "/robots.txt", "", "", "")
	if robots.Code != http.StatusOK {
		t.Fatalf("GET /robots.txt = %d, want 200", robots.Code)
	}
	if ct := robots.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("robots.txt Content-Type = %q, want text/plain", ct)
	}
	if rb := robots.Body.String(); !strings.Contains(rb, "Sitemap:") || !strings.Contains(rb, "GPTBot") || !strings.Contains(rb, "Disallow: /admin") {
		t.Error("robots.txt is missing Sitemap line, AI crawler rules or admin disallow")
	}

	sm := doRequest(t, s, http.MethodGet, "/sitemap.xml", "", "", "")
	if sm.Code != http.StatusOK {
		t.Fatalf("GET /sitemap.xml = %d, want 200", sm.Code)
	}
	if ct := sm.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/xml") {
		t.Errorf("sitemap.xml Content-Type = %q, want application/xml", ct)
	}
	if n := strings.Count(sm.Body.String(), "<loc>"); n != 3 {
		t.Errorf("sitemap.xml has %d <loc> entries, want 3", n)
	}

	llms := doRequest(t, s, http.MethodGet, "/llms.txt", "", "", "")
	if llms.Code != http.StatusOK {
		t.Fatalf("GET /llms.txt = %d, want 200", llms.Code)
	}
	if lb := llms.Body.String(); !strings.Contains(lb, "医答") || !strings.Contains(lb, "/map") {
		t.Error("llms.txt is missing site name or page links")
	}
}

func TestAuthPublicPaths(t *testing.T) {
	s := newPageTestServer(t, func(c *config.Config) {
		c.APIKey = "secret"
		c.PublicBaseURL = "https://yida.example.com"
	})

	// Public marketing/crawler pages stay reachable without a token, so
	// crawlers are not locked out when an API key is configured.
	for _, p := range []string{"/", "/index.html", "/map", "/stats", "/robots.txt", "/sitemap.xml", "/llms.txt"} {
		if rec := doRequest(t, s, http.MethodGet, p, "", "", ""); rec.Code != http.StatusOK {
			t.Errorf("GET %s (no token) = %d, want 200", p, rec.Code)
		}
	}
	// The API itself stays gated.
	if rec := doRequest(t, s, http.MethodPost, "/chat", `{"message":"x"}`, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("POST /chat (no token) = %d, want 401", rec.Code)
	}
	// With PUBLIC_BASE_URL set, pages carry canonical and sitemap absolute locs.
	rec := doRequest(t, s, http.MethodGet, "/", "", "", "")
	if body := rec.Body.String(); !strings.Contains(body, `rel="canonical" href="https://yida.example.com/"`) {
		t.Error("landing page missing canonical URL from PublicBaseURL")
	}
	sm := doRequest(t, s, http.MethodGet, "/sitemap.xml", "", "", "")
	if body := sm.Body.String(); !strings.Contains(body, "<loc>https://yida.example.com/map</loc>") {
		t.Error("sitemap.xml missing absolute /map loc from PublicBaseURL")
	}
}

func TestAuthRequired(t *testing.T) {
	s := newTestServer(t, func(c *config.Config) { c.APIKey = "secret" })

	// Without a token: 401.
	if rec := doRequest(t, s, http.MethodPost, "/chat", `{"message":"x"}`, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token → %d, want 401", rec.Code)
	}
	// Wrong token: 401.
	if rec := doRequest(t, s, http.MethodPost, "/chat", `{"message":"x"}`, "Bearer wrong", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong token → %d, want 401", rec.Code)
	}
	// Correct token passes auth (empty message → 400 from the handler, not 401).
	if rec := doRequest(t, s, http.MethodPost, "/chat", `{"message":""}`, "Bearer secret", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("valid token → %d, want 400 (auth passed)", rec.Code)
	}
	// /health stays open without auth.
	if rec := doRequest(t, s, http.MethodGet, "/health", "", "", ""); rec.Code != http.StatusOK {
		t.Errorf("health without token → %d, want 200", rec.Code)
	}
	// OPTIONS preflight is not gated by auth.
	rec := doRequest(t, s, http.MethodOptions, "/chat", "", "", "")
	if rec.Code != http.StatusNoContent {
		t.Errorf("OPTIONS → %d, want 204", rec.Code)
	}
}

func TestRateLimit(t *testing.T) {
	s := newTestServer(t, func(c *config.Config) { c.RateLimit = 2 })

	payload := `{"message":"` + emergencyMessage + `"}`
	// First two requests pass (emergency short-circuit → 200).
	for i := 0; i < 2; i++ {
		if rec := doRequest(t, s, http.MethodPost, "/chat", payload, "", ""); rec.Code != http.StatusOK {
			t.Fatalf("request %d → %d, want 200", i+1, rec.Code)
		}
	}
	// Third request from the same IP is rate-limited.
	if rec := doRequest(t, s, http.MethodPost, "/chat", payload, "", ""); rec.Code != http.StatusTooManyRequests {
		t.Errorf("3rd request → %d, want 429", rec.Code)
	}
	// /health is exempt.
	if rec := doRequest(t, s, http.MethodGet, "/health", "", "", ""); rec.Code != http.StatusOK {
		t.Errorf("health after limit → %d, want 200", rec.Code)
	}
}

func TestCORSAllowlist(t *testing.T) {
	s := newTestServer(t, func(c *config.Config) {
		c.CORSOrigins = []string{"https://app.example.com"}
	})

	rec := doRequest(t, s, http.MethodGet, "/health", "", "", "https://app.example.com")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("allowed origin header = %q", got)
	}

	rec = doRequest(t, s, http.MethodGet, "/health", "", "", "https://evil.example.com")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("disallowed origin header = %q, want empty", got)
	}
}

func TestDefaultCORSAllowAll(t *testing.T) {
	s := newTestServer(t, nil)
	rec := doRequest(t, s, http.MethodGet, "/health", "", "", "https://anything.example.com")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("default CORS header = %q, want *", got)
	}
}

func TestChatStreamSSE(t *testing.T) {
	s := newTestServer(t, nil)
	payload := `{"message":"` + emergencyMessage + `"}`
	rec := doRequest(t, s, http.MethodPost, "/chat/stream", payload, "", "")

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: done") {
		t.Errorf("SSE body missing done event:\n%s", body)
	}
	if strings.Contains(body, "event: delta") {
		t.Errorf("emergency short-circuit must not stream deltas:\n%s", body)
	}
	// The done event carries a JSON ChatResponse with the emergency text.
	var done ChatResponse
	var curEvent string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "event: ") {
			curEvent = strings.TrimPrefix(line, "event: ")
			continue
		}
		if curEvent == "done" && strings.HasPrefix(line, "data: ") {
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &done); err != nil {
				t.Fatalf("parse done data: %v", err)
			}
			break
		}
	}
	if !done.IsEmergency {
		t.Error("done event IsEmergency = false, want true")
	}
	if done.Reply == "" {
		t.Error("done event Reply is empty")
	}
	// The emergency step is surfaced as a `step` SSE event before `done`.
	if !strings.Contains(body, "event: step") || !strings.Contains(body, "紧急") {
		t.Error("SSE body missing emergency step event")
	}
}
