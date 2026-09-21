package safety

import "testing"

func TestScopeGuardRabiesBiteInScope(t *testing.T) {
	g := NewScopeGuard()

	// Queries that describe the user's own medical situation must NOT be
	// refused as veterinary advice (regression: "狗"/"猫"/"动物" substring
	// previously blocked core rabies/post-exposure questions).
	inScope := []string{
		"狗咬了要打狂犬疫苗吗",
		"被猫抓了一下需要打疫苗吗",
		"动物咬伤后怎么处理",
		"被狗咬了出血了怎么办",
		"my dog bit me, do I need a rabies shot",
	}
	for _, q := range inScope {
		if res := g.Check(q); !res.InScope {
			t.Errorf("Check(%q) = out-of-scope (%s), want in-scope", q, res.Reason)
		}
	}

	// Pure veterinary questions (no human medical intent) must still be blocked.
	outScope := []string{
		"我的猫生病了吃什么药",
		"狗狗发烧了怎么办",
		"my cat is sick what medicine",
		// 口语化的「宠物食欲不振」。它曾经是 internal/knowledge 的一条检索断言
		// （查询字面零命中，但 alias_map 把 不吃→厌食 泛化后仍以 6~8 分召回
		// 人类条目），在检索层无法与合法的口语儿科查询区分（「突然大哭→夜惊」
		// 结构完全相同），所以判定归到这层来做：生产管线 L2 在检索之前拦截。
		"我家猫最近不吃东西",
	}
	for _, q := range outScope {
		if res := g.Check(q); res.InScope {
			t.Errorf("Check(%q) = in-scope, want out-of-scope", q)
		}
	}
}
