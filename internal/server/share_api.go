package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/doctor-agent/internal/database"
	"github.com/doctor-agent/internal/llm"
	"github.com/doctor-agent/internal/session"
)

// 分享快照结构：q 为用户提问（单条分享时才有），a 为 AI 回答正文。
// payload 以 JSON 注入 share.html 的 SHARE 变量，由前端极简渲染器展示。
type sharePair struct {
	Q string `json:"q,omitempty"`
	A string `json:"a"`
}

type sharePayload struct {
	Kind      string      `json:"kind"` // "answer" | "session"
	CreatedAt string      `json:"created_at"`
	M         []sharePair `json:"m"`
}

// shareQAPairs 把会话消息整理为问答对（跳过空 assistant 消息，例如纯工具调用轮）。
func shareQAPairs(msgs []llm.Message) []sharePair {
	var pairs []sharePair
	var pendingQ string
	for _, m := range msgs {
		switch m.Role {
		case "user":
			if m.Content != "" {
				pendingQ = m.Content
			}
		case "assistant":
			if m.Content == "" {
				continue
			}
			pairs = append(pairs, sharePair{Q: pendingQ, A: m.Content})
			pendingQ = ""
		}
	}
	return pairs
}

// handleShare 创建只读分享快照，返回分享链接。
// POST /share {"scope":"answer"|"session","conversation_id":"...","index":N}
// scope=answer 时 index 为第 N 个问答对（0 起）；scope=session 时分享整段会话。
func (s *Server) handleShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "分享功能需要数据库"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var req struct {
		Scope          string `json:"scope"`
		ConversationID string `json:"conversation_id"`
		Index          int    `json:"index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}
	if req.Scope != "answer" && req.Scope != "session" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "scope 必须是 answer 或 session"})
		return
	}
	if req.ConversationID == "" || !session.ValidID(req.ConversationID) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid conversation_id"})
		return
	}

	sess := s.agent.GetOrCreateSession(req.ConversationID)
	pairs := shareQAPairs(sess.GetMessages())
	if len(pairs) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "没有可分享的内容"})
		return
	}

	var payload sharePayload
	switch req.Scope {
	case "answer":
		if req.Index < 0 || req.Index >= len(pairs) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "index 超出范围"})
			return
		}
		payload = sharePayload{Kind: "answer", CreatedAt: time.Now().UTC().Format(time.RFC3339), M: []sharePair{pairs[req.Index]}}
	case "session":
		payload = sharePayload{Kind: "session", CreatedAt: time.Now().UTC().Format(time.RFC3339), M: pairs}
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil || len(payloadBytes) > 4<<20 {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "生成分享内容失败"})
		return
	}

	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal error"})
		return
	}
	shareID := hex.EncodeToString(idBytes)

	title := "健康问答分享"
	if len(payload.M) > 0 && payload.M[0].Q != "" {
		title = truncateRunes(payload.M[0].Q, 48)
	}
	if err := s.db.CreateShare(&database.ShareSnapshot{
		ID: shareID, Kind: req.Scope, Title: title, Payload: string(payloadBytes),
	}); err != nil {
		slog.Error("Failed to save share snapshot", "error", err, "conversation_id", req.ConversationID)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "保存分享失败"})
		return
	}

// 返回专用分享链接格式：/share/{id}，一眼可辨为分享链接
	writeJSON(w, http.StatusOK, map[string]any{"id": shareID, "url": "/share/" + shareID})
}
// handleSharePage 渲染只读分享页：GET /share/{id}。
func (s *Server) handleSharePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.db == nil {
		http.Error(w, "分享功能需要数据库", http.StatusServiceUnavailable)
		return
	}
id := strings.TrimPrefix(r.URL.Path, "/share/")
	if len(id) != 32 || !isHex(id) {
		http.Error(w, "分享链接无效", http.StatusBadRequest)
		return
	}
	snap, err := s.db.GetShare(id)
	if errors.Is(err, database.ErrShareNotFound) {
		http.Error(w, "分享内容不存在或已被删除", http.StatusNotFound)
		return
	}
	if err != nil {
		slog.Error("Failed to load share snapshot", "error", err, "share_id", id)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// "</" 转义为 "<\/"（JSON 合法转义），防止回答内容中出现 </script> 提前闭合脚本标签。
	safePayload := strings.ReplaceAll(snap.Payload, "</", `<\/`)

	page := strings.Replace(s.pageShareTmpl, "__PAYLOAD__", safePayload, 1)
	page = strings.Replace(page, "__TITLE__", html.EscapeString(snap.Title), 2)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, page)
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// truncateRunes 按字符数截断并加省略号。
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
