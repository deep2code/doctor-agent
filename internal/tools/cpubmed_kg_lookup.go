package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// CPubMedKGLookup queries the CPubMed-KG knowledge graph.
type CPubMedKGLookup struct {
	store *knowledge.Store
}

// NewCPubMedKGLookup creates the CPubMed-KG lookup tool.
func NewCPubMedKGLookup(store *knowledge.Store) *CPubMedKGLookup {
	return &CPubMedKGLookup{store: store}
}

func (t *CPubMedKGLookup) Name() string {
	return "cpubmed_kg_lookup"
}

func (t *CPubMedKGLookup) Description() string {
	return "查询CPubMed-KG医学知识图谱(基于PubMed文献)。输入疾病名称，返回该疾病的知识三元组：药物治疗、临床表现、辅助检查、高危因素、并发症、病因、预防等关系。数据来源于PubMed文献挖掘，共37,784条三元组，覆盖8种高发疾病(高血压/糖尿病/冠心病/脑卒中/慢阻肺/慢性肾病/肝硬化/肺癌)。当用户询问某种疾病的治疗方案、检查项目、危险因素时使用。"
}

func (t *CPubMedKGLookup) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"disease": map[string]any{
				"type":        "string",
				"description": "疾病名称，如 '高血压'、'糖尿病'",
			},
			"relation": map[string]any{
				"type":        "string",
				"description": "可选：关系类型过滤，如 '药物治疗'、'临床表现'、'辅助检查'",
			},
		},
		"required": []string{"disease"},
	}
}

func (t *CPubMedKGLookup) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	disease, ok := input["disease"].(string)
	if !ok || strings.TrimSpace(disease) == "" {
		return &ToolResult{Success: false, Error: "请提供疾病名称 disease"}, nil
	}

	disease = strings.TrimSpace(disease)
	relation, _ := input["relation"].(string)

	// Get triples by head entity
	triples := t.store.GetCPubMedTriplesByHead(disease)
	if len(triples) == 0 {
		// Try substring search
		triples = t.store.SearchCPubMedTriples(disease, 100)
	}

	// Filter by relation if specified
	if relation != "" {
		var filtered []*knowledge.CPubMedTriple
		for _, triple := range triples {
			if strings.Contains(triple.Relation, relation) {
				filtered = append(filtered, triple)
			}
		}
		triples = filtered
	}

	if len(triples) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"disease": disease, "result_count": 0,
				"message": fmt.Sprintf("CPubMed-KG(37,784条三元组,8种疾病)中未找到 '%s' 的相关知识。该知识图谱目前覆盖：高血压、糖尿病、冠心病、脑卒中、慢阻肺、慢性肾病、肝硬化、肺癌。", disease),
			},
		}, nil
	}

	// Group by relation
	relationMap := make(map[string][]string)
	for _, triple := range triples {
		relationMap[triple.Relation] = append(relationMap[triple.Relation], triple.Tail)
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"disease":      disease,
			"result_count": len(triples),
			"relations":    relationMap,
			"source":       "CPubMed-KG (PubMed文献挖掘)",
		},
	}, nil
}
