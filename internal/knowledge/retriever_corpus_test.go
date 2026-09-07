package knowledge

import (
	"context"
	"strings"
	"testing"
)

// TestScoreCorpus covers the pure scoring function: English full-query in
// title, zh→en phrase bridge, and CJK windows against TitleZH/keywords.
func TestScoreCorpus(t *testing.T) {
	asthma := &CorpusDoc{Source: "medgen", ID: "medgen-1", Lang: "en",
		Title: "Asthma", Summary: "Asthma is a chronic lung disease.",
		Body: "Asthma causes wheezing and breathlessness."}
	ibuprofen := &CorpusDoc{Source: "lactmed", ID: "lactmed-1", Lang: "en",
		Title: "Ibuprofen", Summary: "Ibuprofen levels in breastmilk are low.",
		Body: "Limited ibuprofen is excreted into breastmilk; alternate drugs may be considered."}

	if s, ok := scoreCorpus(asthma, "asthma", nil, nil, []string{"asthma"}); !ok || s <= 0 {
		t.Errorf("英文 query 未命中: score=%v ok=%v", s, ok)
	}
	if s, ok := scoreCorpus(ibuprofen, "哺乳期布洛芬", []string{"哺乳", "乳期", "期布", "布洛芬", "洛芬"},
		[]string{"breastfeeding", "lactation", "ibuprofen"}, nil); !ok || s <= 0 {
		t.Errorf("中文 query 经词表桥接未命中 LactMed: score=%v ok=%v", s, ok)
	}
	if _, ok := scoreCorpus(asthma, "占星术", nil, nil, nil); ok {
		t.Errorf("无关 query 不应命中")
	}
}

// TestCorpusExcerpt verifies long-body windowing around the match.
func TestCorpusExcerpt(t *testing.T) {
	filler := strings.Repeat("padding text. ", 300)
	d := &CorpusDoc{Source: "statpearls", Title: "T", Body: filler +
		"The key finding is hyperkalemia." + strings.Repeat(" more padding. ", 300)}
	ex := corpusExcerpt(d, "hyperkalemia", nil, nil, []string{"hyperkalemia"})
	if !strings.Contains(ex, "hyperkalemia") {
		t.Errorf("摘要窗口应包含命中词")
	}
	if !strings.HasPrefix(ex, "…") {
		t.Errorf("命中词在正文中部时应带前省略号")
	}
}

// TestSearchICD11 exercises code/zh/en lookup against the seeded database
// (skips when icd11_terms.json has not been seeded yet).
func TestSearchICD11(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(store.SearchICD11("霍乱", 5)) == 0 && len(store.SearchICD11("1A00", 5)) == 0 {
		t.Skip("icd11 数据未 seed")
	}
	if d := store.GetICD11Term("1A00"); d == nil || d.TitleZH != "霍乱" {
		t.Errorf("GetICD11Term(1A00) 期望霍乱，得到 %+v", d)
	}
	if got := store.SearchICD11("cholera", 5); len(got) == 0 {
		t.Errorf("英文子串检索应命中 Cholera")
	}
	if got := store.SearchICD11("霍乱", 5); len(got) == 0 {
		t.Errorf("中文子串检索应命中霍乱")
	}
}

// TestRetrieveCorpusLive exercises the retriever against the seeded database
// (skips when corpus data is not loaded, mirroring other corpus tests).
func TestRetrieveCorpusLive(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	if store.GetCorpusDocCount() == 0 {
		t.Skip("corpus 数据未 seed")
	}
	res, err := r.RetrieveCorpus(context.Background(), "allergic asthma", 3, "medgen")
	if err != nil {
		t.Fatalf("RetrieveCorpus: %v", err)
	}
	if len(res) == 0 {
		t.Fatalf("medgen allergic asthma 应有结果")
	}
	if res[0].Doc.Title != "Allergic asthma" {
		t.Errorf("期望 top1 为 Allergic asthma（精确标题优先），得到 %q", res[0].Doc.Title)
	}
	if res[0].Doc.Source != "medgen" {
		t.Errorf("source 过滤失效: %q", res[0].Doc.Source)
	}
}
