package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// ICD10Lookup queries the ICD-10 disease classification.
type ICD10Lookup struct {
	store *knowledge.Store
}

// NewICD10Lookup creates the ICD-10 lookup tool.
func NewICD10Lookup(store *knowledge.Store) *ICD10Lookup {
	return &ICD10Lookup{store: store}
}

func (t *ICD10Lookup) Name() string {
	return "icd10_lookup"
}

func (t *ICD10Lookup) Description() string {
	return "在国家临床版2.0疾病诊断编码(ICD-10)中查询疾病编码。输入疾病中文名或ICD-10编码，返回对应的编码和疾病名称。当用户询问某疾病的ICD-10编码、或需要查询标准疾病分类时使用。"
}

func (t *ICD10Lookup) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "疾病中文名或ICD-10编码，如 '糖尿病' 或 'E11'",
			},
		},
		"required": []string{"query"},
	}
}

func (t *ICD10Lookup) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return &ToolResult{Success: false, Error: "请提供疾病名或编码 query"}, nil
	}

	query = strings.TrimSpace(query)

	// Direct code lookup
	if d := t.store.GetICD10DiseaseByCode(query); d != nil {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query,
				"result_count": 1,
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
	var matches []map[string]any
	for _, d := range diseases {
		matches = append(matches, map[string]any{
			"icd10_code": d.Code,
			"name_zh":    d.NameZH,
			"category":   d.Category,
		})
	}

	if len(matches) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "result_count": 0,
				"message": fmt.Sprintf("ICD-10编码库(35,862条)中未找到 '%s'。请确认疾病名称或尝试更简短的关键词。", query),
			},
		}, nil
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "result_count": len(matches), "results": matches,
		},
	}, nil
}
