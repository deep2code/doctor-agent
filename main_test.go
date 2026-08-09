package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadDotenvOverridesGlobalEnv guards the priority rule:
// project .env (current dir) > global env vars > ~/.doctor-agent/config.env.
func TestLoadDotenvOverridesGlobalEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"),
		[]byte("DEEPSEEK_API_KEY=from-env-file\nLLM_PROVIDER=openai-compat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Same keys already set in the process environment (global).
	t.Setenv("DEEPSEEK_API_KEY", "from-global")
	t.Setenv("LLM_PROVIDER", "deepseek")

	t.Chdir(dir)
	if err := loadDotenv(); err != nil {
		t.Fatalf("loadDotenv: %v", err)
	}
	if got := os.Getenv("DEEPSEEK_API_KEY"); got != "from-env-file" {
		t.Errorf("DEEPSEEK_API_KEY = %q, want from-env-file (.env must override global env)", got)
	}
	if got := os.Getenv("LLM_PROVIDER"); got != "openai-compat" {
		t.Errorf("LLM_PROVIDER = %q, want openai-compat", got)
	}
}

// TestLoadDotenvMissingFile: no .env in the current dir is not an error.
func TestLoadDotenvMissingFile(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := loadDotenv(); err != nil {
		t.Errorf("loadDotenv on missing .env: %v", err)
	}
}
