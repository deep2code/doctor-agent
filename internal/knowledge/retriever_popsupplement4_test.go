package knowledge

import (
	"context"
	"testing"
)

// TestRetrieverPopSupplement4Recall guards the 2026-09-24 科普补充第四批:
// elderly polypharmacy & fall prevention, symptom triage (which department /
// why emergency waits), women's life-course health, and home self-monitoring
// with chronic-disease follow-up. Everyday colloquial Chinese questions must
// recall the new entries in top5.
func TestRetrieverPopSupplement4Recall(t *testing.T) {
	r := NewRetriever(newTestStore(t))
	cases := map[string]string{
		// 第四批④：老年多重用药与跌倒预防
		"老人摔跤是致死原因吗":     "ef-fall-burden",
		"老人为什么容易跌倒":      "ef-fall-why",
		"练什么运动能防止跌倒":     "ef-fall-exercise",
		"老人要不要用手杖":        "ef-fall-aids",
		"手杖多长合适":          "ef-fall-aids",
		"家里怎么改造防跌倒":      "ef-fall-home",
		"老人出门注意什么防摔":     "ef-fall-outdoor",
		"老人摔倒了怎么自己起来":    "ef-fall-after",
		"看到老人摔倒要不要扶":     "ef-fall-after",
		"骨质疏松补钙会不会便秘":    "ef-calcium-how",
		"钙片什么时候吃最好":      "ef-calcium-how",
		"晒太阳补钙有什么误区":     "ef-calcium-how",
		"每天要补多少维生素D":     "ef-calcium-dose",
		"老人一次吃五种药要紧吗":    "ef-poly-what",
		"什么是多重用药":        "ef-poly-what",
		"医生越开药越多怎么办":     "ef-poly-waterfall",
		"长期吃安定有什么危害":     "ef-poly-sedative",
		"老人吃降压药头晕摔倒":     "ef-poly-cardiac",
		"老人吃降糖药低血糖摔倒":     "ef-poly-glucose",
		"他汀和克拉霉素能一起吃吗":    "ef-poly-statin",
		"长期吃奥美拉唑会骨折吗":    "ef-poly-ppi",
		"优甲乐能和钙片一起吃吗":    "ef-poly-thyroxine",
		"甘草片能和降压药一起吃吗":   "ef-poly-cough",
		"输注庆大霉素会耳聋吗":     "ef-poly-anti-infective",
		"左氧氟沙星能和钙片一起吃吗":  "ef-poly-anti-infective",
		"止痛药能和安定一起吃吗":    "ef-poly-pain",
		"老人骨质疏松怎么自测":     "ef-osteo-selfcheck",
		"骨质疏松是什么病":       "ef-osteo-what",
		"照护老人怎么防跌倒":      "ef-fall-caregiver",
		"跌倒是老年人最常见的伤害吗":  "ef-fall-burden",
		"防跌倒要防治骨质疏松":     "ef-fall-bone",
		// 黏液性水肿表现（全身肿+怕冷）由既有甲减条目回答，非本批新增条目
		"老人全身肿怕冷要查什么":    "hypothyroid-001",
		// 第四批①：常见症状就医分诊
		"急诊为什么要等那么久":      "st-triage-wait",
		"急诊分诊分几级":        "st-triage-levels",
		"急诊红色橙色黄色代表什么":   "st-triage-levels",
		"什么情况下要打120":     "st-call-120",
		"心肺复苏怎么做":        "st-cpr-aed",
		"除颤仪AED怎么用":      "st-cpr-aed",
		"中风怎么快速识别":       "st-stroke-120",
		"嘴角歪斜手无力":        "st-stroke-120",
		"中风风险自测":         "st-stroke-risk",
		"孩子手脚起疹子发烧要马上医院吗": "st-child-hfmd",
		"手足口什么时候要住院":     "st-child-hfmd",
		"误服农药要不要催吐":      "st-poison",
		"头疼挂什么科":         "st-dept-head",
		"眼睛疼伴有头疼挂哪科":    "st-dept-head",
		"牙疼伴下颌发紧是什么问题":   "st-dept-head",
		"胸痛挂什么科":         "st-dept-chest",
		"咳嗽挂哪个科":         "st-dept-chest",
		"肚子痛挂什么科":        "st-dept-abdomen",
		"腹痛伴腹泻看什么科":      "st-dept-abdomen",
		"腰痛挂什么科":         "st-dept-limb",
		"关节痛挂哪个科":        "st-dept-limb",
		"尿血挂什么科":         "st-dept-excretion",
		"便血挂什么科":         "st-dept-excretion",
		"小孩看病挂什么科":       "st-dept-child-women",
		"怀孕出血挂哪个科":       "st-dept-child-women",
		"第一次看病要准备什么":     "st-visit-tips",
		"第一次看病要不要挂专家号":   "st-visit-tips",
		"发烧伴咳嗽挂什么科":      "st-dept-chest",
		"浮肿挂什么科":         "st-dept-excretion",
		"不知道挂什么科怎么办":     "st-visit-tips",
		// 第四批②：女性全周期健康
		"月经多少天算正常":       "wh-period-normal",
		"月经量多少算正常":       "wh-period-normal",
		"月经推迟三个月正常吗":     "wh-period-abnormal",
		"一个月月经不干净":       "wh-period-abnormal",
		"痛经吃什么止疼药":       "wh-dysmenorrhea",
		"痛经会不会越来越严重":     "wh-dysmenorrhea",
		"怎么知道自己是更年期了":    "wh-menopause-signs",
		"更年期激素治疗安全吗":     "wh-mht",
		"更年期能吃雌激素吗":      "wh-mht",
		"更年期怎么调理饮食运动":    "wh-menopause-life",
		"宫颈癌筛查多久一次":      "wh-cervical-screen",
		"打了HPV疫苗还要筛查吗":   "wh-cervical-special",
		"65岁还要查宫颈癌吗":     "wh-cervical-screen",
		"乳腺癌什么时候开始筛查":    "wh-breast-screen",
		"乳腺致密型做什么检查":     "wh-breast-screen",
		"BRCA基因检测谁要做":    "wh-breast-highrisk",
		"叶酸一天吃多少毫克":      "wh-folate-dose",
		"怀过神经管缺陷孩子再备孕叶酸量": "wh-folate-dose",
		"不同年龄怎么补钙护骨":     "wh-bone-lifecycle",
		"宫颈癌筛查率2030目标":   "wh-cervical-plan",
		// 第四批③：居家自测与慢病随访
		"血压计买哪种好":         "hm-bp-device",
		"腕式血压计准吗":         "hm-bp-device",
		"血压计袖带多大合适":      "hm-bp-device",
		"量血压前休息多久":        "hm-bp-posture",
		"在家怎么量血压":         "hm-bp-posture",
		"量血压要不要说话":        "hm-bp-posture",
		"血压一天测几次":         "hm-bp-schedule",
		"晚上什么时候测血压":       "hm-bp-schedule",
		"就诊前要测几天血压":       "hm-bp-schedule",
		"在家测血压多少算高":       "hm-bp-standard",
		"家庭血压135/85":      "hm-bp-standard",
		"血压130算正常吗":       "hm-bp-standard",
		"一进医院血压就高":        "hm-bp-whitecoat",
		"在家血压高在医院正常":      "hm-bp-whitecoat",
		"血压180要紧吗":        "hm-bp-danger",
		"高血压伴剧烈头痛怎么办":     "hm-bp-danger",
		"高血压多久复查一次":       "hm-bp-followup",
		"高血压每年要查什么":       "hm-bp-followup",
		"舌下含服硝苯地平行吗":      "hm-bp-danger",
		"血糖仪准不准":          "hm-glucose-device",
		"血糖试纸过期还能用吗":      "hm-glucose-device",
		"测血糖用酒精还是碘伏":      "hm-glucose-how",
		"测血糖第一滴血要不要":      "hm-glucose-how",
		"手指水肿能测血糖吗":       "hm-glucose-how",
		"餐后两小时血糖从什么时候算":    "hm-glucose-timing",
		"糖化血红蛋白代表几个月":     "hm-glucose-timing",
		"蚕豆病能查糖化血红蛋白吗":    "hm-glucose-timing",
		"做糖耐量试验前怎么吃":      "hm-glucose-timing",
		"糖尿病血糖控制目标":       "hm-dm-target",
		"糖化7点几算达标吗":       "hm-dm-target",
		"糖尿病血压控制多少":       "hm-dm-target",
		"心慌手抖出冷汗是低血糖吗":    "hm-dm-hypoglycemia",
		"低血糖吃多少克糖":        "hm-dm-hypoglycemia",
		"夜间低血糖发现不了":       "hm-dm-hypoglycemia",
		"空腹喝酒会低血糖吗":       "hm-dm-hypoglycemia",
		"我需要查血糖吗":         "hm-dm-screen",
		"糖尿病高危人群":         "hm-dm-screen",
		"父母有糖尿病我要筛查吗":     "hm-dm-screen",
		"空腹血糖6.5正常吗":      "hm-dm-prediabetes",
		"糖尿病前期能恢复吗":       "hm-dm-prediabetes",
		"糖尿病前期多久复查一次":     "hm-dm-prediabetes",
		"脚趾发黑要去医院吗":       "hm-dm-foot",
		"糖尿病为什么要查脚":       "hm-dm-foot",
		// 运动能不能做属血糖控制目标条目的运动禁忌段，非随访节奏条目
		"糖尿病能运动吗":         "hm-dm-target",
		"血糖高还能锻炼吗":        "hm-dm-target",
		"糖尿病多久随访一次":       "hm-dm-followup",
		// 造影前暂停用二甲双胍的口径写在老年降糖药条目里，随访节奏条目只说「减停由医生决定」
		"二甲双胍做增强CT要停吗":    "ef-poly-glucose",
		"BMI多少算胖":         "hm-bmi-waist",
		"腰围多少算超标":         "hm-bmi-waist",
		"怎么量腰围":           "hm-measure-how",
		"什么时候称体重最准":       "hm-measure-how",
		"运动员能用BMI判断胖瘦吗":   "hm-bmi-waist",
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
