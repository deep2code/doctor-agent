package knowledge

import (
	"regexp"
	"strings"
	"testing"
)

// TestBuildCitationNumberingConsistency is the contract between the prompt
// builder and the post-verification source map: the [N] numbers shown to the
// model must match the map keys the verifier checks.
func TestBuildCitationNumberingConsistency(t *testing.T) {
	entries := []RetrievalResult{
		{
			Entry: KnowledgeEntry{
				ID:          "thal-001",
				ConditionZH: "α-地中海贫血",
				Citations: []Citation{
					{Title: "A", Year: 2025},
					{Title: "B", Year: 2024},
				},
			},
		},
		{
			Entry: KnowledgeEntry{
				ID:          "g6pd-001",
				ConditionZH: "G6PD缺乏症",
				Citations: []Citation{
					{Title: "C", Year: 2025},
					{Title: "D", Year: 2023},
					{Title: "E", Year: 2022},
				},
			},
		},
		{
			Entry: KnowledgeEntry{
				ID:          "hbv-001",
				ConditionZH: "慢性乙型肝炎",
				Citations:   []Citation{},
			},
		},
	}

	sources := BuildCitedSources(entries)

	prompt := NewCitationFormatter().BuildCitationMap(entries)
	if prompt == "" {
		t.Fatal("BuildCitationMap 不应为空")
	}

	numRe := regexp.MustCompile(`\[\d+\]`)
	seen := map[string]bool{}
	for _, m := range numRe.FindAllString(prompt, -1) {
		seen[m] = true
	}

	// Every number in the prompt must resolve in the source map...
	for num := range seen {
		key := num[1 : len(num)-1] // strip brackets
		if _, ok := sources[key]; !ok {
			t.Errorf("提示词中的编号 %s 在 sources 中不存在", num)
		}
	}

	// ...and every source number must appear in the prompt (entries with
	// citations only — empty-citation entries contribute no numbers).
	promptNums := map[string]bool{}
	for num := range seen {
		promptNums[num[1:len(num)-1]] = true
	}
	for num := range sources {
		if !promptNums[num] {
			t.Errorf("sources 中的编号 %s 未在提示词中列出", num)
		}
	}

	if len(sources) != 5 {
		t.Fatalf("期望 5 个来源（两个含引用条目），实际 %d", len(sources))
	}
}

func TestBuildCitedSourcesEmpty(t *testing.T) {
	sources := BuildCitedSources(nil)
	if len(sources) != 0 {
		t.Fatalf("空检索应返回空 map，实际 %d", len(sources))
	}
}

func TestAddToolSource(t *testing.T) {
	sources := make(map[string]CitedSource)
	AddToolSource(sources, "A dengue vaccine trial", "10.1000/demo", "42516252", 2025, "pubmed_abstract", "文献: A dengue vaccine trial (2025)")
	if _, ok := sources["42516252"]; !ok {
		t.Fatal("PMID key should be registered")
	}
	if _, ok := sources["doi:10.1000/demo"]; !ok {
		t.Fatal("DOI key should be registered")
	}
	if len(sources) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(sources))
	}
	// No PMID -> only the DOI key exists.
	s2 := make(map[string]CitedSource)
	AddToolSource(s2, "t", "", "10.9999/x", 0, "review", "t")
	if len(s2) != 1 {
		t.Fatalf("expected 1 key without PMID, got %d", len(s2))
	}
}

// TestBuildKnowledgeExcerpts guards the 科普 fix: the article text behind a
// retrieved entry must reach the prompt, once per entry, within the excerpt
// budget, and entries that are pure structured lookups must add nothing.
func TestBuildKnowledgeExcerpts(t *testing.T) {
	unit := "正畸通过持续轻力引导牙齿在牙槽骨中移动。"
	long := strings.Repeat(unit, 200)
	cf := NewCitationFormatter()
	debunk := KnowledgeEntry{ID: "dsc-1", ConditionZH: "正畸让牙齿松动易脱落", Body: long}
	out := cf.BuildKnowledgeExcerpts([]RetrievalResult{
		{Entry: debunk},
		{Entry: debunk},
		{Entry: KnowledgeEntry{ID: "npc-1", ConditionZH: "鼻咽癌"}},
	}, "牙齿矫正会让牙齿松动脱落吗")

	if !strings.Contains(out, "【正畸让牙齿松动易脱落】") {
		t.Fatalf("摘录应带条目标题:\n%.200s", out)
	}
	if strings.Contains(out, "【鼻咽癌】") {
		t.Error("没有正文的结构化条目不应产生摘录区块")
	}
	if n := strings.Count(out, "【"); n != 1 {
		t.Errorf("同一条目被重复检索到应只输出一次，实际 %d 个区块", n)
	}
	if n := len([]rune(out)); n > knowledgeExcerptEntries*knowledgeExcerptRunes+200 {
		t.Errorf("摘录未受预算约束: %d runes", n)
	}
}
