package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// ExactLookup is the unified exact-match / structured-data lookup tool that
// replaces 8 specialized lookup tools:
//   - icd10_lookup      -> ICD-10 disease classification (35,862 entries)
//   - nmpa_drug_lookup   -> NMPA drug catalogue (167,615 entries)
//   - variant_lookup     -> ClinVar pathogenic variants (1,399 entries)
//   - eml_lookup         -> WHO Essential Medicines List (564 drugs)
//   - drug_label_lookup  -> FDA drug labels (Chinese summaries, 344 entries)
//   - ttd_lookup         -> Therapeutic Target Database (4,299 targets + 29,782 drugs)
//   - sider_lookup       -> SIDER side-effect resource (1,430 drugs)
//   - drug_lookup        -> National medical-insurance drug catalogue (1,170 entries)
//
// Each call specifies a "type" parameter selecting the backend, and a "query"
// for the exact or substring match.  Results are returned as structured JSON
// (not pre-formatted strings) so the LLM can compose the final answer.
type ExactLookup struct {
	store            *knowledge.Store
	keywordRetriever *knowledge.KeywordRetriever // for ClinVar / EML / FDA / Medins retriever methods
}

// NewExactLookup creates the unified exact-lookup tool.
// The store provides direct access to structured datasets (ICD-10, NMPA,
// TTD, SIDER); the keywordRetriever is built internally from the store for
// the specialized retriever-based lookups (ClinVar, EML, FDA, Medins).
func NewExactLookup(store *knowledge.Store) *ExactLookup {
	return &ExactLookup{
		store:            store,
		keywordRetriever: knowledge.NewRetriever(store),
	}
}

func (t *ExactLookup) Name() string { return "exact_lookup" }

func (t *ExactLookup) Description() string {
	return "统一精确查询工具，按数据集类型做字段精确/子串匹配查询。" +
		"支持类型: icd10=ICD-10疾病编码(35862条), nmpa=NMPA药品目录(167615种), " +
		"variant=ClinVar基因变异(HBB/HBA1/HBA2/G6PD致病变异), " +
		"eml=WHO基本药物清单(第24版564种), fda_label=FDA药品标签中文要点(344种), " +
		"ttd=治疗靶点数据库(4299靶点+29782药物), sider=药物副作用(1430种药物), " +
		"medins=国家医保药品目录(2024版1170种西药)。" +
		"当需要查询药品编码/批准信息/基因变异/药物副作用/医保类别等精确字段时使用。"
}

func (t *ExactLookup) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "查询关键词：疾病名/编码/药名(中英文)/基因名/变异名/药物ID",
			},
			"type": map[string]any{
				"type":        "string",
				"description": "查询类型: icd10, nmpa, variant, eml, fda_label, ttd, sider, medins",
			},
			"top_k": map[string]any{
				"type":        "integer",
				"description": "返回结果数量上限（默认 5，最大 20）",
			},
			"sub_type": map[string]any{
				"type":        "string",
				"description": "子类型筛选（仅 ttd 使用）: drug 或 target，不传则两者都搜",
			},
		},
		"required": []string{"query", "type"},
	}
}

func (t *ExactLookup) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return &ToolResult{Success: false, Error: "请提供查询关键词 query"}, nil
	}
	query = strings.TrimSpace(query)

	lookupType, _ := input["type"].(string)
	if lookupType == "" {
		return &ToolResult{Success: false, Error: "请提供查询类型 type (icd10/nmpa/variant/eml/fda_label/ttd/sider/medins)"}, nil
	}

	topK := 5
	if v, ok := input["top_k"].(float64); ok && int(v) >= 1 && int(v) <= 20 {
		topK = int(v)
	}

	subType, _ := input["sub_type"].(string)

	switch lookupType {
	case "icd10":
		return t.lookupICD10(ctx, query, topK)
	case "nmpa":
		return t.lookupNMPA(ctx, query, topK)
	case "variant":
		return t.lookupVariant(ctx, query, topK)
	case "eml":
		return t.lookupEML(ctx, query, topK)
	case "fda_label":
		return t.lookupFDALabel(ctx, query, topK)
	case "ttd":
		return t.lookupTTD(ctx, query, topK, subType)
	case "sider":
		return t.lookupSIDER(ctx, query, topK)
	case "medins":
		return t.lookupMedins(ctx, query, topK)
	default:
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("不支持的类型 '%s'，可选: icd10, nmpa, variant, eml, fda_label, ttd, sider, medins", lookupType),
		}, nil
	}
}

