package knowledge

import (
	"context"
	"testing"
)

// TestRetrieverBatch7SportRecall guards the 2026-09-26 科普补充第七批(方向四):
// 运动损伤与颈肩腰膝康复 (腰椎间盘突出保守与手术红线、腰痛居家姿势腰围床垫、牵引与运动疗法、
// 颈椎病分型与踩棉花感红旗、枕高与伏案、骨关节炎减重运动与注射限制、前交叉韧带重返运动标准、
// 距骨骨软骨损伤阶梯治疗)。
func TestRetrieverBatch7SportRecall(t *testing.T) {
	r := NewRetriever(newTestStore(t))
	cases := map[string]string{
		// 腰椎间盘突出症保守治疗
		"腰突能保守治疗吗":     "si-ldh-conservative",
		"腰椎间盘突出一定要手术吗": "si-ldh-conservative",
		// 腰突保守治疗观察窗
		"腰突要保守治疗几周":  "si-ldh-conservative-course",
		"腰突什么时候考虑手术": "si-ldh-conservative-course",
		// 腰突急性期要不要卧床
		"腰椎间盘突出需要一直躺着吗": "si-ldh-bedrest",
		"腰痛卧床休息几天":      "si-ldh-bedrest",
		// 腰突日常姿势与负重
		"腰突能搬东西吗":      "si-ldh-posture",
		"腰椎间盘突出忌讳什么动作": "si-ldh-posture",
		// 腰突手术红旗信号
		"腰突什么情况必须手术": "si-ldh-surgery-redflags",
		"马尾神经受压什么症状": "si-ldh-surgery-redflags",
		// 居家腰痛何时转去医院
		"居家腰痛什么情况要去医院": "si-lowback-referral-signs",
		"腰痛剧烈难以入睡要紧吗":  "si-lowback-referral-signs",
		// 腰围怎么用
		"腰突要戴腰围吗":  "si-ldh-brace",
		"戴腰围肌肉萎缩吗": "si-ldh-brace",
		// 腰痛睡什么床
		"腰突睡硬板床吗":     "si-lowback-mattress",
		"腰椎间盘突出选什么床垫": "si-lowback-mattress",
		// 腰椎牵引
		"腰突牵引有用吗":   "si-ldh-traction",
		"牵引治疗腰痛靠谱吗": "si-ldh-traction",
		// 居家腰背训练怎么做
		"腰背肌锻炼每天做几次":  "si-lowback-training-dose",
		"腰痛训练每个动作做几次": "si-lowback-training-dose",
		// 久坐每小时起身
		"久坐腰疼怎么办":    "si-lowback-sitting-break",
		"站着工作久了对腰好吗": "si-lowback-sitting-break",
		// 蹲位搬重物姿势
		"弯腰搬东西伤腰吗":  "si-lowback-lifting",
		"怎么搬地上的重箱子": "si-lowback-lifting",
		// 腰痛适合哪些运动
		"腰痛能跑步吗":  "si-lowback-activity-choice",
		"腰突可以游泳吗": "si-lowback-activity-choice",
		// 腰痛拔罐
		"腰痛拔罐有用吗": "si-lowback-cupping",
		"拔罐留罐几分钟": "si-lowback-cupping",
		// 推拿按摩的禁忌
		"腰痛能按摩吗":  "si-lowback-massage-contraindications",
		"哪些人不能推拿": "si-lowback-massage-contraindications",
		// 腰痛有多常见、为什么反复
		"腰痛会复发吗":    "si-lowback-burden",
		"为什么腰痛老是反复": "si-lowback-burden",
		// 颈椎病大多不需手术
		"颈椎病要手术吗": "si-cervical-nonsurgical",
		"颈椎病能治好吗": "si-cervical-nonsurgical",
		// 颈椎病保守治疗疗程
		"颈椎病要治疗几周":     "si-cervical-conservative-course",
		"颈椎病3到4周无效怎么办": "si-cervical-conservative-course",
		// 脊髓型颈椎病
		"脊髓型颈椎病是什么": "si-cervical-myelopathy",
		"脊髓型颈椎病会瘫吗": "si-cervical-myelopathy",
		// 双腿发麻踩棉花感
		"脚踩棉花是什么病":  "si-cervical-cotton-feet",
		"走路不稳要查颈椎吗": "si-cervical-cotton-feet",
		// 颈椎病急性期休息
		"颈椎病发作要休息吗":  "si-cervical-acute-rest",
		"颈椎病急性发作怎么办": "si-cervical-acute-rest",
		// 枕头多高合适
		"高枕头有什么危害": "si-cervical-pillow-height",
		"颈椎病选什么枕头": "si-cervical-pillow-height",
		// 伏案一小时改变体位
		"长期低头怎么办":   "si-cervical-desk-break",
		"低头族颈椎怎么保护": "si-cervical-desk-break",
		// 颈部操与颈背肌锻炼
		"颈部操怎么做":   "si-cervical-neck-exercise",
		"颈椎病能转脖子吗": "si-cervical-neck-exercise",
		// 颈围与长途乘车
		"颈托有必要吗":  "si-cervical-collar-riding",
		"颈围能长期戴吗": "si-cervical-collar-riding",
		// 颈椎牵引的参数
		"牵引一个疗程几天": "si-cervical-traction-params",
		"家用牵引器安全吗": "si-cervical-traction-params",
		// 颈椎病什么时候该手术
		"颈椎病什么情况要手术": "si-cervical-surgery-timing",
		"脊髓型颈椎病要开刀吗": "si-cervical-surgery-timing",
		// 骨关节炎有多常见
		"膝盖退化正常吗":    "si-oa-prevalence",
		"女性更容易骨关节炎吗": "si-oa-prevalence",
		// 骨关节炎高危人群
		"什么人容易得骨关节炎": "si-oa-risk-groups",
		"膝盖受伤后会关节炎吗": "si-oa-risk-groups",
		// 骨关节炎的晨僵
		"怎么区分骨关节炎和类风湿": "si-oa-morning-stiffness",
		"关节僵硬几分钟正常":    "si-oa-morning-stiffness",
		// 膝盖要避免的动作
		"膝盖不好避免什么动作": "si-oa-avoid-load",
		"骨关节炎能跑步吗":   "si-oa-avoid-load",
		// 减重缓解膝关节痛
		"减肥能缓解膝盖痛吗": "si-oa-weight-loss",
		"骨关节炎要减重吗":  "si-oa-weight-loss",
		// 骨关节炎运动频率
		"骨关节炎每周锻炼几次": "si-oa-exercise-frequency",
		"关节不好怎么锻炼":   "si-oa-exercise-frequency",
		// 膝髋骨关节炎分别练什么
		"膝OA做什么运动":  "si-oa-exercise-type",
		"多关节OA怎么运动": "si-oa-exercise-type",
		// 手杖、助行器与鞋具
		"膝盖不好用手杖吗": "si-oa-aids-footwear",
		"手杖哪只手拿":   "si-oa-aids-footwear",
		// 膝关节OA一线用药
		"膝盖疼用什么药膏":   "si-oa-topical-nsaid",
		"外用药和口服药哪个好": "si-oa-topical-nsaid",
		// OA镇痛的阿片类限制
		"关节炎疼能用曲马多吗": "si-oa-opioid-limits",
		"关节痛能吃止疼片吗":  "si-oa-opioid-limits",
		// 关节腔激素注射的次数
		"关节打封闭一年几次": "si-oa-steroid-injection-limit",
		"封闭针一年最多几次": "si-oa-steroid-injection-limit",
		// 关节注射后测血糖
		"糖尿病能打封闭吗":  "si-oa-steroid-injection-glucose",
		"封闭针影响血糖几天": "si-oa-steroid-injection-glucose",
		// 玻璃酸钠关节注射
		"玻璃酸钠有用吗":   "si-oa-hyaluronan",
		"玻璃酸钠保护软骨吗": "si-oa-hyaluronan",
		// 前交叉韧带损伤有多常见
		"十字韧带断了能恢复运动吗": "si-acl-frequency",
		"前交叉韧带损伤概率":    "si-acl-frequency",
		// 膝盖打软腿
		"膝盖发软什么原因": "si-acl-giving-way",
		"膝盖打软腿":    "si-acl-giving-way",
		// ACL部分撕裂会不会进展
		"部分撕裂需要手术吗":   "si-acl-partial-tear",
		"ACL部分撕裂会加重吗": "si-acl-partial-tear",
		// 非手术后有多少人转手术
		"前交叉韧带不手术行吗": "si-acl-nonop-conversion",
		"韧带不手术会好吗":   "si-acl-nonop-conversion",
		// 术前康复目标
		"术前为什么要练腿":   "si-acl-prehab-targets",
		"为什么要等消肿再手术": "si-acl-prehab-targets",
		// ACL手术时机
		"等消肿再手术对吗": "si-acl-surgery-timing",
		"伤后几周手术合适": "si-acl-surgery-timing",
		// ACL术后活动度时间表
		"术后肿痛怎么消": "si-acl-rom-schedule",
		"康复要压腿吗":  "si-acl-rom-schedule",
		// 多久能重返运动
		"术后几个月复出":    "si-acl-return-time",
		"为什么不能按时间复出": "si-acl-return-time",
		// 复出前测试能降多少风险
		"ACL再损伤风险": "si-acl-testing-reinjury",
		"复出测试降低风险": "si-acl-testing-reinjury",
		// 重返运动的心理准备度
		"怕再次受伤不敢运动": "si-acl-psychological",
		"ACL心理评估":   "si-acl-psychological",
		// 距骨骨软骨损伤
		"距骨骨软骨损伤是什么": "si-olt-who",
		"OLT是什么病":    "si-olt-who",
		// OLT的阶梯治疗
		"OLT怎么治疗":  "si-olt-stepped-care",
		"先吃药还是先打针": "si-olt-stepped-care",
		// PRP注射的剂量与疗程
		"PRP注射几次": "si-olt-prp-schedule",
		"PRP间隔几周": "si-olt-prp-schedule",
		// PRP注射后的活动限制
		"注射后能运动吗":   "si-olt-prp-aftercare",
		"注射后几天不能锻炼": "si-olt-prp-aftercare",
		// 骨软骨修复术后复查MRI
		"骨软骨修复术后复查":   "si-olt-mri-followup",
		"踝关节术后多久查MRI": "si-olt-mri-followup",
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
