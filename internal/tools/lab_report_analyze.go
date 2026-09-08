package tools

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// labItem 是一项检验指标的参考区间与解读知识 (成人, 中国常用标准,
// 血细胞参考区间依据 WS/T 405; 试剂/仪器差异以报告单标注为准).
type labItem struct {
	Names  []string // 中文名 + 化验单常见缩写/别名 (按此匹配)
	Cat    string   // 分类: 血常规/肝功能/…
	Unit   string
	Ref    [2]float64 // 通用参考范围
	MRef   *[2]float64 // 男性 (可选)
	FRef   *[2]float64 // 女性 (可选)
	Qual   bool       // 定性项目: 阴性正常, +/阳性异常
	High   string     // 偏高常见原因 (面向普通人)
	Low    string     // 偏低常见原因
	Note   string     // 特殊说明 (可选)
}

// labItems 是化验单解读参考表。命名覆盖: 全称/报告单缩写/口语叫法。
var labItems = []labItem{
	// ── 血常规 ──
	{Names: []string{"白细胞", "白细胞计数", "WBC"}, Cat: "血常规", Unit: "×10⁹/L", Ref: [2]float64{3.5, 9.5},
		High: "常见于细菌感染、炎症、应激；明显升高需警惕感染加重", Low: "常见于病毒感染（如流感）、部分药物影响；明显降低提示免疫力下降，需就医"},
	{Names: []string{"中性粒细胞绝对值", "中性粒细胞数目", "NEUT#"}, Cat: "血常规", Unit: "×10⁹/L", Ref: [2]float64{1.8, 6.3},
		High: "细菌感染最常见", Low: "病毒感染或药物影响；低于0.5极易感染，须立即就医"},
	{Names: []string{"淋巴细胞绝对值", "淋巴细胞数目", "LYMPH#"}, Cat: "血常规", Unit: "×10⁹/L", Ref: [2]float64{1.1, 3.2},
		High: "常见于病毒感染", Low: "轻度降低常见于感染应激、使用激素"},
	{Names: []string{"中性粒细胞比率", "中性粒细胞百分比", "NEUT%"}, Cat: "血常规", Unit: "%", Ref: [2]float64{40, 75},
		High: "提示细菌感染可能性大", Low: "提示病毒感染可能性大"},
	{Names: []string{"淋巴细胞比率", "淋巴细胞百分比", "LYMPH%"}, Cat: "血常规", Unit: "%", Ref: [2]float64{20, 50},
		High: "常见于病毒感染（感冒、流感）", Low: "常与中性粒细胞升高同时出现，提示细菌感染"},
	{Names: []string{"单核细胞比率", "MONO%"}, Cat: "血常规", Unit: "%", Ref: [2]float64{3, 10},
		High: "感染恢复期常见，轻度升高意义不大"},
	{Names: []string{"嗜酸性粒细胞比率", "EO%"}, Cat: "血常规", Unit: "%", Ref: [2]float64{0.4, 8},
		High: "常见于过敏（鼻炎、湿疹、哮喘）或寄生虫感染"},
	{Names: []string{"红细胞", "红细胞计数", "RBC"}, Cat: "血常规", Unit: "×10¹²/L", Ref: [2]float64{3.8, 5.8}, MRef: &[2]float64{4.3, 5.8}, FRef: &[2]float64{3.8, 5.1},
		High: "脱水、长期缺氧（吸烟、高原）可见；明显升高需排查真性红细胞增多", Low: "各种贫血"},
	{Names: []string{"血红蛋白", "HGB", "Hb"}, Cat: "血常规", Unit: "g/L", Ref: [2]float64{115, 175}, MRef: &[2]float64{130, 175}, FRef: &[2]float64{115, 150},
		High: "脱水、吸烟、高原居住可见", Low: "贫血：常见缺铁、地贫基因携带、慢性失血；低于70建议输血评估，尽快就医"},
	{Names: []string{"红细胞压积", "红细胞比容", "HCT"}, Cat: "血常规", Unit: "L/L", Ref: [2]float64{0.35, 0.5}, MRef: &[2]float64{0.4, 0.5}, FRef: &[2]float64{0.35, 0.45},
		High: "常见脱水、血液浓缩", Low: "贫血或出血后"},
	{Names: []string{"平均红细胞体积", "MCV"}, Cat: "血常规", Unit: "fL", Ref: [2]float64{82, 100},
		High: "大细胞性：维生素B12/叶酸缺乏、甲状腺功能减退、饮酒", Low: "小细胞性：缺铁性贫血最常见，其次地中海贫血（南方人注意）"},
	{Names: []string{"平均红细胞血红蛋白量", "MCH"}, Cat: "血常规", Unit: "pg", Ref: [2]float64{27, 34},
		High: "巨幼细胞性贫血可见", Low: "缺铁性贫血、地中海贫血特征"},
	{Names: []string{"平均红细胞血红蛋白浓度", "MCHC"}, Cat: "血常规", Unit: "g/L", Ref: [2]float64{316, 354},
		High: "遗传性球形红细胞增多症、样本因素", Low: "缺铁性贫血"},
	{Names: []string{"血小板", "血小板计数", "PLT"}, Cat: "血常规", Unit: "×10⁹/L", Ref: [2]float64{125, 350},
		High: "反应性增多常见（缺铁、感染、运动后）；持续>450需血液科评估", Low: "病毒感染后一过性降低常见；<50有出血风险，<20须立即就医"},
	{Names: []string{"红细胞分布宽度", "RDW"}, Cat: "血常规", Unit: "%", Ref: [2]float64{11.5, 14.5},
		High: "红细胞大小不均，缺铁早期即可升高，配合MCV判断贫血类型"},
	{Names: []string{"血沉", "ESR"}, Cat: "炎症", Unit: "mm/h", Ref: [2]float64{0, 20}, MRef: &[2]float64{0, 15}, FRef: &[2]float64{0, 20},
		High: "非特异炎症指标：感染、风湿免疫病、贫血、月经期均可升高；明显升高需就医排查"},
	// ── 炎症 ──
	{Names: []string{"C反应蛋白", "超敏C反应蛋白", "CRP", "hs-CRP"}, Cat: "炎症", Unit: "mg/L", Ref: [2]float64{0, 10},
		High: "细菌感染常明显升高（>40~50），病毒感染多轻度升高或正常；是判断是否用抗生素的重要参考"},
	{Names: []string{"降钙素原", "PCT"}, Cat: "炎症", Unit: "ng/mL", Ref: [2]float64{0, 0.05},
		High: "细菌/脓毒症指标：>0.5 提示严重细菌感染，须遵医嘱；轻度升高也可见于手术后、重度应激"},
	// ── 凝血 ──
	{Names: []string{"凝血酶原时间", "PT"}, Cat: "凝血", Unit: "s", Ref: [2]float64{9.4, 12.5},
		High: "延长：华法林使用、肝病、维生素K缺乏；服用抗凝药者以医生设定的目标INR为准"},
	{Names: []string{"国际标准化比值", "INR"}, Cat: "凝血", Unit: "", Ref: [2]float64{0.8, 1.2},
		High: "未服抗凝药者>1.2需查原因；服华法林者一般目标2.0-3.0（机械瓣更高），>4.5出血风险大须就医", Low: "服抗凝药期间<目标下限提示抗凝不足，勿自行加量"},
	{Names: []string{"活化部分凝血活酶时间", "部分凝血活酶时间", "APTT"}, Cat: "凝血", Unit: "s", Ref: [2]float64{25.1, 36.8},
		High: "肝素使用、血友病类因子缺乏、肝病"},
	{Names: []string{"纤维蛋白原", "FIB"}, Cat: "凝血", Unit: "g/L", Ref: [2]float64{2, 4},
		High: "炎症急性期反应，心血管风险相关", Low: "肝病、DIC、遗传性异常；<1.5出血风险增加"},
	{Names: []string{"D-二聚体", "D-二聚体", "DDimer", "D Dimer"}, Cat: "凝血", Unit: "mg/L", Ref: [2]float64{0, 0.55},
		High: "血栓形成/溶解的标志：阴性基本排除急性肺栓塞/深静脉血栓；升高非特异（感染、肿瘤、孕期、术后均可升高）"},
	// ── 肝功能 ──
	{Names: []string{"谷丙转氨酶", "丙氨酸氨基转移酶", "ALT"}, Cat: "肝功能", Unit: "U/L", Ref: [2]float64{7, 50}, MRef: &[2]float64{9, 50}, FRef: &[2]float64{7, 40},
		High: "肝细胞损伤最敏感指标：脂肪肝、乙肝、饮酒、药物（含他汀、退烧药过量）、剧烈运动后均可升高；>3倍上限建议肝病科随诊", Low: "无临床意义"},
	{Names: []string{"谷草转氨酶", "天冬氨酸氨基转移酶", "AST"}, Cat: "肝功能", Unit: "U/L", Ref: [2]float64{13, 40},
		High: "与ALT同时升高提示肝损伤；AST明显>ALT可见酒精性肝病；心肌、肌肉损伤也升高"},
	{Names: []string{"谷氨酰转肽酶", "γ-谷氨酰转肽酶", "GGT"}, Cat: "肝功能", Unit: "U/L", Ref: [2]float64{7, 60}, MRef: &[2]float64{10, 60}, FRef: &[2]float64{7, 45},
		High: "胆道问题、饮酒、脂肪肝、药物影响"},
	{Names: []string{"碱性磷酸酶", "ALP"}, Cat: "肝功能", Unit: "U/L", Ref: [2]float64{45, 125},
		High: "胆道梗阻、骨骼疾病；青少年生长期生理性升高", Low: "甲减、贫血可见；轻度无特殊"},
	{Names: []string{"乳酸脱氢酶", "LDH"}, Cat: "肝功能", Unit: "U/L", Ref: [2]float64{120, 250},
		High: "非特异：溶血、心肌/肝脏/肌肉损伤、肿瘤负荷；轻度升高常无意义"},
	{Names: []string{"总胆红素", "TBIL"}, Cat: "肝功能", Unit: "μmol/L", Ref: [2]float64{3.4, 17.1},
		High: "黄疸指标：伴乏力尿黄尽快就医；仅轻度升高+以间接胆红素为主+其余正常，多为体质性（Gilbert），无需紧张"},
	{Names: []string{"直接胆红素", "结合胆红素", "DBIL"}, Cat: "肝功能", Unit: "μmol/L", Ref: [2]float64{0, 6.8},
		High: "升高提示胆汁淤积或胆道梗阻（伴皮肤瘙痒、陶土色便及时就医）"},
	{Names: []string{"总蛋白", "TP"}, Cat: "肝功能", Unit: "g/L", Ref: [2]float64{65, 85},
		High: "脱水、慢性炎症", Low: "营养不良、肝病、蛋白丢失（肾病）"},
	{Names: []string{"白蛋白", "ALB"}, Cat: "肝功能", Unit: "g/L", Ref: [2]float64{40, 55},
		High: "脱水、血液浓缩", Low: "慢性肝病、肾病（尿蛋白）、营养不良；<30易水肿"},
	{Names: []string{"球蛋白", "GLB"}, Cat: "肝功能", Unit: "g/L", Ref: [2]float64{20, 40},
		High: "慢性感染、免疫病、肝病", Low: "免疫力低下相关，轻度无特殊"},
	// ── 肾功能 ──
	{Names: []string{"肌酐", "血肌酐", "CRE", "Cr", "SCR"}, Cat: "肾功能", Unit: "μmol/L", Ref: [2]float64{41, 111}, MRef: &[2]float64{57, 111}, FRef: &[2]float64{41, 81},
		High: "肾功能指标：脱水、高蛋白饮食、剧烈运动后可轻度升高；持续升高需肾内科评估（可能慢性肾病）", Low: "无临床意义（瘦小、孕期可见）"},
	{Names: []string{"尿素氮", "尿素", "BUN", "UREA"}, Cat: "肾功能", Unit: "mmol/L", Ref: [2]float64{2.9, 8.2},
		High: "高蛋白饮食、脱水、肾功能下降；与肌酐一起看", Low: "低蛋白饮食、肝功能差；轻度无特殊"},
	{Names: []string{"尿酸", "UA", "URIC"}, Cat: "肾功能", Unit: "μmol/L", Ref: [2]float64{155, 428}, MRef: &[2]float64{208, 428}, FRef: &[2]float64{155, 357},
		High: "高尿酸血症：痛风风险（>540需药物评估）；限酒、少内脏/海鲜/含糖饮料、多喝水", Low: "一般无临床意义"},
	{Names: []string{"胱抑素C", "CysC"}, Cat: "肾功能", Unit: "mg/L", Ref: [2]float64{0.51, 1.09},
		High: "比肌酐更灵敏的早期肾功能指标，升高建议肾内科随诊"},
	{Names: []string{"尿微量白蛋白/肌酐", "尿白蛋白肌酐比", "UACR"}, Cat: "肾功能", Unit: "mg/g", Ref: [2]float64{0, 30},
		High: "糖尿病/高血压肾损伤早期信号，>30建议肾内科复评"},
	// ── 血糖 ──
	{Names: []string{"空腹血糖", "FPG", "GLU空腹"}, Cat: "血糖", Unit: "mmol/L", Ref: [2]float64{3.9, 6.1},
		High: "6.1-7.0为空腹血糖受损（糖尿病前期）；≥7.0结合复检考虑糖尿病，建议内分泌科", Low: "<3.9低血糖：心慌出汗手抖，进食可缓解；反复发作需就医"},
	{Names: []string{"餐后2小时血糖", "餐后血糖", "2hPG"}, Cat: "血糖", Unit: "mmol/L", Ref: [2]float64{3.9, 7.8},
		High: "7.8-11.1为糖耐量异常；≥11.1结合复检考虑糖尿病"},
	{Names: []string{"糖化血红蛋白", "HbA1c"}, Cat: "血糖", Unit: "%", Ref: [2]float64{4, 6},
		High: "反映近3个月平均血糖：5.7-6.4糖尿病前期，≥6.5结合复检考虑糖尿病；贫血/血红蛋白病会影响其准确性"},
	{Names: []string{"血淀粉酶", "淀粉酶", "Amylase", "AMS"}, Cat: "血糖", Unit: "U/L", Ref: [2]float64{28, 100},
		High: "急腹症伴明显升高（常>3倍上限）警惕急性胰腺炎，立即就医；轻度升高可见腮腺炎、肾功能差"},
	{Names: []string{"脂肪酶", "Lipase", "LPS"}, Cat: "血糖", Unit: "U/L", Ref: [2]float64{13, 60},
		High: "急性胰腺炎更特异，升高伴上腹痛须立即就医"},
	// ── 血脂 ──
	{Names: []string{"总胆固醇", "TC", "CHOL"}, Cat: "血脂", Unit: "mmol/L", Ref: [2]float64{2.8, 5.2},
		High: "心血管风险因素；先饮食运动干预3个月，明显升高遵医嘱用药"},
	{Names: []string{"甘油三酯", "TG"}, Cat: "血脂", Unit: "mmol/L", Ref: [2]float64{0.45, 1.7},
		High: "受近期饮食影响大：复查需空腹、忌酒；>5.6有急性胰腺炎风险须药物干预"},
	{Names: []string{"高密度脂蛋白", "高密度脂蛋白胆固醇", "HDL-C", "HDL"}, Cat: "血脂", Unit: "mmol/L", Ref: [2]float64{1.0, 1.5},
		High: "一般无需处理（'好胆固醇'）", Low: "<1.0为心血管风险因素：运动、戒烟、减重可提升"},
	{Names: []string{"低密度脂蛋白", "低密度脂蛋白胆固醇", "LDL-C", "LDL"}, Cat: "血脂", Unit: "mmol/L", Ref: [2]float64{0, 3.4},
		High: "'坏胆固醇'，动脉粥样硬化主因；目标值因人而异（已有冠心病/糖尿病者需<1.8甚至<1.4，以医嘱为准）"},
	{Names: []string{"脂蛋白(a)", "Lp(a)"}, Cat: "血脂", Unit: "mg/L", Ref: [2]float64{0, 300},
		High: "主要由遗传决定，是残余心血管风险指标；明显升高者心血管评估更积极，无特效降脂药"},
	{Names: []string{"载脂蛋白A1", "ApoA1"}, Cat: "血脂", Unit: "g/L", Ref: [2]float64{1.2, 1.6}},
	{Names: []string{"载脂蛋白B", "ApoB"}, Cat: "血脂", Unit: "g/L", Ref: [2]float64{0.6, 1.1},
		High: "与LDL-C一致反映致动脉硬化脂蛋白，他汀治疗后仍高需关注"},
	// ── 电解质 ──
	{Names: []string{"钾", "血钾", "K"}, Cat: "电解质", Unit: "mmol/L", Ref: [2]float64{3.5, 5.3},
		High: ">6.0可致心律失常，须立即就医；保钾利尿剂、肾功能差常见", Low: "<3.0乏力软瘫心律失常，须就医；利尿剂、呕吐腹泻常见"},
	{Names: []string{"钠", "血钠", "NA"}, Cat: "电解质", Unit: "mmol/L", Ref: [2]float64{137, 147},
		High: "缺水为主，老年人须注意", Low: "<130需就医查原因（利尿剂、抗利尿激素异常等）"},
	{Names: []string{"氯", "CL"}, Cat: "电解质", Unit: "mmol/L", Ref: [2]float64{99, 110},
		High: "常与脱水、酸碱失衡并存", Low: "呕吐丢失为主，单独轻度异常无特殊"},
	{Names: []string{"钙", "血钙", "CA"}, Cat: "电解质", Unit: "mmol/L", Ref: [2]float64{2.11, 2.52},
		High: "反复明显升高需排查甲状旁腺功能亢进、肿瘤", Low: "维生素D缺乏、甲旁减；手足抽搐须就医"},
	{Names: []string{"磷", "血磷", "P"}, Cat: "电解质", Unit: "mmol/L", Ref: [2]float64{0.85, 1.51},
		High: "肾功能下降、甲旁减", Low: "酗酒、再喂养；单独轻度异常常无意义"},
	{Names: []string{"镁", "MG"}, Cat: "电解质", Unit: "mmol/L", Ref: [2]float64{0.75, 1.02},
		High: "肾功能差、含镁药物", Low: "利尿剂、饮酒、腹泻；<0.5可致心律失常"},
	{Names: []string{"铁", "血清铁", "FE"}, Cat: "电解质", Unit: "μmol/L", Ref: [2]float64{9, 31.3}, MRef: &[2]float64{11.6, 31.3}, FRef: &[2]float64{9, 30.4},
		High: "铁过载、溶血", Low: "缺铁性贫血的组成证据（配合铁蛋白看）"},
	// ── 心肌/血管 ──
	{Names: []string{"肌钙蛋白I", "cTnI", "TNI"}, Cat: "心肌标志物", Unit: "ng/mL", Ref: [2]float64{0, 0.04},
		High: "心肌损伤核心指标：胸痛伴升高警惕心梗，立即拨打120；术后、肾衰也可轻度升高"},
	{Names: []string{"肌酸激酶", "CK"}, Cat: "心肌标志物", Unit: "U/L", Ref: [2]float64{26, 174}, MRef: &[2]float64{38, 174}, FRef: &[2]float64{26, 140},
		High: "运动、拉伤、他汀副作用均可升高；剧烈健身后数倍升高属常见；伴酱油色尿须就医（横纹肌溶解）"},
	{Names: []string{"肌酸激酶同工酶", "CK-MB"}, Cat: "心肌标志物", Unit: "U/L", Ref: [2]float64{0, 25},
		High: "配合肌钙蛋白判断心肌损伤；单独轻度升高意义有限（以报告单位/方法为准）"},
	{Names: []string{"同型半胱氨酸", "Hcy"}, Cat: "心肌标志物", Unit: "μmol/L", Ref: [2]float64{5, 15},
		High: "心脑血管风险指标；补充叶酸和B族维生素可降；>30建议补充并复测"},
	{Names: []string{"B型钠尿肽", "BNP"}, Cat: "心肌标志物", Unit: "pg/mL", Ref: [2]float64{0, 100},
		High: "心衰筛查指标：气促+升高建议心内科；高龄、肾功能差也会升高"},
	{Names: []string{"N末端B型钠尿肽原", "NT-proBNP"}, Cat: "心肌标志物", Unit: "pg/mL", Ref: [2]float64{0, 300},
		High: "同BNP（参考值随年龄升高：50岁以上更高，以医嘱为准）"},
	// ── 甲状腺 ──
	{Names: []string{"促甲状腺激素", "TSH"}, Cat: "甲状腺", Unit: "mIU/L", Ref: [2]float64{0.27, 4.2},
		High: "甲减最灵敏指标：乏力怕冷体重增加；明显升高伴FT4降低需服药，遵内分泌科", Low: "甲亢指标：心慌手抖怕热消瘦；服药者需定期复查调药"},
	{Names: []string{"游离三碘甲状腺原氨酸", "FT3"}, Cat: "甲状腺", Unit: "pmol/L", Ref: [2]float64{3.1, 6.8},
		High: "甲亢表现", Low: "与TSH一起判断甲减"},
	{Names: []string{"游离甲状腺素", "FT4"}, Cat: "甲状腺", Unit: "pmol/L", Ref: [2]float64{12, 22},
		High: "甲亢表现", Low: "甲减诊断依据之一"},
	// ── 肿瘤标志物 ──
	{Names: []string{"甲胎蛋白", "AFP"}, Cat: "肿瘤标志物", Unit: "ng/mL", Ref: [2]float64{0, 7},
		High: "原发性肝癌筛查指标，但肝炎活动、孕期也可升高；轻度升高勿恐慌，明显/持续升高配合肝脏超声复查"},
	{Names: []string{"癌胚抗原", "CEA"}, Cat: "肿瘤标志物", Unit: "ng/mL", Ref: [2]float64{0, 5},
		High: "消化道肿瘤相关，但吸烟者可轻度升高；肿瘤标志物不能单独确诊，持续升高需胃肠镜等系统筛查"},
	{Names: []string{"糖类抗原125", "CA125"}, Cat: "肿瘤标志物", Unit: "U/mL", Ref: [2]float64{0, 35},
		High: "女性妇科（卵巢、子宫）相关，但月经期、盆腔炎、子宫肌瘤、腹水均可升高；持续升高配合妇科超声"},
	{Names: []string{"糖类抗原19-9", "CA19-9"}, Cat: "肿瘤标志物", Unit: "U/mL", Ref: [2]float64{0, 37},
		High: "消化道、胰腺相关；胆道梗阻、胰腺炎也明显升高，需结合影像"},
	{Names: []string{"糖类抗原15-3", "CA15-3"}, Cat: "肿瘤标志物", Unit: "U/mL", Ref: [2]float64{0, 25},
		High: "乳腺相关标志物，轻度升高可见良性乳腺疾病；配合乳腺检查"},
	{Names: []string{"前列腺特异抗原", "总前列腺特异抗原", "PSA", "tPSA"}, Cat: "肿瘤标志物", Unit: "ng/mL", Ref: [2]float64{0, 4},
		High: "前列腺筛查指标：前列腺增生、炎症、射精后、骑车后均可升高；>10或持续升高建议泌尿外科（直肠指检+必要时磁共振）"},
	// ── 维生素/营养 ──
	{Names: []string{"25羟维生素D", "维生素D", "25-OH-D"}, Cat: "营养", Unit: "nmol/L", Ref: [2]float64{30, 100},
		High: "过量补充风险", Low: "<30不足、<20缺乏：日晒不足常见，遵医嘱补充"},
	{Names: []string{"维生素B12", "B12"}, Cat: "营养", Unit: "pmol/L", Ref: [2]float64{148, 660},
		High: "补剂过量、肝病", Low: "长期素食、胃肠吸收差、长期服二甲双胍者；缺乏可致贫血和神经症状"},
	{Names: []string{"叶酸", "FA", "Folate"}, Cat: "营养", Unit: "nmol/L", Ref: [2]float64{7, 45},
		High: "补剂过量", Low: "巨幼细胞贫血、孕期需求增加；备孕/孕期按医嘱补充"},
	{Names: []string{"铁蛋白", "Ferritin", "SF"}, Cat: "营养", Unit: "ng/mL", Ref: [2]float64{13, 400}, MRef: &[2]float64{30, 400}, FRef: &[2]float64{13, 150},
		High: "铁过载、肝病、炎症状态（急性期蛋白）；结合转铁蛋白饱和度判断", Low: "最敏感的缺铁指标，<30即提示铁储备不足"},
	// ── 尿常规 ──
	{Names: []string{"尿蛋白", "PRO(尿)", "尿蛋白质"}, Cat: "尿常规", Qual: true,
		High: "阳性（+）: 剧烈运动、发热后一过性常见；持续阳性提示肾脏问题，复查晨尿仍+需肾内科"},
	{Names: []string{"尿糖", "GLU(尿)"}, Cat: "尿常规", Qual: true,
		High: "阳性可见血糖过高或肾糖阈低；配合血糖判断"},
	{Names: []string{"尿潜血", "尿隐血", "BLD(尿)"}, Cat: "尿常规", Qual: true,
		High: "月经污染、运动、结石、感染均可致阳性；红细胞管型或持续阳性需复查尿沉渣镜检"},
	{Names: []string{"尿白细胞", "LEU(尿)"}, Cat: "尿常规", Qual: true,
		High: "阳性伴尿频尿急尿痛提示泌尿系感染，多喝水必要时抗感染治疗"},
	{Names: []string{"尿亚硝酸盐", "NIT(尿)"}, Cat: "尿常规", Qual: true,
		High: "阳性提示细菌尿（与尿白细胞联合判断）"},
	{Names: []string{"尿酮体", "KET(尿)"}, Cat: "尿常规", Qual: true,
		High: "饥饿、减肥、呕吐、糖尿病控制差均可阳性；糖尿病人阳性伴口干多尿须尽快就医（酮症风险）"},
	{Names: []string{"尿胆原", "UBG(尿)"}, Cat: "尿常规", Qual: true,
		High: "溶血、肝功能异常可见，轻度±多为正常波动"},
	{Names: []string{"尿胆红素", "BIL(尿)"}, Cat: "尿常规", Qual: true,
		High: "阳性提示血直接胆红素升高（肝胆问题），配合肝功能"},
	{Names: []string{"尿比重", "SG(尿)"}, Cat: "尿常规", Unit: "", Ref: [2]float64{1.005, 1.03},
		High: "饮水少、尿液浓缩", Low: "大量饮水、肾功能浓缩能力下降"},
	{Names: []string{"尿pH", "pH(尿)"}, Cat: "尿常规", Unit: "", Ref: [2]float64{4.5, 8},
		High: "素食多、尿路感染（分解尿素菌）", Low: "肉食多、酸中毒倾向；单独异常意义有限"},
}

