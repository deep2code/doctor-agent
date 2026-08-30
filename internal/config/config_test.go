package config

import (
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
	for _, k := range []string{"LLM_PROVIDER", "ANTHROPIC_API_KEY", "API_KEY", "CORS_ORIGINS", "RATE_LIMIT", "SESSION_DIR", "POST_VERIFY_SEMANTIC"} {
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
	if cfg.RateLimit != 0 {
		t.Errorf("RateLimit = %d, want 0", cfg.RateLimit)
	}
	if cfg.SessionDir != "" {
		t.Errorf("SessionDir = %q, want empty", cfg.SessionDir)
	}
	if cfg.JudgeEnabled {
		t.Error("JudgeEnabled must default to false (POST_VERIFY_SEMANTIC default false)")
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