// ---------------------------------------------------------------------------
// ICD-10 disease classification (35,862 entries)
// ---------------------------------------------------------------------------

func (t *ExactLookup) lookupICD10(_ context.Context, query string, _ int) (*ToolResult, error) {
	// Direct code lookup first
	if d := t.store.GetICD10DiseaseByCode(query); d != nil {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "type": "icd10", "result_count": 1,
				"results": []map[string]any{
					{
						"icd10_code": d.Code,
						"name_zh":    d.NameZH,
						"category":   d.Category,
					},
				},
			},
		}, nil
	}

	// Name substring search
	diseases := t.store.SearchICD10Diseases(query, 20)
	if len(diseases) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "type": "icd10", "result_count": 0,
				"message": fmt.Sprintf("ICD-10编码库(35,862条)中未找到 '%s'。请确认疾病名称或尝试更简短的关键词。", query),
			},
		}, nil
	}

	results := make([]map[string]any, 0, len(diseases))
	for _, d := range diseases {
		results = append(results, map[string]any{
			"icd10_code": d.Code,
			"name_zh":    d.NameZH,
			"category":   d.Category,
		})
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "type": "icd10", "result_count": len(results), "results": results,
		},
	}, nil
}

// ---------------------------------------------------------------------------
// NMPA drug catalogue (167,615 entries)
// ---------------------------------------------------------------------------

func (t *ExactLookup) lookupNMPA(_ context.Context, query string, topK int) (*ToolResult, error) {
	// Direct name lookup first
	if d := t.store.GetNMPADrugByName(query); d != nil {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "type": "nmpa", "result_count": 1,
				"results": []map[string]any{
					{
						"drug_code": d.Code,
						"name_zh":   d.NameZH,
						"source":    d.Source,
					},
				},
			},
		}, nil
	}

	drugs := t.store.SearchNMPADrugs(query, topK)
	if len(drugs) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "type": "nmpa", "result_count": 0,
				"message": fmt.Sprintf("NMPA药品库(167,615种)中未找到 '%s'。可能为中成药、中药饮片或未收录药品。", query),
			},
		}, nil
	}

	results := make([]map[string]any, 0, len(drugs))
	for _, d := range drugs {
		results = append(results, map[string]any{
			"drug_code": d.Code,
			"name_zh":   d.NameZH,
			"source":    d.Source,
		})
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "type": "nmpa", "result_count": len(results), "results": results,
		},
	}, nil
}

// ---------------------------------------------------------------------------
// ClinVar pathogenic variants (HBB/HBA1/HBA2/G6PD, 1,399 entries)
// ---------------------------------------------------------------------------

func (t *ExactLookup) lookupVariant(ctx context.Context, query string, topK int) (*ToolResult, error) {
	results, _ := t.keywordRetriever.RetrieveClinVar(ctx, query, topK)
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "type": "variant", "result_count": 0,
				"message": fmt.Sprintf("ClinVar子集中未找到与 '%s' 匹配的变异。当前收录 HBB/HBA1/HBA2/G6PD 基因的致病及可能致病变异。", query),
			},
		}, nil
	}

	variants := make([]map[string]any, 0, len(results))
	for _, r := range results {
		v := r.Variant
		variants = append(variants, map[string]any{
			"gene":                  v.Gene,
			"variation":             v.Variation,
			"cdna":                  v.Cdna,
			"clinical_significance": v.ClinicalSignificance,
			"traits":                v.Traits,
			"clinvar_id":            v.ClinVarID,
		})
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "type": "variant", "result_count": len(results), "results": variants,
		},
	}, nil
}

