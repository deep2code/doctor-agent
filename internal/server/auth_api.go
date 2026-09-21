package server

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/doctor-agent/internal/auth"
	"github.com/doctor-agent/internal/database"
	"github.com/doctor-agent/internal/session"
)

// Per-user identity (/login + the ownership checks on /chat, /sessions,
// /family, /share).
//
// Two credentials reach this server and they mean different things:
//
//   - API_KEY is a *deployment* credential — it authorizes whoever runs the
//     backend, and it is what the middleware's auth gate checks.
//   - a login token from POST /login is a *person*. Only a person owns
//     personal data, so only a person's token unlocks sessions/family/share.
//
// Keeping them separate matters because API_KEY is shared: if it resolved to a
// user, every holder of the key would read the same account's medical history,
// and the operator's own automation would silently inherit a patient's data.

// callerKey tags the request context with the resolved identity.
type callerKey struct{}

// caller is the request's identity: the account behind a login token. Only
// resolveCaller can produce one, and it deliberately returns no caller for
// API_KEY, so an anonymous request and a request carrying the shared
// deployment key both end up in the "" bucket.
type caller struct {
	user *database.User
}

// owner is the value stored in sessions.user_id / family_members.user_id for
// data this caller may read or write. Anonymous callers and API_KEY callers
// share the "" bucket, which is exactly what pre-login deployments already
// used, so adding accounts never re-points existing data.
func (c *caller) owner() string {
	if c == nil || c.user == nil {
		return ""
	}
	return c.user.ID
}

// callerOf returns the identity resolved by the middleware (nil = anonymous).
func callerOf(r *http.Request) *caller {
	if v, ok := r.Context().Value(callerKey{}).(*caller); ok {
		return v
	}
	return nil
}

// ownerOf is the personal-data bucket for this request.
func (s *Server) ownerOf(r *http.Request) string {
	return callerOf(r).owner()
}

// bearerToken returns the Authorization: Bearer value, or "" when absent.
func bearerToken(r *http.Request) string {
	authz := r.Header.Get("Authorization")
	if !strings.HasPrefix(authz, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
}

// resolveCaller turns a login token into its account. It returns nil when the
// request carries no user token — including when it carries API_KEY, which is
// deliberately not an identity here.
func (s *Server) resolveCaller(r *http.Request) *caller {
	if s.auth == nil {
		return nil
	}
	token := bearerToken(r)
	if token == "" {
		return nil
	}
	user, err := s.auth.GetUserByToken(token)
	if err != nil {
		if !errors.Is(err, auth.ErrInvalidToken) {
			slog.Error("resolving login token", "error", err)
		}
		return nil
	}
	return &caller{user: user}
}

// userPublicFields renders a user for a client. Fields are listed explicitly
// (rather than marshalling the record) so a new column on database.User cannot
// leak just because someone forgot to add a json:"-" tag.
func userPublicFields(u *database.User) map[string]any {
	return map[string]any{
		"id":       u.ID,
		"username": u.Username,
		"nickname": u.Nickname,
		"is_admin": u.IsAdmin,
	}
}

// handleLogin verifies credentials and returns a bearer token.
//
//	POST /login {"username":"...","password":"..."}
//	→ {"token":"...","expires_at":"...","user":{...}}
//
// Failed attempts are covered by the per-IP rate limiter in withMiddleware,
// and Login answers "no such user" and "wrong password" identically, so this
// endpoint cannot be used to enumerate usernames.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.auth == nil || s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "登录需要数据库"})
		return
	}

	var req auth.LoginInput
	if !decodeJSONBody(w, r, smallBodyLimit, &req) {
		return
	}

	user, err := s.auth.Login(&req)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		slog.Warn("Failed login", "username", strings.TrimSpace(req.Username), "ip", clientIP(r))
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "用户名或密码错误"})
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token":      s.auth.IssueToken(user.ID),
		"expires_at": time.Now().Add(auth.TokenTTL).UTC().Format(time.RFC3339),
		"user":       userPublicFields(user),
	})
}

// handleMe reports who the presented token belongs to, so the UI can tell
// "logged out" from "token expired" without a second round trip.
//
//	GET /me → {"user": {...}} | {"user": null}
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var out any
	if c := callerOf(r); c != nil && c.user != nil {
		out = userPublicFields(c.user)
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": out})
}

// claimConversation returns the conversation the caller is asking to use,
// binding a brand-new id to them on first touch. It answers for the caller:
// false means a response has already been written. Another account's
// conversation is refused as 404, not 403, so probing ids cannot confirm that
// somebody else has a conversation with that id.
func (s *Server) claimConversation(w http.ResponseWriter, r *http.Request, conversationID string) (*session.Session, bool) {
	sess, ok := s.agent.ClaimSession(conversationID, s.ownerOf(r))
	if ok {
		return sess, true
	}
	slog.Warn("Conversation ownership check failed",
		"conversation_id", conversationID, "ip", clientIP(r))
	writeJSON(w, http.StatusNotFound, map[string]any{"error": "会话不存在"})
	return nil, false
}
