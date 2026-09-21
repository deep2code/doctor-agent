package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// assertCompacted checks the core contract: output is always valid JSON,
// valid UTF-8, and within the budget.
func assertCompacted(t *testing.T, toolName string, out string) {
	t.Helper()
	if !json.Valid([]byte(out)) {
		t.Fatalf("compactToolResult(%s) returned invalid JSON:\n%.300s", toolName, out)
	}
	if len(out) > toolResultHardCap {
		t.Fatalf("compactToolResult(%s) returned %d bytes, over cap %d", toolName, len(out), toolResultHardCap)
	}
	if !utf8.ValidString(out) {
		t.Fatalf("compactToolResult(%s) returned invalid UTF-8", toolName)
	}
}

func TestCompactToolResultSmallPassthrough(t *testing.T) {
	data := map[string]any{"query": "高血压", "result_count": 1}
	raw, _ := json.MarshalIndent(data, "", "  ")
	out := compactToolResult("knowledge_search", data)
	if out != string(raw) {
		t.Fatalf("small payload should pass through byte-identical\ngot:  %q\nwant: %q", out, raw)
	}
}

// The real knowledge_search tool stores []map[string]any (a typed slice)
// under "results"; the old []any type switch never matched it, so the
// top-3 field policy was dead code. Normalization must make it fire.
func TestCompactToolResultKnowledgeSearchTypedSlice(t *testing.T) {
	results := make([]map[string]any, 10)
	for i := range results {
		results[i] = map[string]any{
			"condition_zh": fmt.Sprintf("疾病%d", i),
			"title":        fmt.Sprintf("条目%d", i),
			"body":         strings.Repeat("这里是很长的正文内容用于把结果撑过压缩阈值。", 30),
			"relevance":    9 - i,
		}
	}
	data := map[string]any{"query": "头痛", "result_count": 10, "results": results}
	if raw, _ := json.MarshalIndent(data, "", "  "); len(raw) <= toolResultHardCap {
		t.Fatalf("test fixture must exceed the cap to enter compaction, got %d bytes", len(raw))
	}
	out := compactToolResult("knowledge_search", data)
	assertCompacted(t, "knowledge_search", out)
	if !strings.Contains(out, "结果已压缩：原 10 条保留前 3 条并精简字段") {
		t.Fatalf("expected knowledge_search _note, got:\n%s", out)
	}
	if strings.Contains(out, "body") {
		t.Fatal("non-whitelisted field 'body' should have been pruned")
	}
	if strings.Contains(out, "疾病9") {
		t.Fatal("entries beyond top 3 should have been dropped")
	}
	// The caller's data must not be mutated (maybeFollowupRetrieve reuses it).
	if len(data["results"].([]map[string]any)) != 10 {
		t.Fatal("compactToolResult mutated the original data map")
	}
}

func TestCompactToolResultLongListCapped(t *testing.T) {
	items := make([]any, 200)
	for i := range items {
		items[i] = map[string]any{"name": fmt.Sprintf("条目%03d", i), "text": "简短说明文本"}
	}
	data := map[string]any{"query": "x", "results": items}
	out := compactToolResult("msd_search", data)
	assertCompacted(t, "msd_search", out)
	if !strings.Contains(out, "项已省略") {
		t.Fatal("expected omission note for capped list")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if _, ok := parsed["query"]; !ok {
		t.Fatal("scalar top-level fields must survive compaction")
	}
}

func TestCompactToolResultHugeCJKStringStaysValid(t *testing.T) {
	poem := strings.Repeat("长期高血压患者应当规律监测血压并遵医嘱服药。", 400) // >5000 runes
	data := map[string]any{
		"query":   "高血压",
		"results": []any{map[string]any{"body": poem}},
	}
	out := compactToolResult("knowledge_search", data)
	assertCompacted(t, "knowledge_search", out)
	if !strings.Contains(out, "…") {
		t.Fatal("truncated strings should end with an ellipsis marker")
	}
	// Original string must be intact for later consumers.
	if got := len([]rune(poem)); got < 5000 {
		t.Fatalf("test fixture too short: %d runes", got)
	}
	if r := []rune(data["results"].([]any)[0].(map[string]any)["body"].(string)); len(r) != len([]rune(poem)) {
		t.Fatal("compactToolResult mutated nested data")
	}
}

func TestCompactToolResultWideEntryFallsDownLadder(t *testing.T) {
	entry := map[string]any{}
	for i := 0; i < 24; i++ {
		entry[fmt.Sprintf("field_%02d", i)] = strings.Repeat("这是一段比较长的中文说明内容。", 40)
	}
	data := map[string]any{"results": []any{entry}}
	out := compactToolResult("disease_encyclopedia_lookup", data)
	assertCompacted(t, "disease_encyclopedia_lookup", out)
}

func TestCompactToolResultDeepNesting(t *testing.T) {
	leaf := map[string]any{"note": strings.Repeat("深层嵌套的长文本内容示例。", 200)}
	v := any(leaf)
	for i := 0; i < 60; i++ {
		v = map[string]any{fmt.Sprintf("lvl%d", i): v}
	}
	data := v.(map[string]any)
	out := compactToolResult("medical_kg_lookup", data)
	assertCompacted(t, "medical_kg_lookup", out)
}

func TestCompactToolResultTypedNestedSlices(t *testing.T) {
	type citation struct {
		PMID  string `json:"pmid"`
		Title string `json:"title"`
	}
	cites := make([]citation, 50)
	for i := range cites {
		cites[i] = citation{PMID: fmt.Sprintf("PMID%05d", i), Title: strings.Repeat("文献标题", 60)}
	}
	data := map[string]any{
		"query":     "循证",
		"citations": cites, // typed slice([]citation), invisible to a []any switch
		"summaries": map[string]any{"inner": []string{strings.Repeat("甲乙丙丁", 300), "短"}},
	}
	out := compactToolResult("literature_search", data)
	assertCompacted(t, "literature_search", out)
}

// A knowledge_search payload whose results are plain strings (not maps)
// still goes through the top-3 policy and must come out as valid JSON.
func TestCompactToolResultAlwaysValidJSON(t *testing.T) {
	data := map[string]any{"query": "q"}
	big := make([]any, 3000)
	for i := range big {
		big[i] = fmt.Sprintf("条目编号 %d 的中文描述", i)
	}
	data["results"] = big
	out := compactToolResult("knowledge_search", data)
	assertCompacted(t, "knowledge_search", out)
}
