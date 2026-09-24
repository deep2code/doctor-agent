package knowledge

import (
	"context"
	"testing"
)

// TestRetrieverPopSupplement3Recall guards the 2026-09-23 科普补充第三批
// (chronic-disease diet guidelines, safe medication, norovirus control):
// (also the hfmd/influenza/TB/HBV/kitchen-safety merge and the checkup-lab // reference-interval layer) everyday colloquial Chinese questions must recall // the new entries in top5.
func TestRetrieverPopSupplement3Recall(t *testing.T) {
	r := NewRetriever(newTestStore(t))
	cases := map[string]string{
		"痛风不能吃什么东西":       "cd-gout-01",
		"尿酸高要喝多少水":        "cd-gout-03",
		"痛风能喝酒吗":          "cd-gout-03",
		"高血压的人一天吃多少盐":     "cd-htn-01",
		"高血压适合用什么盐":       "cd-htn-01",
		"高血压要运动多久":        "cd-htn-03",
		"糖尿病一天吃多少主食":      "cd-dm-01",
		"糖尿病主食吃什么好":       "cd-dm-03",
		"糖尿病运动时低血糖怎么办":    "cd-dm-04",
		"糖尿病餐前还是餐后运动":    "cd-dm-04",
		"胆固醇高能吃鸡蛋吗":       "cd-lipid-02",
		"血脂高用什么油烧菜":       "cd-lipid-02",
		"营养标签怎么看":         "cd-lipid-05",
		"肾病不能吃蛋白质吗":       "cd-ckd-03",
		"肌酐高饮食要注意什么":      "cd-ckd-01",
		"肾病要限磷吗":          "cd-ckd-04",
		"肾病能喝多少水":         "cd-ckd-05",
		"感冒药可以一起吃吗":       "sm-duplicate-drug",
		"普通感冒需要吃消炎药吗":     "sm-cold-meds",
		"糖浆开封后还能放多久":     "sm-opened-liquid",
		"胶囊能掰开吃吗":         "sm-oral-admin",
		"阿奇霉素为什么要吃三天停四天": "sm-azithromycin",
		"吃头孢能喝酒吗":         "sm-disulfiram",
		"吃什么药不能喝酒":        "sm-alcohol-drugs",
		"鼻炎喷雾能长期用吗":       "sm-nasal-spray",
		"吃药身上起红疹是过敏吗":    "sm-allergy",
		"手术前要停药吗":         "sm-surgery-stop",
		"儿童退烧药怎么保存":       "sm-storage-kids",
		"家庭常备药怎么保存":       "sm-home-storage",
		"紧急避孕药有效率有多少":     "sm-contraception",
		"哺乳期感冒能吃药吗":       "sm-lactation",
		"不知道怀孕吃了药孩子能要吗":   "sm-pregnancy-exposure",
		"说明书不良反应很多可怕吗":    "sm-adr-label",
		"孩子在学校呕吐了老师怎么处理呕吐物": "ifs-noro-03",
		"诺如病毒要隔离几天":       "ifs-noro-04",
		"免洗洗手液可以代替洗手吗":    "ifs-noro-02",
		"84消毒液怎么配":        "ifs-noro-03",
		"诺如病毒一般几天能好":      "ifs-noro-01",
		"一个班好几个孩子上吐下泻要上报吗":  "ifs-noro-06",
		"做饭为什么要生熟分开":      "ifs-noro-05",
		// 第三批③补充：手足口/疱疹性咽峡炎、流感与禽流感、一氧化碳中毒、肺结核、乙肝防控、厨房食品安全
		// 第三批④：体检指标与临界值解读（血脂/血糖/尿酸/血常规/肝肾功能/电解质/甲功/CKD）
		"孩子手脚起疹子是手足口吗": "ifs-hfmd-01",
		"手足口病怎么传播": "ifs-hfmd-01",
		"疱疹性咽峡炎和手足口的区别": "ifs-hfmd-02",
		"孩子高烧喉咙痛不肯吃饭": "ifs-hfmd-02",
		"疹子退了能上学吗": "ifs-hfmd-03",
		"流感和感冒怎么区分": "ifs-flu-01",
		"家里有人得流感怎么不被传染": "ifs-flu-01",
		"禽肉没煮熟能吃吗": "ifs-flu-02",
		"鸡蛋半熟能吃吗": "ifs-flu-02",
		"冬天烧炭火锅会中毒吗": "ifs-env-01",
		"咳嗽超过两周要查什么": "ifs-tb-01",
		"痰中带血是肺结核吗": "ifs-tb-01",
		"肺结核要吃几个月药": "ifs-tb-02",
		"肺结核会传染家人吗": "ifs-tb-02",
		"乙肝疫苗打几针": "ifs-hbv-01",
		"乙肝疫苗漏打了怎么办": "ifs-hbv-01",
		"乙肝妈妈能母乳喂养吗": "ifs-hbv-02",
		"乙肝母婴阻断怎么做": "ifs-hbv-02",
		"入学体检乙肝": "ifs-hbv-03",
		"隔夜菜能吃吗": "ifs-food-01",
		"剩菜怎么保存": "ifs-food-01",
		"菜板发霉还能用吗": "ifs-food-02",
		"筷子发霉要换吗": "ifs-food-02",
		"霉变花生能吃吗": "ifs-food-03",
		"吃野生蘑菇中毒了怎么办": "ifs-food-04",
		"一家人都拉肚子是食物中毒吗": "ifs-food-04",
		"孩子的玩具要消毒吗": "ifs-hfmd-03",
		"体检血脂四项怎么看": "lab-lipid-01",
		"总胆固醇偏高要紧吗": "lab-lipid-01",
		"甘油三酯高怎么办": "lab-lipid-01",
		"低密度脂蛋白胆固醇降到多少": "lab-lipid-02",
		"放过支架血脂目标": "lab-lipid-02",
		"脂蛋白a偏高是什么意思": "lab-lipid-03",
		"空腹血糖7.2是糖尿病吗": "lab-glu-01",
		"糖化血红蛋白6.8": "lab-glu-01",
		"空腹血糖受损要吃药吗": "lab-glu-02",
		"空腹血糖6.5正常吗": "lab-glu-02",
		"尿酸460需要治疗吗": "lab-ua-01",
		"尿酸偏高要紧吗": "lab-ua-01",
		"尿酸高不痛要不要吃药": "lab-ua-02",
		"痛风什么时候开始降尿酸": "lab-ua-03",
		"痛风发作要不要停降尿酸药": "lab-ua-03",
		"血常规白细胞多少正常": "lab-cbc-01",
		"血红蛋白110是贫血吗": "lab-cbc-02",
		"女人血红蛋白正常值": "lab-cbc-02",
		"血小板偏低多少算危险": "lab-cbc-03",
		"孩子白细胞12是高吗": "lab-cbc-04",
		"宝宝血常规参考值": "lab-cbc-04",
		"转氨酶偏高说明什么": "lab-lft-01",
		"乙肝携带者转氨酶正常要治疗吗": "lab-lft-02",
		"肌酸激酶高是怎么回事": "lab-enzyme-01",
		"淀粉酶偏高": "lab-enzyme-01",
		"肌酐100正常吗": "lab-renal-01",
		"肌酐正常肾功能就没问题吗": "lab-renal-01",
		"慢性肾病分期怎么划分": "lab-ckd-01",
		"尿微量白蛋白30": "lab-ckd-01",
		"血钾正常范围": "lab-lyte-01",
		"血钾偏低要紧吗": "lab-lyte-01",
		"TSH5.2是甲减吗": "lab-thy-01",
		"促甲状腺激素偏高": "lab-thy-01",
		"亚临床甲减怎么诊断": "lab-thy-01",
		"备孕TSH要小于2.5": "lab-thy-02",
		"怀孕TSH多少正常": "lab-thy-02",
		"体检报告箭头多要紧吗": "lab-read-01",
		"体检抽血要空腹多久": "lab-read-01",
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
