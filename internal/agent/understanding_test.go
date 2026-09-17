package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/doctor-agent/internal/config"
	"github.com/doctor-agent/internal/knowledge"
	"github.com/doctor-agent/internal/llm"
)

// recordingRetriever logs every query it receives (the understanding step
// fans out concurrently, so access is locked) and returns preset entry IDs
// for queries containing a key.
type recordingRetriever struct {
	mu      sync.Mutex
	queries []string
	bySub   map[string][]string
}

func (r *recordingRetriever) Retrieve(_ context.Context, q string, _ int) ([]knowledge.RetrievalResult, error) {
	r.mu.Lock()
	r.queries = append(r.queries, q)
	r.mu.Unlock()
	for key, ids := range r.bySub {
		if strings.Contains(q, key) {
			out := make([]knowledge.RetrievalResult, 0, len(ids))
			for _, id := range ids {
				out = append(out, knowledge.RetrievalResult{Score: 0.9, Entry: knowledge.KnowledgeEntry{ID: id}})
			}
			return out, nil
		}
	}
	return nil, nil
}

func (r *recordingRetriever) RetrieveDrugs(_ context.Context, _ string, _ int) ([]knowledge.DrugRetrievalResult, error) {
	return nil, nil
}

func (r *recordingRetriever) Name() string { return "recording" }

func (r *recordingRetriever) seenQueries() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.queries...)
}

func understandingConfig() *config.Config {
	cfg := testConfig()
	cfg.KnowledgeEnabled = true
	cfg.QueryUnderstandingEnabled = true
	cfg.KnowledgeTopK = 5
	return cfg
}

func noopStep(StepEvent) {}

// TestRetrieveWithUnderstandingMultiBranch: the verbatim query runs AND the
// LLM-derived concept branches run; results from all branches are merged.
func TestRetrieveWithUnderstandingMultiBranch(t *testing.T) {
	p := &fakeProvider{responses: []*llm.ChatResponse{{Text: "```json\n" + `
		{"symptoms": ["腹泻", "发热", "抽搐"],
		 "suspected_conditions": ["热性惊厥", "急性胃肠炎"],
		 "search_queries": ["婴幼儿 秋季腹泻 发热", "热性惊厥 处理"]}
	` + "\n```"}}}
	ag := newTestAgent(understandingConfig(), p)
	rec := &recordingRetriever{bySub: map[string][]string{
		"烧抽了": {"verbatim-g6pd"},
		"腹泻":  {"branch-rotavirus"},
		"惊厥":  {"branch-febrile"},
	}}
	ag.retriever = rec

	got := ag.retrieveWithUnderstanding(context.Background(), "宝宝烧抽了还拉肚子", noopStep)
	ids := map[string]bool{}
	for _, r := range got {
		ids[r.Entry.ID] = true
	}
	if !ids["verbatim-g6pd"] {
		t.Errorf("原词路命中应保留，实际: %v", ids)
	}
	if !ids["branch-rotavirus"] || !ids["branch-febrile"] {
		t.Errorf("理解分支命中应合入，实际: %v", ids)
	}
	seen := rec.seenQueries()
	if len(seen) != 3 {
		t.Errorf("应有 1 条原词 + 2 条分支查询，实际 %d 条: %v", len(seen), seen)
	}
	if p.chatCalls != 1 {
		t.Errorf("理解步应恰好调用一次 LLM，实际 %d 次", p.chatCalls)
	}
}

// TestRetrieveWithUnderstandingFallbackQueries: model omits search_queries →
// branches built from suspected_conditions.
func TestRetrieveWithUnderstandingFallbackQueries(t *testing.T) {
	p := &fakeProvider{responses: []*llm.ChatResponse{{Text: `{"symptoms": ["腹泻"], "suspected_conditions": ["乳糖不耐受", "秋季腹泻"], "search_queries": []}`}}}
	ag := newTestAgent(understandingConfig(), p)
	rec := &recordingRetriever{bySub: map[string][]string{
		"乳糖":   {"branch-lactose"},
		"秋季腹泻": {"branch-rota"},
	}}
	ag.retriever = rec

	got := ag.retrieveWithUnderstanding(context.Background(), "宝宝喝奶就拉肚子", noopStep)
	ids := map[string]bool{}
	for _, r := range got {
		ids[r.Entry.ID] = true
	}
	if !ids["branch-lactose"] || !ids["branch-rota"] {
		t.Errorf("conditions 应生成检索分支，实际: %v", ids)
	}
}

