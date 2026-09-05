package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// KnowledgeSearch is the unified knowledge retrieval tool that replaces ~20
// specialized retrieval tools (reference_lookup, msd_search, nhc_search,
// fhs_search, aap_search, medline_search, literature_search,
// disease_encyclopedia_lookup, huatuo_qa_lookup, medical_qa_lookup,
// body_part_lookup, milestone_lookup, newborn_care_lookup).
//
// It dispatches to the appropriate retrieval backend based on the dataset
// parameter, combining the hybrid retriever (keyword + vector) for general
// medical knowledge and the keyword retriever for specialized corpora.
type KnowledgeSearch struct {
	store            *knowledge.Store
	retriever        knowledge.Retriever   // hybrid retriever for general medical knowledge
	keywordRetriever *knowledge.KeywordRetriever // for specialized corpora (MSD, NHC, etc.)
}

// NewKnowledgeSearch creates the unified knowledge search tool.
// The retriever should be the agent's hybrid retriever (keyword + vector);
// the store is used for structured-data lookups and to build a keyword
// retriever for specialized corpora.
func NewKnowledgeSearch(store *knowledge.Store, retriever knowledge.Retriever) *KnowledgeSearch {
	return &KnowledgeSearch{
		store:            store,
		retriever:        retriever,
		keywordRetriever: knowledge.NewRetriever(store),
	}
}

func (t *KnowledgeSearch) Name() string { return "knowledge_search" }

func (t *KnowledgeSearch) Description() string {
	return "统一医学知识检索工具，可跨多个数据集检索。输入检索关键词，选择数据集类型（默认 medical），返回匹配的知识条目。" +
		"支持的数据集：medical=通用医学知识库(疾病/药物/急救/食物风险/检验), " +
		"msd=默沙东诊疗手册中文版, nhc=国家卫健委诊疗方案, fhs=香港家庭健康服务育儿百科, " +
		"aap=美国儿科学会育儿百科, medline=MedlinePlus医学百科, literature=欧洲PMC文献, " +
		"disease_encyclopedia=疾病百科(CMeKG 8807种疾病), huatuo_qa=华佗医疗问答(177K条), " +
		"medical_qa=中文医疗问答(50万条), body_part=人体部位分诊, " +
		"milestone=儿童发育里程碑, newborn_care=新生儿护理与筛查。"
}

func (t *KnowledgeSearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "检索关键词，支持中英文，如 '地中海贫血'、'G6PD deficiency'、'登革热诊断'、'高血压 治疗'",
			},
			"dataset": map[string]any{
				"type":        "string",
				"description": "检索数据集（默认 medical）: medical, msd, nhc, fhs, aap, medline, literature, disease_encyclopedia, huatuo_qa, medical_qa, body_part, milestone, newborn_care",
			},
			"top_k": map[string]any{
				"type":        "integer",
				"description": "返回结果数量（默认 5，最大 10）",
			},
			"department": map[string]any{
				"type":        "string",
				"description": "科室筛选（仅 huatuo_qa/medical_qa 数据集使用，可选）",
			},
			"age_months": map[string]any{
				"type":        "integer",
				"description": "月龄（仅 milestone 数据集使用）：提供时返回该月龄对应的发育里程碑清单",
			},
		},
		"required": []string{"query"},
	}
}

func (t *KnowledgeSearch) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return &ToolResult{Success: false, Error: "请提供检索关键词 query"}, nil
	}
	query = strings.TrimSpace(query)

	dataset, _ := input["dataset"].(string)
	if dataset == "" {
		dataset = "medical"
	}

	topK := 5
	if v, ok := input["top_k"].(float64); ok && int(v) >= 1 && int(v) <= 10 {
		topK = int(v)
	}

	dept, _ := input["department"].(string)
	ageMonths := -1
	if v, ok := input["age_months"].(float64); ok {
		ageMonths = int(v)
	}

	switch dataset {
	case "medical", "":
		return t.searchMedical(ctx, query, topK)
	case "msd":
		return t.searchMSD(ctx, query, topK)
	case "nhc":
		return t.searchNHC(ctx, query, topK)
	case "fhs":
		return t.searchFHS(ctx, query, topK)
	case "aap":
		return t.searchAAP(ctx, query, topK)
	case "medline":
		return t.searchMedline(ctx, query, topK)
	case "literature":
		return t.searchLiterature(ctx, query, topK)
	case "disease_encyclopedia":
		return t.searchDiseaseEncyclopedia(query, topK)
	case "huatuo_qa":
		return t.searchHuatuoQA(query, dept, topK)
	case "medical_qa":
		return t.searchMedicalQA(query, dept, topK)
	case "body_part":
		return t.searchBodyPart(query)
	case "milestone":
		return t.searchMilestone(ctx, query, ageMonths)
	case "newborn_care":
		return t.searchNewbornCare(ctx, query, topK)
	default:
		return &ToolResult{Success: false, Error: fmt.Sprintf(
			"未知数据集 '%s'，支持: medical, msd, nhc, fhs, aap, medline, literature, disease_encyclopedia, huatuo_qa, medical_qa, body_part, milestone, newborn_care", dataset)}, nil
	}
}

