# internal/agent — 编排器

`Agent` 是把安全层、检索、提示词、工具循环、后验证串起来的唯一地方。本包只有一个非测试文件，`agent.go`（2175 行），改任何行为都在这里定位。下文的 `:NNN` 不带文件名时都指这个文件；跨包引用一律写全路径。

## 管线（`ProcessMessageStream`，:860）

阶段顺序与行号，全部核实过：

1. 会话串行锁 :869
2. **L1 急救检测** :873-884 → 命中即短路返回，`DisclaimerSent:true`（:881）
3. **L2 范围守卫** :886-897 → 越界即短路返回，`DisclaimerSent:true`（:894）
4. 知识检索 :899-911（`buildContextualQuery` 拼跨轮上下文 → `retrieveWithUnderstanding`）
5. 分层提示词 :913-943：`ComposeStaticPrefix` + `ComposeDynamicSections`（:915-916）→ 空检索时追加 `prompt.NoKnowledgeGuidance`（:920-922）→ 路由工具描述（:926-935）→ 澄清引导（:940）
6. agent loop :952-1080
7. **L3 引用后验证** :1082-1110 —— **log-only**，`!Passed` 时 `slog.Warn` 并显式丢弃 `CorrectedResponse`
8. **L4 不存在**：:1112-1113 注释写明「Disclaimer injection removed 2026-09-06」，`disclaimerSent=false`。正常回答的 `Response.DisclaimerSent` 恒为 false，只有 L1/L2 短路是 true。

图片管线 `ProcessMessageStreamWithImages`（:1205）是同构的第二条链路，但**带图时确实跳过 L1 急救检测**（:1217 的条件是 `len(images) == 0 && a.cfg.EmergencyEnabled`，注释写明理由是图片可能是检查报告）；L2 范围守卫照常执行（:1230）。所以"带图提问说不舒服"不会走急救短路，这是代码里写着的既有行为，不是文档笔误。

`ProcessMessage` 是非流式包装。

## 关键常量（都在这里，不要在别处找）

| 项 | 值 | 位置 |
|---|---|---|
| 迭代上限 | `maxIterations` 默认 5 | :952-954，`internal/config/config.go:185` MAX_TOOL_ITERATIONS |
| 单轮工具预算 | `MaxToolCalls` 默认 5 | :962、:1062，`internal/config/config.go:186` |
| 并行执行上限 | `maxParallelToolExec = 4` | :778 |
| 工具结果硬顶 | `toolResultHardCap = 4000` | :1578 |
| 查询理解超时 | 8s | :663 |

## 并行工具执行 `executeToolBatch`（:778）

记账串行、执行并行：去重与预算是 first-wins 的串行判定（:799-810），实际 `Dispatch` 并发（:819-830），tool-role 消息按**原调用顺序**回填（:849，provider 会拒绝乱序 tool 消息）。`onStep` 背后的 SSE writer 不是 goroutine-safe，所以步骤事件必须串行发。新加共享状态要 mutex 保护。

## 压缩层 `compactToolResult`（:1578）

四级梯子，输出**恒为合法 JSON**：
1. JSON round-trip 归一（:1619）—— 工具把**类型化切片**存在 `results` 里，`[]any` switch 看不见，先归一化才有得可压（早先 knowledge_search 的策略因这点是死代码）
2. knowledge_search 专用策略：top-3 + 字段白名单（:1627，`pruneKnowledgeSearch` :1647）
3. 通用字段感知梯（字符串 rune / 列表项 / 深度上限，四档 :1591-1596）
4. 顶层字段摘要兜底（:1641）

回归门：`tool_compaction_test.go`。中间截断 JSON 会切碎 CJK，那个旧实现已移除。

## 澄清机制（槽位制，非长度制）

`clinicalSlots` 四组：持续时间 / 严重程度 / 伴随与诱因 / 患者信息（:422-441）。规则：
- 触发前提是句子里有症状词（:401-406）
- 常识型问句豁免（能治吗/副作用/用法…，:446-449）
- 超过 60 rune 的叙述不追问（:468）
- **四组全空**才生成追问（:483），guidance 最多列 3 个问题（:509-513）

旧的"20 rune 长度 cutoff"已废弃。回归门：`clarification_test.go`。

## 已核实的坑

1. 工具可见性由 `internal/tools/router.go` 的 `toolGroups` / `relationToTools` 决定，`agent.go:928` 只是转达 —— 所以「在 `agent.go:153-169` 注册过」不等于模型能调到。三个曾经的受害者（`drug_label_lookup`/`lab_report_analyze`/`visit_prep`）已于 2026-10-04 入组，回归门 `internal/tools/router_visibility_test.go` 双向锁死这条不变量；详见 `internal/tools/AGENTS.md` 第 1 条。
2. `RemoveReferralSentences`（safety 层）确实挂在急救分支上，会删掉含"拨打120/立即就医"的句子（:662、:1044 两处调用）。这是 2026-09-21 安全审计里按产品决策**保留**的行为，不要顺手"修好"。
3. `agent.go:150` 注释里的"9 action"、`:163` 的"~28 retired"、`:925` 的"from 35 to <=10"都是历史残留，对不上代码。

## 测试文件

`agent_test.go`（会话 Claim 语义、跨重启归属）、`clarification_test.go`（槽位豁免边界）、`rerank_wiring_test.go`（`RERANK_ENABLED`+`RERANK_BASE_URL` 才接线）、`tool_compaction_test.go`（压缩输出必须是合法 JSON）、`understanding_test.go`（QU 触发条件与 8s 降级）。
