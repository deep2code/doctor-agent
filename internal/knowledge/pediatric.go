package knowledge

// Pediatric growth & development datasets (2026-09):
//   growth_standards.json      — WHO Child Growth Standards SD tables +
//                                China WS/T 423-2022 percentile/SD tables
//   development_milestones.json — CDC "Learn the Signs. Act Early."
//                                milestone checklists (2022 revision), zh+en
//   newborn_care.json          — WHO preterm/LBW care recommendations (26)
//                                + China newborn screening programmes (3)

// GrowthSDRow is one row of a -3SD..+3SD table keyed by month (age-for-X
// indicators) or cm (weight-for-length/height indicators).
type GrowthSDRow struct {
	Month int       `json:"month,omitempty"`
	CM    int       `json:"cm,omitempty"`
	SD    []float64 `json:"sd"` // len 7: -3, -2, -1, 0(中位数), +1, +2, +3
}

// GrowthIndicator is one sex-split indicator series (weight-for-age etc.).
type GrowthIndicator struct {
	Unit  string        `json:"unit"`
	Key   string        `json:"key,omitempty"` // "month"|"cm" (china only)
	Note  string        `json:"note,omitempty"`
	Boys  []GrowthSDRow `json:"boys"`
	Girls []GrowthSDRow `json:"girls"`
}

// GrowthAssessmentRule is one SD-band verdict from WS/T 423-2022 Table 3.
type GrowthAssessmentRule struct {
	Indicator string `json:"indicator"`
	Range     string `json:"range"`
	Verdict   string `json:"verdict"`
}

// GrowthStandardsDoc mirrors growth_standards.json.
type GrowthStandardsDoc struct {
	Version                 string                 `json:"version"`
	Updated                 string                 `json:"updated"`
	Who                     WHOStandards           `json:"who"`
	WhoVelocity             *WHOVelocityDoc        `json:"who_velocity"`
	China                   ChinaGrowthStandards   `json:"china"`
	SchoolAge               *SchoolAgeDoc          `json:"school_age"`
	AssessmentRulesSD       []GrowthAssessmentRule `json:"assessment_rules_sd"`
	WHOZscoreInterpretation []GrowthAssessmentRule `json:"who_zscore_interpretation"`
}

// WHOStandards holds the WHO indicator tables (named fields in the JSON;
// the retriever builds a key->indicator view at load time).
type WHOStandards struct {
	Source                  string           `json:"source"`
	URL                     string           `json:"url"`
	WeightForAge            *GrowthIndicator `json:"weight_for_age"`
	LengthHeightForAge      *GrowthIndicator `json:"length_height_for_age"`
	HeadCircumferenceForAge *GrowthIndicator `json:"head_circumference_for_age"`
	BMIForAge               *GrowthIndicator `json:"bmi_for_age"`
}

// GrowthVelocityRow is one WHO velocity window row ("0-2 mo" etc.).
type GrowthVelocityRow struct {
	From     int       `json:"from"` // interval start (months, 4wks≈1mo)
	To       int       `json:"to"`   // interval end (months)
	SD       []float64 `json:"sd"`   // -3SD..+3SD of the increment
	Interval string    `json:"interval,omitempty"`
}

// WHOVelocityDoc is the 2009 WHO growth velocity standards: per indicator,
// per window (1/2/3/4/6 months) one boys/girls series. Weight increments in
// grams; length/head circumference in cm.
type WHOVelocityDoc struct {
	Source            string                                    `json:"source"`
	URL               string                                    `json:"url"`
	WeightUnit        string                                    `json:"weight_unit"`
	LengthHeadUnit    string                                    `json:"length_head_unit"`
	Note              string                                    `json:"note"`
	Weight            map[string]map[string][]GrowthVelocityRow `json:"weight"`
	Length            map[string]map[string][]GrowthVelocityRow `json:"length"`
	HeadCircumference map[string]map[string][]GrowthVelocityRow `json:"head_circumference"`
}

