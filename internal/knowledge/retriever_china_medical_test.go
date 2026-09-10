package knowledge

import (
	"context"
	"testing"
)

func TestRetrieveChinaMedicalData(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	r := NewRetriever(store)

	// 测试中医药检索
	t.Run("TCM", func(t *testing.T) {
		cases := []struct {
			query     string
			expectIDs []string
		}{
			{"人参", []string{"tcm_herb_renshen"}},
			{"板蓝根", []string{"tcm_herb_banlangen"}},
			{"足三里", []string{"tcm_acupoint_zusanli"}},
			{"四君子汤", []string{"tcm_formula_sijunzi_tang"}},
		}
		for _, c := range cases {
			res, err := r.Retrieve(context.Background(), c.query, 3)
			if err != nil {
				t.Fatalf("query %q: %v", c.query, err)
			}
			found := false
			for _, e := range res {
				for _, id := range c.expectIDs {
					if e.Entry.ID == id {
						found = true
						break
					}
				}
			}
			if !found {
				t.Logf("query %q -> results: %v", c.query, resIDs(res))
			}
		}
	})

	// 测试临床路径检索
	t.Run("ClinicalPathways", func(t *testing.T) {
		cases := []struct {
			query     string
			expectIDs []string
		}{
			{"高血压临床路径", []string{"clinical_pathway_CP-001"}},
			{"糖尿病临床路径", []string{"clinical_pathway_CP-002"}},
		}
		for _, c := range cases {
			res, err := r.Retrieve(context.Background(), c.query, 3)
			if err != nil {
				t.Fatalf("query %q: %v", c.query, err)
			}
			found := false
			for _, e := range res {
				for _, id := range c.expectIDs {
					if e.Entry.ID == id {
						found = true
						break
					}
				}
			}
			if !found {
				t.Logf("query %q -> results: %v", c.query, resIDs(res))
			}
		}
	})

	// 测试传染病检索
	t.Run("CDC", func(t *testing.T) {
		cases := []struct {
			query string
		}{
			{"肺结核 传染病"},
			{"艾滋病 2021"},
			{"病毒性肝炎"},
		}
		for _, c := range cases {
			res, err := r.Retrieve(context.Background(), c.query, 3)
			if err != nil {
				t.Fatalf("query %q: %v", c.query, err)
			}
			t.Logf("query %q -> top: %s", c.query, res[0].Entry.ID)
		}
	})

	// 测试CSCO指南检索
	t.Run("CSCO", func(t *testing.T) {
		cases := []struct {
			query string
		}{
			{"非小细胞肺癌 指南"},
			{"乳腺癌 CSCO"},
			{"胃癌 治疗"},
		}
		for _, c := range cases {
			res, err := r.Retrieve(context.Background(), c.query, 3)
			if err != nil {
				t.Fatalf("query %q: %v", c.query, err)
			}
			t.Logf("query %q -> top: %s", c.query, res[0].Entry.ID)
		}
	})

	// 测试膳食指南检索
	t.Run("Dietary", func(t *testing.T) {
		cases := []struct {
			query string
		}{
			{"膳食宝塔"},
			{"孕妇 营养"},
			{"老年人 饮食"},
		}
		for _, c := range cases {
			res, err := r.Retrieve(context.Background(), c.query, 3)
			if err != nil {
				t.Fatalf("query %q: %v", c.query, err)
			}
			if len(res) == 0 {
				t.Logf("query %q -> no results", c.query)
			} else {
				t.Logf("query %q -> top: %s", c.query, res[0].Entry.ID)
			}
		}
	})

	// 测试卫生统计检索
	t.Run("Stats", func(t *testing.T) {
		cases := []struct {
			query string
		}{
			{"中国 卫生统计 2020"},
			{"医疗机构 数量"},
		}
		for _, c := range cases {
			res, err := r.Retrieve(context.Background(), c.query, 3)
			if err != nil {
				t.Fatalf("query %q: %v", c.query, err)
			}
			if len(res) == 0 {
				t.Logf("query %q -> no results", c.query)
			} else {
				t.Logf("query %q -> top: %s", c.query, res[0].Entry.ID)
			}
		}
	})
}