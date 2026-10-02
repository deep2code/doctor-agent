package knowledge

import (
	"context"
	"testing"
)

// TestRetrieverBatch7DigestiveRecall guards the 2026-09-26 科普补充第七批(方向二):
// 消化与肠道 (胃食管反流与巴雷特随访、功能性便秘与泻剂选择、肠易激分型与低FODMAP、幽门螺杆菌检测根除与复查、
// 消化性溃疡与阿司匹林、脂肪肝减重限酒、肝硬化与静脉曲张随访、溃结与IBD监测、胰腺炎与上消化道出血红线)。
func TestRetrieverBatch7DigestiveRecall(t *testing.T) {
	r := NewRetriever(newTestStore(t))
	cases := map[string]string{
		// 反流症状与胃镜检查
		"烧心需要查胃镜吗": "dg-gerd-gastroscopy",
		"烧心反酸怎么确诊": "dg-gerd-gastroscopy",
		// 胃食管反流生活调理
		"胃食管反流怎么调理": "dg-gerd-lifestyle",
		"烧心晚上加重怎么办": "dg-gerd-lifestyle",
		// 反流病抑酸药疗程
		"反流性食管炎吃药几天": "dg-gerd-ppi",
		"PPI疗程是几周":   "dg-gerd-ppi",
		// 反流病维持治疗
		"PPI能长期吃吗":  "dg-gerd-maintenance",
		"轻度食管炎怎么维持": "dg-gerd-maintenance",
		// 食管炎治疗后复查胃镜
		"食管炎治好后要不要复查胃镜": "dg-gerd-recheck-gastroscopy",
		"重度食管炎随访":       "dg-gerd-recheck-gastroscopy",
		// 巴雷特食管
		"巴雷特食管严重吗":   "dg-barrett",
		"巴雷特食管会不会癌变": "dg-barrett",
		// 难治性胃食管反流
		"双倍剂量PPI还是烧心": "dg-gerd-refractory",
		"反流总是治不好怎么办":  "dg-gerd-refractory",
		// 反流性胸痛
		"胸痛是不是胃病引起的": "dg-gerd-chest-pain",
		"胸痛先查心脏还是胃":  "dg-gerd-chest-pain",
		// 反流引起的慢性咳嗽
		"慢性咳嗽是胃酸反流吗": "dg-gerd-cough",
		// 「哮喘合并反流」问的是合并症处理，归呼吸侧的哮喘合并症条目
		"哮喘合并反流": "ra-asthma-comorbidities",
		// 慢性便秘
		"便秘怎么算严重": "dg-constipation-definition",
		"老人便秘正常吗": "dg-constipation-definition",
		// 便秘警报征象
		"便秘什么情况要警惕":  "dg-constipation-alert",
		"便秘伴便血要查肠镜吗": "dg-constipation-alert",
		// 便秘膳食纤维与饮水
		"多吃菜还是不通便":  "dg-constipation-fiber",
		"便秘吃多少膳食纤维": "dg-constipation-fiber",
		// 便秘排便习惯训练
		"什么时候排便最好": "dg-constipation-habit",
		"排便习惯怎么建立": "dg-constipation-habit",
		// 渗透性泻剂聚乙二醇
		"聚乙二醇能长期吃吗":   "dg-laxative-osmotic",
		"乳果糖和聚乙二醇哪个好": "dg-laxative-osmotic",
		// 刺激性泻剂
		"番泻叶能长期喝吗": "dg-laxative-stimulant",
		"刺激性泻剂有哪些": "dg-laxative-stimulant",
		// 便秘生物反馈治疗
		"便秘要做几次生物反馈": "dg-biofeedback",
		"生物反馈治疗便秘":   "dg-biofeedback",
		// 肠易激综合征警报征象
		"腹泻夜间排便是癌吗": "dg-ibs-alert",
		"便血是肠易激吗":   "dg-ibs-alert",
		// 肠易激综合征的患病与病因
		"一紧张就拉肚子是不是肠易激": "dg-ibs-overview",
		"IBS会癌变吗":       "dg-ibs-overview",
		// 肠易激综合征分型
		"IBS-D是什么意思": "dg-ibs-subtype",
		"肠易激综合征腹泻型":  "dg-ibs-subtype",
		// 低FODMAP饮食
		"腹胀吃什么容易缓解": "dg-lowfodmap",
		"低FODMAP饮食": "dg-lowfodmap",
		// 肠易激综合征运动处方
		"运动能改善腹泻吗": "dg-ibs-exercise",
		"每周锻炼几次有效": "dg-ibs-exercise",
		// 肠易激腹痛解痉剂
		"肠易激腹痛吃什么药": "dg-ibs-antispasmodic",
		"匹维溴铵":      "dg-ibs-antispasmodic",
		// 幽门螺杆菌呼气试验
		"幽门螺杆菌怎么查":  "dg-hp-ubt",
		"吹气检查幽门螺杆菌": "dg-hp-ubt",
		// 幽门螺杆菌根除后复查
		"复查为什么要等几周":   "dg-hp-recheck",
		"幽门螺杆菌没根除怎么办": "dg-hp-recheck",
		// 幽门螺杆菌根除指征
		"哪些人要杀幽门螺杆菌":  "dg-hp-indication",
		"幽门螺杆菌会传染家人吗": "dg-hp-indication",
		// 幽门螺杆菌与胃癌
		"幽门螺杆菌会得胃癌吗":   "dg-hp-gastric-cancer",
		"萎缩性胃炎肠化生要随访吗": "dg-hp-gastric-cancer",
		// 铋剂四联方案
		"四联药要吃几天":   "dg-hp-regimen",
		"为什么必须吃满两周": "dg-hp-regimen",
		// 青霉素过敏根除方案
		"青霉素过敏能治幽门螺杆菌吗": "dg-hp-penicillin",
		"对阿莫西林过敏怎么根除":   "dg-hp-penicillin",
		// 脂肪肝饮酒量界限
		"脂肪肝能喝多少酒":    "dg-fatty-alcohol-limit",
		"非酒精性脂肪肝饮酒标准": "dg-fatty-alcohol-limit",
		// 脂肪肝筛查人群
		"转氨酶高要查脂肪肝吗": "dg-fatty-screen",
		"GGT升高说明什么":  "dg-fatty-screen",
		// 代谢综合征指标界值
		"血压130/85算高吗": "dg-fatty-metabolic",
		"代谢综合征标准":     "dg-fatty-metabolic",
		// 脂肪肝减重目标
		"肝纤维化能逆转吗": "dg-fatty-weight-loss",
		"脂肪肝要减多少斤": "dg-fatty-weight-loss",
		// 脂肪肝饮食热量控制
		"脂肪肝要戒含糖饮料吗":  "dg-fatty-diet-calorie",
		"脂肪肝每天少吃多少热量": "dg-fatty-diet-calorie",
		// 脂肪肝随访与慎用药物
		"脂肪肝会得肝癌吗":   "dg-fatty-followup",
		"脂肪肝需要筛查肝癌吗": "dg-fatty-followup",
		// 肝硬化肝癌监测（间隔问题的正解在按癌种给年龄与间隔的筛查层，
		// 本条目覆盖「超声+AFP」这一组合，故分别断言）
		"肝硬化多久查一次肝癌": "ces-liver-01",
		"肝硬化B超AFP监测": "dg-cirrhosis-hcc",
		// 肝硬化腹水限盐
		"肝硬化腹水门诊还是住院": "dg-ascites-salt",
		"腹水要控制喝水吗":    "dg-ascites-salt",
		// 肝硬化营养支持
		"肝硬化能吃蛋白质吗":  "dg-cirrhosis-nutrition",
		"肝硬化每天吃多少蛋白": "dg-cirrhosis-nutrition",
		// 食管胃底静脉曲张筛查
		"肝硬化为什么要做胃镜": "dg-varices-screen",
		"食管静脉曲张筛查":   "dg-varices-screen",
		// 静脉曲张出血二级预防
		"呕血后怎么预防再出血": "dg-varices-prevention",
		"腹水能用卡维地洛吗":  "dg-varices-prevention",
		// 溃疡性结肠炎就诊信号
		"黏液脓血便":   "dg-uc-symptoms",
		"便血伴里急后重": "dg-uc-symptoms",
		// 溃疡性结肠炎癌变监测
		"溃疡性结肠炎多久查一次肠镜": "dg-uc-colonoscopy",
		"IBD癌变监测":       "dg-uc-colonoscopy",
		// 溃疡性结肠炎维持治疗
		"缓解期还要吃药吗": "dg-uc-maintenance",
		"美沙拉嗪要吃多久": "dg-uc-maintenance",
		// 硫唑嘌呤用药监测
		"硫唑嘌呤要查血常规":  "dg-azathioprine",
		"硫嘌呤类药物骨髓抑制": "dg-azathioprine",
		// 急性胰腺炎病因
		"胰腺炎是什么原因引起的": "dg-pancreatitis-cause",
		"胆结石会引起胰腺炎吗":  "dg-pancreatitis-cause",
		// 急性胰腺炎疼痛识别
		"胰腺炎的疼是什么样的": "dg-pancreatitis-pain",
		"胰腺炎轻症几天能好":  "dg-pancreatitis-pain",
		// 胰腺炎预防（戒烟限酒控体重）
		"怎么预防胰腺炎":   "dg-pancreatitis-prevention",
		"戒烟能减少胰腺炎吗": "dg-pancreatitis-prevention",
		// 胰腺炎复发与恢复期饮食
		"胰腺炎会复发吗":   "dg-pancreatitis-recovery",
		"胰腺炎出院后怎么吃": "dg-pancreatitis-recovery",
		// 上消化道出血内镜时间窗
		"呕血黑便多久做胃镜":  "dg-bleeding-endoscopy",
		"上消化道出血急诊胃镜": "dg-bleeding-endoscopy",
		// 上消化道出血危重信号
		"消化道出血危险信号": "dg-bleeding-redflags",
		"晕厥黑便要抢救":   "dg-bleeding-redflags",
		// 消化道出血不典型表现
		"头晕乏力晕厥是出血吗": "dg-bleeding-atypical",
		"咖啡色呕吐物严重吗":  "dg-bleeding-atypical",
		// 消化道出血高危因素
		"消化道出血高危因素":   "dg-bleeding-risk-factors",
		"年龄大于六十岁出血危险": "dg-bleeding-risk-factors",
		// 溃疡出血后抑酸疗程
		"溃疡出血要复查胃镜吗":    "dg-bleeding-ppi-course",
		"胃溃疡出血后吃多久奥美拉唑": "dg-bleeding-ppi-course",
		// 出血后抗血小板药恢复
		"胃出血后阿司匹林还能吃吗": "dg-bleeding-antiplatelet",
		"支架后胃出血怎么办":    "dg-bleeding-antiplatelet",
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
