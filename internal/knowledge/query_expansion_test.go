package knowledge

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLoadAliasFileAndExpand(t *testing.T) {
	f := filepath.Join(t.TempDir(), "alias.json")
	content := `{"兔唇": ["唇腭裂", "唇裂"], "唇腭裂": ["兔唇", "唇裂"]}`
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := LoadAliasFile(f); err != nil {
		t.Fatalf("LoadAliasFile: %v", err)
	}
	t.Cleanup(func() {
		aliasMu.Lock()
		aliasMap = map[string][]string{}
		aliasMu.Unlock()
	})

	// Alias side expands to standard terms.
	if got := ExpandQuery("宝宝有兔唇"); !strings.Contains(got, "唇腭裂") {
		t.Errorf("alias 命中应扩展出标准词，实际: %q", got)
	}
	// Standard-term side expands back to aliases (双向).
	if got := ExpandQuery("唇腭裂术后复查"); !strings.Contains(got, "兔唇") {
		t.Errorf("标准词命中应扩展出俗称，实际: %q", got)
	}
	// No hit → unchanged.
	if got := ExpandQuery("今天天气不错"); got != "今天天气不错" {
		t.Errorf("无命中时不应改写，实际: %q", got)
	}
}

func TestLoadAliasFileMissingIsNoop(t *testing.T) {
	if err := LoadAliasFile(filepath.Join(t.TempDir(), "absent.json")); err != nil {
		t.Errorf("缺失文件应为 no-op，得到错误: %v", err)
	}
}

func TestExpandQueryAppendsSynonyms(t *testing.T) {
	got := ExpandQuery("9个月女婴，晚上总是突然大哭")
	for _, want := range []string{"哭闹", "夜啼", "婴儿", "肠绞痛"} {
		if !strings.Contains(got, want) {
			t.Errorf("扩展结果缺少同义词 %q，实际: %q", want, got)
		}
	}
	// Original query must stay at the front.
	if !strings.HasPrefix(got, "9个月女婴") {
		t.Errorf("扩展结果应保留原查询开头，实际: %q", got)
	}
	// No synonym group hit → unchanged.
	if same := ExpandQuery("今天天气不错"); same != "今天天气不错" {
		t.Errorf("无同义词命中时不应改写，实际: %q", same)
	}
}

// TestRetrieverInfantNightCryingRecall guards the pediatric colloquial recall
// fix: "9个月女婴，晚上总是突然大哭" (sudden night crying) must recall
// infant crying / colic entries even though the query contains none of the
// indexed keywords verbatim.
func TestRetrieverInfantNightCryingRecall(t *testing.T) {
	r := NewRetriever(newTestStore(t))
	res, err := r.Retrieve(context.Background(), "9个月女婴，晚上总是突然大哭", 5)
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(res) == 0 {
		t.Fatal("儿科夜哭查询检索到 0 条，同义词扩展未生效")
	}
	// At least one recalled entry must be about crying/colic/night issues.
	relevant := false
	for _, rr := range res {
		hay := rr.Entry.ConditionZH + " " + strings.Join(rr.Entry.Keywords, " ") + " " +
			strings.Join(rr.MatchedKeywords, " ")
		for _, kw := range []string{"哭", "夜啼", "夜惊", "肠绞", "腹痛", "绞痛"} {
			if strings.Contains(hay, kw) {
				relevant = true
				break
			}
		}
		if relevant {
			break
		}
	}
	if !relevant {
		t.Errorf("召回条目均与哭闹/肠绞痛无关: %v", entryIDs(res))
	}
}

func TestRetrieverInfantGasTeethBitingRecall(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("data", "common_diseases_batch4.json"))
	if err != nil {
		t.Fatalf("读取儿科条目: %v", err)
	}
	var entries []KnowledgeEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("解析儿科条目: %v", err)
	}
	want := map[string]bool{
		"common-058": false,
		"common-059": false,
		"common-060": false,
	}
	for _, entry := range entries {
		if _, ok := want[entry.ID]; ok {
			want[entry.ID] = true
		}
	}
	for id, present := range want {
		if !present {
			t.Fatalf("缺少回归条目 %s", id)
		}
	}
	for _, entry := range entries {
		if _, ok := want[entry.ID]; ok && len(entry.Citations) == 0 {
			t.Errorf("回归条目 %s 缺少引用", entry.ID)
		}
	}

	store := &Store{MedicalEntries: entries}
	doneOnce := &sync.Once{}
	doneOnce.Do(func() {})
	for _, dataset := range []string{
		DSMedical,
		DSFoodRisk,
		DSLabTest,
		DSFHS,
		DSMSD,
		DSDiseaseEnc,
		DSNHC,
	} {
		store.onces.Store(dataset, doneOnce)
	}
	r := NewRetriever(store)

	res, err := r.Retrieve(context.Background(), "10个月女婴，放屁很臭，有时候会咬牙，还会咬人", 5)
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	found := make(map[string]bool)
	for _, rr := range res {
		if _, ok := want[rr.Entry.ID]; ok {
			found[rr.Entry.ID] = true
		}
	}
	for id := range want {
		if !found[id] {
			t.Errorf("查询未召回 %s，实际: %v", id, entryIDs(res))
		}
	}
}

// TestRetrieverThroatPainRecall 验证"喉咙痛"能检索到咽喉疾病
// 回归测试: 防止同义词扩展与知识库关键词脱节导致检索失效
func TestRetrieverThroatPainRecall(t *testing.T) {
	// 测试同义词扩展是否包含喉咙痛相关词
	expanded := ExpandQuery("喉咙痛")

	// 验证扩展包含咽喉疾病相关词
	expected := []string{"嗓子疼", "咽喉痛", "咽痛", "嗓子痛"}
	missing := []string{}
	for _, w := range expected {
		if !strings.Contains(expanded, w) {
			missing = append(missing, w)
		}
	}
	if len(missing) > 0 {
		t.Errorf("喉咙痛扩展缺少关键词: %v，实际扩展: %q", missing, expanded)
	}

	// 验证扩展后的查询包含"咽痛"等关键词
	if !strings.Contains(expanded, "咽痛") {
		t.Errorf("喉咙痛扩展应包含'咽痛'，实际: %q", expanded)
	}
}