// TestRetrieveWithUnderstandingBadJSON: non-JSON LLM output degrades to
// verbatim-only retrieval, no error.
func TestRetrieveWithUnderstandingBadJSON(t *testing.T) {
	p := &fakeProvider{responses: []*llm.ChatResponse{{Text: "抱歉，我无法解析这个问题。"}}}
	ag := newTestAgent(understandingConfig(), p)
	rec := &recordingRetriever{bySub: map[string][]string{"x": {"hit"}}}
	ag.retriever = rec

	got := ag.retrieveWithUnderstanding(context.Background(), "普通问题", noopStep)
	if len(got) != 0 && got[0].Entry.ID != "hit" {
		t.Errorf("降级后应只有原词路结果: %v", got)
	}
	if n := len(rec.seenQueries()); n != 1 {
		t.Errorf("解析失败不应发起分支检索，实际 %d 条", n)
	}
}

// TestRetrieveWithUnderstandingDisabled: switch off → no LLM call at all.
func TestRetrieveWithUnderstandingDisabled(t *testing.T) {
	cfg := understandingConfig()
	cfg.QueryUnderstandingEnabled = false
	p := &fakeProvider{}
	ag := newTestAgent(cfg, p)
	ag.retriever = &recordingRetriever{}

	ag.retrieveWithUnderstanding(context.Background(), "问题", noopStep)
	if p.chatCalls != 0 {
		t.Errorf("关闭理解时不应调用 LLM，实际 %d 次", p.chatCalls)
	}
}

func TestExtractJSONObject(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"a":1}`, `{"a":1}`},
		{"```json\n{\"a\": 1}\n```", `{"a": 1}`},
		{"好的，这是结果：{\"a\": 1} 以上。", `{"a": 1}`},
		{"没有 JSON", ""},
		{"", ""},
		{"}{", ""}, // degenerate span
	}
	for _, c := range cases {
		if got := extractJSONObject(c.in); got != c.want {
			t.Errorf("extractJSONObject(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestMergeRetrievalBranches: base keeps order and priority; branch-only
// entries appended by fused score; entry hit by more branches ranks higher.
func TestMergeRetrievalBranches(t *testing.T) {
	mk := func(id string) knowledge.RetrievalResult {
		return knowledge.RetrievalResult{Score: 1, Entry: knowledge.KnowledgeEntry{ID: id}}
	}
	base := []knowledge.RetrievalResult{mk("base-1")}
	paths := [][]knowledge.RetrievalResult{
		{mk("both"), mk("only-a")},
		{mk("both"), mk("only-b")},
	}
	got := mergeRetrievalBranches(base, paths, 5)

	var order []string
	for _, r := range got {
		order = append(order, r.Entry.ID)
	}
	if len(got) != 4 {
		t.Fatalf("应 4 条（base 1 + 分支去重 3），实际 %d: %v", len(got), order)
	}
	if order[0] != "base-1" {
		t.Errorf("base 应排最前，实际 %v", order)
	}
	if got[1].Entry.ID != "both" {
		t.Errorf("两个分支都命中的条目应排分支首位，实际 %v", order)
	}
}

// TestMergeRetrievalBranchesCap: output capped at 2×topK.
func TestMergeRetrievalBranchesCap(t *testing.T) {
	mk := func(id string) knowledge.RetrievalResult {
		return knowledge.RetrievalResult{Score: 1, Entry: knowledge.KnowledgeEntry{ID: id}}
	}
	base := []knowledge.RetrievalResult{mk("b0")}
	var path []knowledge.RetrievalResult
	for i := 0; i < 10; i++ {
		path = append(path, mk(fmt.Sprintf("e%d", i)))
	}
	got := mergeRetrievalBranches(base, [][]knowledge.RetrievalResult{path}, 2)
	if len(got) != 4 {
		t.Fatalf("2×topK=4 截断失败，实际 %d 条", len(got))
	}
}
