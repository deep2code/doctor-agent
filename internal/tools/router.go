package tools

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// QueryCategory represents a classified query domain for tool routing.
type QueryCategory string

const (
	CatDrug      QueryCategory = "drug"
	CatSymptom   QueryCategory = "symptom"
	CatGenetic   QueryCategory = "genetic"
	CatLab       QueryCategory = "lab"
	CatChildcare QueryCategory = "childcare"
	CatDisease   QueryCategory = "disease"
	CatLitera    QueryCategory = "literature"
	CatImage     QueryCategory = "image"
	CatGeneral    QueryCategory = "general"
)

// toolGroups maps each category to relevant tool names.
// Each group is capped at ~8 tools to prevent LLM decision paralysis.
var toolGroups = map[QueryCategory][]string{
	CatDrug: {
		"drug_safety_check", "drug_lookup", "drug_interaction_check",
		"eml_lookup", "nmpa_drug_lookup", "drug_label_lookup",
		"sider_lookup", "disease_drug_lookup",
	},
	CatSymptom: {
		"symptom_triage", "triage_department", "body_part_lookup",
		"disease_symptom_lookup", "disease_encyclopedia_lookup",
		"msd_search", "huatuo_qa_lookup",
	},
	CatGenetic: {
		"genetic_risk_calculator", "variant_lookup",
		"drug_safety_check", "food_risk_analyzer",
	},
	CatLab: {
		"lab_interpreter", "lab_report_interpret", "icd10_lookup",
	},
	CatChildcare: {
		"fhs_search", "aap_search", "growth_assessment",
		"milestone_lookup", "newborn_care_lookup", "nhc_search",
	},
	CatDisease: {
		"disease_encyclopedia_lookup", "medical_kg_lookup",
		"cpubmed_kg_lookup", "icd10_lookup", "msd_search",
		"nhc_search", "target_disease_lookup", "disease_symptom_lookup",
	},
	CatLitera: {
		"literature_search", "medline_search", "reference_lookup",
		"huatuo_qa_lookup", "medical_qa_lookup",
	},
	CatImage: {
		"medical_image_analyze",
	},
	CatGeneral: {
		"symptom_triage", "disease_encyclopedia_lookup", "msd_search",
		"drug_safety_check", "icd10_lookup", "huatuo_qa_lookup",
	},
}

// Router classifies user queries and selects a relevant tool subset.
// This prevents the LLM from seeing all 35 tools at once, which causes
// decision paralysis and tool-call loops.
type Router struct {
	keywords map[QueryCategory][]string
}

// NewRouter creates a keyword-based tool router.
func NewRouter() *Router {
	return &Router{
		keywords: map[QueryCategory][]string{
			CatDrug: {
				"药", "药物", "吃药", "服用", "用药", "服药", "处方", "副作用",
				"不良反应", "禁忌", "过敏", "阿莫西林", "布洛芬", "对乙酰氨基酚",
				"阿司匹林", "头孢", "青霉素", "drug", "medicine", "pill",
				"interaction", "medication", "dose", "dosage", "eml", "nmpa", "fda",
			},
			CatSymptom: {
				"症状", "不舒服", "难受", "疼", "痛", "痒", "晕", "恶心",
				"呕吐", "腹泻", "拉肚子", "发烧", "发热", "咳嗽", "头痛",
				"失眠", "乏力", "胸闷", "气短", "symptom", "fever", "cough",
				"pain", "dizzy",
			},
			CatGenetic: {
				"地贫", "地中海贫血", "g6pd", "蚕豆病", "遗传", "基因", "变异",
				"携带", "基因检测", "thalassemia", "variant", "genetic",
				"mutation", "基因型", "蚕豆", "食物", "能吃", "不能吃", "忌口",
			},
			CatLab: {
				"化验", "检查结果", "血常规", "肝功能", "肾功能", "指标",
				"偏高", "偏低", "正常值", "检验", "报告单", "转氨酶",
				"胆固醇", "血糖", "lab", "test", "blood", "report",
			},
			CatChildcare: {
				"孩子", "宝宝", "婴儿", "幼儿", "儿童", "小儿", "疫苗",
				"接种", "辅食", "育儿", "新生儿", "发育", "生长曲线",
				"child", "baby", "infant", "vaccine", "feeding", "pediatric",
			},
			CatDisease: {
				"疾病", "什么病", "诊断", "病因", "治疗", "预防", "并发症",
				"disease", "diagnosis", "treatment", "prevention",
				"科室", "挂号",
			},
			CatLitera: {
				"文献", "研究", "论文", "pubmed", "证据", "循证", "文献检索",
				"literature", "research", "study", "reference", "期刊",
			},
			CatImage: {
				"图片", "照片", "ct", "x光", "x-ray", "b超", "核磁", "mri",
				"报告单", "化验单", "image", "scan", "upload", "上传",
			},
		},
	}
}

