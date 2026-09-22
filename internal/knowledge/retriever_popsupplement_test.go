package knowledge

import (
	"context"
	"testing"
)

// TestRetrieverPopSupplementRecall guards the 2026-09-22 科普补充批次
// (sleep/mental health, maternal diet, exercise/weight, adult vaccines):
// everyday colloquial Chinese questions must recall the new entries in top5.
func TestRetrieverPopSupplementRecall(t *testing.T) {
	r := NewRetriever(newTestStore(t))
	cases := map[string]string{
		"小孩晚上不肯睡总要哄":       "sleep-core-05",
		"老人觉少容易醒正常吗":       "sleep-core-06",
		"晚上睡觉打呼噜憋气是不是病":    "sleep-core-07",
		"失眠了想吃安眠药行不行":       "sleep-core-08",
		"最近两个月高兴不起来什么都没意思":  "mental-core-01",
		"经常突然心慌出汗害怕是怎么了":   "mental-core-02",
		"工作压力太大怎么缓解":        "mental-core-03",
		"备孕体重多少合适":          "maternal-preg-01",
		"备孕要开始吃叶酸吗":         "maternal-preg-02",
		"孕吐厉害吃不下东西怎么办":     "maternal-preg-03",
		"坐月子吃什么下奶":          "maternal-lact-02",
		"哺乳期能喝咖啡吗":          "maternal-lact-05",
		"每天走多少步对身体好":        "exercise-01",
		"老年人怎么锻炼防跌倒":        "exercise-03",
		"想减肥每天要运动多久":        "exercise-01",
		"我身高一米六多少斤算胖":       "weight-01",
		"孩子太胖了怎么管理体重":       "weight-04",
		"流感疫苗什么时候打最好":       "adultvax-flu-02",
		"五十岁打带状疱疹疫苗要打几针":    "adultvax-zsv-01",
		"腰上起了一串水泡针扎一样疼":     "adultvax-zsv-02",
		"九价HPV疫苗几岁可以打":     "adultvax-hpv-01",
		"男性也可以接种HPV疫苗吗":    "adultvax-hpv-01",
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
