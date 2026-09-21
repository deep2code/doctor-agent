package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCacheableSystemBlocks: the static prefix block carries an ephemeral
// cache breakpoint, the dynamic remainder stays uncached, and an empty
// remainder produces a single block (Anthropic rejects empty text blocks).
func TestCacheableSystemBlocks(t *testing.T) {
	blocks := cacheableSystemBlocks("STATIC", "DYNAMIC")
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	if blocks[0].Text != "STATIC" || blocks[1].Text != "DYNAMIC" {
		t.Errorf("unexpected texts: %q / %q", blocks[0].Text, blocks[1].Text)
	}
	if blocks[0].CacheControl.Type != "ephemeral" {
		t.Errorf("prefix block must carry ephemeral cache_control, got %q", blocks[0].CacheControl.Type)
	}
	if blocks[1].CacheControl.Type != "" {
		t.Errorf("dynamic remainder must stay uncached, got %q", blocks[1].CacheControl.Type)
	}

	one := cacheableSystemBlocks("STATIC", "")
	if len(one) != 1 || one[0].CacheControl.Type != "ephemeral" {
		t.Errorf("empty rest should yield one cached block, got %+v", one)
	}

	data, err := json.Marshal(blocks)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"cache_control":{"type":"ephemeral"}`) {
		t.Errorf("marshalled blocks missing cache_control: %s", data)
	}
	if strings.Count(string(data), `"cache_control"`) != 1 {
		t.Error("cache_control must appear exactly once (on the prefix block)")
	}
}
