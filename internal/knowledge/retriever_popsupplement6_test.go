package knowledge

import (
	"context"
	"testing"
)

// TestRetrieverPopSupplement6Recall guards the 2026-09-25 科普补充第六批(方向A):
// tobacco / alcohol / caffeine everyday questions. Unlike earlier batches these
// entries use a SHORT topical condition_zh (「二手烟」「酒驾标准」) so they collect
// the containment bonus instead of needing three keyword hits; the gate keeps
// them honest by asserting plain colloquial phrasing lands in top5.
func TestRetrieverPopSupplement6Recall(t *testing.T) {
	r := NewRetriever(newTestStore(t))
	cases := map[string]string{
		// 吸烟危害与戒烟收益
		"抽烟对身体到底有什么伤害":       "ta-harm-overview",
		"吸烟会不会得肺癌":            "ta-harm-overview",
		"烟草里有多少致癌物":            "ta-harm-overview",
		"我国每年多少人死于吸烟":         "ta-harm-overview",
		"戒烟之后身体会慢慢恢复吗":         "ta-quit-timeline",
		"戒烟多久肺功能能好一点":          "ta-quit-timeline",
		"戒烟以后心脏病风险会下降吗":         "ta-quit-timeline",
		"年纪大了再戒烟还有意义吗":          "ta-quit-age",
		"六十岁戒烟还来得及吗":            "ta-quit-age",
		"戒烟能多活几年":               "ta-quit-age",
		"戒烟以后心里烦躁睡不着是怎么了":       "ta-withdrawal",
		"戒断反应一般持续多久":            "ta-withdrawal",
		"戒烟后特别想吃东西是不是正常":        "ta-withdrawal",
		"戒烟以后胖了十斤怎么办":           "ta-weight-gain",
		"戒烟后食欲特别好担心长肉":         "ta-weight-gain",
		"为什么戒烟的人容易发胖":           "ta-weight-gain",
		"戒烟长胖要不要一起控制饮食":         "ta-weight-gain",
		"有什么科学的办法能戒烟成功":         "ta-set-quit-day",
		"戒烟第一天应该做什么":            "ta-set-quit-day",
		"要不要挑一个日子开始戒烟":          "ta-set-quit-day",
		"尼古丁贴片有用吗":              "ta-nrt",
		"戒烟贴怎么用贴多久":             "ta-nrt",
		"尼古丁口香糖怎么嚼":             "ta-nrt",
		"尼古丁替代安全不安全":            "ta-nrt",
		"有没有吃药能帮助戒烟":            "ta-quit-drugs",
		"伐尼克兰是什么药":              "ta-quit-drugs",
		"安非他酮戒烟效果怎么样":           "ta-quit-drugs",
		"戒烟药有什么副作用":             "ta-quit-drugs",
		"想戒烟去医院挂什么科":            "ta-quit-services",
		"戒烟门诊在哪里找":              "ta-quit-services",
		"戒烟热线电话号码是多少":           "ta-quit-services",
		"老公英子在二手烟环境里要紧吗":        "ta-secondhand-smoke",
		"家里有人抽烟开窗通风还有害吗":        "ta-secondhand-smoke",
		"二手烟会致癌吗":               "ta-secondhand-smoke",
		"怀孕期间老公抽烟对孩子有影响吗":       "ta-secondhand-smoke",
		"抽完烟换了衣服再抱婴儿行不行":        "ta-thirdhand-smoke",
		"三手烟是什么意思":              "ta-thirdhand-smoke",
		"衣服沙发上的烟味多久散掉":          "ta-thirdhand-smoke",
		"办公室能不能设吸烟区":            "ta-smokefree-law",
		"餐厅包间允许抽烟吗":             "ta-smokefree-law",
		"高铁上抽烟会怎么处理":            "ta-smokefree-law",
		"楼道里总有人抽烟可以投诉吗":         "ta-smokefree-law",
		"电子烟到底有没有害":             "ta-ecig-harm",
		"电子烟里面含不含尼古丁":           "ta-ecig-harm",
		"抽电子烟会不会得肺癌":            "ta-ecig-harm",
		"电子烟比卷烟更安全吗":            "ta-ecig-harm",
		"靠抽电子烟戒烟靠谱吗":            "ta-ecig-quit",
		"能不能用电子烟替掉卷烟":           "ta-ecig-quit",
		"一边抽卷烟一边用电子烟行不行":        "ta-ecig-quit",
		"抽电子烟之后发烧喘不上气":           "ta-evali",
		"EVALI是什么病":            "ta-evali",
		"电子烟肺损伤和什么成分有关":          "ta-evali",
		"水果味电子烟为什么被禁":           "ta-ecig-law",
		"电子烟能不能卖给未成年人":           "ta-ecig-law",
		"卖电子烟要不要许可证":            "ta-ecig-law",
		"读初中的孩子抽烟了怎么干预":          "ta-nicotine-youth",
		"孩子偷偷抽电子烟被我发现":           "ta-nicotine-youth",
		"尼古丁对还在发育的大脑影响有多大":       "ta-nicotine-youth",
		"喝酒一天多少算不过量":             "ta-alcohol-limit",
		"膳食指南建议的饮酒上限是多少":         "ta-alcohol-limit",
		"哪些人一点酒都不该喝":            "ta-alcohol-limit",
		"女性每天酒精量不超过多少":           "ta-alcohol-limit",
		"酒是一类致癌物吗":               "ta-alcohol-cancer",
		"喝酒和食管癌有关系吗":            "ta-alcohol-cancer",
		"经常喝酒会得肝癌吗":             "ta-alcohol-cancer",
		"滴酒不沾真的更长寿吗":            "ta-alcohol-cancer",
		"怎么判断是不是喝酒喝成瘾了":          "ta-alcohol-dependence",
		"早上起来就想喝酒是酒精依赖吗":         "ta-alcohol-dependence",
		"酒量越来越大说明什么":            "ta-alcohol-dependence",
		"突然停酒后手抖出汗心慌":            "ta-alcohol-withdrawal",
		"戒酒之后失眠怎么办":             "ta-alcohol-withdrawal",
		"停酒后抽搐是怎么回事":            "ta-alcohol-withdrawal",
		// 裸「震颤谵妄是什么」按设计归默沙东谵妄/震颤专业页；本条断言问的是戒酒场景。
		"戒酒后出现震颤谵妄怎么办":       "ta-alcohol-withdrawal",
		"喝醉了人不清醒该怎么急救":           "ta-acute-alcohol",
		"醉酒呕吐会不会窒息":             "ta-acute-alcohol",
		"旁边人喝到昏睡要怎么处理":           "ta-acute-alcohol",
		"体检发现谷氨酰转肽酶高和喝酒有关吗":      "ta-alcohol-organ",
		"酒精性脂肪肝能恢复吗":            "ta-alcohol-organ",
		"喝酒会喝成肝硬化吗":             "ta-alcohol-organ",
		"转氨酶偏高是不是喝酒引起的":          "ta-alcohol-organ",
		"喝多少算酒驾":                "ta-drunk-driving",
		"饮酒驾驶和醉酒驾驶怎么区分":          "ta-drunk-driving",
		"血液酒精含量多少算醉驾":            "ta-drunk-driving",
		"一天喝几杯咖啡比较安全":            "ta-caffeine-safe",
		"咖啡因每天摄入量上限是多少":          "ta-caffeine-safe",
		"奶茶里的咖啡因高吗":             "ta-caffeine-safe",
		"咖啡喝多了会怎么样":             "ta-caffeine-safe",
		"下午喝咖啡晚上睡不着":            "ta-caffeine-sleep",
		"几点之后最好不要喝咖啡":           "ta-caffeine-sleep",
		"咖啡因代谢要多久":              "ta-caffeine-sleep",
		"喝茶也睡不着是什么原因":           "ta-caffeine-sleep",
		"不喝咖啡就头痛":               "ta-caffeine-headache",
		"戒咖啡头痛几天能好":             "ta-caffeine-headache",
		"头痛药吃得越多越痛是怎么回事":        "ta-caffeine-headache",
		"癌症有多少是可以预防的":           "ta-cancer-risk-factors",
		"不吸烟能少得多少种癌症":           "ta-cancer-risk-factors",
		"常见的致癌危险因素有哪些":          "ta-cancer-risk-factors",
	}
	for query, want := range cases {
		res, err := r.Retrieve(context.Background(), query, 5)
		if err != nil {
			t.Fatalf("检索失败: %v", err)
		}
		found := false
		for _, rr := range res {
			if rr.Entry.ID == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("查询 %q: 期望 top5 含 %s，实际 %v", query, want, entryIDs(res))
		}
	}
}
