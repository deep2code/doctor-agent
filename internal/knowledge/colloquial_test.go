package knowledge

import (
	"strings"
	"testing"
)

// TestColloquialQueriesFromRealPatients 验证真实患者提问的同义词扩展
// 数据来源: medical_dialogues.json 中的患者提问
func TestColloquialQueriesFromRealPatients(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		requiredWords []string // 扩展后必须包含的关键词（至少匹配一个）
	}{
		{
			name:          "喉咙痛-扁桃体发炎",
			query:         "孩子扁桃体发炎了总发烧可以多吃什么",
			requiredWords: []string{"扁桃体", "发烧", "发热", "咽炎"},
		},
		{
			name:          "嗓子疼",
			query:         "说嗓子喝水都觉得疼",
			requiredWords: []string{"嗓子", "咽痛", "喉咙", "咽喉"},
		},
		{
			name:          "磨牙",
			query:         "我为何睡觉磨牙",
			requiredWords: []string{"磨牙", "夜磨牙", "bruxism", "teeth grinding"},
		},
		{
			name:          "咳嗽",
			query:         "感冒引起的咳嗽严重",
			requiredWords: []string{"咳嗽", "咳痰", "干咳"},
		},
		{
			name:          "鼻塞",
			query:         "感冒，睡觉鼻孔堵塞，睡不着怎么办",
			requiredWords: []string{"鼻塞", "鼻孔", "流鼻涕", "鼻炎"},
		},
		{
			name:          "尿频尿痛",
			query:         "两三分钟上次厕所，尿不多尿的时候还痛",
			requiredWords: []string{"尿频", "尿痛", "尿路", "尿"},
		},
		{
			name:          "头痛",
			query:         "右边头痛，摸头发也有疼痛感",
			requiredWords: []string{"头痛", "头疼"},
		},
		{
			name:          "发烧",
			query:         "孩子发烧39度怎么办",
			requiredWords: []string{"发烧", "发热", "体温", "高烧"},
		},
		{
			name:          "拉肚子",
			query:         "宝宝拉肚子三天了",
			requiredWords: []string{"拉肚子", "腹泻", "拉稀"},
		},
		{
			name:          "肚子疼",
			query:         "小孩肚子疼怎么回事",
			requiredWords: []string{"肚子疼", "腹痛", "肚子痛"},
		},
		{
			name:          "流鼻涕",
			query:         "宝宝一直流鼻涕",
			requiredWords: []string{"流鼻涕", "鼻塞", "鼻炎"},
		},
		{
			name:          "呕吐",
			query:         "孩子吃完奶就吐",
			requiredWords: []string{"呕吐", "吐奶", "溢奶", "吐"},
		},
		{
			name:          "便秘",
			query:         "宝宝便秘怎么办",
			requiredWords: []string{"便秘", "排便困难"},
		},
		{
			name:          "皮疹",
			query:         "孩子身上出疹子了",
			requiredWords: []string{"疹子", "皮疹", "红疹"},
		},
		{
			name:          "黄疸",
			query:         "新生儿黄疸怎么处理",
			requiredWords: []string{"黄疸", "新生儿黄疸"},
		},
		{
			name:          "腹泻",
			query:         "宝宝腹泻怎么办",
			requiredWords: []string{"腹泻", "拉肚子", "拉稀"},
		},
		{
			name:          "鼻涕",
			query:         "孩子流鼻涕",
			requiredWords: []string{"流鼻涕", "鼻涕", "鼻塞"},
		},
		{
			name:          "肚子胀",
			query:         "宝宝肚子胀气",
			requiredWords: []string{"腹胀", "胀气", "肚子胀"},
		},
		{
			name:          "食欲不振",
			query:         "孩子不想吃饭",
			requiredWords: []string{"没胃口", "食欲不振", "食欲差"},
		},
		{
			name:          "湿疹",
			query:         "宝宝湿疹怎么办",
			requiredWords: []string{"湿疹", "奶疹", "特应性皮炎"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expanded := ExpandQuery(tt.query)

			// 检查必需关键词（至少匹配一个）
			matched := false
			for _, w := range tt.requiredWords {
				if strings.Contains(expanded, w) {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("查询 %q 扩展后未匹配任何必需关键词: %v\n扩展结果: %q",
					tt.query, tt.requiredWords, expanded)
			}
		})
	}
}