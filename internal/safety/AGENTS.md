# internal/safety — L1/L2/L3 三道安全层

| 文件 | 作用 |
|---|---|
| `emergency.go` | `EmergencyDetector`（`Detect` / `EmergencyResponseZH`）—— L1，管线第一件事 |
| `scope_guard.go` | `ScopeGuard`（`Check`）—— L2 越界拒绝。兽医规则在 :38（关键词 宠物/猫/狗/兽医/动物），`medicalOverrideKeywords` 保护"咬/抓/狂犬病"等人类暴露问题不被误封 |
| `post_verify.go` | `PostVerifier` —— L3 引用真实性检查 + 可选 LLM-as-judge 主张支持度检查（`NewPostVerifierWithJudge`，`POST_VERIFY_SEMANTIC` 默认 false） |
| `deflect.go` | `RemoveReferralSentences` —— 按句删除含"拨打120/立即就医/去医院/看医生/医生指导"的转诊句 |

## 必须知道的两个决定（都不是 bug，别顺手修）

1. **L4 免责声明层不存在**。2026-09-06 产品决策把 L4 注入从回答里移除，`Response.DisclaimerSent` 只在 L1 急救与 L2 拒绝的短路上为 true，正常回答恒为 false（`Session.DisclaimerSent` 只是遗留的持久字段）。
2. **后验证是 log-only**。`!Passed` 时 `slog.Warn` 并显式丢弃 `CorrectedResponse`（2026-09-08），不会往回复上追加任何东西。
3. `RemoveReferralSentences` **挂在急救分支上**（`agent.go:662`、`agent.go:1044`，包在 `EmergencyResponseZH` 外面），所以连呼救指引一起删。这是 2026-09-21 审计里按用户指示**保持不动**的一项，风险已如实记录，改动需要产品决策。

## 召回层的分工

"我家猫最近不吃东西"这类查询由本包（L2）在检索**之前**拒绝，不要在 `internal/knowledge` 的召回断言里测它，也不要靠分数阈值区分 —— 阈值杀不掉标题即"食欲不振"的条目，却会伤害合法的口语儿科查询。回归断言在 `scope_guard_test.go` 的 outScope 列表里。

## 测试

`scope_guard_test.go`、`post_verify_test.go`、`deflect_test.go`。
