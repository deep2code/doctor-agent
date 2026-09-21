package auth

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/doctor-agent/internal/database"
)

// Service handles user authentication.
type Service struct {
	db *database.DB
	// secret signs login tokens; see NewService.
	secret []byte
}

// TokenTTL bounds how long a login token stays valid without re-authenticating.
// Long enough that someone using this for a family health record is not logged
// out mid-question, short enough that a leaked token dies on its own.
const TokenTTL = 7 * 24 * time.Hour

// NewService creates a new auth service. secret is AUTH_SECRET and signs the
// HMAC bearer tokens IssueToken mints. An empty secret generates a random
// per-process key: local single-user use keeps working, but every token fails
// closed once the process restarts (and tokens from one instance are worthless
// to another), which is the safe direction for a forgotten setting.
func NewService(db *database.DB, secret string) *Service {
	s := &Service{db: db}
	if strings.TrimSpace(secret) == "" {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			// An all-zero token key would be forgeable, so stop instead.
			panic("crypto/rand unavailable: " + err.Error())
		}
		s.secret = key
		slog.Warn("AUTH_SECRET 未设置：登录令牌使用随机密钥，进程重启后失效")
	} else {
		s.secret = []byte(secret)
	}
	return s
}

// AdminCreateUserInput holds parameters for admin to create a user.
type AdminCreateUserInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
	Phone    string `json:"phone,omitempty"`
	Email    string `json:"email,omitempty"`
	IsAdmin  bool   `json:"is_admin"`
}

// internal logs a storage failure with its real cause and returns an error
// whose text is safe to show: the driver message stays in the log, while
// errors.Is(err, ErrInternal) still tells the handler this was a 500, not a
// validation problem.
func internal(op string, err error) error {
	slog.Error("auth: "+op+" failed", "error", err)
	return fmt.Errorf("%w (%s)", ErrInternal, op)
}

// AdminCreateUser creates a new user account (admin only).
func (s *Service) AdminCreateUser(input *AdminCreateUserInput, createdBy *database.User) (*database.User, error) {
	if createdBy == nil || !createdBy.IsAdmin {
		return nil, fmt.Errorf("只有管理员才能创建用户")
	}

	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Check if username exists
	existing, err := s.db.GetUserByUsername(input.Username)
	if err != nil {
		return nil, internal("checking username", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("用户名已存在")
	}

	// Generate user ID
	userID, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("generating user ID: %w", err)
	}

	// Hash password
	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		return nil, internal("hashing password", err)
	}

	user := &database.User{
		ID:           userID,
		Username:     input.Username,
		PasswordHash: passwordHash,
		Nickname:     input.Nickname,
		Phone:        input.Phone,
		Email:        input.Email,
		IsAdmin:      input.IsAdmin,
	}

	if err := s.db.CreateUser(user); err != nil {
		return nil, internal("creating user", err)
	}

	return user, nil
}

// Validate validates admin create user input.
func (i *AdminCreateUserInput) Validate() error {
	i.Username = strings.TrimSpace(i.Username)
	i.Password = strings.TrimSpace(i.Password)
	i.Nickname = strings.TrimSpace(i.Nickname)

	if i.Username == "" {
		return fmt.Errorf("用户名不能为空")
	}
	if len(i.Username) < 3 || len(i.Username) > 32 {
		return fmt.Errorf("用户名长度需在3-32之间")
	}
	if i.Password == "" {
		return fmt.Errorf("密码不能为空")
	}
	if len(i.Password) < 6 {
		return fmt.Errorf("密码长度不能少于6位")
	}
	if i.Nickname == "" {
		i.Nickname = i.Username
	}
	return nil
}

// GetUserByID retrieves a user by ID.
func (s *Service) GetUserByID(id string) (*database.User, error) {
	return s.db.GetUser(id)
}

// DeleteUser deletes a user by ID.
func (s *Service) DeleteUser(id string) error {
	return s.db.DeleteUser(id)
}

// ErrInvalidToken marks a bearer token that is malformed, forged or expired.
// Distinct from ErrInternal so handlers can answer 401 without logging a
// routine expired-session as a server error.
var ErrInvalidToken = errors.New("登录凭证无效或已过期")

// IssueToken mints a self-contained bearer token for userID, valid for
// TokenTTL:
//
//	base64url("<userID>|<expiryUnix>") + "." + hex(hmac_sha256(secret, payload))
//
// Stateless by design — no token table, nothing to clean up, and logout is
// just the client dropping the string. GetUserByToken re-reads the account on
// every call, which is what makes deletion act as revocation.
func (s *Service) IssueToken(userID string) string {
	payload := userID + "|" + strconv.FormatInt(time.Now().Add(TokenTTL).Unix(), 10)
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + hex.EncodeToString(s.mac(payload))
}

// mac authenticates a token payload.
func (s *Service) mac(payload string) []byte {
	m := hmac.New(sha256.New, s.secret)
	// hash.Hash.Write never returns an error.
	_, _ = m.Write([]byte(payload))
	return m.Sum(nil)
}

