# internal/prompt — 分层系统提示词

`Composer` 组装静态前缀 + 动态段。**这个包的核心不变量是切分线**：静态段必须逐字节稳定才能吃到 provider 的前缀缓存。

| 文件 | 作用 |
|---|---|
| `composer.go` | `NewComposer` / `ComposeSystemPrompt`（= 静态前缀 + 动态段的**纯拼接**，:28-30）/ `ComposeStaticPrefix:35-70` / `ComposeDynamicSections:78-105`（患者上下文 → 引用表 → 知识原文摘录 :90-95 → Layer 4）/ `ComposeToolPrompt` / `BuildPatientContext` |
| `layers.go` | 各层文本常量 |

## 层清单（`composer.go:35-70`，实测 8 层静态）

0 `LayerFoundation`（伦理）→ 1 `LayerClinicalReasoning` → 2 `LayerSouthernGenetics` → 3 `LayerSouthernEnvironment` → 3.5 `LayerEverydayHealth` → 3.55 `LayerColloquialMapping` → 3.75 `LayerFormatting` → 3.8 `LayerDualOutput`。

**Layer 4（安全）在动态段**（:103-105），不在静态前缀里。根 AGENTS.md 曾说"9 static layers"，那是把 Layer 4 错算进静态。

## 三条硬规矩

1. 任何随请求变化的文字都不能进静态段 —— 顺序也不能调（`composer_test.go` 守着）。
2. `ComposeDynamicSections(retrieved, patientCtx, query)` 里的知识摘录块由 `query` 定位窗口，所以**传的必须是实际检索用的 query**，不能是改写后的字符串。
3. 摘录只能来自 `knowledge.BuildKnowledgeExcerpts`（在 `internal/knowledge/citation.go:87`，不在本包），且每个调用点要自己 `clipRunes` 到 450 字。

## 死代码

`layers.go:403 LayerColloquial` 与 `layers.go:244 LayerColloquialMapping` 内容重复，前者全仓库零引用。

## 测试

`composer_test.go`，含 `TestComposeDynamicSectionsCarriesKnowledgeProse`（双向锁死"正文进动态段、不进静态前缀"）。
