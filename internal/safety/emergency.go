package safety

import "strings"

// EmergencyResult indicates whether an input matches emergency patterns.
type EmergencyResult struct {
	IsEmergency bool
	Matched     string   // The matched keyword pattern
	Action      string   // Immediate action to take
	ActionZH    string   // Chinese version of the action
}

// emergencyPattern defines a keyword pattern that triggers an emergency alert.
type emergencyPattern struct {
	keywords   []string // Lowercase English keywords
	keywordsZH []string // Chinese keywords
	action     string
	actionZH   string
}

// EmergencyDetector scans user input for emergency medical conditions.
// It MUST be called before any LLM invocation to ensure zero-latency
// emergency responses.
type EmergencyDetector struct {
	patterns []emergencyPattern
}

// NewEmergencyDetector creates a detector with pre-defined emergency patterns.
func NewEmergencyDetector() *EmergencyDetector {
	return &EmergencyDetector{
		patterns: []emergencyPattern{
			{
				keywords:   []string{"chest pain", "crushing", "heart attack", "cardiac arrest"},
				keywordsZH: []string{"胸痛", "胸闷压榨", "心梗", "心肌梗死", "心脏骤停"},
				action:     "Call 120 (China emergency) or 911 (US) immediately. Do NOT drive yourself to hospital. Chew 300mg aspirin if not allergic and not on blood thinners.",
				actionZH:   "请立即拨打120急救电话！不要自己开车去医院。如无阿司匹林过敏且未服用抗凝药物，可嚼服300mg阿司匹林。",
			},
			{
				keywords:   []string{"stroke", "facial droop", "arm weakness", "speech difficulty", "hemiplegia", "one side", "f.a.s.t"},
				keywordsZH: []string{"中风", "面瘫", "嘴歪", "半边身体", "半身不遂", "说不出话", "言语不清", "口齿不清", "手臂无力", "FAST"},
				action:     "Call 120 immediately. Time is brain — every minute counts. Note the time symptoms started. Do NOT give food, drink, or medication.",
				actionZH:   "请立即拨打120急救电话！时间就是大脑——每分钟都至关重要。请记下症状开始的时间。不要给患者进食、饮水或服药。",
			},
			{
				keywords:   []string{"severe bleeding", "hemorrhage", "bleeding out", "profuse"},
				keywordsZH: []string{"大出血", "严重出血", "出血不止", "喷射状", "血流不止"},
				action:     "Call 120 immediately. Apply direct pressure to the wound with a clean cloth. If possible, elevate the bleeding site above the heart. Do NOT remove embedded objects.",
				actionZH:   "请立即拨打120急救电话！用干净布料直接压迫伤口。如可能，将出血部位抬高至心脏以上。不要移除嵌入的异物。",
			},
			{
				keywords:   []string{"cannot breathe", "shortness of breath", "respiratory distress", "suffocating", "dyspnea"},
				keywordsZH: []string{"呼吸困难", "喘不上气", "窒息", "无法呼吸", "气短严重"},
				action:     "Call 120 immediately. Sit upright. If you have prescribed rescue inhaler or epinephrine auto-injector, use it now.",
				actionZH:   "请立即拨打120急救电话！保持坐立姿势。如有医生处方的急救吸入剂或肾上腺素自动注射笔，请立即使用。",
			},
			{
				keywords:   []string{"unconscious", "passed out", "unresponsive", "coma", "loss of consciousness"},
				keywordsZH: []string{"昏迷", "不省人事", "失去意识", "意识不清", "昏倒", "叫不醒"},
				action:     "Call 120 immediately. Check if the person is breathing. If not breathing, begin CPR if trained. Place in recovery position if breathing.",
				actionZH:   "请立即拨打120急救电话！检查患者是否有呼吸。如无呼吸且您受过培训，请开始心肺复苏。如有呼吸，将患者置于复苏体位（侧卧）。",
			},
			{
				keywords:   []string{"anaphylaxis", "anaphylactic", "severe allergic", "throat swelling", "tongue swelling"},
				keywordsZH: []string{"严重过敏", "喉头水肿", "喉咙肿胀", "舌头肿胀", "过敏性休克"},
				action:     "Call 120 immediately. Use epinephrine auto-injector (EpiPen) if available. Lie flat with legs elevated unless having breathing difficulty.",
				actionZH:   "请立即拨打120急救电话！如有肾上腺素自动注射笔(EpiPen)，请立即使用。平躺并将双腿抬高，除非有呼吸困难。",
			},
			{
				keywords:   []string{"seizure", "convulsion", "fitting", "epileptic"},
				keywordsZH: []string{"抽搐", "癫痫", "抽风", "全身痉挛", "羊癫疯"},
				action:     "Call 120 if seizure lasts >5 minutes, repeats, or person doesn't regain consciousness. Clear the area of dangerous objects. Do NOT restrain or put anything in mouth. Time the seizure.",
				actionZH:   "如抽搐持续超过5分钟、反复发作或未恢复意识，请立即拨打120。清除周围危险物品。不要按住患者或往嘴里塞东西。记录抽搐持续时间。",
			},
			{
				keywords:   []string{"severe burn", "third degree", "extensive burn"},
				keywordsZH: []string{"严重烧伤", "大面积烧伤", "三度烧伤"},
				action:     "Call 120 for large or deep burns. Cool the burn under cool (not cold) running water for 20 minutes. Cover with clean, non-stick dressing. Do NOT apply ice, butter, or ointments.",
				actionZH:   "大面积或深度烧伤请立即拨打120。用凉水（非冰水）冲洗烧伤部位20分钟。用干净不粘连的敷料覆盖。不要涂抹冰块、黄油或药膏。",
			},
			{
				keywords:   []string{"poison", "overdose", "ingested chemical", "toxic"},
				keywordsZH: []string{"中毒", "服毒", "药物过量", "误食化学品", "农药"},
				action:     "Call 120 immediately. If possible, identify what was ingested, how much, and when. Do NOT induce vomiting unless instructed by poison control.",
				actionZH:   "请立即拨打120急救电话！如可能，请确定误食了什么、多少量、时间。除非中毒控制中心指示，否则不要催吐。",
			},
		},
	}
}

