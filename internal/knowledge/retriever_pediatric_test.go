package knowledge

import (
	"context"
	"math"
	"testing"
)

func TestAssessGrowthBoyWeightMedian(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	// 中国 B.1 男童 24 月(2岁)中位体重 12.6 kg（表值），z 应≈0
	res, err := r.AssessGrowth(context.Background(), "男", 24, "weight", 12.6)
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if res.China == nil {
		t.Fatal("china result missing")
	}
	if math.Abs(res.China.ZScore) > 0.1 {
		t.Errorf("median weight z-score = %v, want ≈0", res.China.ZScore)
	}
	if res.China.Verdict != "正常" {
		t.Errorf("verdict = %q, want 正常", res.China.Verdict)
	}
	if res.WHO == nil {
		t.Fatal("WHO result missing for weight at 24mo")
	}
	if res.WHO.ZScore < -1 || res.WHO.ZScore > 1 {
		t.Errorf("WHO z = %v, want within ±1", res.WHO.ZScore)
	}
	t.Logf("24mo boy 12.6kg: CN z=%v (%s), WHO z=%v (%s)",
		res.China.ZScore, res.China.Verdict, res.WHO.ZScore, res.WHO.Verdict)
}

func TestAssessGrowthGirlStunting(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	// 中国 B.4 女童 24 月 -2SD 身高 80.8（P50 87.0），取 80 应判生长迟缓
	res, err := r.AssessGrowth(context.Background(), "女", 24, "length_height", 80.0)
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if res.China == nil {
		t.Fatal("china result missing")
	}
	if res.China.ZScore > -2 || res.China.ZScore < -3 {
		t.Errorf("80cm girl 24mo z = %v, want in [-3,-2)", res.China.ZScore)
	}
	wantVerdicts := map[string]bool{"生长迟缓": true, "重度生长迟缓": true}
	if !wantVerdicts[res.China.Verdict] {
		t.Errorf("verdict = %q, want 生长迟缓系", res.China.Verdict)
	}
	t.Logf("24mo girl 80cm: CN z=%v (%s)", res.China.ZScore, res.China.Verdict)
}

func TestAssessGrowthHeadCircumferenceInterpolation(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	// 中国 B.11 男童 3 月中位头围 40.5（表值）；WHO 3 月中位约 40.5
	res, err := r.AssessGrowth(context.Background(), "male", 3, "head_circumference", 40.5)
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if res.WHO == nil {
		t.Fatal("WHO head circumference missing")
	}
	if res.WHO.ZScore < -0.6 || res.WHO.ZScore > 0.6 {
		t.Errorf("WHO hc z = %v, want ≈0", res.WHO.ZScore)
	}
	if res.China == nil {
		t.Fatal("china head circumference missing (0-36mo)")
	}
	t.Logf("3mo boy hc 40.5cm: WHO z=%v, CN z=%v", res.WHO.ZScore, res.China.ZScore)
}

func TestAssessGrowthBMIOverweight(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	// 中国 B.9 男童 60 月 +2SD BMI 18.4；取 19 应判肥胖
	res, err := r.AssessGrowth(context.Background(), "男", 60, "bmi", 19.0)
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if res.China == nil {
		t.Fatal("china bmi missing")
	}
	if res.China.ZScore <= 2 {
		t.Errorf("BMI 18 at 60mo z = %v, want >2", res.China.ZScore)
	}
	// 中国标准 0-84 月，WHO 无 BMI 表
	if res.WHO != nil {
		t.Errorf("WHO should not have BMI table, got %+v", res.WHO)
	}
	t.Logf("60mo boy BMI 19: CN z=%v (%s)", res.China.ZScore, res.China.Verdict)
}

func TestAssessGrowthErrors(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	if _, err := r.AssessGrowth(context.Background(), "男", 24, "height", 100); err == nil {
		t.Error("unknown indicator should error")
	}
	if _, err := r.AssessGrowth(context.Background(), "男", 120, "weight", 20); err == nil {
		t.Error("age 120mo beyond both standards should error")
	}
}

func TestRetrieveMilestones(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	// 5 月龄应拿到 4 月清单，next 是 6 月
	res, err := r.RetrieveMilestones(context.Background(), 5)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if res.Age.AgeKey != "4mo" {
		t.Errorf("5mo -> %s, want 4mo", res.Age.AgeKey)
	}
	if res.NextAgeKey != "6mo" {
		t.Errorf("next = %s, want 6mo", res.NextAgeKey)
	}
	if len(res.Age.SocialEmotionalZH) == 0 || len(res.Age.MovementPhysical) == 0 {
		t.Error("zh/en milestone fields empty")
	}
	// 25 月龄 → 2yr 清单
	res2, _ := r.RetrieveMilestones(context.Background(), 25)
	if res2.Age.AgeKey != "2yr" {
		t.Errorf("25mo -> %s, want 2yr", res2.Age.AgeKey)
	}
	if _, err := r.RetrieveMilestones(context.Background(), 72); err == nil {
		t.Error("72mo should be out of range")
	}
	t.Logf("5mo checklist %s (%s), definition=%.40s…", res.Age.AgeKey, res.Age.AgeLabelZH, res.Definition)
}

func TestSearchMilestones(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	m, err := r.SearchMilestones(context.Background(), "走路", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(m) == 0 {
		t.Fatal("no matches for 走路")
	}
	sawWalk := false
	for _, x := range m {
		if x.Domain == "movement_physical" && (containsAny(x.TextZH, "走") || containsAny(x.TextEN, "alk")) {
			sawWalk = true
		}
	}
	if !sawWalk {
		t.Errorf("expected a walking movement milestone, got %+v", m[:2])
	}
	t.Logf("走路 → top: %s %s %s", m[0].AgeKey, m[0].Domain, m[0].TextZH)
}

func containsAny(s string, subs ...string) bool {
	for _, x := range subs {
		for _, r := range x {
			found := false
			for _, sr := range s {
				if sr == r {
					found = true
					break
				}
			}
			if found {
				return true
			}
		}
	}
	return false
}

func TestSearchNewbornCare(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	for _, q := range []string{"袋鼠式护理", "早产儿 母乳", "咖啡因 呼吸暂停", "足跟血", "听力筛查 复筛", "kangaroo"} {
		res, err := r.SearchNewbornCare(context.Background(), q, 3)
		if err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		if len(res) == 0 {
			t.Errorf("query %q: no results", q)
			continue
		}
		t.Logf("%q -> %s %s", q, res[0].Kind, res[0].TitleZH)
	}
	// 足跟血必须命中中国筛查条目而非 WHO 推荐
	res, _ := r.SearchNewbornCare(context.Background(), "足跟血 采血时间", 3)
	if len(res) == 0 || res[0].ID != "cn-nbs-metabolic" {
		t.Errorf("足跟血 top = %+v, want cn-nbs-metabolic", firstID(res))
	}
	// 袋鼠护理应命中 WHO A.1a/1b
	res2, _ := r.SearchNewbornCare(context.Background(), "袋鼠式护理", 3)
	if len(res2) == 0 || res2[0].ID != "A.1a" {
		t.Errorf("袋鼠式护理 top = %s, want A.1a", firstID(res2))
	}
}

func firstID(rs []NewbornResult) string {
	if len(rs) == 0 {
		return ""
	}
	return rs[0].ID
}