// ClassifyMulti determines which tool groups are relevant to the query.
// It merges tools from all matched categories, deduplicates, and caps at 10.
// Falls back to CatGeneral (6 core tools) when no keywords match.
func (r *Router) ClassifyMulti(query string) []string {
	q := strings.ToLower(query)

	matched := make(map[QueryCategory]bool)
	for cat, words := range r.keywords {
		for _, w := range words {
			if strings.Contains(q, w) {
				matched[cat] = true
				break
			}
		}
	}

	if len(matched) == 0 {
		return toolGroups[CatGeneral]
	}

	// Merge tools from all matched categories, dedup preserving order.
	seen := make(map[string]bool)
	var result []string
	for cat := range matched {
		for _, name := range toolGroups[cat] {
			if !seen[name] {
				seen[name] = true
				result = append(result, name)
			}
		}
	}

	// Cap at 10 tools to keep the LLM decision space manageable.
	if len(result) > 10 {
		result = result[:10]
	}

	if len(result) == 0 {
		return toolGroups[CatGeneral]
	}
	return result
}

// ParamsHash returns a short hash of the tool name + arguments for duplicate
// detection. Two calls to the same tool with the same arguments produce the
// same hash, enabling the agent loop to detect and break redundant calls.
func ParamsHash(toolName string, args map[string]any) string {
	b, _ := json.Marshal(args)
	h := md5.Sum(append([]byte(toolName+":"), b...))
	return hex.EncodeToString(h[:8])
}

// ---- KG-guided two-level routing (MedRAG-inspired) ----
//
// MedRAG (WWW 2025) routes by: query → match symptom nodes in KG → traverse
// to disease nodes → vote on disease category → retrieve. We adapt this to
// doctor-agent's existing KG data (OpenCMKG 354k triples, CMeKG 8807 diseases):
//
//   Level 1: extract symptom keywords from query → reverse-lookup
//            disease_has_symptom triples → vote on disease candidates.
//
//   Level 2: for each disease candidate, inspect available KG relations
//            (recommand_drug, need_check, belong_department, ...) → map
//            relation types to tool groups → merge and cap at 10.
//
// Falls back to keyword-based ClassifyMulti when KG lookup yields nothing.

// relationToTools maps OpenCMKG relation types to relevant tool groups.
var relationToTools = map[string][]string{
	"disease_has_symptom":     {"symptom_triage", "disease_symptom_lookup", "disease_encyclopedia_lookup"},
	"disease_recommand_drug":  {"drug_safety_check", "drug_lookup", "drug_interaction_check", "eml_lookup", "disease_drug_lookup"},
	"disease_common_drug":     {"drug_safety_check", "drug_lookup", "nmpa_drug_lookup", "sider_lookup", "disease_drug_lookup"},
	"disease_recommand_food":  {"food_risk_analyzer"},
	"disease_noteat_food":     {"food_risk_analyzer"},
	"disease_eat_food":        {"food_risk_analyzer"},
	"disease_need_check":      {"lab_interpreter", "lab_report_interpret", "icd10_lookup"},
	"disease_need_treatment": {"disease_encyclopedia_lookup", "nhc_search", "msd_search"},
	"disease_belong_department": {"triage_department", "body_part_lookup"},
	"disease_acompany_disease": {"disease_encyclopedia_lookup", "medical_kg_lookup", "cpubmed_kg_lookup"},
}

// symptomVocabulary is a broader symptom lexicon for KG routing, extending
// the Router's basic CatSymptom keywords with common patient expressions.
var symptomVocabulary = []string{
	"发烧", "发热", "头痛", "头晕", "恶心", "呕吐", "腹泻", "拉肚子",
	"咳嗽", "咳痰", "胸闷", "胸痛", "心悸", "气短", "呼吸困难",
	"腹痛", "肚子痛", "胃痛", "胃胀", "便秘", "便血",
	"皮疹", "瘙痒", "湿疹", "荨麻疹", "红肿",
	"关节痛", "腰痛", "背痛", "四肢无力", "麻木",
	"失眠", "嗜睡", "焦虑", "抑郁", "烦躁",
	"乏力", "疲劳", "盗汗", "低热",
	"黄疸", "水肿", "淋巴结肿大",
	"鼻塞", "流涕", "咽痛", "扁桃体",
	"尿频", "尿急", "尿痛", "血尿",
	"月经不调", "痛经", "白带异常",
	"抽搐", "惊厥", "昏迷", "意识模糊",
	"fever", "cough", "headache", "dizzy", "nausea", "vomiting",
	"diarrhea", "rash", "itching", "fatigue", "pain",
}

