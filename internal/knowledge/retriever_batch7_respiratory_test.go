package knowledge

import (
	"context"
	"testing"
)

// TestRetrieverBatch7RespiratoryRecall guards the 2026-09-26 科普补充第七批(方向三):
// 哮喘与慢阻肺 (控制水平自评与ACT/峰流速、吸入装置与雾化安全、急性发作家庭处置与就医红线、缓解期降阶与疗程、
// 慢阻肺分级评估与随访、家庭氧疗与肺康复、戒烟与疫苗、儿童哮喘环境与共病、咳嗽变异性哮喘、支扩与肺结节)。
func TestRetrieverBatch7RespiratoryRecall(t *testing.T) {
	r := NewRetriever(newTestStore(t))
	cases := map[string]string{
		// 哮喘控制水平自查
		"急救喷剂用几次":  "ra-asthma-control-check",
		"哮喘控制得好不好": "ra-asthma-control-check",
		// ACT哮喘控制测试
		"ACT19分什么意思": "ra-act-score",
		"ACT20分代表什么": "ra-act-score",
		// 峰流速仪居家监测
		"峰流速仪怎么用":   "ra-peak-flow",
		"峰流速下降说明什么": "ra-peak-flow",
		// 哮喘诊断的肺功能标准
		"哮喘怎么确诊":    "ra-asthma-diagnosis",
		"可变气流受限怎么查": "ra-asthma-diagnosis",
		// 哮喘病情严重程度分级
		"哮喘严重度分级": "ra-asthma-severity",
		"间歇状态哮喘":  "ra-asthma-severity",
		// FeNO呼出气一氧化氮
		"FeNO是什么检查": "ra-feno",
		"呼出气一氧化氮":   "ra-feno",
		// 哮喘需每天规律用控制药
		"哮喘要每天用药吗":  "ra-asthma-controller-daily",
		"控制性药物和缓解药": "ra-asthma-controller-daily",
		// 哮喘日记自我管理
		"哮喘日记写什么": "ra-asthma-diary",
		"哮喘日记":    "ra-asthma-diary",
		// 哮喘常见合并症
		"哮喘合并过敏性鼻炎": "ra-asthma-comorbidities",
		"哮喘合并胃食管反流": "ra-asthma-comorbidities",
		// 吸入激素后清水漱口
		"喷完药要漱口吗": "ra-ics-rinse-mouth",
		"吸入激素后漱口": "ra-ics-rinse-mouth",
		// 哮喘缓解药沙丁胺醇
		"只用万托林行吗":  "ra-saba-reliever",
		"SABA每月几支": "ra-saba-reliever",
		// 长效支气管扩张剂不可单用
		"福莫特罗单用":    "ra-laba-alone-risk",
		"沙美特罗不配合激素": "ra-laba-alone-risk",
		// 吸入装置正确用法
		"吸入器用不对": "ra-inhaler-technique",
		"吸入装置误用": "ra-inhaler-technique",
		// 家用雾化器类型选择
		"家用雾化器怎么选":  "ra-nebulizer-device",
		"超声雾化器能不能用": "ra-nebulizer-device",
		// 家庭雾化操作规范
		"雾化前能吃饭吗": "ra-nebulization-procedure",
		"雾化前要洗脸吗": "ra-nebulization-procedure",
		// 哮喘急性发作家庭处理
		"哮喘发作在家怎么办":   "ra-asthma-attack-home",
		"哮喘发作几天不好要就医": "ra-asthma-attack-home",
		// 哮喘发作急救喷剂用量
		"沙丁胺醇喷几次": "ra-asthma-attack-saba-dose",
		"一次吸几喷":   "ra-asthma-attack-saba-dose",
		// 异丙托溴铵联合急救
		"异丙托溴铵哮喘": "ra-asthma-attack-ipratropium",
		"爱全乐雾化剂量": "ra-asthma-attack-ipratropium",
		// 布地奈德福莫特罗加量缓解
		"信必可加用几吸":    "ra-asthma-attack-formoterol",
		"布地奈德福莫特罗缓解": "ra-asthma-attack-formoterol",
		// 哮喘发作期激素用量
		"哮喘发作吸入激素加倍": "ra-asthma-attack-steroid",
		"布地奈德1600":   "ra-asthma-attack-steroid",
		// 哮喘发作家庭氧疗目标
		"哮喘吸氧浓度":  "ra-asthma-home-oxygen",
		"血氧饱和度93": "ra-asthma-home-oxygen",
		// 哮喘危重信号
		"重度哮喘发作表现": "ra-asthma-severe-signs",
		"端坐呼吸":     "ra-asthma-severe-signs",
		// 儿童哮喘发作先兆处理
		"流涕打喷嚏是发作前": "ra-child-asthma-warning",
		"孩子哮喘先兆":    "ra-child-asthma-warning",
		// 哮喘降级治疗时机与顺序
		"哮喘药什么时候减": "ra-asthma-stepdown",
		"哮喘能停药吗":   "ra-asthma-stepdown",
		// 哮喘减药速度与不宜降级人群
		"每三个月减百分之二十五": "ra-asthma-stepdown-rate",
		"ICS减量速度":     "ra-asthma-stepdown-rate",
		// 咳嗽变异性哮喘
		"只咳不喘是哮喘吗":    "ra-cva-cough-variant",
		"咳嗽变异型哮喘要查什么": "ra-cva-cough-variant",
		// 咳嗽按病程分类
		"咳嗽几周要查": "ra-cough-duration",
		"急性咳嗽多久": "ra-cough-duration",
		// 不典型哮喘识别
		"不典型哮喘":   "ra-atypical-asthma",
		"胸闷变异性哮喘": "ra-atypical-asthma",
		// 慢阻肺肺功能诊断界值
		"慢阻肺怎么确诊":      "ra-copd-diagnosis",
		"FEV1/FVC低于70": "ra-copd-diagnosis",
		// 四十岁以上肺功能筛查
		"肺功能筛查年龄":   "ra-lung-function-screen",
		"四十岁以上查肺功能": "ra-lung-function-screen",
		// 慢阻肺症状自评量表
		"症状多还是加重风险高": "ra-copd-symptom-scores",
		"mMRC呼吸困难分级": "ra-copd-symptom-scores",
		// 慢阻肺急性加重高风险
		"频繁加重怎么定义": "ra-copd-exacerbation-risk",
		"慢阻肺频繁加重":  "ra-copd-exacerbation-risk",
		// 慢阻肺常见症状与高危人群
		"慢阻肺有什么症状":  "ra-copd-symptoms-risk",
		"二手烟会得慢阻肺吗": "ra-copd-symptoms-risk",
		// 血氧低于92就医
		"血氧92危险吗":   "ra-copd-sp02",
		"什么时候查动脉血气": "ra-copd-sp02",
		// 缩唇腹式呼吸与节能技巧
		"缩唇呼吸怎么做": "ra-copd-breathing-technique",
		"气短怎么省力":  "ra-copd-breathing-technique",
		// 慢阻肺加重出院后随访
		"慢阻肺住院后复查": "ra-copd-discharge-followup",
		"出院一到四周随访": "ra-copd-discharge-followup",
		// 长期家庭氧疗
		"家庭氧疗条件":   "ra-home-oxygen-therapy",
		"长期氧疗每天多久": "ra-home-oxygen-therapy",
		// 呼吸康复疗程与内容
		"肺康复怎么做":   "ra-pulmonary-rehab",
		"呼吸康复至少几周": "ra-pulmonary-rehab",
		// 慢阻肺每年一次肺功能
		"慢阻肺多久查一次肺功能": "ra-spirometry-yearly",
		"每年一次肺量计":     "ra-spirometry-yearly",
		// 戒烟求助渠道与随访
		"戒烟热线号码":     "ra-quit-smoking-service",
		"4008085531": "ra-quit-smoking-service",
		// 一线戒烟药物
		"戒烟药哪种非处方":  "ra-quit-smoking-drugs",
		"戒烟药哪些能自己买": "ra-quit-smoking-drugs",
		// 慢阻肺流感与肺炎球菌疫苗
		"慢阻肺打流感疫苗": "ra-copd-vaccine-flu-pcv",
		"每年接种流感疫苗": "ra-copd-vaccine-flu-pcv",
		// 慢阻肺加种疫苗清单
		"慢阻肺打带状疱疹疫苗": "ra-copd-vaccine-others",
		"RSV疫苗60岁":   "ra-copd-vaccine-others",
		// 支气管扩张症预防
		"支气管扩张疫苗":    "ra-bronchiectasis-vaccine",
		"肺炎疫苗减少急性加重": "ra-bronchiectasis-vaccine",
		// 哮喘发病危险因素
		"为什么会得哮喘":    "ra-asthma-risk-factors",
		"养宠物孩子容易哮喘吗": "ra-asthma-risk-factors",
		// 尘螨过敏与哮喘
		"除螨对哮喘有用吗": "ra-dust-mite-asthma",
		"尘螨过敏哮喘":   "ra-dust-mite-asthma",
		// 哮喘患者能不能运动
		"哮喘能跑步吗":  "ra-asthma-exercise",
		"运动性哮喘预防": "ra-asthma-exercise",
		// 睡眠呼吸暂停自查
		"打呼噜憋醒是什么病": "ra-osa-snoring",
		"睡眠呼吸暂停诊断":  "ra-osa-snoring",
		// 肺结节尺寸分层管理
		"体检肺结节怎么办": "ra-lung-nodule-size",
		"微小结节5毫米":  "ra-lung-nodule-size",
		// 慢阻肺急性加重识别
		"痰液黏度改变是加重": "ra-copd-exacerbation-signs",
		"慢阻肺急性加重表现": "ra-copd-exacerbation-signs",
		// 支气管扩张排痰与咯血
		"痰多怎么排":      "ra-bronchiectasis-care",
		"体位引流什么时候禁忌": "ra-bronchiectasis-care",
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
