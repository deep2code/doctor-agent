package prompt

import (
	"strings"
	"testing"

	"github.com/doctor-agent/internal/knowledge"
)

// TestComposeSystemPromptIncludesFormatting guards the answer-formatting layer
// (tables / mermaid / plain language / 专业原理) being part of every prompt.
func TestComposeSystemPromptIncludesFormatting(t *testing.T) {
	c := NewComposer()
	p := c.ComposeSystemPrompt(nil, "")
	for _, want := range []string{"回答格式要求", "专业原理", "mermaid", "表格"} {
		if !strings.Contains(p, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

// TestComposeSystemPromptWithKnowledge: retrieved knowledge + citations still
// compose without error alongside the new formatting layer.
func TestComposeSystemPromptWithKnowledge(t *testing.T) {
	c := NewComposer()
	entry := knowledge.KnowledgeEntry{
		ID: "t-1", ConditionZH: "乳糖不耐受",
		Citations: []knowledge.Citation{{Type: "journal", Title: "x", Year: 2020, DOI: "10.1/x", Level: "A"}},
	}
	retrieved := []knowledge.RetrievalResult{{Entry: entry, Score: 0.9}}
	p := c.ComposeSystemPrompt(retrieved, "地区: guangdong")
	if !strings.Contains(p, "乳糖不耐受") || !strings.Contains(p, "guangdong") {
		t.Error("retrieved knowledge / patient context not injected")
	}
}
