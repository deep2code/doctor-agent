package tools

import (
	"context"
	"strings"
	"testing"
)

func TestVisitPrepExecute(t *testing.T) {
	tool := NewVisitPrep()
	res, err := tool.Execute(context.Background(), map[string]any{
		"chief_complaint":   "反复咳嗽2周，夜间加重",
		"patient_profile":   "35岁女性",
		"symptom_timeline":  "2周前开始干咳\n1周前夜间加重，影响睡眠",
		"current_meds":      "右美沙芬糖浆 10ml 每晚",
		"red_flags":         "昨晚出现一次痰中带血丝",
		"questions_to_ask":  "是否需要做胸片？\n- 是否可能是咳嗽变异性哮喘？",
		"target_department": "呼吸内科",
	})
	if err != nil || !res.Success {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	md := res.Data["markdown"].(string)
	for _, want := range []string{
		"主诉：反复咳嗽2周", "35岁女性", "呼吸内科", "痰中带血丝",
		"1. 是否需要做胸片？", "2. 是否可能是咳嗽变异性哮喘？", "右美沙芬",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("缺 %q:\n%s", want, md)
		}
	}
}

func TestVisitPrepMissingRequired(t *testing.T) {
	tool := NewVisitPrep()
	res, err := tool.Execute(context.Background(), map[string]any{
		"chief_complaint": "头痛",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Success {
		t.Errorf("缺字段应失败: %+v", res)
	}
}
