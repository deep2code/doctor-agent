package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"https://a.com", []string{"https://a.com"}},
		{"https://a.com, https://b.com", []string{"https://a.com", "https://b.com"}},
		{" , , ", nil},
		{"a,,b,", []string{"a", "b"}},
	}
	for _, c := range cases {
		got := splitCSV(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitCSV(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitCSV(%q) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}

func TestLoadDefaults(t *testing.T) {
	// Ensure a clean environment for the defaults check.
	for _, k := range []string{"LLM_PROVIDER", "ANTHROPIC_API_KEY", "API_KEY", "CORS_ORIGINS", "RATE_LIMIT", "SESSION_DIR", "POST_VERIFY_SEMANTIC", "AUTH_SECRET"} {
		t.Setenv(k, "")
	}

	cfg := Load()
	if cfg.LLMProvider != "deepseek" {
		t.Errorf("LLMProvider = %q, want deepseek", cfg.LLMProvider)
	}
	if cfg.ServerPort != "7071" {
		t.Errorf("ServerPort = %q, want 7071", cfg.ServerPort)
	}
	if cfg.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", cfg.APIKey)
	}
	if cfg.CORSOrigins != nil {
		t.Errorf("CORSOrigins = %v, want nil", cfg.CORSOrigins)
	}
	// Rate limiting is on by default now; an uncapped LLM-backed API is a
	// billing incident, so opting out has to be explicit.
	if cfg.RateLimit != defaultRateLimit {
		t.Errorf("RateLimit = %d, want %d", cfg.RateLimit, defaultRateLimit)
	}
	if cfg.TrustedProxies != nil {
		t.Errorf("TrustedProxies = %v, want nil (proxy headers untrusted by default)", cfg.TrustedProxies)
	}
	if cfg.SessionDir != "" {
		t.Errorf("SessionDir = %q, want empty", cfg.SessionDir)
	}
	if cfg.JudgeEnabled {
		t.Error("JudgeEnabled must default to false (POST_VERIFY_SEMANTIC default false)")
	}
	if cfg.AuthSecret != "" {
		t.Errorf("AuthSecret = %q, want empty (random per-process key)", cfg.AuthSecret)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	t.Setenv("API_KEY", "secret")
	t.Setenv("CORS_ORIGINS", " https://app.example.com, https://b.example.com ")
	t.Setenv("RATE_LIMIT", "30")
	t.Setenv("SESSION_DIR", "/tmp/sess")
	t.Setenv("POST_VERIFY_SEMANTIC", "true")
	t.Setenv("PUBLIC_BASE_URL", "https://yida.example.com/")

	cfg := Load()
	if cfg.APIKey != "secret" {
		t.Errorf("APIKey = %q, want secret", cfg.APIKey)
	}
	if len(cfg.CORSOrigins) != 2 || cfg.CORSOrigins[0] != "https://app.example.com" {
		t.Errorf("CORSOrigins = %v", cfg.CORSOrigins)
	}
	if cfg.RateLimit != 30 {
		t.Errorf("RateLimit = %d, want 30", cfg.RateLimit)
	}
	if cfg.SessionDir != "/tmp/sess" {
		t.Errorf("SessionDir = %q", cfg.SessionDir)
	}
	if !cfg.JudgeEnabled {
		t.Error("JudgeEnabled should be true when POST_VERIFY_SEMANTIC=true")
	}
	if cfg.PublicBaseURL != "https://yida.example.com" {
		t.Errorf("PublicBaseURL = %q, want trailing slash trimmed", cfg.PublicBaseURL)
	}
	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8, 127.0.0.1")
	if got := Load().TrustedProxies; len(got) != 2 || got[1] != "127.0.0.1" {
		t.Errorf("TrustedProxies = %v", got)
	}
}

func TestSecurityWarnings(t *testing.T) {
	// A hardened config must stay quiet, otherwise the warnings become noise
	// operators learn to ignore.
	hardened := &Config{
		APIKey:         "secret",
		CORSOrigins:    []string{"https://app.example.com"},
		RateLimit:      defaultRateLimit,
		TrustedProxies: []string{"127.0.0.1"},
		SessionDir:     "/tmp/sess",
		ServerHost:     "0.0.0.0",
		AuthSecret:     "a-fixed-secret",
	}
	if w := hardened.SecurityWarnings(); len(w) != 0 {
		t.Errorf("hardened config warned: %v", w)
	}

	open := &Config{ServerHost: "0.0.0.0", ServerPort: "7071"}
	joined := strings.Join(open.SecurityWarnings(), "\n")
	for _, want := range []string{"API_KEY", "RATE_LIMIT", "CORS_ORIGINS", "SESSION_DIR", "SERVER_HOST", "AUTH_SECRET"} {
		if !strings.Contains(joined, want) {
			t.Errorf("fail-open defaults did not mention %s:\n%s", want, joined)
		}
	}
	// Proxy trust only matters once something is being limited.
	limited := &Config{APIKey: "k", ServerHost: "127.0.0.1", CORSOrigins: []string{"https://a"}, RateLimit: defaultRateLimit}
	if w := strings.Join(limited.SecurityWarnings(), "\n"); !strings.Contains(w, "TRUSTED_PROXIES") {
		t.Errorf("rate-limited deploy behind an unknown proxy not flagged:\n%s", w)
	}

	// Loopback-only, un-rate-limited dev runs are legitimate; they should not
	// be shouted at for binding a public interface they do not use.
	local := &Config{APIKey: "k", ServerHost: "127.0.0.1", RateLimit: defaultRateLimit,
		CORSOrigins: []string{"https://app.example.com"}, TrustedProxies: []string{"127.0.0.1"}, SessionDir: "s",
		AuthSecret: "a-fixed-secret"}
	if w := local.SecurityWarnings(); len(w) != 0 {
		t.Errorf("local config warned: %v", w)
	}
}

func TestParseWarningsOnBadEnv(t *testing.T) {
	// An unparsable value must not be silently swapped for the default —
	// operators need to know their RATE_LIMIT was ignored.
	var logs bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(restore)

	t.Setenv("RATE_LIMIT", "many")
	t.Setenv("TEMPERATURE", "hot")
	t.Setenv("KNOWLEDGE_RETRIEVAL_ENABLED", "yes-please")
	cfg := Load()
	if cfg.RateLimit != defaultRateLimit || cfg.Temperature != 0.3 || !cfg.KnowledgeEnabled {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
	for _, key := range []string{"RATE_LIMIT", "TEMPERATURE", "KNOWLEDGE_RETRIEVAL_ENABLED"} {
		if !strings.Contains(logs.String(), key) {
			t.Errorf("no warning logged for %s:\n%s", key, logs.String())
		}
	}
}

func TestValidate(t *testing.T) {
	cfg := &Config{LLMProvider: "anthropic"}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate should fail without ANTHROPIC_API_KEY")
	}
	cfg.AnthropicAPIKey = "k"
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate with key: %v", err)
	}
	cfg.LLMProvider = "unknown"
	if err := cfg.Validate(); err == nil {
		t.Error("Validate should reject unknown provider")
	}
}
