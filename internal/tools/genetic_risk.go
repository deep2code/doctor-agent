package tools

import (
	"context"
	"fmt"

	"github.com/doctor-agent/internal/knowledge"
)

// GeneticRiskCalculator computes inheritance probabilities for thalassemia.
type GeneticRiskCalculator struct {
	store *knowledge.Store
}

// NewGeneticRiskCalculator creates the genetic risk tool.
func NewGeneticRiskCalculator(store *knowledge.Store) *GeneticRiskCalculator {
	return &GeneticRiskCalculator{store: store}
}

func (t *GeneticRiskCalculator) Name() string {
	return "genetic_risk_calculator"
}

func (t *GeneticRiskCalculator) Description() string {
	return "计算夫妇地中海贫血遗传风险。输入父母的α-地贫和β-地贫基因型状态，使用Punnett方阵计算子代患病概率（正常、携带者、中间型、重型），并提供基于中国2025年指南的婚育建议。"
}

func (t *GeneticRiskCalculator) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"parent1_alpha": map[string]any{
				"type":        "string",
				"description": "父方α-地贫基因型: normal, silent_carrier (-α/αα), trait (--/αα or -α/-α), hbh_disease (--/-α), homozygous (--/--)",
				"enum":        []string{"normal", "silent_carrier", "trait", "hbh_disease", "homozygous"},
			},
			"parent1_beta": map[string]any{
				"type":        "string",
				"description": "父方β-地贫基因型: normal, trait (β⁰/β or β+/β), intermedia (β+/β+ or β+/β⁰), major (β⁰/β⁰)",
				"enum":        []string{"normal", "trait", "intermedia", "major"},
			},
			"parent2_alpha": map[string]any{
				"type":        "string",
				"description": "母方α-地贫基因型: normal, silent_carrier, trait, hbh_disease, homozygous",
				"enum":        []string{"normal", "silent_carrier", "trait", "hbh_disease", "homozygous"},
			},
			"parent2_beta": map[string]any{
				"type":        "string",
				"description": "母方β-地贫基因型: normal, trait, intermedia, major",
				"enum":        []string{"normal", "trait", "intermedia", "major"},
			},
		},
		"required": []string{"parent1_alpha", "parent1_beta", "parent2_alpha", "parent2_beta"},
	}
}

func (t *GeneticRiskCalculator) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	p1Alpha, _ := input["parent1_alpha"].(string)
	p1Beta, _ := input["parent1_beta"].(string)
	p2Alpha, _ := input["parent2_alpha"].(string)
	p2Beta, _ := input["parent2_beta"].(string)

	alphaRisk := t.calcAlphaRisk(p1Alpha, p2Alpha)
	betaRisk := t.calcBetaRisk(p1Beta, p2Beta)

	result := &ToolResult{
		Success: true,
		Data: map[string]any{
			"parent1": map[string]string{
				"alpha": p1Alpha,
				"beta":  p1Beta,
			},
			"parent2": map[string]string{
				"alpha": p2Alpha,
				"beta":  p2Beta,
			},
			"alpha_thalassemia_risk": alphaRisk,
			"beta_thalassemia_risk":  betaRisk,
			"recommendations":        t.buildRecommendations(alphaRisk, betaRisk),
		},
		Citations: []CitationRef{
			{
				Title: "中国儿童输血依赖型地中海贫血输血管理指南（2025年）",
				DOI:   "10.7499/j.issn.1008-8830.2410119",
				Level: "national_guideline",
				Year:  2025,
			},
		},
	}

	return result, nil
}

