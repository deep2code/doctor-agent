package knowledge

import (
	"context"
	"fmt"
	"math"
	"sort"
)

// GrowthAssessment evaluates one measurement (weight/length/head
// circumference/BMI) against the WHO Child Growth Standards and China
// WS/T 423-2022 SD tables, interpolating a z-score between tabulated ages.
type GrowthAssessment struct {
	// Input echo
	Sex         string  `json:"sex"`        // "男"/"女" (male/female also accepted)
	AgeMonths   int     `json:"age_months"` // 0..84 (china) / 0..60 (who)
	Indicator   string  `json:"indicator"`  // weight|length_height|head_circumference|bmi|weight_for_length|weight_for_height
	Value       float64 `json:"value"`
	IndicatorZH string  `json:"indicator_zh"`
	Unit        string  `json:"unit"`

	// Results per standard (nil when the standard lacks this indicator/age).
	WHO   *GrowthZResult `json:"who,omitempty"`
	China *GrowthZResult `json:"china,omitempty"`
}

// GrowthZResult is the interpolated evaluation against one standard.
type GrowthZResult struct {
	Standard   string    `json:"standard"` // "WHO Child Growth Standards" | "WS/T 423-2022"
	ZScore     float64   `json:"z_score"`
	ZScoreText string    `json:"z_score_text"` // e.g. "-1.4"
	SDBand     string    `json:"sd_band"`      // e.g. "-2 SD ≤ z < -1 SD"
	Verdict    string    `json:"verdict"`      // 正常/低体重/生长迟缓/消瘦/超重/肥胖/...
	VerdictEN  string    `json:"verdict_en,omitempty"`
	P50        float64   `json:"p50"` // median for context
	Row        []float64 `json:"row"` // the 7 SD cut-offs used
	OutOfTable bool      `json:"out_of_table,omitempty"`
	Note       string    `json:"note,omitempty"`
}

// growthIndicatorKeys maps the public indicator token to the JSON keys of
// both standards.
var growthIndicatorKeys = map[string]struct{ who, cn, zh, unit string }{
	"weight":             {"weight_for_age", "weight_for_age", "年龄别体重", "kg"},
	"length_height":      {"length_height_for_age", "length_height_for_age", "年龄别身长/身高", "cm"},
	"head_circumference": {"head_circumference_for_age", "head_circumference_for_age", "年龄别头围", "cm"},
	"bmi":                {"", "bmi_for_age", "年龄别 BMI", "kg/m²"},
	"weight_for_length":  {"", "weight_for_length", "身长别体重(0-2岁)", "kg"},
	"weight_for_height":  {"", "weight_for_height", "身高别体重(2-7岁)", "kg"},
}

