package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/doctor-agent/internal/database"
)

// Service handles user authentication.
type Service struct {
	db *database.DB
}

// NewService creates a new auth service.
func NewService(db *database.DB) *Service {
	return &Service{db: db}
}

// RegisterInput holds registration parameters.
type RegisterInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
	Phone    string `json:"phone,omitempty"`
	Email    string `json:"email,omitempty"`
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

// Validate validates registration input.
func (i *RegisterInput) Validate() error {
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

// Register creates a new user account (admin-only).
func (s *Service) Register(input *RegisterInput) (*database.User, error) {
	// Public registration is disabled - use AdminCreateUser instead
	return nil, fmt.Errorf("公开注册已禁用，请联系管理员创建账号")
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
		return nil, fmt.Errorf("checking username: %w", err)
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
	passwordHash := hashPassword(input.Password)

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
		return nil, fmt.Errorf("creating user: %w", err)
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

// GetUserByToken retrieves a user by token (simplified implementation).
func (s *Service) GetUserByToken(token string) (*database.User, error) {
	// In a real implementation, you would validate the token against a tokens table
	// For now, return nil (token auth not fully implemented)
	return nil, nil
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
		return nil, fmt.Errorf("getting user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("用户名或密码错误")
	}

	if !verifyPassword(password, user.PasswordHash) {
		return nil, fmt.Errorf("用户名或密码错误")
	}

	// Update last login
	if err := s.db.UpdateUserLastLogin(user.ID); err != nil {
		// Log but don't fail
		fmt.Printf("failed to update last login: %v\n", err)
	}

	return user, nil
}

// GetUser retrieves a user by ID.
func (s *Service) GetUser(id string) (*database.User, error) {
	return s.db.GetUser(id)
}

// hashPassword hashes a password with SHA256 + salt.
func hashPassword(password string) string {
	salt := generateSalt()
	hash := sha256.Sum256([]byte(salt + password))
	return salt + hex.EncodeToString(hash[:])
}

// verifyPassword verifies a password against its hash.
func verifyPassword(password, hash string) bool {
	if len(hash) < 16 {
		return false
	}
	salt := hash[:16]
	expectedHash := hash[16:]
	actualHash := sha256.Sum256([]byte(salt + password))
	return hex.EncodeToString(actualHash[:]) == expectedHash
}

// generateSalt generates a random 8-byte salt.
func generateSalt() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// generateID generates a random 16-byte ID.
func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Token represents an authentication token.
type Token struct {
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	ExpiresAt time.Time `json:"expires_at"`
}

// GenerateToken generates a simple token (in production, use JWT).
func GenerateToken(user *database.User) *Token {
	return &Token{
		UserID:    user.ID,
		Username:  user.Username,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
}

// ValidateToken validates a token string (simplified - in production use JWT).
func ValidateToken(tokenStr string, db *database.DB) (*Token, error) {
	if tokenStr == "" {
		return nil, fmt.Errorf("token is empty")
	}

	// Simple token format: userID:username:expiresAt
	parts := strings.SplitN(tokenStr, ":", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	token := &Token{
		UserID:   parts[0],
		Username: parts[1],
	}

	// In production, parse and validate expiry
	// For now, just check if user exists
	user, err := db.GetUser(token.UserID)
	if err != nil || user == nil {
		return nil, fmt.Errorf("invalid token")
	}

	return token, nil
}
