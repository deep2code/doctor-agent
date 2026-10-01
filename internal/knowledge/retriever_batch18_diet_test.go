package knowledge

import (
	"context"
	"testing"
)

// TestRetrieverBatch18DietRecall guards the 2026-10-01 科普扩充第十八批:
//
//	cd-ob-   国家卫健委《成人肥胖食养指南(2024年版)》
//	cd-cob-  国家卫健委《儿童青少年肥胖食养指南(2024年版)》
//	cd-st-   国家卫健委《儿童青少年生长迟缓食养指南(2023年版)》
//
// (chronic_diet_obesity.json, merged into the medical dataset)
// Each case is a natural spoken question (not the condition_zh verbatim) that
// must land the target entry inside the top5. Recall failures are fixed in the
// data layer only (addkw --target 3 + leak_scan), never by moving a threshold.
func TestRetrieverBatch18DietRecall(t *testing.T) {
	r := NewRetriever(newTestStore(t))
	cases := map[string]string{
		// ---------- 成人肥胖食养指南 cd-ob- ----------
		"减肥每天应该摄入多少能量":     "cd-ob-energy",
		"控制总能量摄入怎么算":       "cd-ob-energy",
		"理想体重是怎么计算的":       "cd-ob-energy",
		"减肥期间早中晚三餐怎么分配":    "cd-ob-balance",
		"减脂期蛋白质占多少能量":      "cd-ob-balance",
		"减肥主食吃全谷物还是白米饭":    "cd-ob-balance",
		"营养代餐可以减肥吗":        "cd-ob-balance",
		"哪些食物算高能量食物":       "cd-ob-highenergy",
		"减肥期间每天油盐和糖的限量":    "cd-ob-highenergy",
		"喝酒对减肥的影响 酒精热量":    "cd-ob-highenergy",
		"晚餐几点吃比较好":         "cd-ob-mealtime",
		"吃饭顺序怎样更利于减重":      "cd-ob-mealtime",
		"吃得太快容易长胖吗":        "cd-ob-mealtime",
		"肥胖的人每周运动多久合适":     "cd-ob-activity",
		"抗阻运动一周做几次":        "cd-ob-activity",
		"久坐不动和肥胖的关系":       "cd-ob-activity",
		"过劳肥是怎么回事 睡眠不足会胖吗": "cd-ob-activity",
		"痰湿体质肥胖怎么调理":       "cd-ob-tcm",
		"脾虚肥胖吃什么食药物质":      "cd-ob-tcm",
		"肥胖的中医辨证分型有哪些":     "cd-ob-tcm",
		"一个月减多少公斤比较安全":     "cd-ob-speed",
		"减肥进入平台期体重不降了怎么办":  "cd-ob-speed",
		"为什么快速减肥容易反弹":      "cd-ob-speed",

		// ---------- 儿童青少年肥胖食养指南 cd-cob- ----------
		"孩子胖不胖怎么判定":       "cd-cob-judge",
		"小孩腰围身高比多少算中心型肥胖": "cd-cob-judge",
		"儿童超重和肥胖的界值是多少":   "cd-cob-judge",
		"孩子减肥要减少多少能量摄入":   "cd-cob-portion",
		"小孩每天喝多少牛奶才够":     "cd-cob-portion",
		"孩子吃饭吃八分饱够吗":      "cd-cob-portion",
		"儿童三餐能量各占多少":      "cd-cob-behavior",
		"孩子晚上几点以后不要吃东西":   "cd-cob-behavior",
		"孩子零食吃多少算超标":      "cd-cob-behavior",
		"孩子肥胖每天运动多长时间":    "cd-cob-sport",
		"学龄前儿童每天户外活动要多久":  "cd-cob-sport",
		"肥胖孩子运动从多久开始":     "cd-cob-sport",
		"小学生每天应该睡几个小时":    "cd-cob-sleep",
		"孩子看电视时间控制在多久以内":  "cd-cob-sleep",
		"孩子体重多久测一次":       "cd-cob-monitor",
		"小孩肥胖能吃药减肥吗":      "cd-cob-monitor",
		"儿童肥胖可以做缩胃手术吗":    "cd-cob-monitor",
		"孩子食养怎么按季节调整":     "cd-cob-season",
		"冬天给孩子吃牛羊肉会上火吗":   "cd-cob-season",

		// ---------- 儿童青少年生长迟缓食养指南 cd-st- ----------
		"孩子生长迟缓是怎么判定的":     "cd-st-define",
		"身高低于两个标准差是什么意思":   "cd-st-define",
		"孩子长得矮是营养问题还是疾病":   "cd-st-scope",
		"特发性矮小靠食补有用吗":      "cd-st-scope",
		"两岁孩子每天几次正餐几次加餐":   "cd-st-diet",
		"幼儿每天饮奶量多少合适":      "cd-st-diet",
		"孩子不长个应该补充什么营养":    "cd-st-nutrients",
		"孩子补铁吃什么食物好":       "cd-st-nutrients",
		"小孩缺维生素D要额外补吗":     "cd-st-nutrients",
		"小儿脾胃气虚有什么表现":      "cd-st-tcm",
		"孩子吃凉的就拉肚子是脾胃虚寒吗":  "cd-st-tcm",
		"小孩健脾增食的食养原则":      "cd-st-regulate",
		"给孩子食补是不是越多越好":     "cd-st-regulate",
		"两岁宝宝的食物要做得多软多碎":   "cd-st-cook",
		"整粒花生豆类会呛到孩子吗":     "cd-st-cook",
		"孩子吃饭时能看电视玩玩具吗":    "cd-st-cook",
		"想让孩子长高做什么运动好":     "cd-st-activity",
		"孩子午睡需要多长时间":       "cd-st-sleep",
		"消化不好会影响孩子生长激素分泌吗": "cd-st-sleep",
		"学校的营养课每学期要上几节课":   "cd-st-school",
		"孩子挑食家长能强迫他多吃吗":    "cd-st-school",

		// ---------- 第二波: 三本指南附录 ----------
		// 成人肥胖食养方(附录4 五证型)
		"减肥老是饿口干口臭喝什么汤": "cd-ob-fang-heat",
		"铁皮石斛玉竹煲瘦肉怎么做":  "cd-ob-fang-heat",
		"三豆饮的做法和用量":     "cd-ob-fang-heat",
		"痰湿体质减肥喝什么汤":    "cd-ob-fang-phlegm",
		"冬瓜薏米汤减肥要不要放盐":  "cd-ob-fang-phlegm",
		"身体困重舌苔厚腻怎么食养":  "cd-ob-fang-phlegm",
		"压力大情绪性长胖吃什么粥":  "cd-ob-fang-stasis",
		"佛手橘皮山楂粥的做法":    "cd-ob-fang-stasis",
		"山楂内金粥怎么做":      "cd-ob-fang-stasis",
		"吃得不多还是长胖是不是脾虚": "cd-ob-fang-spleen",
		"荷叶减肥茶的配方":      "cd-ob-fang-spleen",
		"扁豆山药粥减肥可以吃吗":   "cd-ob-fang-spleen",
		"怕冷浮肿型肥胖喝什么汤":   "cd-ob-fang-kidney",
		"姜桂茶干姜肉桂怎么泡":    "cd-ob-fang-kidney",
		"山药黄芪炖鸭肉怎么做":    "cd-ob-fang-kidney",

		// 食物交换表 / 地区食谱 / 活动强度 / 中心型肥胖前期(附录2·3·5·6)
		"食物交换份一份是多少千卡":   "cd-ob-exchange",
		"25克生大米等于多少米饭":   "cd-ob-exchange",
		"减肥一份水果能吃多少克":    "cd-ob-exchange",
		"减肥期间坚果一次吃多少":    "cd-ob-exchange",
		"1200千卡减肥一天吃什么":  "cd-ob-menu",
		"减肥食谱里的油盐用量多少合适": "cd-ob-menu",
		"东北地区减肥食谱举例":     "cd-ob-menu",
		"代谢当量MET是什么意思":   "cd-ob-met",
		"走路每小时3公里算中等强度吗": "cd-ob-met",
		"跳绳的代谢当量是多少":     "cd-ob-met",
		"拖地洗衣服算不算运动":     "cd-ob-met",
		"中心型肥胖前期是什么意思":   "cd-ob-central",
		"男性腰围85到90算不算肥胖": "cd-ob-central",
		"腰围在哪个位置量才标准":    "cd-ob-central",

		// 儿童青少年肥胖附录(食谱·食养方)
		"10岁孩子减肥食疗方举例":  "cd-cob-fang",
		"芦根竹叶饮怎么做给小孩":   "cd-cob-fang",
		"小孩脾虚不运吃什么主食":   "cd-cob-fang",
		"孩子肢体轻度浮肿喝什么汤":  "cd-cob-fang-sym",
		"三仁馒头怎么做":       "cd-cob-fang-sym",
		"孩子大便干结咳嗽喘息食养方": "cd-cob-fang-sym",
		"10岁肥胖孩子一天吃什么":  "cd-cob-menu",
		"儿童减肥食谱按生重还是熟重": "cd-cob-menu",
		"孩子减肥一天植物油和盐多少": "cd-cob-menu",

		// 生长迟缓附录(食养方·身高界值表·地区食谱)
		"孩子不爱吃饭喝什么汤调理":     "cd-st-fang",
		"山楂麦芽消食汤的做法":       "cd-st-fang",
		"小孩健脾开胃的粥有哪些":      "cd-st-fang",
		"六岁孩子身高多少算生长迟缓":    "cd-st-table",
		"8岁男孩身高115厘米算矮吗":   "cd-st-table",
		"两岁宝宝身高82厘米是不是矮小":  "cd-st-table",
		"孩子身高对照哪个标准判断生长迟缓": "cd-st-table",
		"8岁孩子长个子一天三餐食谱":    "cd-st-menu",
		"西北地区生长迟缓儿童食谱举例":   "cd-st-menu",
	}
	ctx := context.Background()
	for query, want := range cases {
		res, err := r.Retrieve(ctx, query, 5)
		if err != nil {
			t.Fatalf("查询 %q 检索失败: %v", query, err)
		}
		found := false
		for _, item := range res {
			if item.Entry.ID == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("查询 %q: 期望 top5 含 %s，实际 %v", query, want, entryIDs(res))
		}
	}
}
