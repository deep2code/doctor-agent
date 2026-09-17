package safety

import "strings"

// EmergencyResult indicates whether an input matches emergency patterns.
type EmergencyResult struct {
	IsEmergency bool
	Matched     string // The matched keyword pattern
	Action      string // Immediate action to take
	ActionZH    string // Chinese version of the action
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
//
// 文案原则 (2026-09-08)：不劝导就医/拨打急救电话，只给可立即执行的
// 自救动作、危险信号观察与病因线索记录，引导用户深挖病因而非转移问题。
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
				action:     "Stop all activity and sit or semi-recline immediately. If not allergic and not on blood thinners, chew 300mg aspirin. Loosen clothing, keep air circulating. Record every minute: pain character, radiation range, sweating. Do NOT stay alone or drive.",
				actionZH:   "立即停止一切活动，坐下或半卧休息。如无阿司匹林过敏且未服抗凝药，可嚼服300mg阿司匹林。解开衣领保持通风。逐分钟记录：疼痛性质、放射范围、是否冷汗。不要独处，不要自行驾车。",
			},
			{
				keywords:   []string{"stroke", "facial droop", "arm weakness", "speech difficulty", "hemiplegia", "one side", "f.a.s.t"},
				keywordsZH: []string{"中风", "面瘫", "嘴歪", "半边身体", "半身不遂", "说不出话", "言语不清", "口齿不清", "手臂无力", "FAST"},
				action:     "Keep the person resting quietly on their side. Record the EXACT time symptoms started. No food, drink, or medication. Track consciousness, pupil size, and limb strength changes. Gather medication history and medical records for cause analysis.",
				actionZH:   "让患者安静侧卧。准确记录症状开始的时间。不要进食、饮水或服药。持续观察并记录：意识状态、瞳孔大小、肢体力量变化。整理用药史与既往病史，用于病因判断。",
			},
			{
				keywords:   []string{"severe bleeding", "hemorrhage", "bleeding out", "profuse"},
				keywordsZH: []string{"大出血", "严重出血", "出血不止", "喷射状", "血流不止"},
				action:     "Apply firm direct pressure with a clean cloth. Elevate the bleeding site above heart level. Do NOT remove embedded objects. Record blood loss amount, color, and whether it is pulsing.",
				actionZH:   "用干净布料直接用力压迫伤口。将出血部位抬高至心脏以上。不要移除嵌入的异物。记录出血量、颜色，以及是否呈喷射状（提示动脉出血）。",
			},
			{
				keywords:   []string{"cannot breathe", "shortness of breath", "respiratory distress", "suffocating", "dyspnea"},
				keywordsZH: []string{"呼吸困难", "喘不上气", "窒息", "无法呼吸", "气短严重"},
				action:     "Sit upright, lean slightly forward, loosen tight clothing. If prescribed a rescue inhaler or epinephrine auto-injector, use it now. Slow breathing: inhale through nose 4s, exhale through mouth 6s. Record lip/fingernail color and ability to speak full sentences.",
				actionZH:   "坐直身体略前倾，松开紧身衣物。如有医生处方的急救吸入剂或肾上腺素自动注射笔，请立即使用。放慢呼吸：鼻吸4秒、口呼6秒。记录嘴唇/指甲颜色，以及能否说完整句子（判断气道阻塞程度）。",
			},
			{
				keywords:   []string{"unconscious", "passed out", "unresponsive", "coma", "loss of consciousness"},
				keywordsZH: []string{"昏迷", "不省人事", "失去意识", "意识不清", "昏倒", "叫不醒"},
				action:     "Check breathing. If not breathing and trained, begin CPR. If breathing, place in recovery position (on side). Record the time found and consciousness changes. Collect recent food, drug, and health history to find the cause.",
				actionZH:   "检查是否有呼吸。如无呼吸且您受过培训，立即开始心肺复苏。如有呼吸，置于复苏体位（侧卧）。记录发现时间与意识变化过程。收集近期饮食、用药与健康史，用于查找病因。",
			},
			{
				keywords:   []string{"anaphylaxis", "anaphylactic", "severe allergic", "throat swelling", "tongue swelling"},
				keywordsZH: []string{"严重过敏", "喉头水肿", "喉咙肿胀", "舌头肿胀", "过敏性休克"},
				action:     "If available, use an epinephrine auto-injector (EpiPen) into the outer thigh immediately. Lie flat with legs elevated unless breathing is difficult. Remove the suspected allergen. Record exposure time and symptom progression.",
				actionZH:   "如有肾上腺素自动注射笔(EpiPen)，立即在大腿外侧使用。平躺并抬高双腿（呼吸困难则保持坐直）。立即脱离可疑过敏原。记录接触时间与症状进展速度。",
			},
			{
				keywords:   []string{"seizure", "convulsion", "fitting", "epileptic"},
				keywordsZH: []string{"抽搐", "癫痫", "抽风", "全身痉挛", "羊癫疯"},
				action:     "Clear the area of dangerous objects. Do NOT restrain or put anything in the mouth. Time the seizure precisely. After it ends, place on side and record how long it takes to regain consciousness. Note any triggers observed.",
				actionZH:   "清除周围危险物品。不要按住患者，不要往嘴里塞任何东西。精确记录抽搐开始与结束时间。抽搐后侧卧，记录恢复意识所需时长。回忆并记录可能的诱因（睡眠不足、闪光、漏服药等）。",
			},
			{
				keywords:   []string{"severe burn", "third degree", "extensive burn"},
				keywordsZH: []string{"严重烧伤", "大面积烧伤", "三度烧伤"},
				action:     "Cool the burn under cool (not cold) running water for 20 minutes. Cover with clean, non-stick dressing. Do NOT apply ice, butter, or ointments. Record burn area size and depth appearance.",
				actionZH:   "用凉水（非冰水）持续冲洗烧伤部位20分钟。用干净不粘连的敷料覆盖。不要涂抹冰块、黄油或药膏。记录烧伤面积与创面深度外观变化。",
			},
			{
				keywords:   []string{"poison", "overdose", "ingested chemical", "toxic"},
				keywordsZH: []string{"中毒", "服毒", "药物过量", "误食化学品", "农药"},
				action:     "Identify what was ingested, how much, and when — keep the packaging. Do NOT induce vomiting. Record consciousness level and vomiting episodes; this information is critical for cause analysis.",
				actionZH:   "确定误食了什么、多少量、什么时间——保留包装物。不要催吐。记录意识水平与呕吐次数；这些信息对判断毒物机制至关重要。",
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
// 不劝导就医；给自救动作 + 危险信号观察 + 病因线索记录。
func EmergencyResponseZH(result *EmergencyResult) string {
	var sb strings.Builder
	sb.WriteString("⚠️ **紧急医疗情况检测**\n\n")
	sb.WriteString("根据您的描述（匹配关键词：" + result.Matched + "），")
	sb.WriteString("这可能是**需要立即处理的紧急情况**。\n\n")
	sb.WriteString("### 请立即执行（自救动作）\n\n")
	sb.WriteString(result.ActionZH)
	sb.WriteString("\n\n### 深挖病因线索\n\n")
	sb.WriteString("- 记录症状开始的确切时间与进展过程\n")
	sb.WriteString("- 回溯诱因：活动、饮食、药物、情绪、环境暴露\n")
	sb.WriteString("- 整理既往病史、用药史、过敏史、家族史\n")
	sb.WriteString("\n### 重要提示\n\n")
	sb.WriteString("- 本系统是AI辅助工具，极端情况下请联系身边人协助处理\n")
	sb.WriteString("- 保持冷静，密切监测上述危险信号的变化并持续记录\n")
	return sb.String()
}
