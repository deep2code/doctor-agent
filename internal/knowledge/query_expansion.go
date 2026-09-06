package knowledge

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// synonymGroups maps colloquial symptom/disease words to their clinical/equivalent
// forms (pediatric-focused). Retrieval matches by substring ("query contains
// keyword"), so a query like "突然大哭" never contains the indexed keyword
// "哭闹" and the entry is missed. ExpandQuery appends the rest of each group
// whose any member appears in the query, so all equivalent forms become
// matchable. Extend this list for high-frequency colloquialisms; use
// LoadAliasFile for bulk dictionaries (e.g. imported from CHIP-2019 Yidu-N7K
// or ICD-10 Chinese synonym tables).
var synonymGroups = [][]string{
	// --- 哭闹/睡眠/行为 ---
	{"哭", "大哭", "啼哭", "夜啼", "哭闹", "夜惊", "肠绞痛", "黄昏闹"},
	{"睡不着", "失眠", "睡眠差", "入睡困难", "睡眠不安", "睡觉不踏实"},
	{"惊跳", "易惊", "抽筋", "痉挛", "抽搐", "惊跳反射", "睡觉一惊一惊"},
	{"多汗", "爱出汗", "盗汗", "虚汗", "自汗"},
	{"磨牙", "夜里磨牙", "夜磨牙"},
	{"爱吃手", "吃手", "吮指"},
	{"口吃", "结巴"},
	{"遗尿", "尿床"},
	{"挑食", "偏食", "不爱吃饭", "厌食"},

	// --- 发热/呼吸道 ---
	{"发烧", "发热", "体温高", "低烧", "高烧", "退烧"},
	{"感冒", "上呼吸道感染", "着凉", "伤风"},
	{"流感", "流行性感冒", "甲流", "乙流"},
	{"咳嗽", "咳痰", "干咳", "夜咳", "空空咳", "犬吠样咳嗽"},
	{"喘", "气促", "呼吸急促", "哮喘", "呼吸困难", "喘息"},
	{"喉炎", "急性喉炎"},
	{"支气管炎", "气管炎"},
	{"肺炎", "肺部感染", "支气管肺炎"},
	{"支原体", "支原体肺炎"},
	{"鼻塞", "流鼻涕", "流涕", "鼻炎", "鼻窦炎"},
	{"嗓子疼", "咽喉痛", "咽痛", "嗓子痛", "喉咙痛"},
	{"扁桃体炎", "扁桃体", "扁桃体肥大", "腺样体肥大", "化脓性扁桃体炎"},
	{"中耳炎", "耳朵发炎", "耳朵疼", "抓耳朵"},
	{"腮腺炎", "痄腮", "蛤蟆瘟"},

	// --- 消化 ---
	{"拉肚子", "拉稀", "腹泻", "大便稀", "水样便", "蛋花汤样大便"},
	{"便秘", "排便困难", "不拉大便", "攒肚"},
	{"肚子疼", "腹痛", "肚子痛", "胃痛", "胃疼", "肚子不舒服", "肚子绞痛"},
	{"腹胀", "胀气", "肚子胀", "放屁多"},
	{"恶心", "反胃", "想吐"},
	{"呕吐", "吐奶", "溢奶", "喷奶"},
	{"便血", "大便带血", "血便", "黑便", "果酱样大便"},
	{"反酸", "烧心", "胃酸"},
	{"没胃口", "食欲不振", "食欲差"},
	{"乳糖不耐受", "乳糖酶缺乏", "喝奶就拉肚子"},
	{"蛋白过敏", "牛奶蛋白过敏", "奶粉过敏"},
	{"轮状病毒", "秋季腹泻"},
	{"诺如病毒", "诺如"},
	{"蛔虫", "寄生虫", "肚子里有虫", "蛲虫", "挠屁股"},
	{"疝气", "腹股沟疝", "小肠串气", "脐疝", "肚脐突出"},
	{"肠套叠", "阵发性哭闹"},
	{"肛裂", "肛门裂", "排便哭闹出血"},

	// --- 皮肤/五官 ---
	{"疹子", "皮疹", "红疹", "出疹子"},
	{"湿疹", "奶疹", "特应性皮炎", "过敏性皮炎"},
	{"荨麻疹", "风团", "风疙瘩"},
	{"红屁股", "尿布疹", "尿布皮炎"},
	{"手足口病", "手足口"},
	{"幼儿急疹", "玫瑰疹", "假烧", "烧退疹出"},
	{"水痘", "出水痘"},
	{"鹅口疮", "雪口病", "口腔白点"},
	{"疱疹性咽峡炎", "疱疹咽峡"},
	{"血管瘤", "红胎记"},
	{"胎记", "青记", "蒙古斑"},
	{"黄疸", "新生儿黄疸", "退黄", "黄疸高", "母乳性黄疸", "病理性黄疸"},
	{"斜颈", "歪脖子"},
	{"斜视", "对眼", "斗鸡眼"},
	{"近视", "视力差", "散光", "弱视"},

	// --- 生长/发育/营养 ---
	{"超重", "肥胖", "体重超标", "小胖墩"},
	{"消瘦", "体重轻", "体重不增", "不长肉", "生长迟缓"},
	{"个子矮", "生长迟缓", "长不高", "身材矮小", "矮小症", "侏儒症", "生长激素缺乏"},
	{"性早熟", "发育早", "早熟", "乳房发育早"},
	{"佝偻病", "缺钙", "维生素D缺乏", "方颅", "肋骨外翻"},
	{"缺铁性贫血", "贫血", "血色素低", "面色苍白"},
	{"O型腿", "罗圈腿", "X型腿", "膝外翻"},
	{"鸡胸", "漏斗胸", "胸廓畸形"},
	{"囟门", "囟门凸起", "囟门凹陷", "天灵盖"},
	{"枕秃", "枕头圈"},
	{"地图舌"},
	{"说话晚", "语言发育迟缓", "不开口说话"},
	{"抬头晚", "翻身晚", "走路晚", "大运动落后", "运动发育迟缓"},
	{"多动症", "注意力缺陷", "ADHD"},
	{"自闭症", "孤独症", "ASD"},

	// --- 泌尿/其他 ---
	{"尿频", "排尿频繁", "尿多", "尿量多"},
	{"包茎", "包皮过长"},
	{"隐睾", "睾丸下降不全", "摸不到睾丸"},
	{"鞘膜积液", "水疝"},
	{"淋巴结", "淋巴结肿大", "脖子有疙瘩", "耳后有疙瘩"},
	{"心慌", "心悸", "胸闷"},
	{"头晕", "眩晕", "头昏"},
	{"头疼", "头痛"},
	{"乏力", "疲倦", "无力", "没精神", "蔫"},

	// --- 喂养/护理 ---
	{"转奶", "换奶粉"},
	{"断奶", "戒奶", "离乳"},
	{"辅食", "添加辅食", "辅食添加"},
	{"热性惊厥", "高热惊厥", "烧抽了", "发烧抽搐"},
	{"幽门螺杆菌", "胃里有菌"},

	// --- 人群称呼 ---
	{"女婴", "婴儿", "宝宝", "新生儿", "婴幼儿", "小儿", "幼儿", "儿童"},
	{"男婴", "男宝", "男宝宝"},
}

