package dialogue

import "strings"

// Intent represents a user intent.
type Intent string

const (
	IntentGreeting     Intent = "greeting"
	IntentSymptom      Intent = "symptom"
	IntentDrug         Intent = "drug"
	IntentTreatment    Intent = "treatment"
	IntentEmergency    Intent = "emergency"
	IntentHospital     Intent = "hospital"
	IntentCheckup      Intent = "checkup"
	IntentDiet         Intent = "diet"
	IntentUnknown      Intent = "unknown"
)

// IntentRecognizer classifies user intents based on keywords.
type IntentRecognizer struct {
	keywords map[Intent][]string
}

// NewIntentRecognizer creates a new intent recognizer.
func NewIntentRecognizer() *IntentRecognizer {
	r := &IntentRecognizer{keywords: make(map[Intent][]string)}
	r.initKeywords()
	return r
}

func (r *IntentRecognizer) initKeywords() {
	r.keywords[IntentGreeting] = []string{"你好", "您好", "嗨", "在吗", "请问", "帮帮我"}
	r.keywords[IntentSymptom] = []string{"症状", "不舒服", "难受", "疼", "痛", "痒", "晕", "恶心", "呕吐", "腹泻", "发烧", "发热", "咳嗽", "头痛", "失眠", "乏力"}
	r.keywords[IntentDrug] = []string{"药", "药物", "吃药", "用药", "服药", "处方", "副作用", "禁忌", "过敏"}
	r.keywords[IntentTreatment] = []string{"治疗", "治愈", "手术", "化疗", "放疗", "怎么治", "治疗方案"}
	r.keywords[IntentEmergency] = []string{"急诊", "急救", "120", "来不及了", "很严重", "大出血", "昏迷", "中毒"}
	r.keywords[IntentHospital] = []string{"医院", "挂号", "就诊", "看病", "门诊", "住院", "挂什么科"}
	r.keywords[IntentCheckup] = []string{"体检", "检查", "化验", "B超", "CT", "X光", "验血"}
	r.keywords[IntentDiet] = []string{"饮食", "食物", "营养", "食疗", "忌口", "不能吃", "能吃吗"}
}

// Recognize identifies the intent of a user message.
func (r *IntentRecognizer) Recognize(message string) Intent {
	message = strings.ToLower(message)
	scores := make(map[Intent]int)

	for intent, words := range r.keywords {
		for _, word := range words {
			if strings.Contains(message, word) {
				scores[intent]++
			}
		}
	}

	bestIntent := IntentUnknown
	bestScore := 0
	for intent, score := range scores {
		if score > bestScore {
			bestScore = score
			bestIntent = intent
		}
	}

	return bestIntent
}
