package safety

import (
	"context"
	"strings"
	"testing"

	"github.com/doctor-agent/internal/knowledge"
	"github.com/doctor-agent/internal/llm"
)

// fakeJudge is a deterministic LLMProvider stub returning preset text.
type fakeJudge struct{ text string }

func (f *fakeJudge) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, systemPrompt string) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{Text: f.text}, nil
}

func (f *fakeJudge) StreamChat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, systemPrompt string, onDelta func(string)) (*llm.ChatResponse, error) {
	if onDelta != nil {
		onDelta(f.text)
	}
	return &llm.ChatResponse{Text: f.text}, nil
}
func (f *fakeJudge) Name() string { return "fake-judge" }

// sampleSources builds a sources map with citation numbers "1".."2".
func sampleSources() map[string]knowledge.CitedSource {
	return map[string]knowledge.CitedSource{
		"1": {
			Number:      "1",
			EntryID:     "thal-001",
			ConditionZH: "α-地中海贫血",
			Citation:    knowledge.Citation{Title: "广西地贫流行病学研究", Year: 2025, DOI: "10.1000/example"},
			EntryText:   "广西α-地贫携带率约14.95%。",
		},
		"2": {
			Number:      "2",
			EntryID:     "g6pd-001",
			ConditionZH: "G6PD缺乏症",
			Citation:    knowledge.Citation{Title: "G6PD诊疗指南", Year: 2025},
			EntryText:   "磺胺类药物在G6PD缺乏者中属禁忌。",
		},
	}
}

func TestExtractCitedClaims(t *testing.T) {
	sources := sampleSources()
	response := "广西地区α-地贫携带率约为14.95% [1]。\n\n## 参考文献\n[1] 广西地贫流行病学研究. 2025."

	claims := extractCitedClaims(response, sources)
	if len(claims) != 1 {
		t.Fatalf("期望提取 1 条声明，实际 %d: %+v", len(claims), claims)
	}
	if len(claims[0].numbers) != 1 || claims[0].numbers[0] != "1" {
		t.Errorf("声明编号应为 [1]，实际 %v", claims[0].numbers)
	}
	if !strings.Contains(claims[0].text, "14.95%") {
		t.Errorf("声明文本应为正文句，实际: %q", claims[0].text)
	}
}

func TestExtractCitedClaimsMultiNumbers(t *testing.T) {
	sources := sampleSources()
	claims := extractCitedClaims("广西携带率约14.95% [1]，而磺胺类药物属禁忌 [2]。", sources)
	if len(claims) != 1 {
		t.Fatalf("期望提取 1 条声明，实际 %d: %+v", len(claims), claims)
	}
	if len(claims[0].numbers) != 2 {
		t.Errorf("多引用句应捕获所有编号，实际 %v", claims[0].numbers)
	}
}

func TestExtractCitedClaimsSkipsUnknownNumbers(t *testing.T) {
	sources := sampleSources()
	claims := extractCitedClaims("某事实 [9]。", sources)
	if len(claims) != 0 {
		t.Fatalf("未知编号 [9] 不应被提取，实际 %+v", claims)
	}
}

func TestVerifyCitationNumberCheck(t *testing.T) {
	v := NewPostVerifier(nil)
	result := v.Verify(context.Background(), "本地发生率约5% [5]。", sampleSources())
	if result.Passed {
		t.Fatal("引用 [5] 不在检索上下文中，应校验失败")
	}
	if len(result.UnverifiedClaims) == 0 {
		t.Fatal("应产生未验证引用说明")
	}
}

func TestVerifyWithoutSourcesSkipsNumberCheck(t *testing.T) {
	v := NewPostVerifier(nil)
	// No retrieval context: number-existence check must not false-positive.
	result := v.Verify(context.Background(), "本地发生率约5% [5]。", nil)
	if result.CitationCount != 1 {
		t.Fatalf("应识别到 1 个引用，实际 %d", result.CitationCount)
	}
	if !result.Passed {
		t.Fatalf("无检索上下文时不应因编号检查失败: %v", result.UnverifiedClaims)
	}
	if len(result.UnverifiedClaims) != 0 {
		t.Fatalf("不应产生未验证引用说明: %v", result.UnverifiedClaims)
	}
}

func TestVerifyJudgeFlagsUnsupportedClaim(t *testing.T) {
	judge := &fakeJudge{text: `[{"claim":"广西携带率14.95%","supported":false,"reason":"文献中无此数据"}]`}
	v := NewPostVerifierWithJudge(nil, judge)

	result := v.Verify(context.Background(), "广西α-地贫携带率约为14.95% [1]。", sampleSources())
	if result.Passed {
		t.Fatal("judge 判定不支持时 Verify 应失败")
	}
	if len(result.UnsupportedClaims) == 0 {
		t.Fatal("应记录不支持声明")
	}
	if !strings.Contains(result.CorrectedResponse, "未验证") {
		t.Errorf("修正响应应包含核查说明，实际: %q", result.CorrectedResponse)
	}
}

