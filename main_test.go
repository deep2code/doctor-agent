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

// TestLoadDotenvFallsBackToHome: when the current dir has no .env, the user
// home directory's ~/.env is used (and still overrides global env vars).
func TestLoadDotenvFallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".env"),
		[]byte("DEEPSEEK_API_KEY=from-home\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEEPSEEK_API_KEY", "from-global")

	t.Chdir(t.TempDir()) // current dir has NO .env
	if err := loadDotenv(); err != nil {
		t.Fatalf("loadDotenv: %v", err)
	}
	if got := os.Getenv("DEEPSEEK_API_KEY"); got != "from-home" {
		t.Errorf("DEEPSEEK_API_KEY = %q, want from-home (~/.env fallback)", got)
	}
}

// TestLoadDotenvCWDWinsOverHome: with both present, ./env wins over ~/.env.
func TestLoadDotenvCWDWinsOverHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".env"),
		[]byte("DEEPSEEK_API_KEY=from-home\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, ".env"),
		[]byte("DEEPSEEK_API_KEY=from-cwd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwd)
	if err := loadDotenv(); err != nil {
		t.Fatalf("loadDotenv: %v", err)
	}
	if got := os.Getenv("DEEPSEEK_API_KEY"); got != "from-cwd" {
		t.Errorf("DEEPSEEK_API_KEY = %q, want from-cwd (current dir .env wins)", got)
	}
}

// TestLoadDotenvMissingFile: neither ./env nor ~/.env is not an error.
func TestLoadDotenvMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // empty home, no .env
	t.Chdir(t.TempDir())
	if err := loadDotenv(); err != nil {
		t.Errorf("loadDotenv with no .env anywhere: %v", err)
	}
}
