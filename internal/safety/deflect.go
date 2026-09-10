package safety

import (
	"strings"
	"unicode"
)

// referralMarkers 出现任一标记的句子视为"劝导就医"句，从回答中剔除。
// (2026-09-08 产品决策：回答不劝导就医，改为引导用户深挖病因。)
var referralMarkers = []string{
	"拨打120", "拨打 120", "呼叫120", "呼叫 120", "打120", "打 120",
	"救护车", "急救电话",
	"立即就医", "尽快就医", "及时就医", "尽早就医", "就医治疗",
	"去医院", "到医院", "前往医院",
	"看医生", "看大夫", "求医", "就诊",
	"咨询医生", "咨询医师", "咨询专业医生", "医生指导", "医师指导",
	"紧急治疗处理",
}

// RemoveReferralSentences 按句剔除劝导就医的表述。
// 保留含"危险信号/警惕/记录"等引导深挖病因的句子。
func RemoveReferralSentences(text string) string {
	if text == "" {
		return text
	}
	var out strings.Builder
	for _, para := range strings.Split(text, "\n") {
		var kept []string
		for _, sent := range splitSentences(para) {
			if containsReferral(sent) {
				continue
			}
			kept = append(kept, sent)
		}
		line := strings.Join(kept, "")
		out.WriteString(line)
		out.WriteString("\n")
	}
	res := strings.TrimRight(out.String(), "\n")
	// 清理连续空行
	for strings.Contains(res, "\n\n\n") {
		res = strings.ReplaceAll(res, "\n\n\n", "\n\n")
	}
	return res
}

func containsReferral(sent string) bool {
	for _, m := range referralMarkers {
		if strings.Contains(sent, m) {
			return true
		}
	}
	return false
}

// splitSentences 按中英句末标点断句，标点保留在句尾。
func splitSentences(line string) []string {
	var sents []string
	var cur strings.Builder
	for _, r := range line {
		cur.WriteRune(r)
		if r == '。' || r == '！' || r == '？' || r == '!' || r == '?' || r == ';' || r == '；' {
			sents = append(sents, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		rest := cur.String()
		// 表格行含劝医词时整行丢弃，否则原样保留
		if !(isTableLike(rest) && containsReferral(rest)) {
			sents = append(sents, rest)
		}
	}
	return sents
}

func isTableLike(s string) bool {
	return strings.HasPrefix(strings.TrimLeftFunc(s, unicode.IsSpace), "|")
}
