package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// BodyPartLookup resolves a body region (as picked on the interactive body
// map) into common conditions, red-flag warnings, suggested departments and
// home-care advice. It closes the loop between "I can point to where it
// hurts" and evidence-based guidance.
type BodyPartLookup struct {
	store *knowledge.Store
}

// NewBodyPartLookup creates the body-part triage tool.
func NewBodyPartLookup(store *knowledge.Store) *BodyPartLookup {
	return &BodyPartLookup{store: store}
}

func (t *BodyPartLookup) Name() string {
	return "body_part_lookup"
}

func (t *BodyPartLookup) Description() string {
	return "根据人体部位（正面/背面）查询该部位常见疾病、红旗警示症状、建议就诊科室和家庭护理建议。当用户能指出'哪里不舒服'但说不清疾病名称时使用，例如'右下腹痛'、'膝盖疼'、'腰痛'。"
}

func (t *BodyPartLookup) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"body_part": map[string]any{
				"type":        "string",
				"description": "人体部位，如 '右下腹'、'膝盖'、'腰'、'头部'、'小腿'",
			},
			"side": map[string]any{
				"type":        "string",
				"description": "正面(front)或背面(back)，默认自动匹配",
			},
		},
		"required": []string{"body_part"},
	}
}

func (t *BodyPartLookup) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	part, _ := input["body_part"].(string)
	side, _ := input["side"].(string)
	part = strings.TrimSpace(part)
	if part == "" {
		return &ToolResult{Success: false, Error: "请提供人体部位"}, nil
	}

	parts := t.store.GetAllBodyParts()
	if len(parts) == 0 {
		return &ToolResult{Success: false, Error: "人体部位知识库未加载"}, nil
	}

	// 1. exact part_key match
	if e := t.store.GetBodyPartByKey(part); e != nil {
		if side == "" || e.Side == side {
			return &ToolResult{Success: true, Data: bodyPartData(e), Citations: toCitationRefs(e.Citations)}, nil
		}
	}

	// 2. normalized match: strip 部位/痛/疼/不适 suffixes, compare zh name & aliases
	norm := normalizePart(part)
	for i := range parts {
		e := &parts[i]
		if normalizePart(e.PartZH) == norm ||
			normalizePart(e.PartKey) == norm {
			if side != "" && e.Side != side {
				continue
			}
			return &ToolResult{Success: true, Data: bodyPartData(e), Citations: toCitationRefs(e.Citations)}, nil
		}
		for _, a := range e.Aliases {
			if normalizePart(a) == norm {
				if side != "" && e.Side != side {
					continue
				}
				return &ToolResult{Success: true, Data: bodyPartData(e), Citations: toCitationRefs(e.Citations)}, nil
			}
		}
	}

	// 3. substring match (e.g. "左小腿" -> 小腿)
	for i := range parts {
		e := &parts[i]
		if strings.Contains(part, e.PartZH) || strings.Contains(e.PartZH, part) {
			if side != "" && e.Side != side {
				continue
			}
			return &ToolResult{Success: true, Data: bodyPartData(e), Citations: toCitationRefs(e.Citations)}, nil
		}
		for _, a := range e.Aliases {
			if strings.Contains(part, a) || strings.Contains(a, part) {
				if side != "" && e.Side != side {
					continue
				}
				return &ToolResult{Success: true, Data: bodyPartData(e), Citations: toCitationRefs(e.Citations)}, nil
			}
		}
	}

	return &ToolResult{
		Success: false,
		Error:   fmt.Sprintf("未找到部位 '%s' 的分诊信息，请换一种说法（如 '腹部'、'胸部'、'腿部'）或提供更具体的位置", part),
	}, nil
}

// normalizePart strips common suffixes/prefixes so fuzzy matches are robust.
func normalizePart(s string) string {
	s = strings.TrimSpace(s)
	for _, suf := range []string{"部位", "区域", "里面", "前面", "后面", "区域痛"} {
		s = strings.TrimSuffix(s, suf)
	}
	s = strings.TrimSuffix(s, "疼痛")
	s = strings.TrimSuffix(s, "不舒服")
	s = strings.TrimSuffix(s, "不适")
	s = strings.TrimSuffix(s, "酸痛")
	s = strings.TrimSuffix(s, "痛")
	s = strings.TrimSuffix(s, "疼")
	s = strings.TrimSuffix(s, "的")
	// 部 as a single char: 腹部/头部/胸部/颈部/背部 all collapse to their stem
	// so cross-form queries (腹痛 vs 腹部) match. Order matters: 部位 above first.
	s = strings.TrimSuffix(s, "部")
	return s
}

func bodyPartData(e *knowledge.BodyPartTriage) map[string]any {
	return map[string]any{
		"part":        e.PartZH,
		"side":        e.Side,
		"conditions":  e.Conditions,
		"red_flags":   e.RedFlags,
		"departments": e.Departments,
		"self_care":   e.SelfCare,
	}
}

func toCitationRefs(cs []knowledge.Citation) []CitationRef {
	refs := make([]CitationRef, 0, len(cs))
	for _, c := range cs {
		refs = append(refs, CitationRef{
			Title: c.Title,
			DOI:   c.DOI,
			PMID:  c.PMID,
			Level: c.Level,
			Year:  c.Year,
		})
	}
	return refs
}
