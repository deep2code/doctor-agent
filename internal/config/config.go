package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// LLM Provider
	LLMProvider     string // "anthropic", "deepseek", or "openai-compat"
	AnthropicAPIKey string
	AnthropicModel  string
	DeepSeekAPIKey  string
	DeepSeekModel   string
	// OpenAI-compatible endpoint (Zhipu/Qwen/SiliconFlow/...)
	OpenAICompatBaseURL string
	OpenAICompatAPIKey  string
	OpenAICompatModel   string

	MaxTokens       int
	Temperature     float64
	MaxHistoryTurns int

	// Knowledge Retrieval
	KnowledgeTopK    int
	KnowledgeEnabled bool

	// Vector Database
	VectorDBProvider string // "qdrant" or "" (keyword-only)
	QdrantHost       string
	QdrantPort       int

	// Embedding
	EmbeddingProvider string // "deepseek", "voyage", or "" (no embedding)
	EmbeddingModel    string
	VoyageAPIKey      string

	// Safety
	EmergencyEnabled  bool
	ScopeGuardEnabled bool
	PostVerifyEnabled bool

	// Semantic claim verification (LLM-as-judge)
	JudgeEnabled bool
	JudgeModel   string

	// Server
	ServerHost string
	ServerPort string

	// Server security
	// APIKey enables Bearer-token auth on /chat endpoints when non-empty.
	APIKey string
	// CORSOrigins is an allowlist of origins (empty = allow all with "*").
	CORSOrigins []string
	// RateLimit caps requests per IP per minute; 0 disables rate limiting.
	RateLimit int

	// SessionDir persists conversation snapshots as JSON files under this
	// directory; empty disables persistence (in-memory sessions only).
	SessionDir string

	// Logging
	LogLevel string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		LLMProvider:     getEnv("LLM_PROVIDER", "deepseek"),
		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", ""),
		AnthropicModel:  getEnv("ANTHROPIC_MODEL", "claude-sonnet-4-20250514"),
		DeepSeekAPIKey:  getEnv("DEEPSEEK_API_KEY", ""),
		DeepSeekModel:   getEnv("DEEPSEEK_MODEL", "deepseek-v4-pro"),

		OpenAICompatBaseURL: getEnv("OPENAI_COMPAT_BASE_URL", ""),
		OpenAICompatAPIKey:  getEnv("OPENAI_COMPAT_API_KEY", ""),
		OpenAICompatModel:   getEnv("OPENAI_COMPAT_MODEL", ""),

		MaxTokens:       getEnvInt("MAX_TOKENS", 4096),
		Temperature:     getEnvFloat("TEMPERATURE", 0.3),
		MaxHistoryTurns: getEnvInt("MAX_HISTORY_TURNS", 20),

		KnowledgeTopK:    getEnvInt("KNOWLEDGE_TOP_K", 5),
		KnowledgeEnabled: getEnvBool("KNOWLEDGE_RETRIEVAL_ENABLED", true),

		VectorDBProvider: getEnv("VECTOR_DB_PROVIDER", ""),
		QdrantHost:       getEnv("QDRANT_HOST", "localhost"),
		QdrantPort:       getEnvInt("QDRANT_PORT", 6334),

		EmbeddingProvider: getEnv("EMBEDDING_PROVIDER", ""),
		EmbeddingModel:    getEnv("EMBEDDING_MODEL", "voyage-multilingual-2"),
		VoyageAPIKey:      getEnv("VOYAGE_API_KEY", ""),

		EmergencyEnabled:  getEnvBool("EMERGENCY_DETECTION_ENABLED", true),
		ScopeGuardEnabled: getEnvBool("SCOPE_GUARD_ENABLED", true),
		PostVerifyEnabled: getEnvBool("POST_VERIFY_ENABLED", true),

		JudgeEnabled: getEnvBool("POST_VERIFY_SEMANTIC", false),
		JudgeModel:   getEnv("POST_VERIFY_JUDGE_MODEL", ""),

		ServerHost: getEnv("SERVER_HOST", "0.0.0.0"),
		ServerPort: getEnv("SERVER_PORT", "8080"),

		APIKey:      getEnv("API_KEY", ""),
		CORSOrigins: splitCSV(getEnv("CORS_ORIGINS", "")),
		RateLimit:   getEnvInt("RATE_LIMIT", 0),

		SessionDir: getEnv("SESSION_DIR", ""),

		LogLevel: getEnv("LOG_LEVEL", "info"),
	}
}

// splitCSV splits a comma-separated list, trimming whitespace and dropping
// empty entries (used for CORS_ORIGINS).
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Validate checks that required configuration values are set based on the selected provider.
func (c *Config) Validate() error {
	switch c.LLMProvider {
	case "anthropic":
		if c.AnthropicAPIKey == "" {
			return fmt.Errorf("ANTHROPIC_API_KEY is required when LLM_PROVIDER=anthropic")
		}
	case "deepseek":
		if c.DeepSeekAPIKey == "" {
			return fmt.Errorf("DEEPSEEK_API_KEY is required when LLM_PROVIDER=deepseek")
		}
	case "openai-compat":
		if c.OpenAICompatAPIKey == "" || c.OpenAICompatBaseURL == "" {
			return fmt.Errorf("OPENAI_COMPAT_API_KEY and OPENAI_COMPAT_BASE_URL are required when LLM_PROVIDER=openai-compat")
		}
	default:
		return fmt.Errorf("unknown LLM_PROVIDER: %s (must be 'anthropic', 'deepseek' or 'openai-compat')", c.LLMProvider)
	}

	if c.VectorDBProvider == "qdrant" && c.EmbeddingProvider == "" {
		return fmt.Errorf("EMBEDDING_PROVIDER is required when VECTOR_DB_PROVIDER=qdrant")
	}

	return nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}