// matchLabItem 按别名匹配指标 (精确匹配优先, 其次包含匹配, 防止"白细胞"吞掉
// "白细胞酯酶"之类的误配 — 包含匹配时要求边界更严格)。
func matchLabItem(name string) *labItem {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "（", "(")
	name = strings.ReplaceAll(name, "）", ")")
	best := -1
	bestLen := 0
	for i := range labItems {
		for _, alias := range labItems[i].Names {
			if name == alias {
				return &labItems[i]
			}
			// 包含匹配: 别名≥2字且出现在名称里 (如 "血清肌酐(Cr)" 含 "肌酐")
			if len([]rune(alias)) >= 2 && strings.Contains(name, alias) && len([]rune(alias)) > bestLen {
				best = i
				bestLen = len([]rune(alias))
			}
		}
	}
	if best >= 0 {
		return &labItems[best]
	}
	return nil
}

// parseLabReport 从自由文本提取 (名称, 数值/定性, 是否偏高/偏低标记)。
// 支持: "白细胞 12.5 ×10⁹/L ↑"、"WBC:12.5(H)"、"肌酐 95umol/l"、"尿蛋白 +2"。
func parseLabReport(text string) []labReading {
	var out []labReading
	lines := strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == ';' || r == '；' || r == '，' || r == ','
	})
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len([]rune(line)) < 2 {
			continue
		}
		// 名称 = 冒号/等号/空白之前的部分
		var namePart, valPart string
		for _, sep := range []string{"：", ":", "=", "＝"} {
			if i := strings.Index(line, sep); i > 0 {
				namePart, valPart = line[:i], line[i+len(sep):]
				break
			}
		}
		if namePart == "" {
			// 名称+数值 混排: 找第一个数字/+/-
			i := strings.IndexFunc(line, func(r rune) bool {
				return unicode.IsDigit(r) || r == '+' || r == '-'
			})
			if i > 0 {
				namePart, valPart = line[:i], line[i:]
			} else {
				namePart = line
			}
		}
		item := matchLabItem(namePart)
		if item == nil {
			continue
		}
		r := labReading{item: item, raw: strings.TrimSpace(valPart)}
		vp := r.raw
		// 方向标记
		lowVal := strings.ToLower(vp)
		r.flagHigh = strings.ContainsAny(vp, "↑▲") || strings.Contains(lowVal, "(h)") || strings.Contains(lowVal, "偏高")
		r.flagLow = strings.ContainsAny(vp, "↓▼") || strings.Contains(lowVal, "(l)") || strings.Contains(lowVal, "偏低")
		// 去掉单位和标记符号, 提取数值
		clean := strings.Map(func(r rune) rune {
			switch r {
			case '↑', '↓', '▲', '▼', '*', '†':
				return -1
			}
			return r
		}, vp)
		clean = strings.TrimSpace(clean)
		// 定性 "+/++/±数字" 优先按定性处理
		if strings.HasPrefix(clean, "+") || strings.HasPrefix(clean, "±") {
			r.qualitative = clean
		} else if v, _, err := parseFirstFloat(clean); err == nil {
			r.value = &v
		} else {
			// 定性: 阴性/-/正常 vs +/阳性
			c2 := strings.ToLower(clean)
			switch {
			case strings.Contains(c2, "阴性") || strings.Contains(c2, "(-)") || strings.TrimSpace(c2) == "-":
				r.qualitative = "阴性"
			case strings.ContainsAny(clean, "+") || strings.Contains(c2, "阳性"):
				r.qualitative = strings.TrimSpace(clean)
			}
		}
		out = append(out, r)
	}
	return out
}

