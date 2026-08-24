package safety

import "strings"

// ScopeResult indicates whether a query is out of scope.
type ScopeResult struct {
	InScope   bool
	Reason    string // Why it was rejected (if out of scope)
	Redirect  string // Redirect message to show the user
}

// ScopeGuard detects queries that fall outside the agent's domain.
// The agent is specialized in evidence-based clinical medicine for
// Chinese populations (with emphasis on high-burden conditions in southern
// provinces). Queries about veterinary medicine,
// legal advice, drug manufacturing, self-harm, etc. are rejected.
type ScopeGuard struct {
	blockedPatterns []scopePattern
}

type scopePattern struct {
	keywords   []string
	keywordsZH []string
	reason     string
	redirect   string
}

// NewScopeGuard creates a scope guard with pre-defined blocked patterns.
func NewScopeGuard() *ScopeGuard {
	return &ScopeGuard{
		blockedPatterns: []scopePattern{
			{
				keywords:   []string{"veterinary", "cat ", "dog ", "pet ", "animal"},
				keywordsZH: []string{"宠物", "猫", "狗", "兽医", "动物"},
				reason:     "veterinary_medicine",
				redirect:   "本智能体专注于人类循证医学，不提供兽医学建议。请咨询专业兽医。",
			},
			{
				keywords:   []string{"malpractice", "sue ", "lawsuit", "legal advice", "sue my doctor"},
				keywordsZH: []string{"医疗纠纷", "起诉", "医疗事故", "律师", "法律咨询", "赔偿"},
				reason:     "legal_advice",
				redirect:   "本智能体专注于临床医学咨询，不提供法律建议。医疗法律问题请咨询专业律师。",
			},
			{
				keywords:   []string{"synthesize", "manufacture", "illicit", "recreational drug", "meth", "heroin", "cocaine"},
				keywordsZH: []string{"制毒", "合成毒品", "毒品制作", "制毒方法"},
				reason:     "drug_manufacturing",
				redirect:   "本智能体不支持与非法药物制造相关的咨询。如果您有物质滥用问题，建议拨打全国戒毒热线：12355。",
			},
			{
				keywords:   []string{"suicide method", "kill myself", "end my life", "best way to die"},
				keywordsZH: []string{"自杀方法", "怎么死", "自杀方式", "怎么自杀", "想死"},
				reason:     "self_harm",
				redirect:   "如果您有自杀念头，请立即寻求帮助。全国24小时心理援助热线：400-161-9995。北京心理危机研究与干预中心：010-82951332。生命是可贵的——请给专业人士一个帮助您的机会。",
			},
			{
				keywords:   []string{"traditional chinese medicine prescription", "tcm formula", "herbal formula", "acupuncture point"},
				keywordsZH: []string{"中药方", "偏方", "秘方", "祖传", "民间偏方", "土方"},
				reason:     "non_evidence_based",
				redirect:   "本智能体严格遵循循证医学（Evidence-Based Medicine）原则，不提供未经科学验证的中药方剂或民间偏方建议。建议您咨询持证医师，或提供具体症状我们可以基于循证指南进行分析。",
			},
		},
	}
}

// medicalOverrideKeywords: when present in a query it is treated as in-scope
// even if it also matches a blocked pattern, because it describes the user's
// own medical situation. This prevents wrongly refusing core medical questions
// such as rabies/post-exposure care ("狗咬了要打狂犬疫苗吗") as veterinary advice.
var medicalOverrideKeywords = []string{
	"疫苗", "咬伤", "抓伤", "狂犬病", "暴露", "破伤风", "过敏", "中毒",
	"发热", "胸痛", "呼吸困难", "出血", "昏迷", "抽搐", "中风", "心梗",
	"休克", "窒息", "腹痛", "腹泻", "呕吐", "症状", "诊断", "治疗",
	"用药", "剂量", "就医", "就诊", "急诊", "门诊", "医院", "医生", "护理", "康复",
	"咬", "抓",
	"bite", "scratch", "rabies", "vaccine", "vaccination",
}

// Check evaluates whether a user query falls within the agent's scope.
func (g *ScopeGuard) Check(text string) *ScopeResult {
	lower := strings.ToLower(text)

	// If the query describes the user's own medical situation, never block it
	// as out-of-scope (e.g. animal-bite / rabies questions are core medicine).
	for _, o := range medicalOverrideKeywords {
		if strings.Contains(lower, strings.ToLower(o)) {
			return &ScopeResult{InScope: true}
		}
	}

	for _, p := range g.blockedPatterns {
		for _, kw := range p.keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return &ScopeResult{
					InScope:  false,
					Reason:   p.reason,
					Redirect: p.redirect,
				}
			}
		}
		for _, kw := range p.keywordsZH {
			if strings.Contains(text, kw) {
				return &ScopeResult{
					InScope:  false,
					Reason:   p.reason,
					Redirect: p.redirect,
				}
			}
		}
	}

	return &ScopeResult{InScope: true}
}