// ---- dataset-specific search methods ----

// searchMedical uses the hybrid retriever for general medical knowledge.
func (t *KnowledgeSearch) searchMedical(ctx context.Context, query string, topK int) (*ToolResult, error) {
	results, _ := t.retriever.Retrieve(ctx, query, topK)
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "dataset": "medical", "result_count": 0,
				"message": fmt.Sprintf("通用医学知识库中未找到与 '%s' 直接相关的条目。建议尝试不同的关键词或换用其他数据集。", query),
			},
		}, nil
	}

	entries := make([]map[string]any, 0, len(results))
	citations := make([]CitationRef, 0)
	for _, result := range results {
		entry := result.Entry
		item := map[string]any{
			"condition_zh":    entry.ConditionZH,
			"condition_en":    entry.ConditionEN,
			"category":        entry.Category,
			"relevance_score": result.Score,
		}
		if entry.ICD10 != "" {
			item["icd10"] = entry.ICD10
		}
		if len(entry.Regions) > 0 {
			item["regions"] = entry.Regions
		}
		if len(entry.Treatment) > 0 {
			treatments := make([]map[string]any, 0, len(entry.Treatment))
			for _, tx := range entry.Treatment {
				treatments = append(treatments, map[string]any{
					"method":         tx.Method,
					"indication":     tx.Indication,
					"evidence_level": tx.EvidenceLevel,
					"notes":          tx.Notes,
				})
			}
			item["treatment"] = treatments
		}
		if len(entry.RiskFactors) > 0 {
			item["risk_factors"] = entry.RiskFactors
		}
		if len(entry.Complications) > 0 {
			item["complications"] = entry.Complications
		}
		if len(entry.Prevention) > 0 {
			item["prevention"] = entry.Prevention
		}
		if len(entry.WhenToSeekCare) > 0 {
			item["when_to_seek_care"] = entry.WhenToSeekCare
		}
		if len(entry.DifferentialDiagnosis) > 0 {
			item["differential_diagnosis"] = entry.DifferentialDiagnosis
		}

		// Citations
		citeList := make([]map[string]any, 0)
		for _, c := range entry.Citations {
			citeList = append(citeList, map[string]any{
				"title":       c.Title,
				"journal":     c.Journal,
				"year":        c.Year,
				"doi":         c.DOI,
				"pmid":        c.PMID,
				"level":       c.Level,
				"level_label": evidenceLevelLabel(c.Level),
			})
			citations = append(citations, CitationRef{
				Title: c.Title, DOI: c.DOI, PMID: c.PMID, Level: c.Level, Year: c.Year,
			})
		}
		item["citations"] = citeList
		entries = append(entries, item)
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query":        query,
			"dataset":      "medical",
			"result_count": len(results),
			"results":      entries,
		},
		Citations: citations,
	}, nil
}

// searchMSD searches the MSD Manual (默沙东诊疗手册) Chinese edition.
func (t *KnowledgeSearch) searchMSD(ctx context.Context, query string, topK int) (*ToolResult, error) {
	results, _ := t.keywordRetriever.RetrieveMSD(ctx, query, topK)
	if len(results) == 0 {
		return emptyResult(query, "msd", "默沙东诊疗手册中未找到与 '%s' 直接相关的章节。")
	}
	pages := make([]map[string]any, 0, len(results))
	for _, r := range results {
		pages = append(pages, map[string]any{
			"title":     r.Entry.Title,
			"url":       r.Entry.URL,
			"updated":   r.Entry.Updated,
			"source":    r.Entry.Source,
			"content":   truncateRunes(r.Entry.Content, 2000),
			"relevance": r.Score,
		})
	}
	return successResult(query, "msd", pages), nil
}

