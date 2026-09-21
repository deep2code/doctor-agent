package config

import (
	"database/sql"
	"fmt"
	"log/slog"
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
	// DeepSeekVisionModel handles requests that carry images. DeepSeek's
	// mainline text models reject image input; only the vision variant
	// accepts it.
	DeepSeekVisionModel string
	// OpenAI-compatible endpoint (Zhipu/Qwen/SiliconFlow/...)
	OpenAICompatBaseURL string
	OpenAICompatAPIKey  string
	OpenAICompatModel   string
	// OpenAICompatVisionModel handles image-carrying requests when the main
	// model is text-only. Empty = route images to the main model as-is.
	OpenAICompatVisionModel string

	MaxTokens         int
	Temperature       float64
	MaxHistoryTurns   int
	MaxToolIterations int // max LLM tool-use loops per message (default 5)
	MaxToolCalls      int // max successful tool calls per message before forcing text answer (default 5)

	// Knowledge Retrieval
	KnowledgeTopK    int
	KnowledgeEnabled bool

	// Colloquial→clinical query understanding: every user message passes an
	// LLM step that extracts structured clinical concepts and generates
	// multiple retrieval queries (recall-oriented; ambiguity becomes extra
	// recall branches). On-demand: the LLM call only runs when verbatim
	// retrieval returns fewer than KnowledgeTopK hits, so ordinary queries
	// skip its latency. UnderstandModel optionally routes this step to a
	// cheaper/faster OpenAI-compatible model (empty = main provider).
	// AliasMapPath points at an optional JSON dictionary (alias → standard
	// terms) loaded into query expansion at startup.
	QueryUnderstandingEnabled  bool
	QueryUnderstandingBranches int // max parallel retrieval branches from understanding (default 5)
	UnderstandModel            string
	AliasMapPath               string

	// Media (3D 渲染动画 webm 磁盘目录)
	MediaDir string // 由 /media/ 提供服务，默认 data/media

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
	// RateLimit caps requests per IP per minute. Defaults to defaultRateLimit;
	// explicit RATE_LIMIT=0 disables rate limiting.
	RateLimit int
	// TrustedProxies lists CIDR networks or literal IPs whose forwarding
	// headers may be believed. Empty (default) means no proxy header is ever
	// trusted, so per-IP limits and logs use the TCP peer address. Set this
	// when the app runs behind nginx/Caddy/a load balancer, otherwise every
	// visitor shares one bucket.
	TrustedProxies []string
	// PublicBaseURL is the site's canonical public origin
	// ("https://yida.example.com"), used to build absolute URLs for
	// sitemap.xml, robots.txt, llms.txt and the pages' canonical/og:url
	// tags. When empty: crawler text files fall back to per-request Host
	// inference, and canonical/og:url tags are omitted from pages.
	PublicBaseURL string

	// SessionDir persists conversation snapshots as JSON files under this
	// directory; empty disables persistence (in-memory sessions only).
	SessionDir string

	// In-memory session map bounds. Every conversation id a client invents gets
	// a Session object cached for the process lifetime, so these have to be
	// finite. SessionIdleMinutes drops conversations nobody has touched for
	// that long (they reload from the store on the next request);
	// MaxActiveSessions is the hard ceiling, evicting least-recently-touched
	// conversations first. Set either to 0 to disable that rule.
	SessionIdleMinutes int
	MaxActiveSessions  int

	// MariaDB (shared instance; knowledge store + app store as two databases)
	MariaDBHost        string
	MariaDBPort        int
	MariaDBUser        string
	MariaDBPassword    string
	MariaDBKnowledgeDB string // knowledge store database name
	MariaDBAppDB       string // users/sessions/feedback database name

	// Admin
	AdminPassword string // Initial admin password

	// AuthSecret signs login tokens issued by POST /login (see internal/auth).
	// Empty generates a random per-process key, so tokens stop validating on
	// restart — safe (they cannot be forged or replayed against a new process)
	// but inconvenient, so set a fixed secret for any real deployment.
	AuthSecret string

	// Vector Store
	VectorStoreEnabled bool
	VectorStoreHost    string
	VectorStorePort    int
	VectorCollection   string

	// Embedding
	EmbeddingEnabled bool
	EmbeddingBaseURL string
	EmbeddingAPIKey  string

	// Cross-encoder rerank (post-RRF precision pass). Off unless a TEI-style
	// /rerank service is reachable; failures degrade silently to RRF order.
	// The candidate pool is 2×KnowledgeTopK (retrieval over-fetches so the
	// reranker can promote entries RRF buried).
	RerankEnabled bool
	RerankBaseURL string
	RerankModel   string

	// Logging
	LogLevel string
}