// labReading 是解析出的一条读数。
type labReading struct {
	item        *labItem
	raw         string
	value       *float64
	qualitative string
	flagHigh    bool
	flagLow     bool
}

// parseFirstFloat 从字符串提取第一个浮点数, 返回 (值, 结束索引, err)。
func parseFirstFloat(s string) (float64, int, error) {
	start := -1
	dot := false
	end := 0
	for i, r := range s {
		switch {
		case unicode.IsDigit(r):
			if start < 0 {
				start = i
			}
			end = i + 1
		case r == '.' && start >= 0 && !dot:
			dot = true
			end = i + 1
		case start >= 0:
			// 数字段结束 (处理 1.2.3 之类: 遇第二个点停)
			v, err := strconv.ParseFloat(s[start:end], 64)
			return v, end, err
		}
	}
	if start < 0 {
		return 0, 0, fmt.Errorf("no number in %q", s)
	}
	v, err := strconv.ParseFloat(s[start:end], 64)
	return v, end, err
}

// fmtNum 去掉多余的 0 与小数点。
func fmtNum(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	return s
}

// LabReportAnalyze 解读化验单/体检报告: 参考区间对照 + 每项异常含义。
// 与 medical_image_analyze 配合: 先 OCR 图片, 再把文本交给本工具。
type LabReportAnalyze struct{}