// searchNHC searches National Health Commission clinical guidelines.
func (t *KnowledgeSearch) searchNHC(ctx context.Context, query string, topK int) (*ToolResult, error) {
	results, _ := t.keywordRetriever.RetrieveNHCGuide(ctx, query, topK)
	if len(results) == 0 {
		return emptyResult(query, "nhc", "国家卫健委诊疗方案中未找到与 '%s' 直接相关的条目。")
	}
	pages := make([]map[string]any, 0, len(results))
	for _, r := range results {
		pages = append(pages, map[string]any{
			"title":     r.Guide.Title,
			"url":       r.Guide.URL,
			"year":      r.Guide.Year,
			"source":    r.Guide.Source,
			"content":   truncateRunes(r.Guide.Content, 2000),
			"relevance": r.Score,
		})
	}
	return successResult(query, "nhc", pages), nil
}

// searchFHS searches Hong Kong Family Health Service parenting guides.
func (t *KnowledgeSearch) searchFHS(ctx context.Context, query string, topK int) (*ToolResult, error) {
	results, _ := t.keywordRetriever.RetrieveFHSGuide(ctx, query, topK)
	if len(results) == 0 {
		return emptyResult(query, "fhs", "香港家庭健康服务育儿百科中未找到与 '%s' 直接相关的条目。")
	}
	pages := make([]map[string]any, 0, len(results))
	for _, r := range results {
		pages = append(pages, map[string]any{
			"title":     r.Guide.Title,
			"url":       r.Guide.URL,
			"content":   truncateRunes(r.Guide.Content, 2000),
			"relevance": r.Score,
		})
	}
	return successResult(query, "fhs", pages), nil
}

// searchAAP searches American Academy of Pediatrics articles.
func (t *KnowledgeSearch) searchAAP(ctx context.Context, query string, topK int) (*ToolResult, error) {
	results, _ := t.keywordRetriever.RetrieveAAP(ctx, query, topK)
	if len(results) == 0 {
		return emptyResult(query, "aap", "美国儿科学会育儿百科中未找到与 '%s' 直接相关的条目。")
	}
	pages := make([]map[string]any, 0, len(results))
	for _, r := range results {
		pages = append(pages, map[string]any{
			"title":     r.Entry.Title,
			"url":       r.Entry.URL,
			"content":   truncateRunes(r.Entry.Content, 2000),
			"relevance": r.Score,
		})
	}
	return successResult(query, "aap", pages), nil
}

// searchMedline searches MedlinePlus medical encyclopedia.
func (t *KnowledgeSearch) searchMedline(ctx context.Context, query string, topK int) (*ToolResult, error) {
	results, _ := t.keywordRetriever.RetrieveMedlinePlus(ctx, query, topK)
	if len(results) == 0 {
		return emptyResult(query, "medline", "MedlinePlus医学百科中未找到与 '%s' 直接相关的条目。")
	}
	pages := make([]map[string]any, 0, len(results))
	for _, r := range results {
		pages = append(pages, map[string]any{
			"title":     r.Entry.Title,
			"url":       r.Entry.URL,
			"content":   truncateRunes(r.Entry.Content, 2000),
			"relevance": r.Score,
		})
	}
	return successResult(query, "medline", pages), nil
}

// searchLiterature searches Europe PMC literature abstracts.
func (t *KnowledgeSearch) searchLiterature(ctx context.Context, query string, topK int) (*ToolResult, error) {
	results, _ := t.keywordRetriever.RetrieveLiterature(ctx, query, topK)
	if len(results) == 0 {
		return emptyResult(query, "literature", "欧洲PMC文献库中未找到与 '%s' 直接相关的文献。")
	}
	articles := make([]map[string]any, 0, len(results))
	citations := make([]CitationRef, 0)
	for _, r := range results {
		articles = append(articles, map[string]any{
			"title":     r.Entry.Title,
			"abstract":  truncateRunes(r.Entry.Abstract, 500),
			"journal":   r.Entry.Journal,
			"year":      r.Entry.Year,
			"doi":       r.Entry.DOI,
			"pmid":      r.Entry.PMID,
			"topic":     r.Topic.NameZH,
			"relevance": r.Score,
		})
		if r.Entry.DOI != "" || r.Entry.PMID != "" {
			citations = append(citations, CitationRef{
				Title: r.Entry.Title, DOI: r.Entry.DOI, PMID: r.Entry.PMID, Year: r.Entry.Year,
			})
		}
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query":        query,
			"dataset":      "literature",
			"result_count": len(articles),
			"results":      articles,
		},
		Citations: citations,
	}, nil
}

