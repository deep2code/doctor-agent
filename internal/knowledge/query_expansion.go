package knowledge

import "strings"

// synonymGroups maps colloquial symptom words to their clinical/equivalent
// forms. Retrieval matches by substring ("query contains keyword"), so a
// query like "突然大哭" never contains the indexed keyword "哭闹" and the
// entry is missed. ExpandQuery appends the rest of each group whose any
// member appears in the query, so all equivalent forms become matchable.
var synonymGroups = [][]string{
	{"哭", "大哭", "啼哭", "夜啼", "哭闹", "夜惊", "肠绞痛"},
	{"发烧", "发热", "体温高", "低烧", "高烧"},
	{"拉肚子", "拉稀", "腹泻", "大便稀", "水样便"},
	{"便秘", "排便困难", "不拉大便"},
	{"肚子疼", "腹痛", "肚子痛", "胃痛", "胃疼", "肚子不舒服"},
	{"腹胀", "胀气", "肚子胀"},
	{"头晕", "眩晕", "头昏"},
	{"头疼", "头痛"},
	{"咳嗽", "咳痰", "干咳", "夜咳"},
	{"喘", "气促", "呼吸急促", "哮喘", "呼吸困难"},
	{"心慌", "心悸", "胸闷"},
	{"恶心", "反胃", "想吐"},
	{"呕吐", "吐奶", "溢奶"},
	{"抽筋", "痉挛", "抽搐", "惊跳"},
	{"疹子", "皮疹", "湿疹", "荨麻疹", "红疹"},
	{"睡不着", "失眠", "睡眠差", "入睡困难", "睡眠不安"},
	{"没胃口", "食欲不振", "食欲差", "厌食"},
	{"乏力", "疲倦", "无力", "没精神"},
	{"尿频", "排尿频繁", "尿多"},
	{"便血", "大便带血", "血便", "黑便"},
	{"超重", "肥胖", "体重超标"},
	{"消瘦", "体重轻", "体重不增"},
	{"个子矮", "生长迟缓", "长不高", "身材矮小"},
	{"鼻塞", "流鼻涕", "流涕", "鼻炎"},
	{"嗓子疼", "咽喉痛", "咽痛", "嗓子痛"},
	{"反酸", "烧心", "胃酸"},
	{"女婴", "婴儿", "宝宝", "新生儿", "婴幼儿", "小儿", "幼儿", "儿童"},
	{"男婴", "男宝", "男宝宝"},
}

// ExpandQuery returns the query with synonyms of matched groups appended,
// so keyword retrieval can match clinical phrasings the user did not use.
// The original query always comes first.
func ExpandQuery(query string) string {
	var extra []string
	seen := make(map[string]bool)
	for _, group := range synonymGroups {
		hit := false
		for _, term := range group {
			if term != "" && strings.Contains(query, term) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		for _, term := range group {
			if !seen[term] {
				seen[term] = true
				extra = append(extra, term)
			}
		}
	}
	if len(extra) == 0 {
		return query
	}
	return query + " " + strings.Join(extra, " ")
}
