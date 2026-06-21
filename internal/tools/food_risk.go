package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// FoodRiskAnalyzer analyzes food-related health risks for southern Chinese populations.
type FoodRiskAnalyzer struct {
	store *knowledge.Store
}

// NewFoodRiskAnalyzer creates the food risk tool.
func NewFoodRiskAnalyzer(store *knowledge.Store) *FoodRiskAnalyzer {
	return &FoodRiskAnalyzer{store: store}
}

func (t *FoodRiskAnalyzer) Name() string {
	return "food_risk_analyzer"
}

func (t *FoodRiskAnalyzer) Description() string {
	return "分析特定食物或饮食模式对中国南方人群的健康风险。评估因素包括：G6PD安全性（蚕豆等）、嘌呤含量（痛风风险）、亚硝胺（咸鱼/NPC风险）、乳糖含量、常见过敏原、黄曲霉毒素风险（潮湿气候霉变食物）。"
}

func (t *FoodRiskAnalyzer) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"food_name": map[string]any{
				"type":        "string",
				"description": "食物名称或饮食模式，如 '蚕豆'、'咸鱼'、'老火汤'、'海鲜'、'牛奶'",
			},
		},
		"required": []string{"food_name"},
	}
}

func (t *FoodRiskAnalyzer) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	foodName, ok := input["food_name"].(string)
	if !ok || strings.TrimSpace(foodName) == "" {
		return &ToolResult{
			Success: false,
			Error:   "请提供食物名称参数 food_name",
		}, nil
	}

	foodName = strings.TrimSpace(foodName)
	result := t.analyze(foodName)

	return result, nil
}