// SchoolAgeBand is one age's wasting/normal/overweight BMI cut-offs.
type SchoolAgeBand struct {
	WastingMax    float64 `json:"wasting_max"`    // BMI ≤ → 消瘦
	NormalMax     float64 `json:"normal_max"`     // BMI ≤ → 正常
	OverweightMax float64 `json:"overweight_max"` // BMI ≤ → 超重；> → 肥胖
}

// SchoolAgeDoc is the 6-18 岁合并筛查标准 (WS/T 456-2014 + WS/T 586-2018).
type SchoolAgeDoc struct {
	Source           string   `json:"source"`
	URLs             []string `json:"urls"`
	AgeRange         string   `json:"age_range"`
	StuntingHeightCM struct {
		Note  string             `json:"note"`
		Boys  map[string]float64 `json:"boys"` // "6.0".."18.0" (半岁档) -> cm 界值
		Girls map[string]float64 `json:"girls"`
	} `json:"stunting_height_cm"`
	BMIBands struct {
		Note  string                   `json:"note"`
		Boys  map[string]SchoolAgeBand `json:"boys"` // "6".."17" (整岁档)
		Girls map[string]SchoolAgeBand `json:"girls"`
	} `json:"bmi_bands"`
}

// ChinaGrowthStandards is WS/T 423-2022 (7 岁以下儿童生长标准).
type ChinaGrowthStandards struct {
	Source     string                      `json:"source"`
	URL        string                      `json:"url"`
	Note       string                      `json:"note"`
	Indicators map[string]*GrowthIndicator `json:"indicators"`
}

// MilestoneAge is one CDC milestone checklist by age.
type MilestoneAge struct {
	AgeKey                  string   `json:"age_key"` // "2mo".."5yr"
	AgeLabelEN              string   `json:"age_label_en"`
	AgeLabelZH              string   `json:"age_label_zh"`
	URL                     string   `json:"url,omitempty"`
	SocialEmotional         []string `json:"social_emotional"`
	LanguageCommunication   []string `json:"language_communication"`
	Cognitive               []string `json:"cognitive"`
	MovementPhysical        []string `json:"movement_physical"`
	SocialEmotionalZH       []string `json:"social_emotional_zh"`
	LanguageCommunicationZH []string `json:"language_communication_zh"`
	CognitiveZH             []string `json:"cognitive_zh"`
	MovementPhysicalZH      []string `json:"movement_physical_zh"`
}

// MilestonesDoc mirrors development_milestones.json.
type MilestonesDoc struct {
	Source     string         `json:"source"`
	Definition string         `json:"definition"`
	Ages       []MilestoneAge `json:"ages"`
}

// WHORecommendation is one row of the WHO preterm/LBW guideline Table 1.
type WHORecommendation struct {
	ID               string `json:"id"` // "A.1a".."C.4"
	Domain           string `json:"domain"`
	TopicZH          string `json:"topic_zh"`
	RecommendationEN string `json:"recommendation_en"`
	RecommendationZH string `json:"recommendation_zh"`
	StrengthEN       string `json:"strength_en"`
	Status           string `json:"status"`
}

// ChinaScreeningProgramme is one national newborn screening programme.
type ChinaScreeningProgramme struct {
	ID      string   `json:"id"`
	TitleZH string   `json:"title_zh"`
	Source  string   `json:"source"`
	URL     string   `json:"url"`
	Points  []string `json:"points"`
}

// NewbornCareDoc mirrors newborn_care.json.
type NewbornCareDoc struct {
	Version       string `json:"version"`
	Updated       string `json:"updated"`
	WHOPretermLBW struct {
		Source          string              `json:"source"`
		URL             string              `json:"url"`
		Recommendations []WHORecommendation `json:"recommendations"`
	} `json:"who_preterm_lbw"`
	ChinaScreening []ChinaScreeningProgramme `json:"china_screening"`
}