// ---- External alias dictionary ----

var (
	aliasMu  sync.RWMutex
	aliasMap = map[string][]string{}
)

// LoadAliasFile loads an optional JSON dictionary mapping colloquial phrases
// to their standard terms and synonyms:
//
//	{"兔唇": ["唇腭裂", "唇裂", "腭裂"], "唇腭裂": ["兔唇", "唇裂"]}
//
// Import tools should emit both directions (alias→standard and
// standard→aliases) so a query containing either form expands to all forms.
// Keys should be ≥2 runes — single-character keys substring-match far too
// broadly. A missing file is not an error: the built-in synonymGroups still
// apply. Call once at startup before serving traffic.
func LoadAliasFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("alias map not present, using built-in synonyms only", "path", path)
			return nil
		}
		return fmt.Errorf("reading alias map: %w", err)
	}
	m := map[string][]string{}
	if err := json.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("parsing alias map %s: %w", path, err)
	}
	aliasMu.Lock()
	defer aliasMu.Unlock()
	aliasMap = m
	slog.Info("alias map loaded", "path", path, "entries", len(m))
	return nil
}

// ExpandQuery returns the query with synonyms of matched groups appended,
// so keyword retrieval can match clinical phrasings the user did not use.
// Two sources: the built-in synonymGroups and (if loaded) the external alias
// dictionary. The original query always comes first.
func ExpandQuery(query string) string {
	var extra []string
	seen := make(map[string]bool)
	add := func(terms ...string) {
		for _, t := range terms {
			if t != "" && !seen[t] {
				seen[t] = true
				extra = append(extra, t)
			}
		}
	}

	// Built-in groups: substring hit on any member appends all members.
	for _, group := range synonymGroups {
		hit := false
		for _, term := range group {
			if term != "" && strings.Contains(query, term) {
				hit = true
				break
			}
		}
		if hit {
			add(group...)
		}
	}

	// External dictionary: same substring rule, per-entry.
	aliasMu.RLock()
	defer aliasMu.RUnlock()
	for alias, terms := range aliasMap {
		if alias == "" || !strings.Contains(query, alias) {
			continue
		}
		add(alias)
		add(terms...)
	}

	if len(extra) == 0 {
		return query
	}
	return query + " " + strings.Join(extra, " ")
}