func TestVerifyJudgePassesSupportedClaims(t *testing.T) {
	judge := &fakeJudge{text: `[{"claim":"广西携带率14.95%","supported":true,"reason":"与文献一致"}]`}
	v := NewPostVerifierWithJudge(nil, judge)

	result := v.Verify(context.Background(), "广西α-地贫携带率约为14.95% [1]。", sampleSources())
	if !result.Passed {
		t.Fatalf("judge 支持时 Verify 应通过，warnings=%v", result.Warnings)
	}
	if len(result.UnsupportedClaims) != 0 {
		t.Fatalf("不应有不支持声明: %v", result.UnsupportedClaims)
	}
}

func TestVerifyJudgeUnparseableOutputDegradesGracefully(t *testing.T) {
	judge := &fakeJudge{text: "抱歉，我无法完成此任务。"}
	v := NewPostVerifierWithJudge(nil, judge)

	result := v.Verify(context.Background(), "广西α-地贫携带率约为14.95% [1]。", sampleSources())
	// Judge failure must not block: rule checks alone decide.
	if result.CitationCount != 1 {
		t.Fatalf("规则检查应照常执行，引用数=%d", result.CitationCount)
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "语义核查未完成") {
			found = true
		}
	}
	if !found {
		t.Errorf("judge 输出无法解析时应记录警告，warnings=%v", result.Warnings)
	}
}

func TestBuildCitedSourcesNumbering(t *testing.T) {
	entries := []knowledge.RetrievalResult{
		{
			Entry: knowledge.KnowledgeEntry{
				ID:          "thal-001",
				ConditionZH: "α-地中海贫血",
				Citations: []knowledge.Citation{
					{Title: "A", Year: 2025},
					{Title: "B", Year: 2024},
				},
			},
		},
		{
			Entry: knowledge.KnowledgeEntry{
				ID:          "g6pd-001",
				ConditionZH: "G6PD缺乏症",
				Citations: []knowledge.Citation{
					{Title: "C", Year: 2025},
				},
			},
		},
	}

	sources := knowledge.BuildCitedSources(entries)
	if len(sources) != 3 {
		t.Fatalf("期望 3 个来源，实际 %d", len(sources))
	}
	if sources["1"].Citation.Title != "A" || sources["2"].Citation.Title != "B" || sources["3"].Citation.Title != "C" {
		t.Errorf("平铺编号与提示词不一致: %+v", sources)
	}
	if sources["2"].EntryID != "thal-001" {
		t.Errorf("编号 2 应归属 thal-001，实际 %s", sources["2"].EntryID)
	}
	if !strings.Contains(sources["1"].EntryText, "α-地中海贫血") {
		t.Errorf("EntryText 应包含条目摘要，实际: %q", sources["1"].EntryText)
	}
}

func TestParseJudgeVerdictsFenced(t *testing.T) {
	text := "好的，以下是结果：\n```json\n[{\"claim\":\"甲\",\"supported\":false,\"reason\":\"文中含]号测试\"}]\n```\n"
	verdicts, err := parseJudgeVerdicts(text)
	if err != nil {
		t.Fatalf("解析带围栏的输出失败: %v", err)
	}
	if len(verdicts) != 1 || verdicts[0].Supported {
		t.Fatalf("解析结果不符: %+v", verdicts)
	}
	if !strings.Contains(verdicts[0].Reason, "]") {
		t.Errorf("reason 中的 ] 应被正确处理，实际: %q", verdicts[0].Reason)
	}
}

func TestVerifyRuleChecksSkipReferenceTail(t *testing.T) {
	v := NewPostVerifier(nil)
	response := "建议按说明书使用，剂量请遵医嘱。\n\n## 参考文献\n[1] 每天1片维生素C的RCT研究. 2024."
	result := v.Verify(context.Background(), response, nil)
	if result.HasDosageAdvice {
		t.Fatal("参考文献尾部的'每天1片'不应触发剂量警告")
	}
}

func TestVerifyForbiddenDiagnosticPattern(t *testing.T) {
	v := NewPostVerifier(nil)
	result := v.Verify(context.Background(), "根据您的症状，您得了胃癌，必须马上手术。", sampleSources())
	if !result.HasDiagnosisClaim {
		t.Fatal("确定性诊断断言应被标记")
	}
	if result.Passed {
		t.Fatal("确定性诊断断言应导致校验失败")
	}
}

func TestVerifyForbiddenDosagePattern(t *testing.T) {
	v := NewPostVerifier(nil)
	result := v.Verify(context.Background(), "您每天应该服用2片维生素C。", sampleSources())
	if !result.HasDosageAdvice {
		t.Fatal("剂量建议应被标记")
	}
	if result.Passed {
		t.Fatal("剂量建议应导致校验失败")
	}
}