// defaultRateLimit is the per-IP request budget per minute used when
// RATE_LIMIT is unset. It is deliberately non-zero: an uncapped API in front
// of a paid LLM endpoint is a billing incident. Set RATE_LIMIT=0 to opt out
// explicitly (e.g. a private LAN deployment).
const defaultRateLimit = 120

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		LLMProvider:         getEnv("LLM_PROVIDER", "deepseek"),
		AnthropicAPIKey:     getEnv("ANTHROPIC_API_KEY", ""),
		AnthropicModel:      getEnv("ANTHROPIC_MODEL", "claude-sonnet-4-20250514"),
		DeepSeekAPIKey:      getEnv("DEEPSEEK_API_KEY", ""),
		DeepSeekModel:       getEnv("DEEPSEEK_MODEL", "deepseek-flash"),
		DeepSeekVisionModel: getEnv("DEEPSEEK_VISION_MODEL", "deepseek-flash"),

		OpenAICompatBaseURL:     getEnv("OPENAI_COMPAT_BASE_URL", ""),
		OpenAICompatAPIKey:      getEnv("OPENAI_COMPAT_API_KEY", ""),
		OpenAICompatModel:       getEnv("OPENAI_COMPAT_MODEL", ""),
		OpenAICompatVisionModel: getEnv("OPENAI_COMPAT_VISION_MODEL", ""),

		MaxTokens:         getEnvInt("MAX_TOKENS", 8192),
		Temperature:       getEnvFloat("TEMPERATURE", 0.3),
		MaxHistoryTurns:   getEnvInt("MAX_HISTORY_TURNS", 20),
		MaxToolIterations: getEnvInt("MAX_TOOL_ITERATIONS", 5),
		MaxToolCalls:      getEnvInt("MAX_TOOL_CALLS", 5),

		KnowledgeTopK:    getEnvInt("KNOWLEDGE_TOP_K", 8),
		KnowledgeEnabled: getEnvBool("KNOWLEDGE_RETRIEVAL_ENABLED", true),

		QueryUnderstandingEnabled:  getEnvBool("QUERY_UNDERSTANDING_ENABLED", true),
		QueryUnderstandingBranches: getEnvInt("QUERY_UNDERSTANDING_BRANCHES", 5),
		UnderstandModel:            getEnv("UNDERSTAND_MODEL", ""),
		AliasMapPath:               getEnv("ALIAS_MAP_PATH", "data/alias_map.json"),
		MediaDir:                   getEnv("MEDIA_DIR", "data/media"),

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

		APIKey:         getEnv("API_KEY", ""),
		CORSOrigins:    splitCSV(getEnv("CORS_ORIGINS", "")),
		RateLimit:      getEnvInt("RATE_LIMIT", defaultRateLimit),
		TrustedProxies: splitCSV(getEnv("TRUSTED_PROXIES", "")),
		PublicBaseURL:  strings.TrimRight(getEnv("PUBLIC_BASE_URL", ""), "/"),

		SessionDir: getEnv("SESSION_DIR", ""),

		SessionIdleMinutes: getEnvInt("SESSION_IDLE_MINUTES", 120),
		MaxActiveSessions:  getEnvInt("MAX_ACTIVE_SESSIONS", 500),

		MariaDBHost:        getEnv("MARIA_DB_HOST", "localhost"),
		MariaDBPort:        getEnvInt("MARIA_DB_PORT", 3306),
		MariaDBUser:        getEnv("MARIA_DB_USER", "root"),
		MariaDBPassword:    getEnv("MARIA_DB_PASSWORD", ""),
		MariaDBKnowledgeDB: getEnv("MARIA_DB_KNOWLEDGE_DB", "doctor_knowledge"),
		MariaDBAppDB:       getEnv("MARIA_DB_APP_DB", "doctor_agent"),

		AdminPassword: getEnv("ADMIN_PASSWORD", ""),

		AuthSecret: getEnv("AUTH_SECRET", ""),

		VectorStoreEnabled: getEnvBool("VECTOR_STORE_ENABLED", true),
		VectorStoreHost:    getEnv("VECTOR_STORE_HOST", "localhost"),
		VectorStorePort:    getEnvInt("VECTOR_STORE_PORT", 6334),
		VectorCollection:   getEnv("VECTOR_COLLECTION", "medical_knowledge"),

		EmbeddingEnabled: getEnvBool("EMBEDDING_ENABLED", true),
		EmbeddingBaseURL: getEnv("EMBEDDING_BASE_URL", ""),
		EmbeddingAPIKey:  getEnv("EMBEDDING_API_KEY", ""),

		RerankEnabled: getEnvBool("RERANK_ENABLED", false),
		RerankBaseURL: getEnv("RERANK_BASE_URL", ""),
		RerankModel:   getEnv("RERANK_MODEL", "bge-reranker-v2-m3"),

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

