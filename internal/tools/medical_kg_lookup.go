package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// MedicalKGLookup queries the medical knowledge graph (OpenCMKG).
type MedicalKGLookup struct {
	store *knowledge.Store
}

// NewMedicalKGLookup creates the medical KG lookup tool.
func NewMedicalKGLookup(store *knowledge.Store) *MedicalKGLookup {
	return &MedicalKGLookup{store: store}
}

func (t *MedicalKGLookup) Name() string {
	return "medical_kg_lookup"
}

func (t *MedicalKGLookup) Description() string {
	return "查询医学知识图谱(OpenCMKG, 354,752条三元组)。支持13种关系类型：疾病-症状、疾病-推荐药物、疾病-推荐食物、疾病-禁忌食物、疾病-需要检查、疾病-伴随疾病、疾病-治疗方式、疾病-常见药物、药物-生产商、疾病-所属科室。输入实体名称(如'糖尿病'、'布洛芬')和关系类型(可选)，返回相关的三元组。"
}

func (t *MedicalKGLookup) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"entity": map[string]any{
				"type":        "string",
				"description": "实体名称，如 '糖尿病'、'布洛芬'、'发热'",
			},
			"relation": map[string]any{
				"type":        "string",
				"description": "关系类型(可选)，如 'disease_has_symptom'、'disease_recommand_drug'",
				"enum": []string{
					"disease_has_symptom",
					"disease_recommand_drug",
					"disease_recommand_food",
					"disease_noteat_food",
					"disease_need_check",
					"disease_acompany_disease",
					"disease_eat_food",
					"disease_need_treatment",
					"disease_common_drug",
					"disease_belong_department",
				},
			},
		},
		"required": []string{"entity"},
	}
}

func (t *MedicalKGLookup) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	entity, ok := input["entity"].(string)
	if !ok || strings.TrimSpace(entity) == "" {
		return &ToolResult{Success: false, Error: "请提供实体名称 entity"}, nil
	}

	entity = strings.TrimSpace(entity)
	relation, _ := input["relation"].(string)

	// Search for triples containing the entity
	triples := t.store.SearchMedicalKG(entity, relation, 50)
	var matches []map[string]any
	for _, triple := range triples {
		matches = append(matches, map[string]any{
			"entity1":  triple.Entity1,
			"relation": triple.Relation,
			"entity2":  triple.Entity2,
		})
	}

	if len(matches) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"entity": entity, "result_count": 0,
				"message": fmt.Sprintf("知识图谱(354,752条)中未找到与 '%s' 相关的三元组。请确认实体名称或尝试更简短的关键词。", entity),
			},
		}, nil
	}

	// Group by relation type
	byRelation := make(map[string][]map[string]any)
	for _, m := range matches {
		rel := m["relation"].(string)
		byRelation[rel] = append(byRelation[rel], m)
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"entity": entity, "result_count": len(matches),
			"by_relation": byRelation,
		},
	}, nil
}
