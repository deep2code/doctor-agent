package knowledge

import (
	"strings"
	"testing"
)

func TestExcerptAround(t *testing.T) {
	// 构造 1 万行正文, 目标病种 "戈谢病" 深处正文, 开头是别的病.
	var b strings.Builder
	b.WriteString("成骨不全症\n一、病因\n骨骼脆弱……\n")
	for i := 0; i < 5000; i++ {
		b.WriteString("血友病甲 二、流行病学 治疗原则 替代治疗 输注凝血因子 定期预防\n")
	}
	b.WriteString("戈谢病\n一、病因\n葡萄糖脑苷脂酶缺乏，伊米苷酶酶替代治疗。\n")
	for i := 0; i < 5000; i++ {
		b.WriteString("其他章节 内容填充 行 用于撑开正文长度 非目标疾病\n")
	}
	content := b.String()

	ex := ExcerptAround(content, "戈谢病 酶替代治疗", 4800)
	if len([]rune(ex)) >= len([]rune(content)) {
		t.Fatalf("应截取片段而非全文: %d", len([]rune(ex)))
	}
	if !strings.Contains(ex, "戈谢病") || !strings.Contains(ex, "伊米苷酶") {
		t.Errorf("片段应含目标病种章节, got: %.120s", ex)
	}
	if strings.Contains(ex, "成骨不全症") {
		t.Errorf("片段不应落在开头无关章节")
	}
}

func TestExcerptAroundShortContent(t *testing.T) {
	short := "流行性感冒诊疗方案\n一、病原学\n流感病毒……"
	if got := ExcerptAround(short, "流感", 4800); got != short {
		t.Errorf("短文应原样返回, got: %q", got)
	}
}

func TestExcerptAroundNoHit(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 5000; i++ {
		b.WriteString("无关内容 行 无查询窗口命中 填充\n")
	}
	content := b.String()
	got := ExcerptAround(content, "戈谢病", 4800)
	if len([]rune(got)) >= len([]rune(content)) {
		t.Fatalf("无命中应回退开头片段")
	}
	if !strings.HasPrefix(got, "无关内容") {
		t.Errorf("无命中应返回开头, got: %.40s", got)
	}
}