// ClassifyKG performs two-level KG-guided tool routing:
//   1. Extract symptom keywords from query.
//   2. Reverse-lookup disease candidates via store.FindDiseasesBySymptom.
//   3. For top disease candidates, inspect KG relations → map to tools.
//   4. Merge, deduplicate, cap at 10.
// Returns tool names; falls back to ClassifyMulti when KG yields nothing.
func (r *Router) ClassifyKG(query string, store *knowledge.Store) []string {
	q := strings.ToLower(query)

	// --- Level 1: symptom → disease candidates ---
	diseaseVotes := make(map[string]int)
	symptomsFound := 0

	for _, sym := range symptomVocabulary {
		if strings.Contains(q, sym) {
			symptomsFound++
			hits := store.FindDiseasesBySymptom(sym, 10)
			for disease, vote := range hits {
				diseaseVotes[disease] += vote
			}
		}
	}

	// Also check the basic CatSymptom keywords for broader coverage.
	for _, kw := range r.keywords[CatSymptom] {
		if strings.Contains(q, kw) {
			symptomsFound++
			hits := store.FindDiseasesBySymptom(kw, 10)
			for disease, vote := range hits {
				diseaseVotes[disease] += vote
			}
		}
	}

	if len(diseaseVotes) == 0 {
		// No KG hit — fall back to keyword-based routing.
		slog.Debug("KG routing: no disease candidates, falling back to keyword routing",
			"query", query)
		return r.ClassifyMulti(query)
	}

	slog.Debug("KG routing: disease candidates found",
		"query", query, "symptoms_matched", symptomsFound,
		"disease_count", len(diseaseVotes))

	// Sort disease candidates by vote count (descending), take top 5.
	type diseaseScore struct {
		name  string
		votes int
	}
	var ranked []diseaseScore
	for name, v := range diseaseVotes {
		ranked = append(ranked, diseaseScore{name, v})
	}
	for i := 0; i < len(ranked); i++ {
		for j := i + 1; j < len(ranked); j++ {
			if ranked[j].votes > ranked[i].votes {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}
	topN := 5
	if len(ranked) < topN {
		topN = len(ranked)
	}

	// --- Level 2: disease → KG relations → tool group mapping ---
	seen := make(map[string]bool)
	var result []string

	// Always include core tools for any medical query.
	coreTools := []string{
		"disease_encyclopedia_lookup", "medical_kg_lookup", "msd_search",
	}
	for _, t := range coreTools {
		if !seen[t] {
			seen[t] = true
			result = append(result, t)
		}
	}

	for i := 0; i < topN; i++ {
		disease := ranked[i].name
		relations := store.GetDiseaseKGRelations(disease)

		for rel := range relations {
			if tools, ok := relationToTools[rel]; ok {
				for _, t := range tools {
					if !seen[t] {
						seen[t] = true
						result = append(result, t)
					}
				}
			}
		}

		// Also check DiseaseEncyclopedia structured fields for extra tools.
		enc := store.GetDiseaseEncyclopedia(disease)
		if enc != nil {
			if len(enc.CommonDrugs) > 0 || len(enc.RecommendedDrugs) > 0 {
				for _, t := range []string{"drug_safety_check", "drug_lookup", "disease_drug_lookup"} {
					if !seen[t] {
						seen[t] = true
						result = append(result, t)
					}
				}
			}
			if len(enc.DiagnosticTests) > 0 {
				for _, t := range []string{"lab_interpreter", "icd10_lookup"} {
					if !seen[t] {
						seen[t] = true
						result = append(result, t)
					}
				}
			}
			if len(enc.FoodsToAvoid) > 0 || len(enc.RecommendedFoods) > 0 {
				if !seen["food_risk_analyzer"] {
					seen["food_risk_analyzer"] = true
					result = append(result, "food_risk_analyzer")
				}
			}
			if len(enc.TreatmentDepartments) > 0 {
				if !seen["triage_department"] {
					seen["triage_department"] = true
					result = append(result, "triage_department")
				}
			}
		}

		// Cap at 10 tools to keep the LLM decision space manageable.
		if len(result) >= 10 {
			break
		}
	}

	if len(result) > 10 {
		result = result[:10]
	}

	if len(result) == 0 {
		return r.ClassifyMulti(query)
	}

	slog.Debug("KG routing result",
		"query", query,
		"top_diseases", topN,
		"tools", result)
	return result
}
