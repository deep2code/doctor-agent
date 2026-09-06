package knowledge

import (
	"context"
	"strings"
	"testing"
)

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