// AssessGrowth interpolates a z-score for the given measurement. It prefers
// the China standard (WS/T 423-2022, 0-84 months) and also reports WHO when
// applicable (0-60 months, 3 indicators).
func (r *KeywordRetriever) AssessGrowth(ctx context.Context, sex string, ageMonths int, indicator string, value float64) (*GrowthAssessment, error) {
	r.store.ensureGrowth()
	doc := r.store.GrowthStandards
	if doc == nil {
		return nil, fmt.Errorf("growth standards not loaded")
	}
	keys, ok := growthIndicatorKeys[indicator]
	if !ok {
		return nil, fmt.Errorf("unknown indicator %q (weight|length_height|head_circumference|bmi|weight_for_length|weight_for_height)", indicator)
	}
	isGirl := sex == "女" || sex == "female" || sex == "girl" || sex == "f"
	if !isGirl && !(sex == "男" || sex == "male" || sex == "boy" || sex == "m") {
		return nil, fmt.Errorf("sex must be 男/female/男/male style, got %q", sex)
	}
	out := &GrowthAssessment{
		Sex:       map[bool]string{true: "女", false: "男"}[isGirl],
		AgeMonths: ageMonths, Indicator: indicator, Value: value,
		IndicatorZH: keys.zh, Unit: keys.unit,
	}

	if keys.cn != "" && doc.China.Indicators != nil {
		if ind := doc.China.Indicators[keys.cn]; ind != nil {
			rows := ind.Boys
			if isGirl {
				rows = ind.Girls
			}
			if res := assessAgainst(rows, ageMonths, value); res != nil {
				res.Standard = "WS/T 423-2022 中国《7岁以下儿童生长标准》"
				verdict, band := chinaVerdict(keys.cn, res.ZScore)
				res.SDBand = band
				res.Verdict = verdict
				res.Note = chinaIndicatorNote(keys.cn)
				out.China = res
			}
		}
	}
	if keys.who != "" {
		var ind *GrowthIndicator
		switch keys.who {
		case "weight_for_age":
			ind = doc.Who.WeightForAge
		case "length_height_for_age":
			ind = doc.Who.LengthHeightForAge
		case "head_circumference_for_age":
			ind = doc.Who.HeadCircumferenceForAge
		}
		if ind != nil {
			rows := ind.Boys
			if isGirl {
				rows = ind.Girls
			}
			if res := assessAgainst(rows, ageMonths, value); res != nil {
				res.Standard = "WHO Child Growth Standards (2006/2007)"
				res.SDBand, res.VerdictEN = whoVerdict(res.ZScore)
				res.Verdict = whoVerdictZH(res.VerdictEN)
				res.Note = "WHO 标准基于多中心生长参考研究(6国母乳喂养婴儿)"
				out.WHO = res
			}
		}
	}
	if out.China == nil && out.WHO == nil {
		return nil, fmt.Errorf("no standard covers %s at %d months (中国标准 0-84 月，WHO 0-60 月)", keys.zh, ageMonths)
	}
	return out, nil
}

// assessAgainst linearly interpolates the z-score of value against the 7-point
// SD table at the given age, averaging the SD rows that bracket the age.
func assessAgainst(rows []GrowthSDRow, month int, value float64) *GrowthZResult {
	if len(rows) == 0 {
		return nil
	}
	// Rows sorted by month; find bracketing rows.
	lo, hi := -1, -1
	for i, r := range rows {
		if r.Month <= month {
			lo = i
		}
		if r.Month >= month && hi < 0 {
			hi = i
		}
	}
	if lo < 0 || hi < 0 {
		return nil
	}
	// Interpolate the 7 SD cut-offs to the exact age.
	sd := make([]float64, 7)
	if lo == hi {
		copy(sd, rows[lo].SD)
	} else {
		span := float64(rows[hi].Month - rows[lo].Month)
		frac := float64(month-rows[lo].Month) / span
		for k := 0; k < 7; k++ {
			a, b := rows[lo].SD[k], rows[hi].SD[k]
			sd[k] = a + (b-a)*frac
		}
	}
	z := interpolateZ(sd, value)
	res := &GrowthZResult{ZScore: math.Round(z*10) / 10, P50: sd[3], Row: sd}
	res.ZScoreText = fmt.Sprintf("%+.1f", res.ZScore)
	return res
}

// interpolateZ maps value onto the [-3..+3] SD ladder with linear
// interpolation inside each band; values beyond ±3 SD are extrapolated but
// clamped to ±4.
func interpolateZ(sd []float64, value float64) float64 {
	if value >= sd[3] {
		for k := 3; k < 6; k++ {
			if value <= sd[k+1] {
				frac := (value - sd[k]) / (sd[k+1] - sd[k])
				return float64(k-3) + frac
			}
		}
		frac := (value - sd[6]) / (sd[6] - sd[5])
		return math.Min(3+frac, 4)
	}
	for k := 3; k > 0; k-- {
		if value >= sd[k-1] {
			// value sits in [sd[k-1], sd[k]] = [z(k-4), z(k-3)]
			frac := (value - sd[k-1]) / (sd[k] - sd[k-1])
			return float64(k-4) + frac
		}
	}
	frac := (sd[0] - value) / (sd[1] - sd[0])
	return math.Max(-3-frac, -4)
}