// SecurityWarnings lists configuration combinations that leave a deployed
// server open to abuse. Nothing here is fatal — each entry names the env var
// that closes the gap — so main prints them at startup instead of leaving the
// operator to discover them in an incident report.
func (c *Config) SecurityWarnings() []string {
	var out []string
	if c.APIKey == "" {
		out = append(out, "API_KEY 为空：/chat、/sessions、/admin 等所有接口无需凭证即可调用")
		if c.ServerHost != "127.0.0.1" && c.ServerHost != "localhost" {
			out = append(out, fmt.Sprintf(
				"SERVER_HOST=%s 监听对外地址且未设 API_KEY：任何能连到 :%s 的主机都在消耗你的 LLM 额度",
				c.ServerHost, c.ServerPort))
		}
	}
	if c.RateLimit <= 0 {
		out = append(out, "RATE_LIMIT<=0：限流已关闭，单个客户端可无限请求")
	}
	if len(c.CORSOrigins) == 0 {
		out = append(out, "CORS_ORIGINS 为空：Access-Control-Allow-Origin 回退为 *，任意网页都能跨域调用本 API")
	}
	if len(c.TrustedProxies) == 0 && c.RateLimit > 0 {
		out = append(out, "TRUSTED_PROXIES 为空：部署在 nginx/负载均衡后面时，所有访客会共用同一个限流桶")
	}
	if c.SessionDir == "" {
		out = append(out, "SESSION_DIR 为空：会话只存在内存里，进程重启即丢失")
	}
	if c.AuthSecret == "" {
		out = append(out, "AUTH_SECRET 为空：登录令牌用本次随机密钥签名，服务重启后所有用户都需要重新登录，多实例部署彼此不认凭证")
	}
	return out
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
		parseWarn(key, val)
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
		parseWarn(key, val)
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
		parseWarn(key, val)
	}
	return defaultVal
}

// parseWarn reports an env value that was ignored in favour of its default.
// Silently swallowing "fasle" (or "6O") would leave a feature in the opposite
// state from the one the operator typed, which is far harder to debug than a
// log line at startup.
func parseWarn(key, val string) {
	slog.Warn("invalid environment variable, using default", "key", key, "value", val)
}