// VerifyToken checks a token's signature and expiry, returning its user ID.
func (s *Service) VerifyToken(token string) (string, error) {
	encoded, sig, ok := strings.Cut(strings.TrimSpace(token), ".")
	if !ok || encoded == "" || sig == "" {
		return "", ErrInvalidToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrInvalidToken
	}
	payload := string(raw)
	want, err := hex.DecodeString(sig)
	if err != nil || subtle.ConstantTimeCompare(s.mac(payload), want) != 1 {
		return "", ErrInvalidToken
	}
	userID, expiry, ok := strings.Cut(payload, "|")
	if !ok || userID == "" {
		return "", ErrInvalidToken
	}
	exp, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil || time.Now().Unix() >= exp {
		return "", ErrInvalidToken
	}
	return userID, nil
}

// GetUserByToken resolves a bearer token to its account.
func (s *Service) GetUserByToken(token string) (*database.User, error) {
	userID, err := s.VerifyToken(token)
	if err != nil {
		return nil, err
	}
	user, err := s.db.GetUser(userID)
	if err != nil {
		return nil, internal("resolving token user", err)
	}
	if user == nil {
		// The account was deleted after the token was issued.
		return nil, ErrInvalidToken
	}
	return user, nil
}

// LoginInput holds login parameters.
type LoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login authenticates a user and returns the user record.
func (s *Service) Login(input *LoginInput) (*database.User, error) {
	username := strings.TrimSpace(input.Username)
	password := strings.TrimSpace(input.Password)

	if username == "" || password == "" {
		return nil, fmt.Errorf("用户名和密码不能为空")
	}

	user, err := s.db.GetUserByUsername(username)
	if err != nil {
		return nil, internal("getting user", err)
	}
	if user == nil {
		return nil, ErrInvalidCredentials
	}

	matched, legacy := verifyPassword(password, user.PasswordHash)
	if !matched {
		return nil, ErrInvalidCredentials
	}
	if legacy {
		newHash, herr := hashPassword(password)
		if herr != nil {
			slog.Error("failed to derive upgraded password hash", "user_id", user.ID, "error", herr)
		} else if err := s.db.UpdateUserPasswordHash(user.ID, newHash); err != nil {
			// The credentials are valid, so a failed upgrade must not deny the
			// login — the old verifier stays and the upgrade is retried next time.
			slog.Warn("failed to upgrade legacy password hash", "user_id", user.ID, "error", err)
		} else {
			slog.Info("upgraded legacy password hash", "user_id", user.ID)
		}
	}

	// Update last login
	if err := s.db.UpdateUserLastLogin(user.ID); err != nil {
		slog.Warn("failed to update last login", "user_id", user.ID, "error", err)
	}

	return user, nil
}

// Password verifier formats. Current: PBKDF2-HMAC-SHA256, self-describing so
// verification never has to guess its parameters —
//
//	pbkdf2$sha256$<iterations>$<saltHex>$<derivedKeyHex>
//
// Legacy: the pre-2026-09 "<16 hex salt><64 hex sha256(salt||password)>" rows,
// which cost a single SHA256 round and are therefore wordlist-cheap to crack.
// They are still accepted at login and rewritten on the spot (see Login), so
// no user has to reset a password to get the stronger store.
const (
	pbkdf2Prefix     = "pbkdf2$sha256$"
	pbkdf2Iterations = 210000
	pbkdf2KeyLen     = 32
	legacySaltChars  = 16
)

// ErrInvalidCredentials covers "no such user" and "wrong password" with one
// message so the login API cannot be used to enumerate usernames.
var ErrInvalidCredentials = errors.New("用户名或密码错误")

// ErrInternal marks a failure whose detail (SQL text, DSN fragments) belongs in
// the log only. Handlers map it to a 500 with a fixed message.
var ErrInternal = errors.New("服务暂时不可用")

// hashPassword derives a PBKDF2 verifier for password with a fresh random salt.
func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		// Failing loudly beats silently hashing with an all-zero salt.
		panic("crypto/rand unavailable: " + err.Error())
	}
	dk, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iterations, pbkdf2KeyLen)
	if err != nil {
		return "", fmt.Errorf("deriving password hash: %w", err)
	}
	return pbkdf2Prefix + strconv.Itoa(pbkdf2Iterations) + "$" +
		hex.EncodeToString(salt) + "$" + hex.EncodeToString(dk), nil
}

// verifyPassword reports whether password matches stored. The second result is
// true only when a *legacy* verifier matched: the caller must then re-store
// hashPassword(password).
func verifyPassword(password, stored string) (bool, bool) {
	if strings.HasPrefix(stored, pbkdf2Prefix) {
		fields := strings.Split(stored, "$")
		if len(fields) != 5 {
			return false, false
		}
		iter, err := strconv.Atoi(fields[2])
		if err != nil || iter <= 0 {
			return false, false
		}
		salt, err := hex.DecodeString(fields[3])
		if err != nil {
			return false, false
		}
		want, err := hex.DecodeString(fields[4])
		if err != nil {
			return false, false
		}
		dk, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
		if err != nil {
			return false, false
		}
		return subtle.ConstantTimeCompare(dk, want) == 1, false
	}

	if len(stored) != legacySaltChars+64 {
		return false, false
	}
	want, err := hex.DecodeString(stored[legacySaltChars:])
	if err != nil {
		return false, false
	}
	sum := sha256.Sum256([]byte(stored[:legacySaltChars] + password))
	matched := subtle.ConstantTimeCompare(sum[:], want) == 1
	return matched, matched
}

// generateID generates a random 16-byte ID.
func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