func (t *GeneticRiskCalculator) calcAlphaRisk(p1, p2 string) map[string]any {
	// Simplified Punnett square logic for autosomal recessive inheritance
	// Alpha-thalassemia: severity depends on number of functional α-globin genes (0-4)
	risk := map[string]any{
		"parent1_genotype": p1,
		"parent2_genotype": p2,
	}

	// Both trait (--/αα each): 25% normal, 50% trait, 25% Hb Barts hydrops (--/--)
	if p1 == "trait" && p2 == "trait" {
		risk["offspring_risk"] = map[string]string{
			"normal_αα_αα":     "25% — 正常，4个功能α基因",
			"trait_--_αα":      "50% — α-地贫携带者（标准型），2个功能基因缺失",
			"hb_barts_hydrops": "25% — Hb Bart's 水肿胎（--/--），0个功能α基因，通常致死性",
		}
		risk["severe_risk"] = "25% (Hb Bart's 水肿胎 — 致死性，产前诊断至关重要)"
		risk["recommendation"] = "强烈建议进行产前基因诊断（孕10-13周绒毛膜取样或16-20周羊膜腔穿刺）。若胎儿为--/--，建议终止妊娠（此为致死性疾病）。"
	} else if (p1 == "trait" && p2 == "normal") || (p1 == "normal" && p2 == "trait") {
		risk["offspring_risk"] = map[string]string{
			"normal_αα_αα": "50% — 正常",
			"trait_--_αα":  "50% — α-地贫携带者（标准型），通常无症状或轻度贫血",
		}
		risk["severe_risk"] = "0% (无重型患儿风险)"
		risk["recommendation"] = "子代有50%概率为携带者。无需特殊产前干预，但建议子女成年后进行地贫筛查以便婚育指导。"
	} else if p1 == "normal" && p2 == "normal" {
		risk["offspring_risk"] = map[string]string{"normal": "100%"}
		risk["severe_risk"] = "0%"
		risk["recommendation"] = "子代无地贫风险。"
	} else {
		risk["offspring_risk"] = map[string]string{"note": "需要更详细的基因型信息（具体缺失片段）来精确计算风险"}
		risk["recommendation"] = "建议进行专业的遗传咨询和基因检测以精确评估风险。"
	}

	return risk
}

func (t *GeneticRiskCalculator) calcBetaRisk(p1, p2 string) map[string]any {
	risk := map[string]any{
		"parent1_genotype": p1,
		"parent2_genotype": p2,
	}

	// Both β-thal trait: 25% normal, 50% trait, 25% major
	if p1 == "trait" && p2 == "trait" {
		risk["offspring_risk"] = map[string]string{
			"normal_β_β":   "25% — 正常",
			"trait_β0_β":   "50% — β-地贫携带者，通常无症状或轻度贫血",
			"major_β0_β0":  "25% — 重型β-地贫（Cooley贫血），需终身输血和祛铁治疗",
		}
		risk["severe_risk"] = "25% (重型β-地贫 — 需要终身输血+祛铁治疗或HSCT)"
		risk["recommendation"] = "强烈建议产前基因诊断。重型β-地贫可通过造血干细胞移植（HSCT）治愈（2-7岁最佳时机），也可考虑基因治疗临床试验。Luspatercept可用于成人β-TDT。"
	} else if (p1 == "trait" && p2 == "normal") || (p1 == "normal" && p2 == "trait") {
		risk["offspring_risk"] = map[string]string{
			"normal": "50% — 正常",
			"trait":  "50% — β-地贫携带者",
		}
		risk["severe_risk"] = "0%"
		risk["recommendation"] = "子代50%为携带者，无重型患儿风险。建议子女成年后婚育筛查。"
	} else if p1 == "normal" && p2 == "normal" {
		risk["offspring_risk"] = map[string]string{"normal": "100%"}
		risk["severe_risk"] = "0%"
		risk["recommendation"] = "子代无地贫风险。"
	} else {
		risk["offspring_risk"] = map[string]string{"note": fmt.Sprintf("亲本基因型组合 %s × %s 需要更详细的分子诊断信息", p1, p2)}
		risk["recommendation"] = "建议进行专业的遗传咨询。"
	}

	return risk
}

func (t *GeneticRiskCalculator) buildRecommendations(alphaRisk, betaRisk map[string]any) []string {
	recs := make([]string, 0)

	if severe, ok := alphaRisk["severe_risk"].(string); ok && severe != "0%" {
		recs = append(recs, fmt.Sprintf("α-地贫: %s → %s", severe, alphaRisk["recommendation"]))
	}
	if severe, ok := betaRisk["severe_risk"].(string); ok && severe != "0%" {
		recs = append(recs, fmt.Sprintf("β-地贫: %s → %s", severe, betaRisk["recommendation"]))
	}

	if len(recs) == 0 {
		recs = append(recs, "该夫妇组合无重型地贫患儿风险。建议常规产前检查。子女成年后建议进行地贫携带者筛查以便将来婚育指导。")
	}

	return recs
}
