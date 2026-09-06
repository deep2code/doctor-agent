package config

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"

	_ "github.com/go-sql-driver/mysql"
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
	MaxToolIterations int // max LLM tool-use loops per message (default 5)

	// Knowledge Retrieval
	KnowledgeTopK    int
	KnowledgeEnabled bool

	// Colloquial→clinical query understanding: every user message passes an
	// LLM step that extracts structured clinical concepts and generates
	// multiple retrieval queries (recall-oriented; ambiguity becomes extra
	// recall branches). UnderstandModel optionally routes this step to a
	// cheaper/faster OpenAI-compatible model (empty = main provider).
	// AliasMapPath points at an optional JSON dictionary (alias → standard
	// terms) loaded into query expansion at startup.
	QueryUnderstandingEnabled bool
	UnderstandModel           string
	AliasMapPath              string

	// Vector Database
	VectorDBProvider string // "qdrant" or "" (keyword-only)
	QdrantHost       string
	QdrantPort       int

	// Embedding
	EmbeddingProvider   string // "deepseek", "voyage", or "" (no embedding)
	EmbeddingModel      string
	EmbeddingDimensions int // 0 = API default; 1024 forces 1024 for embedding-3-pro
	VoyageAPIKey        string

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
	// PublicBaseURL is the site's canonical public origin
	// ("https://yida.example.com"), used to build absolute URLs for
	// sitemap.xml, robots.txt, llms.txt and the pages' canonical/og:url
	// tags. When empty: crawler text files fall back to per-request Host
	// inference, and canonical/og:url tags are omitted from pages.
	PublicBaseURL string

	// SessionDir persists conversation snapshots as JSON files under this
	// directory; empty disables persistence (in-memory sessions only).
	SessionDir string

	// MariaDB (shared instance; knowledge store + app store as two databases)
	MariaDBHost        string
	MariaDBPort        int
	MariaDBUser        string
	MariaDBPassword    string
	MariaDBKnowledgeDB string // knowledge store database name
	MariaDBAppDB       string // users/sessions/feedback database name

	// Admin
	AdminPassword string // Initial admin password

	// Vector Store
	VectorStoreEnabled bool
	VectorStoreHost    string
	VectorStorePort    int
	VectorCollection   string

	// Embedding
	EmbeddingEnabled bool
	EmbeddingBaseURL string
	EmbeddingAPIKey  string

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

		MaxTokens:        getEnvInt("MAX_TOKENS", 4096),
		Temperature:      getEnvFloat("TEMPERATURE", 0.3),
		MaxHistoryTurns:  getEnvInt("MAX_HISTORY_TURNS", 20),
		MaxToolIterations: getEnvInt("MAX_TOOL_ITERATIONS", 5),

		KnowledgeTopK:    getEnvInt("KNOWLEDGE_TOP_K", 5),
		KnowledgeEnabled: getEnvBool("KNOWLEDGE_RETRIEVAL_ENABLED", true),

		QueryUnderstandingEnabled: getEnvBool("QUERY_UNDERSTANDING_ENABLED", true),
		UnderstandModel:           getEnv("UNDERSTAND_MODEL", ""),
		AliasMapPath:              getEnv("ALIAS_MAP_PATH", "data/alias_map.json"),

		VectorDBProvider: getEnv("VECTOR_DB_PROVIDER", ""),
		QdrantHost:       getEnv("QDRANT_HOST", "localhost"),
		QdrantPort:       getEnvInt("QDRANT_PORT", 6334),

		EmbeddingProvider:   getEnv("EMBEDDING_PROVIDER", ""),
		EmbeddingModel:      getEnv("EMBEDDING_MODEL", "bge-m3"),
		EmbeddingDimensions: getEnvInt("EMBEDDING_DIMENSIONS", 0),
		VoyageAPIKey:        getEnv("VOYAGE_API_KEY", ""),

		EmergencyEnabled:  getEnvBool("EMERGENCY_DETECTION_ENABLED", true),
		ScopeGuardEnabled: getEnvBool("SCOPE_GUARD_ENABLED", true),
		PostVerifyEnabled: getEnvBool("POST_VERIFY_ENABLED", true),

		JudgeEnabled: getEnvBool("POST_VERIFY_SEMANTIC", false),
		JudgeModel:   getEnv("POST_VERIFY_JUDGE_MODEL", ""),

		ServerHost: getEnv("SERVER_HOST", "0.0.0.0"),
		ServerPort: getEnv("SERVER_PORT", "7071"),

		APIKey:      getEnv("API_KEY", ""),
		CORSOrigins: splitCSV(getEnv("CORS_ORIGINS", "")),
		RateLimit:   getEnvInt("RATE_LIMIT", 0),
		PublicBaseURL: strings.TrimRight(getEnv("PUBLIC_BASE_URL", ""), "/"),

		SessionDir: getEnv("SESSION_DIR", ""),

		MariaDBHost:        getEnv("MARIA_DB_HOST", "localhost"),
		MariaDBPort:        getEnvInt("MARIA_DB_PORT", 3306),
		MariaDBUser:        getEnv("MARIA_DB_USER", "root"),
		MariaDBPassword:    getEnv("MARIA_DB_PASSWORD", ""),
		MariaDBKnowledgeDB: getEnv("MARIA_DB_KNOWLEDGE_DB", "doctor_knowledge"),
		MariaDBAppDB:       getEnv("MARIA_DB_APP_DB", "doctor_agent"),

		AdminPassword: getEnv("ADMIN_PASSWORD", ""),

		VectorStoreEnabled: getEnvBool("VECTOR_STORE_ENABLED", true),
		VectorStoreHost:    getEnv("VECTOR_STORE_HOST", "localhost"),
		VectorStorePort:    getEnvInt("VECTOR_STORE_PORT", 6334),
		VectorCollection:   getEnv("VECTOR_COLLECTION", "medical_knowledge"),

		EmbeddingEnabled: getEnvBool("EMBEDDING_ENABLED", true),
		EmbeddingBaseURL: getEnv("EMBEDDING_BASE_URL", ""),
		EmbeddingAPIKey:  getEnv("EMBEDDING_API_KEY", ""),

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