// ---------------------------------------------------------------------------
// WHO Essential Medicines List (24th edition, 564 drugs)
// ---------------------------------------------------------------------------

func (t *ExactLookup) lookupEML(ctx context.Context, query string, topK int) (*ToolResult, error) {
	results, _ := t.keywordRetriever.RetrieveEMLDrug(ctx, query, topK)
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "type": "eml", "result_count": 0,
				"message": fmt.Sprintf("WHO基本药物清单(第24版)中未找到与 '%s' 匹配的药品。清单收录564种药物，可在 https://list.essentialmeds.org 查询完整列表。", query),
			},
		}, nil
	}

	entries := make([]map[string]any, 0, len(results))
	for _, r := range results {
		e := r.Entry
		listLabel := "核心清单"
		if e.List == "complementary" {
			listLabel = "补充清单"
		}
		indications := make([]string, 0, len(e.Indications))
		for _, ind := range e.Indications {
			choiceLabel := "一线"
			switch ind.Choice {
			case "second":
				choiceLabel = "二线"
			case "both":
				choiceLabel = "一线/二线"
			}
			indications = append(indications, fmt.Sprintf("%s: %s", choiceLabel, ind.Text))
		}
		entries = append(entries, map[string]any{
			"name":                    e.Name,
			"name_zh":                 e.NameZH,
			"section":                 e.Section,
			"list":                    listLabel,
			"forms":                   e.Forms,
			"indications":             indications,
			"note":                    e.Note,
			"children_list":           e.Children,
			"square_box_listing":      e.SquareBox,
			"therapeutic_alternatives": e.TherapeuticAlternatives,
		})
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "type": "eml", "result_count": len(results), "results": entries,
		},
	}, nil
}

// ---------------------------------------------------------------------------
// FDA drug labels (Chinese summaries, 344 entries)
// ---------------------------------------------------------------------------

func (t *ExactLookup) lookupFDALabel(ctx context.Context, query string, topK int) (*ToolResult, error) {
	results, _ := t.keywordRetriever.RetrieveFDALabel(ctx, query, topK)
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "type": "fda_label", "result_count": 0,
				"message": fmt.Sprintf("FDA标签库中未找到与 '%s' 匹配的药品。当前收录WHO基本药物清单对应药品的FDA标签中文要点。", query),
			},
		}, nil
	}

	drugs := make([]map[string]any, 0, len(results))
	for _, r := range results {
		d := r.Drug
		drugs = append(drugs, map[string]any{
			"name_zh":           d.NameZH,
			"name_en":           d.NameEN,
			"category":          d.Category,
			"indications":       d.Indications,
			"contraindications": d.Contraindications,
			"warnings":          d.Warnings,
			"interactions":      d.Interactions,
			"adverse_reactions": d.AdverseReactions,
			"dosage":            d.Dosage,
			"source_url":        d.SourceURL,
		})
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "type": "fda_label", "result_count": len(results), "results": drugs,
		},
	}, nil
}

// ---------------------------------------------------------------------------
// Therapeutic Target Database (4,299 targets + 29,782 drugs)
// ---------------------------------------------------------------------------