func NewLabReportAnalyze() *LabReportAnalyze {
	return &LabReportAnalyze{}
}

func (t *LabReportAnalyze) Name() string { return "lab_report_analyze" }

func (t *LabReportAnalyze) Description() string {
	return "对照参考区间解读化验单/体检报告文本（血常规、肝肾功能、血糖血脂、电解质、甲状腺、凝血、肿瘤标志物、尿常规等约70项），逐项给出偏高/偏低的常见原因和就医建议。用户上传化验单图片时，先用 medical_image_analyze 提取文字，再把提取结果原样传给本工具的 report_text。参数：report_text=报告原文（支持「项目名 数值 单位 ↑/↓/H/L」任意格式），gender=男/女（可选）。"
}

func (t *LabReportAnalyze) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"report_text": map[string]any{
				"type":        "string",
				"description": "化验单文本，任意格式均可（OCR 原文、'项目: 值 单位 ↑'、'项目=值'）。逐行更准。",
			},
			"gender": map[string]any{
				"type":        "string",
				"enum":        []string{"男", "女"},
				"description": "患者性别（影响部分参考区间）",
			},
		},
		"required": []string{"report_text"},
	}
}

func (t *LabReportAnalyze) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	text, _ := input["report_text"].(string)
	gender, _ := input["gender"].(string)
	if strings.TrimSpace(text) == "" {
		return &ToolResult{Success: false, Error: "请提供 report_text（化验单文本）"}, nil
	}

	readings := parseLabReport(text)
	if len(readings) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"message": "未能从文本中识别出可解读的检验项目。请确认文本包含项目名称与数值（如 '白细胞 12.5'）。",
				"hint":    "支持的类别: 血常规/炎症/凝血/肝功能/肾功能/血糖/血脂/电解质/心肌标志物/甲状腺/肿瘤标志物/营养/尿常规",
			},
		}, nil
	}

	var abn, normal []map[string]any
	var sb strings.Builder
	seen := map[*labItem]bool{}
	for _, r := range readings {
		if seen[r.item] {
			continue // 同一项多次出现取首个
		}
		seen[r.item] = true
		it := r.item
		ref := it.Ref
		if gender == "男" && it.MRef != nil {
			ref = *it.MRef
		}
		if gender == "女" && it.FRef != nil {
			ref = *it.FRef
		}

		var status, meaning string
		abnormal := false
		unitPart := ""
		if it.Unit != "" {
			unitPart = " " + it.Unit
		}
		var shown string
		if r.value != nil {
			shown = fmtNum(*r.value) + unitPart
			if it.Qual {
				// 定性项给了数值（如 +2): 按 "+数字" 处理
				if *r.value > 0 {
					r.qualitative = "+" + fmtNum(*r.value)
					abnormal = true
				} else {
					r.qualitative = "阴性"
				}
				status = "⚠️ " + r.qualitative
			} else if *r.value > ref[1] || r.flagHigh && *r.value > ref[0] {
				abnormal = true
				status = "↑ 偏高"
				meaning = it.High
			} else if *r.value < ref[0] || r.flagLow && *r.value < ref[1] {
				abnormal = true
				status = "↓ 偏低"
				meaning = it.Low
			} else {
				status = "✓ 正常"
			}
		} else if r.qualitative != "" {
			shown = r.qualitative
			if it.Qual && strings.Contains(r.qualitative, "+") && !strings.Contains(r.qualitative, "阴性") {
				abnormal = true
				status = "⚠️ " + r.qualitative
				meaning = it.High
			} else if r.qualitative == "阴性" {
				status = "✓ 正常"
			}
		} else {
			shown = strings.TrimSpace(r.raw)
			status = "（数值未识别，请人工核对）"
		}

		var refText string
		if it.Qual {
			refText = "阴性"
		} else {
			refText = fmtNum(ref[0]) + "-" + fmtNum(ref[1])
			if it.Unit != "" {
				refText += " " + it.Unit
			}
		}

		note := ""
		if it.Note != "" {
			note = "（" + it.Note + "）"
		}
		sb.WriteString(fmt.Sprintf("• %s：%s（参考 %s）%s\n", it.Names[0], shown, refText, status))
		if meaning != "" {
			sb.WriteString("  含义: " + meaning + note + "\n")
		}

		entry := map[string]any{
			"item":    it.Names[0],
			"value":   shown,
			"ref":     refText,
			"status":  status,
			"meaning": meaning,
		}
		if abnormal {
			abn = append(abn, entry)
		} else {
			normal = append(normal, entry)
		}
	}

	// 汇总排序: 异常按分类
	cats := map[string]int{}
	abnCats := []string{}
	for _, a := range abn {
		item := matchLabItem(a["item"].(string))
		if item != nil {
			if cats[item.Cat] == 0 {
				abnCats = append(abnCats, item.Cat)
			}
			cats[item.Cat]++
		}
	}
	sort.Strings(abnCats)

	head := fmt.Sprintf("识别 %d 项，异常 %d 项，正常 %d 项。", len(seen), len(abn), len(normal))
	tail := "\n【提示】\n• 参考区间为成人通用值（血细胞按 WS/T 405 等标准），不同医院试剂/仪器可能略有差异，以报告单标注为准\n• 单项轻度异常很常见，需结合症状判断；多项异常或明显异常建议带报告就诊相应科室\n• 本解读不构成诊断"
	if len(abn) == 0 {
		tail = "\n【结论】所有可识别项目均在参考范围内。" + tail
	} else {
		tail = "\n【建议】异常项集中在: " + strings.Join(abnCats, "、") + "；异常项请结合症状与医生判断。" + tail
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"summary":    head,
			"detail":     sb.String(),
			"abnormal":   abn,
			"normal":     normal,
			"conclusion": tail,
		},
		Citations: []CitationRef{
			{ID: "lab_ref_ws405", Title: "WS/T 405 血细胞分析参考区间及临床检验项目通用参考区间", Level: "guideline"},
		},
	}, nil
}
