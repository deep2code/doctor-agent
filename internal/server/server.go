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

// Server wraps the HTTP API server for the doctor agent.
type Server struct {
	cfg    *config.Config
	agent  *agent.Agent
	auth   *auth.Service
	http   *http.Server
	limiter *rateLimiter
}

// New creates a new HTTP server.
func New(cfg *config.Config, ag *agent.Agent, authSvc *auth.Service) *Server {
	s := &Server{
		cfg:     cfg,
		agent:   ag,
		auth:    authSvc,
		limiter: newRateLimiter(cfg.RateLimit),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWebUI)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/chat", s.handleChat)
	mux.HandleFunc("/chat/stream", s.handleChatStream)
	mux.HandleFunc("/feedback", s.handleFeedback)
	// Admin endpoints
	mux.HandleFunc("/admin/users", s.handleAdminUsers)
	mux.HandleFunc("/admin/users/", s.handleAdminUser)
	// Sync endpoints
	mux.HandleFunc("/admin/sync", s.handleAdminSync)
	mux.HandleFunc("/admin/sync/status", s.handleAdminSyncStatus)

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
	Message        string       `json:"message"`
	ConversationID string       `json:"conversation_id,omitempty"`
	Images         []ChatImage  `json:"images,omitempty"`
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
			"id":        user.ID,
			"username":  user.Username,
			"nickname":  user.Nickname,
			"is_admin":  user.IsAdmin,
			"message":   "用户创建成功",
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
			"id":        user.ID,
			"username":  user.Username,
			"nickname":  user.Nickname,
			"is_admin":  user.IsAdmin,
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
	// Check for API key
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

	// Check for token-based auth
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
	if source == "" {
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

	embedder, err := embedding.NewOpenAICompat(embedding.Config{
		Provider: "openai-compat",
		BaseURL:  s.cfg.EmbeddingBaseURL,
		APIKey:   s.cfg.EmbeddingAPIKey,
		Model:    s.cfg.EmbeddingModel,
	})
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
