package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/doctor-agent/internal/agent"
	"github.com/doctor-agent/internal/config"
)

// Server wraps the HTTP API server for the doctor agent.
type Server struct {
	cfg    *config.Config
	agent  *agent.Agent
	http   *http.Server
}

// New creates a new HTTP server.
func New(cfg *config.Config, ag *agent.Agent) *Server {
	s := &Server{
		cfg:   cfg,
		agent: ag,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/chat", s.handleChat)
	mux.HandleFunc("/chat/stream", s.handleChatStream)

	s.http = &http.Server{
		Addr:         fmt.Sprintf("%s:%s", cfg.ServerHost, cfg.ServerPort),
		Handler:      withMiddleware(mux),
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

// ChatRequest is the JSON body for /chat endpoints.
type ChatRequest struct {
	Message        string `json:"message"`
	ConversationID string `json:"conversation_id,omitempty"`
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

	if req.ConversationID == "" {
		req.ConversationID = fmt.Sprintf("conv-%d", time.Now().UnixNano())
	}

	sess := s.agent.GetOrCreateSession(req.ConversationID)

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	resp, err := s.agent.ProcessMessage(ctx, sess, req.Message)
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

// handleChatStream provides SSE streaming responses.
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}

	// For now, stream mode uses the same ProcessMessage and sends the response as SSE.
	// Full token-by-token streaming can be added later using the SDK's streaming API.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if req.ConversationID == "" {
		req.ConversationID = fmt.Sprintf("conv-%d", time.Now().UnixNano())
	}

	sess := s.agent.GetOrCreateSession(req.ConversationID)

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	resp, err := s.agent.ProcessMessage(ctx, sess, req.Message)
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: {\"error\":\"%s\"}\n\n", err.Error())
		w.(http.Flusher).Flush()
		return
	}

	// Send as single SSE event (future: chunk the response)
	responseJSON, _ := json.Marshal(ChatResponse{
		ConversationID: req.ConversationID,
		Reply:          resp.Text,
		IsEmergency:    resp.IsEmergency,
		IsOutOfScope:   resp.IsOutOfScope,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	})

	fmt.Fprintf(w, "event: message\ndata: %s\n\n", string(responseJSON))
	w.(http.Flusher).Flush()
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// withMiddleware adds common middleware to the handler.
func withMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
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