// searchDiseaseEncyclopedia searches the CMeKG disease encyclopedia (8,807 diseases).
func (t *KnowledgeSearch) searchDiseaseEncyclopedia(query string, topK int) (*ToolResult, error) {
	// 1. exact name match
	if d := t.store.GetDiseaseEncyclopediaByName(query); d != nil {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query":        query,
				"dataset":      "disease_encyclopedia",
				"result_count": 1,
				"results": []map[string]any{{
					"name":                  d.NameZH,
					"description":           d.Description,
					"category":              d.Category,
					"symptoms":              d.Symptoms,
					"etiology":              d.Etiology,
					"prevention":            d.Prevention,
					"treatment_methods":     d.TreatmentMethods,
					"treatment_departments": d.TreatmentDepartments,
					"common_drugs":          d.CommonDrugs,
					"recommended_drugs":     d.RecommendedDrugs,
					"recommended_foods":     d.RecommendedFoods,
					"foods_to_avoid":        d.FoodsToAvoid,
					"complications":         d.Complications,
					"diagnostic_tests":      d.DiagnosticTests,
					"treatment_duration":    d.TreatmentDuration,
					"cure_rate":             d.CureRate,
					"cost_estimate":         d.CostEstimate,
					"high_risk_groups":      d.HighRiskGroups,
					"incidence_rate":        d.IncidenceRate,
				}},
			},
		}, nil
	}
	// 2. fuzzy name search
	diseases := t.store.SearchDiseaseEncyclopedias(query, topK)
	if len(diseases) == 0 {
		return emptyResult(query, "disease_encyclopedia",
			"疾病百科(8807种)中未找到 '%s'。请确认疾病名称或尝试更简短的关键词。")
	}
	matches := make([]map[string]any, 0, len(diseases))
	for _, d := range diseases {
		matches = append(matches, map[string]any{
			"name":        d.NameZH,
			"description": truncateRunes(d.Description, 100),
			"symptoms":    d.Symptoms,
		})
	}
	return successResult(query, "disease_encyclopedia", matches), nil
}

// searchHuatuoQA searches the Huatuo26M-Lite medical QA dataset (177K pairs).
func (t *KnowledgeSearch) searchHuatuoQA(query, dept string, limit int) (*ToolResult, error) {
	pairs := t.store.GetHuatuoQA()
	if pairs == nil {
		return &ToolResult{Success: false, Error: "Huatuo QA data not loaded"}, nil
	}
	if limit > 20 {
		limit = 20
	}

	lq := strings.ToLower(query)
	type qaResult struct {
		ID       int
		Question string
		Answer   string
		Dept     string
		Score    int
		Disease  string
	}
	var results []qaResult

	for _, qa := range pairs.QAPairs {
		if dept != "" && qa.Department != dept {
			continue
		}
		score := 0
		q := strings.ToLower(qa.Question)
		a := strings.ToLower(qa.Answer)
		d := strings.ToLower(qa.RelatedDiseases)
		if strings.Contains(q, lq) || strings.Contains(d, lq) {
			score += 10
		}
		if strings.Contains(a, lq) {
			score += 5
		}
		for _, word := range strings.Fields(lq) {
			if len(word) >= 2 {
				if strings.Contains(q, word) {
					score += 3
				}
				if strings.Contains(d, word) {
					score += 2
				}
			}
		}
		if score > 0 {
			results = append(results, qaResult{
				ID: qa.ID, Question: qa.Question, Answer: qa.Answer,
				Dept: qa.Department, Score: score, Disease: qa.RelatedDiseases,
			})
		}
	}

	// Selection sort by score descending
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	if len(results) > limit {
		results = results[:limit]
	}
	if len(results) == 0 {
		return emptyResult(query, "huatuo_qa", "华佗医疗问答库中未找到与 '%s' 相关的问答。")
	}

	items := make([]map[string]any, 0, len(results))
	for _, r := range results {
		items = append(items, map[string]any{
			"id":            r.ID,
			"department":    r.Dept,
			"question":      truncate(r.Question, 100),
			"answer":        truncate(r.Answer, 300),
			"related_disease": r.Disease,
			"score":          r.Score,
		})
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query":        query,
			"dataset":      "huatuo_qa",
			"result_count": len(items),
			"results":      items,
		},
		Citations: []CitationRef{
			{ID: "huatuo26m-lite", Title: "Huatuo26M-Lite Medical QA Dataset", Level: "community"},
		},
	}, nil
}