func (t *ExactLookup) lookupTTD(_ context.Context, query string, topK int, subType string) (*ToolResult, error) {
	data := t.store.GetTTDData()
	if data == nil {
		return &ToolResult{Success: false, Error: "TTD data not loaded"}, nil
	}

	q := strings.ToLower(query)
	var targetResults []map[string]any
	var drugResults []map[string]any

	// Search targets
	if subType == "" || subType == "target" {
		for _, target := range data.Targets {
			if strings.Contains(strings.ToLower(target.Name), q) ||
				strings.Contains(strings.ToLower(target.Uniprot), q) {
				targetResults = append(targetResults, map[string]any{
					"id":      target.ID,
					"name":    target.Name,
					"uniprot": target.Uniprot,
					"type":    target.Type,
				})
				if len(targetResults) >= topK {
					break
				}
			}
		}
	}

	// Search drugs
	if subType == "" || subType == "drug" {
		for _, drug := range data.Drugs {
			matched := false
			if strings.Contains(strings.ToLower(drug.Name), q) {
				matched = true
			}
			if !matched {
				for _, syn := range drug.Synonyms {
					if strings.Contains(strings.ToLower(syn), q) {
						matched = true
						break
					}
				}
			}
			if matched {
				syns := drug.Synonyms
				if len(syns) > 5 {
					syns = syns[:5]
				}
				drugResults = append(drugResults, map[string]any{
					"id":        drug.ID,
					"name":      drug.Name,
					"synonyms":  syns,
				})
				if len(drugResults) >= topK {
					break
				}
			}
		}
	}

	if len(targetResults) == 0 && len(drugResults) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "type": "ttd", "result_count": 0,
				"message": fmt.Sprintf("未找到与'%s'相关的TTD数据", query),
			},
		}, nil
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "type": "ttd",
			"target_count": len(targetResults),
			"drug_count":   len(drugResults),
			"targets":      targetResults,
			"drugs":        drugResults,
		},
		Citations: []CitationRef{
			{ID: "ttd", Title: "Therapeutic Target Database", Level: "database"},
		},
	}, nil
}

// ---------------------------------------------------------------------------
// SIDER side-effect resource (1,430 drugs)
// ---------------------------------------------------------------------------

func (t *ExactLookup) lookupSIDER(_ context.Context, query string, topK int) (*ToolResult, error) {
	data := t.store.GetSIDERData()
	if data == nil {
		return &ToolResult{Success: false, Error: "SIDER data not loaded"}, nil
	}

	q := strings.ToLower(query)
	var matches []map[string]any

	for _, drug := range data.Drugs {
		if strings.Contains(strings.ToLower(drug.ID), q) {
			sideEffects := drug.SideEffects
			if len(sideEffects) > 10 {
				sideEffects = sideEffects[:10]
			}
			indications := drug.Indications
			if len(indications) > 5 {
				indications = indications[:5]
			}
			matches = append(matches, map[string]any{
				"drug_id":      drug.ID,
				"side_effects": sideEffects,
				"indications":  indications,
			})
			if len(matches) >= topK {
				break
			}
		}
	}

	if len(matches) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "type": "sider", "result_count": 0,
				"message": fmt.Sprintf("未找到药物'%s'的SIDER数据", query),
			},
		}, nil
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "type": "sider", "result_count": len(matches), "results": matches,
			"note": "副作用信息仅供参考，具体用药请遵医嘱。",
		},
		Citations: []CitationRef{
			{ID: "sider", Title: "SIDER Side Effect Resource", Level: "database"},
		},
	}, nil
}

// ---------------------------------------------------------------------------
// National medical-insurance drug catalogue (2024, 1,170 western drugs)
// ---------------------------------------------------------------------------

func (t *ExactLookup) lookupMedins(ctx context.Context, query string, topK int) (*ToolResult, error) {
	results, _ := t.keywordRetriever.RetrieveMedinsDrug(ctx, query, topK)
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "type": "medins", "result_count": 0,
				"message": fmt.Sprintf("国家医保药品目录(2024)中未找到 '%s'。可能为目录外药品、中成药或中药饮片(当前仅收录西药部分)。", query),
			},
		}, nil
	}

	drugs := make([]map[string]any, 0, len(results))
	for _, r := range results {
		cat := r.Drug.Category
		catDesc := "乙类(需自付一定比例)"
		if cat == "甲" {
			catDesc = "甲类(全额纳入医保报销)"
		}
		drugs = append(drugs, map[string]any{
			"name":          r.Drug.Name,
			"category":      cat,
			"category_desc": catDesc,
			"forms":         r.Drug.Forms,
		})
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "type": "medins", "result_count": len(results), "results": drugs,
		},
	}, nil
}
