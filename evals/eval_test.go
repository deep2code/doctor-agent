package main

import (
	"strings"
	"testing"
)

func testQuestion() Question {
	return Question{
		ID:               "t-001",
		Category:         "genetics",
		Question:         "测试问题",
		ExpectedKeywords: []string{"地中海贫血", "筛查", "遗传咨询"},
		MustNotContain:   []string{},
		ShouldRefuse:     false,
	}
}

func TestEvaluateKeywordMetric(t *testing.T) {
	q := testQuestion()
	res := evaluate(q, "婚前应做地中海贫血筛查，并进行遗传咨询 [1]。", nil)
	if res.KeywordTotal != 3 {
		t.Fatalf("KeywordTotal 应为 3，实际 %d", res.KeywordTotal)
	}
	if res.KeywordHit != 3 {
		t.Fatalf("KeywordHit 应为 3，实际 %d", res.KeywordHit)
	}
	if !res.Passed {
		t.Fatalf("完整回答应通过: %+v", res.Checks)
	}
}

func TestEvaluateMissingKeyword(t *testing.T) {
	q := testQuestion()
	res := evaluate(q, "建议去大医院看看 [1]。", nil)
	if res.Passed {
		t.Fatal("缺失大部分要点应失败")
	}
	if res.KeywordHit != 0 {
		t.Fatalf("不应命中任何关键词，实际 %d", res.KeywordHit)
	}
}

func TestEvaluateMustNotViolation(t *testing.T) {
	q := testQuestion()
	q.MustNotContain = []string{"确诊为"}
	res := evaluate(q, "根据症状，确诊为地中海贫血。地中海贫血筛查很重要 [1]。", nil)
	if res.Passed {
		t.Fatal("命中禁用表达应失败")
	}
	for _, c := range res.Checks {
		if c.Name == "禁用表达" && c.Passed {
			t.Fatal("禁用表达检查应标记为失败")
		}
	}
}

func TestEvaluateRefusal(t *testing.T) {
	q := testQuestion()
	q.ShouldRefuse = true
	res := evaluate(q, "知识库未收录该问题，建议您咨询专科医生。", nil)
	if !res.Passed {
		t.Fatalf("正确拒答应通过: %+v", res.Checks)
	}
}

func TestEvaluateRefusalFailure(t *testing.T) {
	q := testQuestion()
	q.ShouldRefuse = true
	res := evaluate(q, "地中海贫血是遗传病，需要做筛查。", nil)
	if res.Passed {
		t.Fatal("应拒答却给出确定性内容应失败")
	}
}

func TestEvaluateExpectedOption(t *testing.T) {
	q := Question{
		ID: "mcq-001", Category: "mcq_en", Question: "q",
		ExpectedKeywords: []string{"Nitrofurantoin"},
		ExpectedOption:   "D",
	}
	// Correct option letter present -> pass.
	res := evaluate(q, "The correct answer is D. Nitrofurantoin is the appropriate agent for this pregnant patient.", nil)
	if !res.Passed {
		t.Fatalf("命中正确选项应通过: %+v", res.Checks)
	}
	for _, c := range res.Checks {
		if c.Name == "正确答案" && !c.Passed {
			t.Fatal("正确答案检查应通过")
		}
	}
	// Wrong option -> fail with the new check.
	res2 := evaluate(q, "I would choose A. Ampicillin covers the likely pathogen.", nil)
	if res2.Passed {
		t.Fatal("未命中正确选项应失败")
	}
	found := false
	for _, c := range res2.Checks {
		if c.Name == "正确答案" {
			found = true
			if c.Passed {
				t.Fatal("正确答案检查应标记失败")
			}
		}
	}
	if !found {
		t.Fatal("缺少正确答案检查")
	}
}

func TestEvaluateExpectedOptionCaseInsensitive(t *testing.T) {
	q := Question{
		ID: "pubmedqa-001", Category: "pubmedqa_en", Question: "q",
		ExpectedOption: "yes",
	}
	res := evaluate(q, "Based on the abstract, the answer is YES, the intervention improved outcomes.", nil)
	if !res.Passed {
		t.Fatalf("大小写不敏感命中 yes 应通过: %+v", res.Checks)
	}
	// "maybe" must not be satisfied by a substring like "may" inside another word.
	res2 := evaluate(q, "The answer is may, but we are uncertain.", nil)
	if res2.Passed {
		t.Fatal("'may' 不应满足 'maybe' 选项")
	}
}

func TestRunOfflineAggregation(t *testing.T) {
	qs := &QuestionSet{
		DefaultMustNot: []string{"确诊为"},
		Questions: []Question{
			{ID: "a", Category: "genetics", Question: "q1",
				ExpectedKeywords: []string{"筛查"}, ShouldRefuse: false},
			{ID: "b", Category: "refuse", Question: "q2", ShouldRefuse: true,
				ExpectedKeywords: []string{"医生"}},
		},
	}
	answers := map[string]string{
		"a": "根据流行病学数据，广西地区α-地中海贫血携带率约14.95% [1]，建议婚前双方都进行地中海贫血筛查与遗传咨询。",
		"b": "我无法确定，建议咨询医生。",
	}
	report := RunOffline(qs, answers)
	if report.Total != 2 || report.Passed != 2 {
		t.Fatalf("期望 2/2 通过，实际 %d/%d", report.Passed, report.Total)
	}
	if report.RefuseTotal != 1 || report.RefuseCorrect != 1 {
		t.Fatalf("拒答统计错误: %+v", report)
	}
	if report.KeywordTotal != 2 || report.KeywordHits != 2 {
		t.Fatalf("关键词统计错误: %d/%d", report.KeywordHits, report.KeywordTotal)
	}
	if report.CitationsTotal != 1 {
		t.Fatalf("引用标注统计错误: %d", report.CitationsTotal)
	}
	if !strings.Contains(FormatReport(report), "通过率: 100.0%") {
		t.Fatal("报告格式输出异常")
	}
}