// searchMedicalQA searches the Chinese medical QA dataset (500K pairs).
func (t *KnowledgeSearch) searchMedicalQA(query, dept string, limit int) (*ToolResult, error) {
	data := t.store.GetMedicalQA()
	if data == nil {
		return &ToolResult{Success: false, Error: "Medical QA data not loaded"}, nil
	}
	if limit > 20 {
		limit = 20
	}

	lq := strings.ToLower(query)
	type qaResult struct {
		Question string
		Answer   string
		Dept     string
		Score    int
	}
	var results []qaResult

	for _, qa := range data.QAPairs {
		if dept != "" && qa.Department != dept {
			continue
		}
		score := 0
		q := strings.ToLower(qa.Question)
		a := strings.ToLower(qa.Answer)
		if strings.Contains(q, lq) {
			score += 10
		}
		if strings.Contains(a, lq) {
			score += 5
		}
		for _, word := range strings.Fields(lq) {
			if len(word) >= 2 && strings.Contains(q, word) {
				score += 3
			}
		}
		if score > 0 {
			results = append(results, qaResult{
				Question: qa.Question, Answer: qa.Answer,
				Dept: qa.Department, Score: score,
			})
		}
	}

	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	if len(results) > limit {
		results = results[:limit]
	}
	if len(results) == 0 {
		return emptyResult(query, "medical_qa", "中文医疗问答库中未找到与 '%s' 相关的问答。")
	}

	items := make([]map[string]any, 0, len(results))
	for _, r := range results {
		items = append(items, map[string]any{
			"department": r.Dept,
			"question":   truncate(r.Question, 100),
			"answer":     truncate(r.Answer, 300),
			"score":      r.Score,
		})
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query":        query,
			"dataset":      "medical_qa",
			"result_count": len(items),
			"results":      items,
		},
		Citations: []CitationRef{
			{ID: "medical-qa", Title: "Chinese Medical Dialogue Dataset", Level: "community"},
		},
	}, nil
}

// searchBodyPart looks up body-part triage info (conditions, red flags, departments).
func (t *KnowledgeSearch) searchBodyPart(query string) (*ToolResult, error) {
	part := strings.TrimSpace(query)
	parts := t.store.GetAllBodyParts()
	if len(parts) == 0 {
		return &ToolResult{Success: false, Error: "人体部位知识库未加载"}, nil
	}

	// 1. exact part_key match
	if e := t.store.GetBodyPartByKey(part); e != nil {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query":        query,
				"dataset":      "body_part",
				"result_count": 1,
				"results":      []map[string]any{bodyPartData(e)},
			},
			Citations: toCitationRefs(e.Citations),
		}, nil
	}

	// 2. normalized match
	norm := normalizePart(part)
	for i := range parts {
		e := &parts[i]
		if normalizePart(e.PartZH) == norm || normalizePart(e.PartKey) == norm {
			return &ToolResult{
				Success: true,
				Data: map[string]any{
					"query": query, "dataset": "body_part", "result_count": 1,
					"results": []map[string]any{bodyPartData(e)},
				},
				Citations: toCitationRefs(e.Citations),
			}, nil
		}
		for _, a := range e.Aliases {
			if normalizePart(a) == norm {
				return &ToolResult{
					Success: true,
					Data: map[string]any{
						"query": query, "dataset": "body_part", "result_count": 1,
						"results": []map[string]any{bodyPartData(e)},
					},
					Citations: toCitationRefs(e.Citations),
				}, nil
			}
		}
	}

	// 3. substring match
	for i := range parts {
		e := &parts[i]
		if strings.Contains(part, e.PartZH) || strings.Contains(e.PartZH, part) {
			return &ToolResult{
				Success: true,
				Data: map[string]any{
					"query": query, "dataset": "body_part", "result_count": 1,
					"results": []map[string]any{bodyPartData(e)},
				},
				Citations: toCitationRefs(e.Citations),
			}, nil
		}
		for _, a := range e.Aliases {
			if strings.Contains(part, a) || strings.Contains(a, part) {
				return &ToolResult{
					Success: true,
					Data: map[string]any{
						"query": query, "dataset": "body_part", "result_count": 1,
						"results": []map[string]any{bodyPartData(e)},
					},
					Citations: toCitationRefs(e.Citations),
				}, nil
			}
		}
	}

	return &ToolResult{
		Success: false,
		Error:   fmt.Sprintf("未找到部位 '%s' 的分诊信息，请换一种说法（如 '腹部'、'胸部'、'腿部'）", part),
	}, nil
}