func (t *FoodRiskAnalyzer) analyze(foodName string) *ToolResult {
	lower := strings.ToLower(foodName)

	// G6PD food triggers
	if strings.Contains(lower, "蚕豆") || strings.Contains(lower, "fava") || strings.Contains(lower, "broad bean") {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"food_name":      foodName,
				"risk_category":  "G6PD触发食物 — 急性溶血性贫血风险",
				"risk_level":     "high",
				"detail":         "蚕豆含有蚕豆嘧啶（vicine）和伴蚕豆嘧啶（convicine），在G6PD缺乏症患者体内被代谢为强氧化剂divicine和isouramil，导致血红蛋白氧化变性、Heinz小体形成和急性血管内溶血。溶血通常在食用后12-48小时发生，严重者可致急性肾功能衰竭。即使吸入蚕豆花粉也可能在高度敏感个体中诱发溶血。",
				"affected_population": "G6PD缺乏症患者（南方人群携带率：广西~17.5%，广东~4%，海南~3.7%）",
				"severity":           "高 — 可危及生命",
				"safe_alternatives":  []string{"黄豆（大豆）", "绿豆", "鹰嘴豆", "扁豆", "豌豆", "四季豆（菜豆属，与蚕豆属不同）"},
			},
			Citations: []CitationRef{
				{Title: "Favism: Clinical Features, Pathophysiology, and Management in the Genomic Era", DOI: "10.1182/blood.2022015529", Level: "review", Year: 2022},
				{Title: "小儿G6PD缺乏症诊疗指南（2025年版）", Level: "national_guideline", Year: 2025},
			},
		}
	}

	// Salted fish / NPC risk
	if strings.Contains(lower, "咸鱼") || strings.Contains(lower, "salted fish") || strings.Contains(lower, "腌制鱼") {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"food_name":     foodName,
				"risk_category": "IARC Group 1 致癌物 — 鼻咽癌（NPC）风险",
				"risk_level":    "high",
				"detail":        "广东式咸鱼被国际癌症研究机构（IARC）列为Group 1致癌物（对人类有明确致癌性）。咸鱼在腌制和干燥过程中，硝酸盐被细菌还原为亚硝酸盐，与鱼肉中的胺类反应形成N-亚硝胺类化合物（如N-亚硝基二甲胺NDMA），这些化合物是强致癌物。流行病学研究一致显示，幼年时期咸鱼摄入量与成年后NPC风险呈剂量-反应关系。结合EBV感染和遗传易感性（HLA位点），咸鱼是华南地区NPC高发的关键环境因素。",
				"affected_population": "华南地区居民（特别是广东/广西/香港），幼年暴露风险更高",
				"severity":           "高 — 致癌风险是累积性和不可逆的",
				"safe_alternatives":  []string{"新鲜鱼类", "清蒸鱼", "炖鱼", "冷藏/冷冻鱼"},
			},
			Citations: []CitationRef{
				{Title: "IARC Monographs: Chinese-style Salted Fish", Level: "international_guideline", Year: 2012},
				{Title: "Nasopharyngeal Carcinoma: Epidemiology, Etiology, and Screening in Southern China", DOI: "10.1016/S1470-2045(24)00001-X", Level: "epidemiology", Year: 2024},
			},
		}
	}

	// Old-fire soup / gout
	if strings.Contains(lower, "老火汤") || strings.Contains(lower, "炖汤") || strings.Contains(lower, "煲汤") || strings.Contains(lower, "骨头汤") {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"food_name":     foodName,
				"risk_category": "高嘌呤 — 高尿酸血症/痛风风险",
				"risk_level":    "moderate",
				"detail":        "老火汤（长时间熬煮的肉汤/骨头汤）嘌呤含量极高。长时间高温熬煮（通常3-6小时）使肉类和骨骼中的嘌呤（特别是次黄嘌呤）大量溶入汤中。一碗老火汤的嘌呤含量可达200-400mg（相当于一次急性痛风发作的触发剂量）。广东地区痛风患病率显著高于全国平均水平，与老火汤文化密切相关。此外，某些传统陶罐在长时间酸性汤汁熬煮中可能析出铅等重金属。",
				"affected_population": "高尿酸血症/痛风患者、有痛风家族史者、肾功能不全者",
				"severity":           "中等 — 痛风发作风险（急性关节炎、剧烈疼痛）",
				"safe_alternatives":  []string{"清汤（短时间烹煮）", "蔬菜汤", "蒸菜（保留营养、减少嘌呤溶出）", "花胶/海参汤（低嘌呤海味）"},
			},
			Citations: []CitationRef{
				{Title: "Dietary Purine Intake and the Risk of Hyperuricemia and Gout: A Systematic Review", DOI: "10.1002/art.41089", Level: "meta_analysis", Year: 2020},
			},
		}
	}

	// Seafood allergy
	if strings.Contains(lower, "海鲜") || strings.Contains(lower, "虾") || strings.Contains(lower, "蟹") || strings.Contains(lower, "贝") || strings.Contains(lower, "seafood") {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"food_name":     foodName,
				"risk_category": "常见过敏原 + 重金属暴露风险",
				"risk_level":    "moderate",
				"detail":        "海鲜是华南地区最常见的食物过敏原之一（虾、蟹、贝类为最常见的致敏海鲜）。海鲜过敏可表现为轻度（口腔瘙痒、荨麻疹）至重度（过敏性休克）。此外，某些大型掠食性鱼类（金枪鱼、鲨鱼、剑鱼）和贝类可能在体内富集甲基汞——一种神经毒性重金属。华南沿海地区长期高海鲜摄入人群应注意汞暴露，特别是孕妇（甲基汞可通过胎盘影响胎儿神经系统发育）。",
				"affected_population": "过敏体质者、孕妇（汞暴露风险）、痛风患者（高嘌呤海鲜）",
				"severity":           "中等 — 过敏反应可危及生命，重金属风险为长期累积性",
				"safe_alternatives":  []string{"淡水鱼（低汞）", "小型海鱼（沙丁鱼、鲭鱼 — 高Omega-3、低汞）", "植物蛋白"},
			},
		}
	}

	// Lactose / dairy
	if strings.Contains(lower, "牛奶") || strings.Contains(lower, "奶制") || strings.Contains(lower, "乳糖") || strings.Contains(lower, "dairy") || strings.Contains(lower, "lactose") {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"food_name":     foodName,
				"risk_category": "乳糖不耐受 — 消化系统症状",
				"risk_level":    "low",
				"detail":        "华南地区成人乳糖不耐受比例超过80%（全球最高之一）。乳糖酶非持续性（LCT基因-13910 C/C基因型）导致乳糖不能被分解吸收，在结肠被细菌发酵 → 产气、腹胀、渗透性腹泻。症状通常在摄入后30分钟至2小时出现。这不是过敏（不涉及IgE），而是消化酶缺乏。大多数乳糖不耐受者可耐受少量乳制品（每日约12g乳糖≈250ml牛奶分次摄入）。",
				"affected_population": "绝大多数华南成人（>80%）",
				"severity":           "低 — 不适但非危及生命",
				"safe_alternatives":  []string{"无乳糖牛奶（乳糖酶预处理）", "酸奶（乳糖已被发酵）", "硬质奶酪（乳糖含量极低）", "豆奶（加钙强化）", "杏仁奶"},
			},
			Citations: []CitationRef{
				{Title: "Lactose Intolerance in Adults: Biological Mechanism and Dietary Management", DOI: "10.3390/nu13020303", PMID: "33498789", Level: "review", Year: 2021},
			},
		}
	}

	// General food analysis
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"food_name":     foodName,
			"risk_category": "未在南方特异性食物风险数据库中找到",
			"risk_level":    "unknown",
			"detail":        fmt.Sprintf("'%s' 未匹配到已知的南方人群特异性食物风险模式。如需进一步分析，可以提供更多信息（食材成分、烹饪方式、食用频率）。常规食物安全建议：确保食物新鲜、彻底烹煮、避免霉变（华南潮湿气候下黄曲霉毒素风险增加）。", foodName),
		},
	}
}
