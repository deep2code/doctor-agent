package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
