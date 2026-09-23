package knowledge

import (
	"context"
	"testing"
)

// TestRetrieverPopSupplement2Recall guards the 2026-09-23 科普补充第二批
// (adult diet guidelines, myopia prevention, oral health, cancer prevention):
// everyday colloquial Chinese questions must recall the new entries in top5.
func TestRetrieverPopSupplement2Recall(t *testing.T) {
	r := NewRetriever(newTestStore(t))
	cases := map[string]string{
		"每天吃多少盐比较合适":      "diet22-05",
		"一天应该喝多少水":        "diet22-06",
		"粗粮多吃好吗":          "diet22-01",
		"剩米饭可以再加热吃吗":      "diet22-08",
		"低钠盐适合所有人吗":       "diet22-10",
		"减肥期间怎么吃":         "diet22-11",
		"孩子贫血吃什么好":        "diet22-12",
		"孩子每天要户外活动多久":     "myopia-02",
		"假性近视怎么确诊":        "myopia-07",
		"看手机多久要休息一下":      "myopia-04",
		"600度近视算高度近视吗":   "myopia-09",
		"孩子近视了要一直戴眼镜吗":    "myopia-08",
		"巴氏刷牙法怎么刷":        "oral-01",
		"牙刷多久换一次":         "oral-02",
		"塞牙了用什么清理":        "oral-03",
		"洗牙会让牙齿松动吗":       "oral-06",
		"备孕要先看牙吗":         "oral-07",
		"乳牙蛀了用不用治":        "oral-10",
		"窝沟封闭有没有用":        "oral-11",
		"牙齿缺失不镶行不行":       "oral-12",
		"癌症有哪些早期信号":       "cancer-02",
		"大便带血会不会是癌症":      "cancer-02",
		"怎么预防癌症":          "cancer-04",
		"肿瘤标志物升高就是癌吗":     "cancer-06",
		"brca基因突变要做哪些检查":  "cancer-07",
		"压力大会不会容易得癌症":     "cancer-08",
	}
	for query, want := range cases {
		res, err := r.Retrieve(context.Background(), query, 5)
		if err != nil {
			t.Fatalf("检索失败: %v", err)
		}
		found := false
		for _, rr := range res {
			if rr.Entry.ID == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("查询 %q: 期望 top5 含 %s，实际 %v", query, want, entryIDs(res))
		}
	}
}
