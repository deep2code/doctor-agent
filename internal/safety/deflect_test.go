package safety

import (
	"strings"
	"testing"
)

func TestRemoveReferralSentences(t *testing.T) {
	in := "胸痛伴冷汗可能提示心肌缺血。请立即拨打120急救电话！可嚼服阿司匹林。尽快就医查明病因。"
	out := RemoveReferralSentences(in)
	if strings.Contains(out, "120") {
		t.Errorf("120 指令未被剔除: %q", out)
	}
	if strings.Contains(out, "尽快就医") {
		t.Errorf("劝医句未被剔除: %q", out)
	}
	if !strings.Contains(out, "胸痛伴冷汗可能提示心肌缺血。") {
		t.Errorf("正常句被误删: %q", out)
	}
	if !strings.Contains(out, "可嚼服阿司匹林。") {
		t.Errorf("自救句被误删: %q", out)
	}
}

func TestRemoveReferralTableRows(t *testing.T) {
	in := "| 信号 | 处理 |\n|---|---|\n| 剧痛 | 立即就医 |\n| 轻痛 | 观察记录 |"
	out := RemoveReferralSentences(in)
	if strings.Contains(out, "立即就医") {
		t.Errorf("表格劝医行未被剔除: %q", out)
	}
	if !strings.Contains(out, "观察记录") {
		t.Errorf("正常表格行被误删: %q", out)
	}
}

func TestRemoveReferralKeepsWarningGuidance(t *testing.T) {
	in := "请密切关注并记录以下危险信号：疼痛加重、意识改变。"
	out := RemoveReferralSentences(in)
	if out != in {
		t.Errorf("深挖病因引导句不应被删除: %q", out)
	}
}
