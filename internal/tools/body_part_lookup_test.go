package tools

import (
	"context"
	"testing"

	"github.com/doctor-agent/internal/knowledge"
)

// normalizePart is DB-free and pure — test it exhaustively.
func TestNormalizePart(t *testing.T) {
	cases := map[string]string{
		"右下腹":    "右下腹",
		"右下腹痛":   "右下腹",
		"右下腹部":   "右下腹",
		"膝盖":     "膝盖",
		"膝盖疼":    "膝盖",
		"膝盖痛":    "膝盖",
		"腰痛":     "腰",
		"腰":      "腰",
		"头痛":     "头",
		"头部不适":   "头",
		"小腿不舒服":  "小腿",
		"左侧上腹部痛": "左侧上腹",
	}
	for in, want := range cases {
		if got := normalizePart(in); got != want {
			t.Errorf("normalizePart(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBodyPartLookup(t *testing.T) {
	store, err := knowledge.Load()
	if err != nil {
		t.Skipf("knowledge store not available: %v", err)
	}
	tool := NewBodyPartLookup(store)

	cases := []struct {
		part     string
		wantPart string
	}{
		{"右下腹", "右下腹"},
		{"右下腹痛", "右下腹"},
		{"膝盖", "膝部"},
		{"膝盖疼", "膝部"},
		{"腰痛", "下背部/腰"},
		{"小腿", "小腿"},
		{"头部", "头部"},
	}
	for _, c := range cases {
		res, err := tool.Execute(context.Background(), map[string]any{"body_part": c.part})
		if err != nil {
			t.Fatalf("body_part=%q: %v", c.part, err)
		}
		if !res.Success {
			t.Errorf("body_part=%q: 未匹配: %s", c.part, res.Error)
			continue
		}
		got, _ := res.Data["part"].(string)
		if got != c.wantPart {
			t.Errorf("body_part=%q: 匹配到 %q, want %q", c.part, got, c.wantPart)
		}
	}
}

// 左右镜像正确性：正面视图患者右侧显示在观察者左侧，腹部四象限必须按患者视角。
// 右上腹(abd_ur)的常见病是肝胆/胆囊（患者右侧），不能标成左侧。
func TestBodyPartMirrorCorrectness(t *testing.T) {
	store, err := knowledge.Load()
	if err != nil {
		t.Skipf("knowledge store not available: %v", err)
	}
	tool := NewBodyPartLookup(store)

	res, err := tool.Execute(context.Background(), map[string]any{"body_part": "右上腹"})
	if err != nil || !res.Success {
		t.Fatalf("右上腹查询失败: %v %v", err, res)
	}
	conds, _ := res.Data["conditions"].([]string)
	foundGallbladder := false
	for _, c := range conds {
		if containsAny(c, "胆囊", "肝") {
			foundGallbladder = true
		}
	}
	if !foundGallbladder {
		t.Errorf("右上腹应含胆囊/肝相关疾病（患者视角右侧），实际: %v", conds)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}