// chinaVerdict applies WS/T 423-2022 Table 3 to the z-score.
func chinaVerdict(indicatorKey string, z float64) (verdict, band string) {
	band = sdBand(z)
	switch indicatorKey {
	case "weight_for_age":
		switch {
		case z < -3:
			return "重度低体重", band
		case z < -2:
			return "低体重", band
		default:
			return "正常", band
		}
	case "length_height_for_age":
		switch {
		case z < -3:
			return "重度生长迟缓", band
		case z < -2:
			return "生长迟缓", band
		default:
			return "正常", band
		}
	case "weight_for_length", "weight_for_height", "bmi_for_age":
		switch {
		case z < -3:
			return "重度消瘦", band
		case z < -2:
			return "消瘦", band
		case z >= 3:
			return "重度肥胖", band
		case z >= 2:
			return "肥胖", band
		case z >= 1:
			return "超重", band
		default:
			return "正常", band
		}
	case "head_circumference_for_age":
		return "头围参考（标准未设判定阈值，请结合临床）", band
	}
	return "正常", band
}

func chinaIndicatorNote(key string) string {
	switch key {
	case "length_height_for_age":
		return "2 岁以下为身长(卧位)，2 岁及以上为身高(立位)"
	case "weight_for_length":
		return "适用于 0-2 岁按身长(cm)评估，本工具按月龄近似"
	case "weight_for_height":
		return "适用于 2-7 岁按身高(cm)评估，本工具按月龄近似"
	}
	return ""
}

// sdBand describes which SD interval z falls in.
func sdBand(z float64) string {
	switch {
	case z < -3:
		return "< -3 SD"
	case z < -2:
		return "-3 SD ≤ z < -2 SD"
	case z < -1:
		return "-2 SD ≤ z < -1 SD"
	case z <= 1:
		return "-1 SD ≤ z ≤ +1 SD"
	case z <= 2:
		return "+1 SD < z ≤ +2 SD"
	case z <= 3:
		return "+2 SD < z ≤ +3 SD"
	default:
		return "> +3 SD"
	}
}

// whoVerdict returns the band text plus an English verdict per WHO z-score
// interpretation.
func whoVerdict(z float64) (band, verdictEN string) {
	band = sdBand(z)
	switch {
	case z < -3:
		return band, "severe undernutrition"
	case z < -2:
		return band, "moderate undernutrition"
	case z > 3:
		return band, "obese"
	case z > 2:
		return band, "overweight"
	default:
		return band, "normal"
	}
}

func whoVerdictZH(en string) string {
	switch en {
	case "severe undernutrition":
		return "重度营养不足"
	case "moderate undernutrition":
		return "中度营养不足"
	case "overweight":
		return "超重"
	case "obese":
		return "肥胖"
	default:
		return "正常"
	}
}

// GrowthTableRows returns the tabulated rows (for one sex) of an indicator
// across both standards — used by tools to expose reference cut-offs.
func (r *KeywordRetriever) GrowthTableRows(ctx context.Context, sex, indicator, standard string) (rows []GrowthSDRow, unit string, err error) {
	r.store.ensureGrowth()
	doc := r.store.GrowthStandards
	if doc == nil {
		return nil, "", fmt.Errorf("growth standards not loaded")
	}
	keys, ok := growthIndicatorKeys[indicator]
	if !ok {
		return nil, "", fmt.Errorf("unknown indicator %q", indicator)
	}
	isGirl := sex == "女" || sex == "female"
	var ind *GrowthIndicator
	switch standard {
	case "who":
		if keys.who == "" {
			return nil, "", fmt.Errorf("WHO standard has no %s table", keys.zh)
		}
		switch keys.who {
		case "weight_for_age":
			ind = doc.Who.WeightForAge
		case "length_height_for_age":
			ind = doc.Who.LengthHeightForAge
		case "head_circumference_for_age":
			ind = doc.Who.HeadCircumferenceForAge
		}
	case "china":
		if doc.China.Indicators == nil {
			return nil, "", fmt.Errorf("china indicators missing")
		}
		ind = doc.China.Indicators[keys.cn]
	default:
		return nil, "", fmt.Errorf("standard must be who|china")
	}
	if ind == nil {
		return nil, "", fmt.Errorf("indicator %s not found in %s", keys.zh, standard)
	}
	unit = ind.Unit
	if isGirl {
		return ind.Girls, unit, nil
	}
	sort.SliceStable(ind.Boys, func(i, j int) bool { return ind.Boys[i].Month < ind.Boys[j].Month })
	return ind.Boys, unit, nil
}