// MariaDBDSN builds a Go MySQL driver DSN for the given database name.
// interpolateParams avoids a server-side prepare round-trip per statement,
// which matters for the multi-row INSERTs used during seed-knowledge.
func (c *Config) MariaDBDSN(database string) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci&interpolateParams=true",
		c.MariaDBUser, c.MariaDBPassword, c.MariaDBHost, c.MariaDBPort, database)
}

// KnowledgeDBDSN returns the DSN for the knowledge store. An explicit
// KNOWLEDGE_DB_DSN env var overrides the composed MariaDB DSN.
func (c *Config) KnowledgeDBDSN() string {
	if d := os.Getenv("KNOWLEDGE_DB_DSN"); d != "" {
		return d
	}
	return c.MariaDBDSN(c.MariaDBKnowledgeDB)
}

// AppDBDSN returns the DSN for the application store (users/sessions/feedback).
func (c *Config) AppDBDSN() string {
	if d := os.Getenv("APP_DB_DSN"); d != "" {
		return d
	}
	return c.MariaDBDSN(c.MariaDBAppDB)
}

// MariaDBServerDSN returns a DSN without a database name, used to create
// databases at startup (so deployment needs no external init SQL).
func (c *Config) MariaDBServerDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci&interpolateParams=true",
		c.MariaDBUser, c.MariaDBPassword, c.MariaDBHost, c.MariaDBPort)
}

// EnsureKnowledgeDB creates the knowledge database if it does not exist.
func (c *Config) EnsureKnowledgeDB() error {
	return ensureDatabase(c.MariaDBServerDSN(), c.MariaDBKnowledgeDB)
}

// EnsureAppDB creates the application database if it does not exist.
func (c *Config) EnsureAppDB() error {
	return ensureDatabase(c.MariaDBServerDSN(), c.MariaDBAppDB)
}

func ensureDatabase(serverDSN, dbName string) error {
	conn, err := sql.Open("mysql", serverDSN)
	if err != nil {
		return fmt.Errorf("open server connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.Exec(fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		dbName)); err != nil {
		return fmt.Errorf("create database %s: %w", dbName, err)
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
