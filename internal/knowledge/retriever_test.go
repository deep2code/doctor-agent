package knowledge

import (
	"context"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Load()
	if err != nil {
		t.Fatalf("加载知识库失败: %v", err)
	}
	return store
}

// topEntryID returns the ID of the top-ranked result.
func topEntryID(t *testing.T, r *KeywordRetriever, query string) string {
	t.Helper()
	res, err := r.Retrieve(context.Background(), query, 3)
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(res) == 0 {
		return ""
	}
	return res[0].Entry.ID
}

// TestRetrieverSymptomStyleChineseRecall guards the core fix: symptom-style
// Chinese questions (the common way real users ask) must recall the right
// entry — not just questions containing the full disease name.
func TestRetrieverSymptomStyleChineseRecall(t *testing.T) {
	r := NewRetriever(newTestStore(t))
	cases := map[string]string{
		"我一喝牛奶就拉肚子":     "lact-001",
		"耳鸣鼻塞回吸性血涕":     "npc-001",
		"孩子反复湿疹怎么护理":    "fung-002",
		"脚趾缝脱皮发痒":       "fung-001",
		"喝酒脸红是不是不好":     "aldh2-001",
		"发热三天出皮疹关节痛":    "dengue-001",
		"宝宝出生黄疸不退":      "g6pd-001",
	}
	for query, want := range cases {
		res, err := r.Retrieve(context.Background(), query, 5)
		if err != nil {
			t.Fatalf("检索失败: %v", err)
		}
		found := false
		for _, rr := range res {
			if rr.Entry.ID == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("查询 %q: 期望 top5 含 %s，实际 %v", query, want, entryIDs(res))
		}
	}
}

// TestRetrieverEnglishRecall: Latin queries still work via token matching.
func TestRetrieverEnglishRecall(t *testing.T) {
	r := NewRetriever(newTestStore(t))
	if got := topEntryID(t, r, "G6PD deficiency"); got != "g6pd-001" {
		t.Errorf("英文查询 G6PD deficiency: 期望 g6pd-001，实际 %q", got)
	}
	if got := topEntryID(t, r, "lactose intolerance"); got != "lact-001" {
		t.Errorf("英文查询 lactose intolerance: 期望 lact-001，实际 %q", got)
	}
}

// TestRetrieverFullNameRecall: questions naming the disease directly still
// rank it at top.
func TestRetrieverFullNameRecall(t *testing.T) {
	r := NewRetriever(newTestStore(t))
	if got := topEntryID(t, r, "地中海贫血"); got != "thal-001" {
		t.Errorf("查询 地中海贫血: 期望 thal-001，实际 %q", got)
	}
	if got := topEntryID(t, r, "蚕豆病"); got != "g6pd-001" {
		t.Errorf("查询 蚕豆病: 期望 g6pd-001，实际 %q", got)
	}
}

// TestRetrieverNoRecallForUnrelated: irrelevant queries must not force a
// recall — the agent should steer the user instead.
func TestRetrieverNoRecallForUnrelated(t *testing.T) {
	r := NewRetriever(newTestStore(t))
	cases := []string{
		"今天股市怎么样",
		"我今天感冒了怎么办",     // 感冒不在知识库
		"广东的天气怎么样",      // 仅地区命中，低于相关阈值
		"推荐一家好吃的餐厅",
	}
	for _, q := range cases {
		res, err := r.Retrieve(context.Background(), q, 3)
		if err != nil {
			t.Fatalf("检索失败: %v", err)
		}
		if len(res) != 0 {
			t.Errorf("查询 %q 不应召回（低于相关性阈值），实际 %d 条: %s",
				q, len(res), res[0].Entry.ConditionZH)
		}
	}
}

// TestRetrieverFoodAndLabRecall: food-risk and lab-test knowledge is indexed
// through their KnowledgeEntry projection and must be recallable.
func TestRetrieverFoodAndLabRecall(t *testing.T) {
	r := NewRetriever(newTestStore(t))
	cases := map[string]string{
		"广东人经常喝老火汤会不会痛风":   "food-gout-oldfiresoup",
		"蚕豆能放心吃吗":         "food-g6pd-fava",
		"体检报告MCV只有72":     "lab-mcv",
	}
	for query, want := range cases {
		res, err := r.Retrieve(context.Background(), query, 3)
		if err != nil {
			t.Fatalf("检索失败: %v", err)
		}
		if len(res) == 0 {
			t.Errorf("查询 %q 应召回 %s（食物/实验室知识已索引），实际无召回", query, want)
			continue
		}
		found := false
		for _, rr := range res {
			if rr.Entry.ID == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("查询 %q: 期望 top3 含 %s，实际 %v",
				query, want, entryIDs(res))
		}
	}
}

func entryIDs(res []RetrievalResult) []string {
	ids := make([]string, 0, len(res))
	for _, rr := range res {
		ids = append(ids, rr.Entry.ID)
	}
	return ids
}
