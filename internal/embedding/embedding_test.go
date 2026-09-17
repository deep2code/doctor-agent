package embedding

import "testing"

// TestNewDefaultWithBaseURLOnly verifies the Ollama-style config: baseURL
// without an API key still selects the OpenAI-compatible provider.
func TestNewDefaultWithBaseURLOnly(t *testing.T) {
	p, err := NewDefault("http://localhost:11434/v1", "", "bge-m3", 0)
	if err != nil {
		t.Fatalf("NewDefault with baseURL only: %v", err)
	}
	if got := p.Name(); got != "openai-compat:bge-m3" {
		t.Fatalf("Name() = %q, want %q", got, "openai-compat:bge-m3")
	}
}

// TestNewDefaultRequiresBaseURL verifies there is no silent fallback: an
// empty baseURL must error, because hashed lexical vectors would mismatch
// the baked (bge-m3) vector space.
func TestNewDefaultRequiresBaseURL(t *testing.T) {
	if _, err := NewDefault("", "", "", 0); err == nil {
		t.Fatal("NewDefault with empty baseURL should error, got nil")
	}
}