// Detect scans the input text for emergency patterns.
// Returns nil if no emergency is detected.
func (d *EmergencyDetector) Detect(text string) *EmergencyResult {
	lower := strings.ToLower(text)

	for _, p := range d.patterns {
		// Check English keywords
		for _, kw := range p.keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return &EmergencyResult{
					IsEmergency: true,
					Matched:     kw,
					Action:      p.action,
					ActionZH:    p.actionZH,
				}
			}
		}
		// Check Chinese keywords
		for _, kw := range p.keywordsZH {
			if strings.Contains(text, kw) {
				return &EmergencyResult{
					IsEmergency: true,
					Matched:     kw,
					Action:      p.action,
					ActionZH:    p.actionZH,
				}
			}
		}
	}

	return nil
}

// EmergencyResponseZH returns the Chinese emergency response template.
func EmergencyResponseZH(result *EmergencyResult) string {
	var sb strings.Builder
	sb.WriteString("⚠️ **紧急医疗情况检测**\n\n")
	sb.WriteString("根据您的描述（匹配关键词：" + result.Matched + "），")
	sb.WriteString("这可能是**需要立即就医的紧急情况**。\n\n")
	sb.WriteString("### 请立即执行\n\n")
	sb.WriteString(result.ActionZH)
	sb.WriteString("\n\n### 重要提示\n\n")
	sb.WriteString("- **本系统是AI辅助工具**，紧急检测不能替代专业急救判断\n")
	sb.WriteString("- 如有任何疑虑，请立即拨打120\n")
	sb.WriteString("- 保持冷静，按照急救调度员的指导操作\n")
	return sb.String()
}
