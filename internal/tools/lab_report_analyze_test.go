package tools

import (
	"context"
	"strings"
	"testing"
)

func TestParseLabReportMixed(t *testing.T) {
	text := `白细胞 12.5 ×10⁹/L ↑
WBC:12.5(H)
血红蛋白:95 g/L ↓
肌酐（Cr） 95 umol/l
尿蛋白 +2
尿亚硝酸盐：阴性
血小板 150 ×10⁹/L
甘油三酯 3.2 mmol/L ↑`
	rs := parseLabReport(text)
	got := map[string]bool{}
	for _, r := range rs {
		got[r.item.Names[0]] = true
		if r.item.Names[0] == "白细胞" && (r.value == nil || *r.value != 12.5 || !r.flagHigh) {
			t.Errorf("白细胞解析错: %+v", r)
		}
		if r.item.Names[0] == "血红蛋白" && (r.value == nil || *r.value != 95 || !r.flagLow) {
			t.Errorf("血红蛋白解析错: %+v", r)
		}
		if r.item.Names[0] == "肌酐" && (r.value == nil || *r.value != 95) {
			t.Errorf("肌酐解析错: %+v", r)
		}
		if r.item.Names[0] == "尿蛋白" && r.qualitative != "+2" {
			t.Errorf("尿蛋白解析错: %+v", r)
		}
	}
	for _, want := range []string{"白细胞", "血小板", "甘油三酯"} {
		// WBC 是白细胞别名, 两条都会命中同一项
		if want == "白细胞" && !got[want] {
			t.Errorf("缺 %s", want)
		}
	}
	if !got["血小板"] || !got["甘油三酯"] || !got["尿亚硝酸盐"] {
		t.Errorf("项目缺失: %v", got)
	}
}

func TestLabReportAnalyzeExecute(t *testing.T) {
	tool := NewLabReportAnalyze()
	res, err := tool.Execute(context.Background(), map[string]any{
		"report_text": "白细胞 12.5 10*9/L ↑\n血红蛋白 95 g/L ↓\nMCV 68 fL ↓\n血小板 150\n空腹血糖 7.5 mmol/L ↑",
		"gender":      "男",
	})
	if err != nil || !res.Success {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	b := res.Data["detail"].(string)
	for _, want := range []string{"白细胞", "↑ 偏高", "↓ 偏低", "小细胞性", "糖尿病"} {
		if !strings.Contains(b, want) {
			t.Errorf("detail 缺 %q: %s", want, b)
		}
	}
	if len(res.Data["abnormal"].([]map[string]any)) != 4 {
		t.Errorf("应有 4 项异常: %v", res.Data["abnormal"])
	}
}

func TestLabReportAnalyzeGenderRange(t *testing.T) {
	tool := NewLabReportAnalyze()
	// HGB 120: 男正常(130-175 低限以下→低), 女(115-150)正常
	for _, tc := range []struct {
		gender, want string
	}{
		{"男", "↓ 偏低"},
		{"女", "✓ 正常"},
	} {
		res, err := tool.Execute(context.Background(), map[string]any{
			"report_text": "血红蛋白 120 g/L",
			"gender":      tc.gender,
		})
		if err != nil || !res.Success {
			t.Fatalf("%s: err=%v", tc.gender, err)
		}
		if !strings.Contains(res.Data["detail"].(string), tc.want) {
			t.Errorf("%s: 期望 %s, got %s", tc.gender, tc.want, res.Data["detail"])
		}
	}
}
