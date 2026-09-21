package agent

import (
	"strings"
	"testing"
)

func TestNeedsClarification(t *testing.T) {
	vague := []string{
		"头疼",
		"肚子疼",
		// 25 runes of pure symptom listing, no clinical slot at all — the
		// old >20-rune rule let this through without any follow-up.
		"我头痛头晕没精神浑身不舒服食欲也变差了整个人都蔫了",
	}
	rich := []string{
		"头疼3天了还有点恶心", // 持续时间 + 伴随
		"我最近总是头疼",    // 时间槽位（旧规则没有"最近/总是"，会误追问）
		"布洛芬能治头痛吗",   // 通用知识问题豁免（旧规则会误追问）
		"孩子发烧39度怎么办", // 患者信息 + 严重程度
		"怀孕3个月头痛",    // 患者信息 + 时间
		"头痛吃什么药",     // 诱因背景槽位（药）
		"什么是高血压",     // 无症状词
		"我这边的情况是这样的，就是头疼，整个头都在疼，太阳穴的位置疼得我想撞墙，睁眼也疼闭眼也疼，按一按会好那么一点点但是很快又不行了，所以想请您看看这是什么问题", // >60 runes：长叙述不再 interrogate
	}
	for _, msg := range vague {
		if !needsClarification(msg) {
			t.Errorf("needsClarification(%q) = false, want true", msg)
		}
	}
	for _, msg := range rich {
		if needsClarification(msg) {
			t.Errorf("needsClarification(%q) = true, want false", msg)
		}
	}
}

func TestClarificationGuidanceNamesMissingSlots(t *testing.T) {
	g := clarificationGuidance("头疼")
	if g == "" {
		t.Fatal("expected guidance for bare symptom")
	}
	for _, want := range []string{"## 信息不足时的澄清指引", "持续时间", "严重程度", "患者信息"} {
		if !strings.Contains(g, want) {
			t.Errorf("guidance missing %q:\n%s", want, g)
		}
	}
	if strings.Count(g, "- 症状") < 1 {
		t.Errorf("guidance should carry targeted question hints:\n%s", g)
	}
	// Question hints are capped: at most 3 bullets.
	if strings.Count(g, "\n- ") > 3 {
		t.Errorf("guidance should cap follow-up hints at 3:\n%s", g)
	}
	if clarificationGuidance("头疼3天了还有点恶心") != "" {
		t.Error("expected no guidance when slots are filled")
	}
}