// searchMilestone searches CDC developmental milestones by keyword or age.
func (t *KnowledgeSearch) searchMilestone(ctx context.Context, query string, ageMonths int) (*ToolResult, error) {
	data := map[string]any{}

	if ageMonths >= 0 {
		checklist, err := t.keywordRetriever.RetrieveMilestones(ctx, ageMonths)
		if err != nil {
			return &ToolResult{Success: false, Error: err.Error()}, nil
		}
		data["checklist"] = checklist
	}
	if query != "" {
		matches, err := t.keywordRetriever.SearchMilestones(ctx, query, 8)
		if err != nil {
			return &ToolResult{Success: false, Error: err.Error()}, nil
		}
		data["matches"] = matches
		data["match_count"] = len(matches)
	}
	if len(data) == 0 {
		return &ToolResult{Success: false, Error: "请提供 query（里程碑关键词）或 age_months（月龄）"}, nil
	}
	data["query"] = query
	data["dataset"] = "milestone"
	data["definition"] = "发育里程碑：大多数（75% 或更多）儿童在该年龄能做到的行为；若孩子未达成某项、丧失已会技能或家长有担忧，应尽早与医生沟通并要求发育筛查"
	return &ToolResult{
		Success: true,
		Data:    data,
		Citations: []CitationRef{
			{ID: "cdc-milestones", Title: "CDC Developmental Milestones (Learn the Signs. Act Early., 2022 revision)", Level: "public_health_authority", Year: 2022},
		},
	}, nil
}

// searchNewbornCare searches WHO preterm/LBW recommendations and China newborn screening.
func (t *KnowledgeSearch) searchNewbornCare(ctx context.Context, query string, topK int) (*ToolResult, error) {
	results, _ := t.keywordRetriever.SearchNewbornCare(ctx, query, topK)
	if len(results) == 0 {
		return emptyResult(query, "newborn_care", "新生儿护理知识库中未找到与 '%s' 直接相关的条目。")
	}
	items := make([]map[string]any, 0, len(results))
	for _, r := range results {
		items = append(items, map[string]any{
			"kind":      r.Kind,
			"id":        r.ID,
			"title_zh":  r.TitleZH,
			"domain":    r.Domain,
			"body_zh":   r.BodyZH,
			"body_en":   r.BodyEN,
			"strength":  r.Strength,
			"url":       r.URL,
			"relevance": r.Score,
		})
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query":        query,
			"dataset":      "newborn_care",
			"result_count": len(items),
			"results":      items,
		},
		Citations: []CitationRef{
			{ID: "who-preterm-lbw-2022", Title: "WHO recommendations for care of the preterm or low-birth-weight infant", Level: "who_guideline", Year: 2022},
			{ID: "cn-nbs", Title: "新生儿疾病筛查管理办法/新生儿先天性心脏病筛查项目", Level: "national_policy", Year: 2009},
		},
	}, nil
}

// ---- helpers ----

// truncateRunes truncates a string to at most maxRunes Unicode code points,
// appending an ellipsis if truncation occurred.
func truncateRunes(s string, maxRunes int) string {
	if len([]rune(s)) <= maxRunes {
		return s
	}
	return string([]rune(s)[:maxRunes]) + "\u2026"
}

// emptyResult returns a standard "no results" ToolResult for the given dataset.
func emptyResult(query, dataset, format string) (*ToolResult, error) {
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query":        query,
			"dataset":      dataset,
			"result_count": 0,
			"message":      fmt.Sprintf(format, query),
		},
	}, nil
}

// successResult returns a standard success ToolResult with results.
func successResult(query, dataset string, results []map[string]any) *ToolResult {
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query":        query,
			"dataset":      dataset,
			"result_count": len(results),
			"results":      results,
		},
	}
}
