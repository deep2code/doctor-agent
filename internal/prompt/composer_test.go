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
	p := c.ComposeSystemPrompt(nil, "", "")
	for _, want := range []string{"回答格式要求", "专业描述", "mermaid", "表格"} {
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
	p := c.ComposeSystemPrompt(retrieved, "地区: guangdong", "喝牛奶拉肚子")
	if !strings.Contains(p, "乳糖不耐受") || !strings.Contains(p, "guangdong") {
		t.Error("retrieved knowledge / patient context not injected")
	}
}

// TestComposeStaticPrefixSplitsCleanly guards the prompt-cache contract:
// static + dynamic must reassemble the exact full prompt, the static part
// must be byte-identical across requests, and must not contain any
// per-request content (patient context, knowledge, safety layer).
func TestComposeStaticPrefixSplitsCleanly(t *testing.T) {
	c := NewComposer()
	entry := knowledge.KnowledgeEntry{ID: "t-2", ConditionZH: "鼻咽癌"}
	retrieved := []knowledge.RetrievalResult{{Entry: entry, Score: 0.8}}

	static := c.ComposeStaticPrefix()
	dynamic := c.ComposeDynamicSections(retrieved, "地区: 广东", "鼻咽癌")
	full := c.ComposeSystemPrompt(retrieved, "地区: 广东", "鼻咽癌")

	if full != static+dynamic {
		t.Error("static+dynamic 拼接必须与完整提示词逐字节一致")
	}
	if static != NewComposer().ComposeStaticPrefix() {
		t.Error("静态前缀必须与请求内容无关、逐字节稳定")
	}
	for _, marker := range []string{"CLINICAL REASONING FRAMEWORK", "DUAL-VERSION OUTPUT"} {
		if !strings.Contains(static, marker) {
			t.Errorf("静态前缀缺少层标记 %q", marker)
		}
	}
	for _, marker := range []string{"PATIENT CONTEXT", "SAFETY RULES", "鼻咽癌", "广东"} {
		if strings.Contains(static, marker) {
			t.Errorf("静态前缀不应包含动态内容 %q（会破坏缓存前缀）", marker)
		}
	}
	if !strings.Contains(dynamic, "SAFETY RULES") {
		t.Error("动态段应包含安全层（层序保持不变）")
	}
}

// TestComposeDynamicSectionsCarriesKnowledgeProse guards the 科普 fix: article
// text must reach the model through the dynamic section only — anything that
// leaks into the static prefix silently breaks prompt caching.
func TestComposeDynamicSectionsCarriesKnowledgeProse(t *testing.T) {
	c := NewComposer()
	entry := knowledge.KnowledgeEntry{
		ID: "dsc-1", ConditionZH: "正畸让牙齿松动易脱落",
		Body: "真相：牙齿移动不等于牙齿松动。正畸的原理是通过持续轻力，引导牙齿在牙槽骨中移动。",
	}
	retrieved := []knowledge.RetrievalResult{{Entry: entry, Score: 9.0}}

	dynamic := c.ComposeDynamicSections(retrieved, "", "牙齿矫正会让牙齿松动脱落吗")
	if !strings.Contains(dynamic, "牙齿移动不等于牙齿松动") {
		t.Error("条目有正文时，动态段应包含原文摘录")
	}
	if strings.Contains(c.ComposeStaticPrefix(), "牙齿移动不等于牙齿松动") {
		t.Error("原文摘录属动态内容，不能进入可缓存的静态前缀")
	}
	bare := []knowledge.RetrievalResult{{Entry: knowledge.KnowledgeEntry{ID: "npc-1", ConditionZH: "鼻咽癌"}, Score: 9.0}}
	if strings.Contains(c.ComposeDynamicSections(bare, "", "鼻咽癌"), "检索到的知识原文摘录") {
		t.Error("无正文条目不应产生摘录区块")
	}
}
