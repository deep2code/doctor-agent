package server

import (
	"context"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/doctor-agent/internal/agent"
	"github.com/doctor-agent/internal/auth"
	"github.com/doctor-agent/internal/config"
	"github.com/doctor-agent/internal/database"
	"github.com/doctor-agent/internal/embedding"
	"github.com/doctor-agent/internal/knowledge"
	"github.com/doctor-agent/internal/llm"
	"github.com/doctor-agent/internal/session"
)

//go:embed web/index.html
var webUIIndex string

//go:embed web/admin.html
var adminUIIndex string

// Server wraps the HTTP API server for the doctor agent.
type Server struct {
	cfg   *config.Config
	agent *agent.Agent
	auth  *auth.Service
	db    *database.DB
	http  *http.Server
	limiter *rateLimiter
}

// New creates a new HTTP server.
func New(cfg *config.Config, ag *agent.Agent, authSvc *auth.Service) *Server {
	return NewWithDB(cfg, ag, authSvc, nil)
}

// NewWithDB creates a new HTTP server with access to the application
// database, enabling server-side session persistence and session APIs.
func NewWithDB(cfg *config.Config, ag *agent.Agent, authSvc *auth.Service, db *database.DB) *Server {
	s := &Server{
		cfg:     cfg,
		agent:   ag,
		auth:    authSvc,
		db:      db,
		limiter: newRateLimiter(cfg.RateLimit),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWebUI)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/chat", s.handleChat)
	mux.HandleFunc("/chat/stream", s.handleChatStream)
	mux.HandleFunc("/feedback", s.handleFeedback)
	// Session APIs (server-side persistence for the chat UI)
	if db != nil {
		mux.HandleFunc("/sessions", s.handleSessions)
		mux.HandleFunc("/sessions/", s.handleSessionByID)
	}
	// Admin endpoints
	mux.HandleFunc("/admin/users", s.handleAdminUsers)
	mux.HandleFunc("/admin/users/", s.handleAdminUser)
	// Sync endpoints
	mux.HandleFunc("/admin/sync", s.handleAdminSync)
	mux.HandleFunc("/admin/sync/status", s.handleAdminSyncStatus)
	// Knowledge management (upload/update medical knowledge)
	mux.HandleFunc("/admin/knowledge", s.handleAdminKnowledge)
	mux.HandleFunc("/admin/knowledge/stats", s.handleAdminKnowledgeStats)
	mux.HandleFunc("/admin/knowledge/export", s.handleAdminKnowledgeExport)
	mux.HandleFunc("/admin/knowledge/versions", s.handleAdminKnowledgeVersions)
	// Session management
	mux.HandleFunc("/admin/sessions", s.handleAdminSessions)
	mux.HandleFunc("/admin/sessions/", s.handleAdminSessionByID)
	// Feedback with details
	mux.HandleFunc("/admin/feedback", s.handleAdminFeedback)
	mux.HandleFunc("/admin/feedback/stats", s.handleAdminFeedbackStats)
	// Audit logs
	mux.HandleFunc("/admin/audit-logs", s.handleAdminAuditLogs)
	// System config
	mux.HandleFunc("/admin/config", s.handleAdminConfig)
	mux.HandleFunc("/admin/config/", s.handleAdminConfigByKey)
	// API stats
	mux.HandleFunc("/admin/api-stats", s.handleAdminAPIStats)
	mux.HandleFunc("/admin/api-stats/summary", s.handleAdminAPIStatsSummary)
	// User behavior analysis
	mux.HandleFunc("/admin/analytics", s.handleAdminAnalytics)
	// Batch operations
	mux.HandleFunc("/admin/batch/users", s.handleAdminBatchUsers)
	mux.HandleFunc("/admin/batch/knowledge", s.handleAdminBatchKnowledge)
	// Data export
	mux.HandleFunc("/admin/export", s.handleAdminExport)
	// Admin UI (single-file page, Basic-auth guarded by the browser)
	mux.HandleFunc("/admin", s.handleAdminUI)

	s.http = &http.Server{
		Addr:         fmt.Sprintf("%s:%s", cfg.ServerHost, cfg.ServerPort),
		Handler:      s.withMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// Start begins listening and serving HTTP requests.
func (s *Server) Start() error {
	slog.Info("Starting HTTP server", "addr", s.http.Addr)
	return s.http.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

// handleWebUI serves the built-in chat web interface (embedded single-file
// HTML — no build step, no external assets; works offline).
func (s *Server) handleWebUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, webUIIndex)
}

// handleHealth responds with server health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// ChatImage represents an uploaded image in a chat message.
type ChatImage struct {
	Base64Data string `json:"base64_data"`
	MediaType  string `json:"media_type"`
}

// ChatRequest is the JSON body for /chat endpoints.
type ChatRequest struct {
	Message        string      `json:"message"`
	ConversationID string      `json:"conversation_id,omitempty"`
	Images         []ChatImage `json:"images,omitempty"`
}

// ChatResponse is the JSON response for /chat.
type ChatResponse struct {
	ConversationID string `json:"conversation_id"`
	Reply          string `json:"reply"`
	IsEmergency    bool   `json:"is_emergency"`
	IsOutOfScope   bool   `json:"is_out_of_scope"`
	Timestamp      string `json:"timestamp"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Cap the request body to bound memory use; reject oversized messages.
	// Allow up to 10MB for image uploads
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10 MB
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": fmt.Sprintf("invalid request body: %v", err),
		})
		return
	}

	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "message field is required",
		})
		return
	}
	if len(req.Message) > 20000 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "message too long (max 20000 chars)",
		})
		return
	}

	if req.ConversationID == "" || !session.ValidID(req.ConversationID) {
		req.ConversationID = fmt.Sprintf("conv-%d", time.Now().UnixNano())
	}

	sess := s.agent.GetOrCreateSession(req.ConversationID)

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Convert images to llm.ImageInput format
	var images []llm.ImageInput
	for _, img := range req.Images {
		images = append(images, llm.ImageInput{
			Base64Data: img.Base64Data,
			MediaType:  img.MediaType,
		})
	}

	var resp *agent.Response
	var err error
	if len(images) > 0 {
		resp, err = s.agent.ProcessMessageWithImages(ctx, sess, req.Message, images)
	} else {
		resp, err = s.agent.ProcessMessage(ctx, sess, req.Message)
	}
	if err != nil {
		slog.Error("Agent processing error", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "internal processing error",
		})
		return
	}

	writeJSON(w, http.StatusOK, ChatResponse{
		ConversationID: req.ConversationID,
		Reply:          resp.Text,
		IsEmergency:    resp.IsEmergency,
		IsOutOfScope:   resp.IsOutOfScope,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	})
}

// handleChatStream streams the agent's response as SSE events:
//   - `delta` events carry incremental text chunks as they are generated
//   - a final `done` event carries the complete ChatResponse
//   - `error` events are emitted on processing failure
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Allow up to 10MB for image uploads
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10 MB
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "message field is required"})
		return
	}
	if len(req.Message) > 20000 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "message too long (max 20000 chars)"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Streaming responses can outlive the server's global write timeout.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
	_ = rc.EnableFullDuplex()

	if req.ConversationID == "" || !session.ValidID(req.ConversationID) {
		req.ConversationID = fmt.Sprintf("conv-%d", time.Now().UnixNano())
	}
	sess := s.agent.GetOrCreateSession(req.ConversationID)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	sendEvent := func(event string, payload any) {
		data, _ := json.Marshal(payload)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	// Collect deltas during generation; the live, pre-verification text must
	// NOT be streamed to the client. L3 citation verification and L4 disclaimer
	// run inside the agent only after the full response is produced, so we
	// buffer and then stream the VERIFIED text below (otherwise post-
	// verification corrections reach the user unguarded via raw deltas).
	var rawDelta strings.Builder
	onDelta := func(chunk string) {
		rawDelta.WriteString(chunk)
	}

	onStep := func(ev agent.StepEvent) {
		sendEvent("step", ev)
	}

	// Convert images to llm.ImageInput format
	var images []llm.ImageInput
	for _, img := range req.Images {
		images = append(images, llm.ImageInput{
			Base64Data: img.Base64Data,
			MediaType:  img.MediaType,
		})
	}

	var resp *agent.Response
	var err error
	if len(images) > 0 {
		resp, err = s.agent.ProcessMessageStreamWithImages(ctx, sess, req.Message, images, onDelta, onStep)
	} else {
		resp, err = s.agent.ProcessMessageStream(ctx, sess, req.Message, onDelta, onStep)
	}
	if err != nil {
		sendEvent("error", map[string]any{"error": "internal processing error"})
		return
	}

	// Stream the post-verification, disclaimer-applied response (resp.Text) so
	// the client only ever renders safe content, preserving token-level
	// streaming UX without exposing unverified model output. Emergency
	// short-circuits stay atomic (no deltas), matching the prior contract.
	if !resp.IsEmergency {
		streamVerifiedText(sendEvent, resp.Text)
	}

	sendEvent("done", ChatResponse{
		ConversationID: req.ConversationID,
		Reply:          resp.Text,
		IsEmergency:    resp.IsEmergency,
		IsOutOfScope:   resp.IsOutOfScope,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	})
}

// streamVerifiedText emits the (already post-verification / disclaimer-applied)
// text as SSE delta chunks so the client never sees unverified model output
// during generation.
func streamVerifiedText(send func(string, any), text string) {
	const chunkSize = 24
	runes := []rune(text)
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		send("delta", map[string]any{"text": string(runes[i:end])})
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Warn("Failed to encode JSON response", "error", err)
	}
}

// handleFeedback collects user feedback (thumbs up/down) for responses.
func (s *Server) handleFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4<<10) // 4 KiB
	var req struct {
		SessionID string `json:"session_id"`
		MessageID string `json:"message_id"`
		Rating    string `json:"rating"` // "up" or "down"
		Comment   string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}

	if req.Rating != "up" && req.Rating != "down" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "rating must be 'up' or 'down'"})
		return
	}

	// Log feedback for now (could persist to database later)
	slog.Info("User feedback received",
		"session_id", req.SessionID,
		"message_id", req.MessageID,
		"rating", req.Rating,
		"comment", req.Comment,
		"ip", clientIP(r),
	)

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// handleSessions lists persisted conversations (most recently updated first).
// GET /sessions
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "session persistence disabled"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	recs, err := s.db.ListAllSessions(200)
	if err != nil {
		slog.Error("Listing sessions", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to list sessions"})
		return
	}

	type item struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		UpdatedAt string `json:"updated_at"`
		CreatedAt string `json:"created_at"`
	}
	out := make([]item, 0, len(recs))
	for _, r2 := range recs {
		if r2.ID == "" {
			continue
		}
		out = append(out, item{
			ID:        r2.ID,
			Title:     r2.Title,
			UpdatedAt: r2.UpdatedAt.Format(time.RFC3339),
			CreatedAt: r2.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

// handleSessionByID reads or deletes one persisted conversation.
//   GET    /sessions/{id}  → {id, title, messages:[{role, content}]}
//   DELETE /sessions/{id}  → 204
func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "session persistence disabled"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/sessions/")
	if id == "" || !session.ValidID(id) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid session id"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		rec, err := s.db.GetSession(id)
		if err != nil {
			slog.Error("Getting session", "id", id, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to get session"})
			return
		}
		if rec == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "session not found"})
			return
		}
		msgs, err := s.db.GetSessionMessages(id)
		if err != nil {
			slog.Error("Getting session messages", "id", id, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to get session messages"})
			return
		}
		type m struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		out := make([]m, 0, len(msgs))
		for _, msg := range msgs {
			out = append(out, m{Role: msg.Role, Content: msg.Content})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":       rec.ID,
			"title":    rec.Title,
			"messages": out,
		})
	case http.MethodDelete:
		if err := s.db.DeleteSession(id); err != nil {
			slog.Error("Deleting session", "id", id, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to delete session"})
			return
		}
		// Drop the in-memory copy too, so a stale agent session can't resurrect it.
		s.agent.DeleteSession(id)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleAdminUsers handles admin user management (create list users).
func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	// Check admin authentication
	admin := s.getAdminFromRequest(r)
	if admin == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	switch r.Method {
	case http.MethodPost:
		// Create user
		var input auth.AdminCreateUserInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}

		user, err := s.auth.AdminCreateUser(&input, admin)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"id":       user.ID,
			"username": user.Username,
			"nickname": user.Nickname,
			"is_admin": user.IsAdmin,
			"message":  "用户创建成功",
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleAdminUser handles single user operations.
func (s *Server) handleAdminUser(w http.ResponseWriter, r *http.Request) {
	// Check admin authentication
	admin := s.getAdminFromRequest(r)
	if admin == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	// Extract user ID from path
	userID := strings.TrimPrefix(r.URL.Path, "/admin/users/")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "用户ID不能为空"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Get user
		user, err := s.auth.GetUserByID(userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		if user == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "用户不存在"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"id":         user.ID,
			"username":   user.Username,
			"nickname":   user.Nickname,
			"is_admin":   user.IsAdmin,
			"created_at": user.CreatedAt,
		})

	case http.MethodDelete:
		// Delete user
		if err := s.auth.DeleteUser(userID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"message": "用户删除成功"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// getAdminFromRequest extracts admin user from request (checks API key or token).
func (s *Server) getAdminFromRequest(r *http.Request) *database.User {
	// 1) Basic auth (admin console): base64 user:pass → Login check.
	if u, p, ok := r.BasicAuth(); ok {
		if s.auth == nil {
			return nil
		}
		user, err := s.auth.Login(&auth.LoginInput{Username: u, Password: p})
		if err == nil && user != nil && user.IsAdmin {
			return user
		}
		// Fall through: a non-admin login is still rejected.
		return nil
	}

	// 2) Bearer API key: matches the configured API key → virtual admin.
	apiKey := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

	// If API key matches, return admin user
	if s.cfg.APIKey != "" && subtle.ConstantTimeCompare([]byte(apiKey), []byte(s.cfg.APIKey)) == 1 {
		// For API key auth, return a virtual admin user
		return &database.User{
			ID:       "admin-api",
			Username: "admin",
			IsAdmin:  true,
		}
	}

	// 3) Token-based auth
	if s.auth != nil {
		user, err := s.auth.GetUserByToken(apiKey)
		if err == nil && user != nil && user.IsAdmin {
			return user
		}
	}

	return nil
}

// withMiddleware adds security + logging middleware to the handler.
// Order: CORS headers → OPTIONS short-circuit → rate limit → auth → logging.
func (s *Server) withMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.applyCORS(w, r)

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// /health and the UI page stay open for probes/browsers; the API is gated.
		if r.URL.Path != "/health" && r.URL.Path != "/" {
			if !s.limiter.allow(clientIP(r)) {
				slog.Warn("Rate limit exceeded", "ip", clientIP(r), "path", r.URL.Path)
				writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate limit exceeded"})
				return
			}
			if !s.authenticated(r) {
				slog.Warn("Unauthorized request", "ip", clientIP(r), "path", r.URL.Path)
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
				return
			}
		}

		// Logging
		start := time.Now()
		h.ServeHTTP(w, r)
		slog.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start).String(),
		)
	})
}

// applyCORS sets permissive or allowlisted CORS headers.
func (s *Server) applyCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if len(s.cfg.CORSOrigins) == 0 {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	} else {
		for _, o := range s.cfg.CORSOrigins {
			if o == origin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
				break
			}
		}
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

// authenticated checks the Bearer token when APIKey is configured.
// With an empty APIKey auth is disabled and every request passes.
func (s *Server) authenticated(r *http.Request) bool {
	if s.cfg.APIKey == "" {
		return true
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	got := strings.TrimPrefix(auth, "Bearer ")
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.APIKey)) == 1
}

// clientIP extracts the caller IP from RemoteAddr ("ip:port").
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimiter is a minimal fixed-window per-IP limiter (no external deps).
type rateLimiter struct {
	mu     sync.Mutex
	limit  int // requests per window per IP; 0 disables
	window time.Duration
	counts map[string]*rateWindow
}

type rateWindow struct {
	start time.Time
	count int
}

func newRateLimiter(limit int) *rateLimiter {
	return &rateLimiter{
		limit:  limit,
		window: time.Minute,
		counts: make(map[string]*rateWindow),
	}
}

// allow reports whether a request from ip is within the rate limit.
func (rl *rateLimiter) allow(ip string) bool {
	if rl.limit <= 0 {
		return true
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	w, ok := rl.counts[ip]
	if !ok || now.Sub(w.start) >= rl.window {
		rl.counts[ip] = &rateWindow{start: now, count: 1}
		return true
	}
	w.count++
	allowed := w.count <= rl.limit

	// Opportunistically prune expired entries so the map cannot grow unbounded
	// under bursts of distinct source IPs.
	if len(rl.counts) > 256 {
		for k, v := range rl.counts {
			if now.Sub(v.start) >= rl.window {
				delete(rl.counts, k)
			}
		}
	}
	return allowed
}

// handleAdminSync handles POST /admin/sync for file upload sync.
func (s *Server) handleAdminSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check admin authentication
	admin := s.getAdminFromRequest(r)
	if admin == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	// Parse multipart form (10MB max)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": fmt.Sprintf("failed to parse form: %v", err),
		})
		return
	}

	// Get uploaded file
	file, handler, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": fmt.Sprintf("failed to get file: %v", err),
		})
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Debug("Failed to close uploaded file", "error", err)
		}
	}()

	// Create temp file
	tempFile, err := os.CreateTemp("", "sync-*.json")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to create temp file: %v", err),
		})
		return
	}
	defer func() {
		if err := os.Remove(tempFile.Name()); err != nil {
			slog.Debug("Failed to remove temp file", "path", tempFile.Name(), "error", err)
		}
	}()

	// Copy uploaded file to temp file
	if _, err := io.Copy(tempFile, file); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to save file: %v", err),
		})
		return
	}
	if err := tempFile.Close(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to close temp file: %v", err),
		})
		return
	}

	// Get source parameter
	source := r.FormValue("source")
	if source == "" || source == "all" {
		// "all" (or absent) means sync every dataset; no filename fallback.
		source = ""
	} else if source == "auto" {
		source = strings.TrimSuffix(handler.Filename, ".json")
		source = strings.TrimSuffix(source, ".json")
	}

	// Get full sync parameter
	fullSync := r.FormValue("full") == "true"

	// Initialize sync components
	store, err := knowledge.Load()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to load knowledge: %v", err),
		})
		return
	}

	vecStore, err := knowledge.NewVectorStore(knowledge.VectorStoreConfig{
		Host:       s.cfg.VectorStoreHost,
		Port:       s.cfg.VectorStorePort,
		Collection: s.cfg.VectorCollection,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to connect to vector store: %v", err),
		})
		return
	}
	defer func() {
		if err := vecStore.Close(); err != nil {
			slog.Debug("Failed to close vector store", "error", err)
		}
	}()

	embedder, err := embedding.NewDefault(s.cfg.EmbeddingBaseURL, s.cfg.EmbeddingAPIKey, s.cfg.EmbeddingModel)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to init embedding: %v", err),
		})
		return
	}

	syncer := knowledge.NewSyncer(store, vecStore, embedder)

	// Perform sync
	ctx := context.Background()
	cfg := knowledge.SyncConfig{
		Full:      fullSync,
		Source:    source,
		FilePath:  tempFile.Name(),
		BatchSize: 100,
	}

	var status *knowledge.SyncStatus
	if fullSync {
		status, err = syncer.FullSync(ctx, cfg)
	} else {
		status, err = syncer.IncrementalSync(ctx, cfg)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("sync failed: %v", err),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": "sync completed",
		"sync":    status,
	})
}

// handleAdminSyncStatus handles GET /admin/sync/status.
func (s *Server) handleAdminSyncStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check admin authentication
	admin := s.getAdminFromRequest(r)
	if admin == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	// Check if vector store is enabled
	if !s.cfg.VectorStoreEnabled {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "disabled",
			"message": "vector store not enabled",
		})
		return
	}

	// Connect to vector store
	vecStore, err := knowledge.NewVectorStore(knowledge.VectorStoreConfig{
		Host:       s.cfg.VectorStoreHost,
		Port:       s.cfg.VectorStorePort,
		Collection: s.cfg.VectorCollection,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to connect to vector store: %v", err),
		})
		return
	}
	defer func() {
		if err := vecStore.Close(); err != nil {
			slog.Debug("Failed to close vector store", "error", err)
		}
	}()

	// Get stats
	ctx := context.Background()
	stats, err := vecStore.GetSyncStats(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to get stats: %v", err),
		})
		return
	}

	total, err := vecStore.Count(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to count points: %v", err),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"total":  total,
		"stats":  stats,
	})
}

// handleAdminUI serves the single-file admin console (embedded HTML).
// The page itself asks for admin credentials and calls the admin APIs with
// Basic auth; no server-side session is involved.
func (s *Server) handleAdminUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, adminUIIndex)
}

// handleAdminKnowledge handles uploading/updating a medical knowledge dataset.
// POST multipart with a "file" part: <dataset>.json or <dataset>.json.gz.
// The file is classified, replaces the existing dataset rows in MariaDB, and
// the in-memory store is refreshed so running queries see the update.
func (s *Server) handleAdminKnowledge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if err := r.ParseMultipartForm(200 << 20); err != nil { // 200MB headroom for big JSON.gz
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("failed to parse form: %v", err)})
		return
	}
	file, handler, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("failed to get file: %v", err)})
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("reading upload: %v", err)})
		return
	}
	if len(raw) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "empty file"})
		return
	}

	ds, n, err := knowledge.IngestUpload(s.cfg.KnowledgeDBDSN(), handler.Filename, raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	// Refresh the in-memory store so the running agent sees the new data.
	knowledge.Reload()

	slog.Info("Admin updated knowledge dataset", "dataset", ds, "rows", n, "file", handler.Filename, "ip", clientIP(r))
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": fmt.Sprintf("数据集 %s 已更新（%d 条）", ds, n),
		"dataset": ds,
		"rows":    n,
	})
}

// handleAdminKnowledgeStats reports per-dataset row counts in the knowledge
// store (MariaDB). GET /admin/knowledge/stats
func (s *Server) handleAdminKnowledgeStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	stats, err := knowledge.DatasetStats(s.cfg.KnowledgeDBDSN())
	if err != nil {
		slog.Error("Admin knowledge stats", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to read stats"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stats": stats})
}

// handleAdminKnowledgeExport exports a knowledge dataset as JSON.
func (s *Server) handleAdminKnowledgeExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	dataset := r.URL.Query().Get("dataset")
	if dataset == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "dataset parameter required"})
		return
	}

	data, err := knowledge.ExportDataset(s.cfg.KnowledgeDBDSN(), dataset)
	if err != nil {
		slog.Error("Admin knowledge export", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.json", dataset))
	_, _ = w.Write(data)
}

// handleAdminKnowledgeVersions handles knowledge version management.
func (s *Server) handleAdminKnowledgeVersions(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		dataset := r.URL.Query().Get("dataset")
		if dataset == "" {
			// List all datasets with version counts
			datasets, err := s.db.ListAllKnowledgeDatasets()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"datasets": datasets})
			return
		}
		versions, err := s.db.ListKnowledgeVersions(dataset)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"versions": versions})

	case http.MethodPost:
		// Record a new version
		var input struct {
			Dataset    string `json:"dataset"`
			EntryCount int    `json:"entry_count"`
			Checksum   string `json:"checksum"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		admin := s.getAdminFromRequest(r)
		// Get latest version
		latest, _ := s.db.GetLatestKnowledgeVersion(input.Dataset)
		newVersion := 1
		if latest != nil {
			newVersion = latest.Version + 1
		}
		record := &database.KnowledgeVersionRecord{
			Dataset:    input.Dataset,
			Version:    newVersion,
			EntryCount: input.EntryCount,
			Checksum:   input.Checksum,
			CreatedBy:  admin.Username,
		}
		if err := s.db.AddKnowledgeVersion(record); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": record})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleAdminSessions handles session management.
func (s *Server) handleAdminSessions(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Get pagination params
		limit := 50
		offset := 0
		if v := r.URL.Query().Get("limit"); v != "" {
			fmt.Sscanf(v, "%d", &limit)
		}
		if v := r.URL.Query().Get("offset"); v != "" {
			fmt.Sscanf(v, "%d", &offset)
		}

		recs, err := s.db.ListAllSessions(limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		// Get message counts for each session
		type sessionWithCount struct {
			database.SessionRecord
			MessageCount int `json:"message_count"`
		}
		var sessions []sessionWithCount
		for _, rec := range recs {
			msgs, _ := s.db.GetSessionMessages(rec.ID)
			sessions = append(sessions, sessionWithCount{
				SessionRecord: rec,
				MessageCount:  len(msgs),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})

	case http.MethodDelete:
		// Bulk delete sessions
		var input struct {
			SessionIDs []string `json:"session_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		var deleted int
		for _, id := range input.SessionIDs {
			if err := s.db.DeleteSession(id); err == nil {
				s.agent.DeleteSession(id)
				deleted++
			}
		}
		// Record audit log
		admin := s.getAdminFromRequest(r)
		if admin != nil {
			_ = s.db.AddAuditLog(&database.AuditLogRecord{
				AdminID:      admin.ID,
				AdminUsername: admin.Username,
				Action:       "batch_delete_sessions",
				TargetType:   "session",
				Details:      fmt.Sprintf("deleted %d sessions", deleted),
				IPAddress:    clientIP(r),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "deleted": deleted})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleAdminSessionByID handles single session operations.
func (s *Server) handleAdminSessionByID(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/admin/sessions/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "session id required"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Get session with messages
		rec, err := s.db.GetSession(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		if rec == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "session not found"})
			return
		}
		msgs, err := s.db.GetSessionMessages(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		type msg struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		var messages []msg
		for _, m := range msgs {
			messages = append(messages, msg{Role: m.Role, Content: m.Content})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":       rec.ID,
			"title":    rec.Title,
			"messages": messages,
		})

	case http.MethodDelete:
		if err := s.db.DeleteSession(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		s.agent.DeleteSession(id)
		// Record audit log
		admin := s.getAdminFromRequest(r)
		if admin != nil {
			_ = s.db.AddAuditLog(&database.AuditLogRecord{
				AdminID:      admin.ID,
				AdminUsername: admin.Username,
				Action:       "delete_session",
				TargetType:   "session",
				TargetID:     id,
				IPAddress:    clientIP(r),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleAdminFeedback handles feedback management.
func (s *Server) handleAdminFeedback(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		fmt.Sscanf(v, "%d", &offset)
	}

	feedback, err := s.db.GetFeedbackWithDetails(limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"feedback": feedback})
}

// handleAdminFeedbackStats handles feedback statistics.
func (s *Server) handleAdminFeedbackStats(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "day"
	}

	stats, err := s.db.GetFeedbackStatsByPeriod(period)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	// Get overall stats
	up, down, _ := s.db.GetFeedbackStats()
	writeJSON(w, http.StatusOK, map[string]any{
		"overall": map[string]any{"up": up, "down": down},
		"by_period": stats,
	})
}

// handleAdminAuditLogs handles audit log management.
func (s *Server) handleAdminAuditLogs(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		fmt.Sscanf(v, "%d", &offset)
	}

	logs, err := s.db.ListAuditLogs(limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	total, _ := s.db.GetAuditLogsCount()
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs, "total": total})
}

// handleAdminConfig handles system configuration.
func (s *Server) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		configs, err := s.db.ListSystemConfigs()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"configs": configs})

	case http.MethodPost:
		var input struct {
			Key         string `json:"key"`
			Value       string `json:"value"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		admin := s.getAdminFromRequest(r)
		if err := s.db.SetSystemConfig(input.Key, input.Value, input.Description, admin.Username); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		// Record audit log
		_ = s.db.AddAuditLog(&database.AuditLogRecord{
			AdminID:      admin.ID,
			AdminUsername: admin.Username,
			Action:       "set_config",
			TargetType:   "config",
			TargetID:     input.Key,
			Details:      input.Value,
			IPAddress:    clientIP(r),
		})
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleAdminConfigByKey handles single config operations.
func (s *Server) handleAdminConfigByKey(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	key := strings.TrimPrefix(r.URL.Path, "/admin/config/")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "config key required"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		value, err := s.db.GetSystemConfig(key)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"key": key, "value": value})

	case http.MethodDelete:
		if err := s.db.DeleteSystemConfig(key); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		// Record audit log
		admin := s.getAdminFromRequest(r)
		if admin != nil {
			_ = s.db.AddAuditLog(&database.AuditLogRecord{
				AdminID:      admin.ID,
				AdminUsername: admin.Username,
				Action:       "delete_config",
				TargetType:   "config",
				TargetID:     key,
				IPAddress:    clientIP(r),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleAdminAPIStats handles API usage statistics.
func (s *Server) handleAdminAPIStats(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		fmt.Sscanf(v, "%d", &offset)
	}

	stats, err := s.db.ListAPIStats(limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stats": stats})
}

// handleAdminAPIStatsSummary handles API statistics summary.
func (s *Server) handleAdminAPIStatsSummary(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	hours := 24
	if v := r.URL.Query().Get("hours"); v != "" {
		fmt.Sscanf(v, "%d", &hours)
	}

	summary, err := s.db.GetAPIStatsSummary(hours)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// handleAdminAnalytics handles user behavior analysis.
func (s *Server) handleAdminAnalytics(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	stats, err := s.db.GetUserBehaviorStats()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// handleAdminBatchUsers handles batch user operations.
func (s *Server) handleAdminBatchUsers(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	switch r.Method {
	case http.MethodPost:
		// Batch create users
		var input struct {
			Users []struct {
				Username string `json:"username"`
				Password string `json:"password"`
				Nickname string `json:"nickname"`
				IsAdmin  bool   `json:"is_admin"`
			} `json:"users"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}

		admin := s.getAdminFromRequest(r)
		var created, failed int
		for _, u := range input.Users {
			_, err := s.auth.AdminCreateUser(&auth.AdminCreateUserInput{
				Username: u.Username,
				Password: u.Password,
				Nickname: u.Nickname,
				IsAdmin:  u.IsAdmin,
			}, admin)
			if err == nil {
				created++
			} else {
				failed++
			}
		}
		// Record audit log
		_ = s.db.AddAuditLog(&database.AuditLogRecord{
			AdminID:      admin.ID,
			AdminUsername: admin.Username,
			Action:       "batch_create_users",
			TargetType:   "user",
			Details:      fmt.Sprintf("created %d, failed %d", created, failed),
			IPAddress:    clientIP(r),
		})
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "created": created, "failed": failed})

	case http.MethodDelete:
		// Batch delete users
		var input struct {
			UserIDs []string `json:"user_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		var deleted int
		for _, id := range input.UserIDs {
			if err := s.auth.DeleteUser(id); err == nil {
				deleted++
			}
		}
		// Record audit log
		admin := s.getAdminFromRequest(r)
		if admin != nil {
			_ = s.db.AddAuditLog(&database.AuditLogRecord{
				AdminID:      admin.ID,
				AdminUsername: admin.Username,
				Action:       "batch_delete_users",
				TargetType:   "user",
				Details:      fmt.Sprintf("deleted %d users", deleted),
				IPAddress:    clientIP(r),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "deleted": deleted})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleAdminBatchKnowledge handles batch knowledge operations.
func (s *Server) handleAdminBatchKnowledge(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	// Parse multipart form (200MB max for multiple files)
	if err := r.ParseMultipartForm(200 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("failed to parse form: %v", err)})
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "no files provided"})
		return
	}

	type result struct {
		Filename string `json:"filename"`
		Dataset  string `json:"dataset,omitempty"`
		Rows     int    `json:"rows,omitempty"`
		Error    string `json:"error,omitempty"`
	}
	var results []result

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			results = append(results, result{Filename: fileHeader.Filename, Error: err.Error()})
			continue
		}

		raw, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			results = append(results, result{Filename: fileHeader.Filename, Error: err.Error()})
			continue
		}

		ds, n, err := knowledge.IngestUpload(s.cfg.KnowledgeDBDSN(), fileHeader.Filename, raw)
		if err != nil {
			results = append(results, result{Filename: fileHeader.Filename, Error: err.Error()})
			continue
		}

		results = append(results, result{Filename: fileHeader.Filename, Dataset: ds, Rows: n})
	}

	// Refresh knowledge store
	knowledge.Reload()

	// Record audit log
	admin := s.getAdminFromRequest(r)
	if admin != nil {
		_ = s.db.AddAuditLog(&database.AuditLogRecord{
			AdminID:      admin.ID,
			AdminUsername: admin.Username,
			Action:       "batch_upload_knowledge",
			TargetType:   "knowledge",
			Details:      fmt.Sprintf("uploaded %d files", len(files)),
			IPAddress:    clientIP(r),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "results": results})
}

// handleAdminExport handles data export.
func (s *Server) handleAdminExport(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	exportType := r.URL.Query().Get("type")
	if exportType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "type parameter required (sessions, feedback, audit_logs, config)"})
		return
	}

	switch exportType {
	case "sessions":
		sessions, err := s.db.ListAllSessions(10000)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=sessions.json")
		json.NewEncoder(w).Encode(sessions)

	case "feedback":
		feedback, err := s.db.GetFeedbackWithDetails(10000, 0)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=feedback.json")
		json.NewEncoder(w).Encode(feedback)

	case "audit_logs":
		logs, err := s.db.ListAuditLogs(10000, 0)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=audit_logs.json")
		json.NewEncoder(w).Encode(logs)

	case "config":
		configs, err := s.db.ListSystemConfigs()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=config.json")
		json.NewEncoder(w).Encode(configs)

	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown export type"})
	}
}
