package knowledge

import (
	"context"
	"testing"
)

// TestRetrieverBatch7GeriatricRecall guards the 2026-09-26 科普补充第七批(方向一):
// 老年大脑、心理与营养 (轻度认知障碍与痴呆识别、抑郁焦虑与睡眠、肌少症与蛋白摄入、骨质疏松与跌倒、
// 钙与维生素D、营养不良筛查、吞咽障碍与误吸、多重用药与维生素缺乏、慢病共病管理)。
// Condition names are topical phrases, so the gate asserts plain colloquial
// phrasing reaches top5 (same convention as 批次4/5/6)。
func TestRetrieverBatch7GeriatricRecall(t *testing.T) {
	r := NewRetriever(newTestStore(t))
	cases := map[string]string{
		// 轻度认知障碍
		"轻度认知障碍能恢复吗":   "gb-mci",
		"记忆力减退是不是痴呆前兆": "gb-mci",
		// 痴呆早期信号
		"老人痴呆早期有什么表现": "gb-early-signals",
		"痴呆早期信号":      "gb-early-signals",
		// 认知筛查量表
		"认知筛查量表": "gb-cognitive-screening",
		"MoCA":   "gb-cognitive-screening",
		// 认知障碍随访
		"轻度认知障碍要复查吗": "gb-mci-followup",
		"认知障碍随访":     "gb-mci-followup",
		// 认知训练
		"怎么预防老年痴呆":    "gb-mci-nonpharm",
		"老人多动脑能预防痴呆吗": "gb-mci-nonpharm",
		// 银杏叶制剂
		"银杏能预防痴呆吗":   "gb-ginkgo",
		"长期吃银杏叶片有害吗": "gb-ginkgo",
		// 太极八段锦
		"打太极对认知有好处吗": "gb-taiji",
		"老人练什么运动好":   "gb-taiji",
		// 老年人抗阻运动
		"老年人力量训练几次": "gb-resistance-training",
		"老人练肌肉安全吗":  "gb-resistance-training",
		// 内在能力
		"内在能力":  "gb-intrinsic-capacity",
		"ICOPE": "gb-intrinsic-capacity",
		// 起坐测试
		"起坐测试": "gb-chair-stand",
		"5次起坐": "gb-chair-stand",
		// 小腿围
		"老人小腿细是肌少症吗":   "gb-calf-circumference",
		"没有体脂秤怎么判断肌肉少": "gb-calf-circumference",
		// 握力
		"握力":  "gb-grip-strength",
		"握力计": "gb-grip-strength",
		// 肌少症
		"肌少症吃什么": "gb-sarcopenia",
		"肌少症怎么查": "gb-sarcopenia",
		// 衰弱
		"怎么判断老人衰弱": "gb-frailty",
		"衰弱":       "gb-frailty",
		// 跌倒自评
		"老人走路不稳怎么办": "gb-fall-screening",
		"跌倒风险怎么查":   "gb-fall-screening",
		// 非刻意体重下降
		"半年瘦了十斤正常吗": "gb-weight-loss-alert",
		"非刻意体重下降":   "gb-weight-loss-alert",
		// 视听力自测
		"老人耳朵背怎么办": "gb-sensory-check",
		"视听力自测":    "gb-sensory-check",
		// 老年多重用药
		"吃六种药安全吗": "gb-polypharmacy",
		"老年多重用药":  "gb-polypharmacy",
		// 老年营养不良
		"老年营养不良": "gb-malnutrition",
		"营养不良":   "gb-malnutrition",
		// 营养风险筛查
		"营养风险筛查": "gb-nutrition-screening",
		"NRS":    "gb-nutrition-screening",
		// 口服营养补充
		"营养补充剂怎么用":   "gb-ons",
		"吃饭不够要加营养粉吗": "gb-ons",
		// 老年人蛋白质
		"老年人蛋白质": "gb-protein",
		"蛋白质摄入":  "gb-protein",
		// 老年人运动量
		"怎么判断中等强度": "gb-exercise-volume",
		"老年人运动量":   "gb-exercise-volume",
		// 平衡训练
		"平衡训练":   "gb-balance-training",
		"平衡能力练习": "gb-balance-training",
		// 老年抑郁
		"老年抑郁怎么发现": "gb-depression-recognize",
		"老年抑郁":     "gb-depression-recognize",
		// 老年抑郁用药
		"老年抑郁用药":   "gb-depression-treatment",
		"抗抑郁药起效时间": "gb-depression-treatment",
		// 痴呆精神症状用药
		"痴呆精神症状用药": "gb-antipsychotic-dementia",
		"老年痴呆狂躁用药": "gb-antipsychotic-dementia",
		// 直立性低血压
		"直立性低血压": "gb-orthostatic-hypotension",
		"体位性低血压": "gb-orthostatic-hypotension",
		// 骨质疏松筛查
		"骨质疏松怎么诊断": "gb-osteoporosis-screening",
		"骨质疏松筛查":   "gb-osteoporosis-screening",
		// 骨折后再骨折
		"骨折后再骨折": "gb-refracture",
		"再次骨折风险": "gb-refracture",
		// 骨质疏松补钙
		"骨密度低吃什么": "gb-calcium-diet",
		"骨质疏松补钙":  "gb-calcium-diet",
		// 维生素D补充
		"维生素D补充":  "gb-vitamin-d-dosing",
		"维生素D3剂量": "gb-vitamin-d-dosing",
		// 阿仑膦酸钠
		"阿仑膦酸钠怎么吃": "gb-alendronate",
		"吃药后能吃饭吗":  "gb-alendronate",
		// 肺炎疫苗
		"肺炎疫苗":   "gb-pneumococcal-vaccine",
		"肺炎球菌疫苗": "gb-pneumococcal-vaccine",
		// 老年人能力评估
		"老年人能力评估": "gb-elderly-ability-assessment",
		"能力评估指标":  "gb-elderly-ability-assessment",
		// 老年居家护理服务
		"行动不便老人怎么就医": "gb-home-nursing-services",
		"老年居家护理服务":   "gb-home-nursing-services",
	}

	ctx := context.Background()
	for query, want := range cases {
		res, err := r.Retrieve(ctx, query, 5)
		if err != nil {
			t.Fatalf("查询 %q 检索失败: %v", query, err)
		}
		found := false
		for _, item := range res {
			if item.Entry.ID == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("查询 %q: 期望 top5 含 %s，实际 %v", query, want, entryIDs(res))
		}
	}
}
