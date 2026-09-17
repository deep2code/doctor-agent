package knowledge

import (
	"context"
	"testing"
)

func TestFeedingGuidelinesLoad(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if store.GetMedicalByID("cn-feeding-0to6") == nil {
		t.Fatal("feeding_guidelines.json 未嵌入（cn-feeding-0to6 缺失）")
	}
	t.Logf("feeding_guidelines entries embedded")
}

func TestRetrieveFeedingGuidelines(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if store.GetMedicalByID("cn-feeding-0to6") == nil {
		t.Skip("feeding_guidelines.json 未嵌入")
	}
	r := NewRetriever(store)
	cases := []struct{ q, expectID string }{
		{"宝宝几个月加辅食", "cn-feeding-7to24"},
		{"母乳喂养", "cn-feeding-0to6"},
		{"孩子挑食怎么办", "cn-feeding-preschool"},
		{"辅食怎么加", "cn-feeding-7to24"},
	}
	for _, c := range cases {
		res, err := r.Retrieve(context.Background(), c.q, 5)
		if err != nil {
			t.Fatalf("query %q: %v", c.q, err)
		}
		found := false
		for _, e := range res {
			if e.Entry.ID == c.expectID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("query %q: 期望命中 %s，实际结果: %v", c.q, c.expectID, resIDs(res))
		}
		t.Logf("query %q -> top: %s", c.q, res[0].Entry.ID)
	}
}
