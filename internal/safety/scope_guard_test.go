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
	}
	for _, q := range outScope {
		if res := g.Check(q); res.InScope {
			t.Errorf("Check(%q) = in-scope, want out-of-scope", q)
		}
	}
}
