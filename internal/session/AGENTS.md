# internal/session — 会话与归属

对话历史是 `[]llm.Message`（provider-agnostic）+ 患者上下文。内存 map 是唯一真相，两种持久化后端可选。

| 文件 | 作用 |
|---|---|
| `session.go` | `Session`（mutex + `[]llm.Message` + `PatientContext` + `UserID`）、`New` / `AddUserMessage` / `AddAssistantMessage` / `GetMessages` / `LastTouched` / `Clear` / `TrimHistory`；**归属**：`Owner():104`、`ClaimOwner():118`（一次带写锁的 check-and-bind） |
| `file_store.go` | `FileStore` 把会话快照成 JSON 到 `SESSION_DIR`。`ValidID:43` 防路径穿越；目录 0700、文件 **0600**（:80-83，快照含医疗内容与归属绑定）；`Save` 必须序列化 `Snapshot()` —— 它现在会复制 `UserID`，漏了它就等于重启丢归属 |
| `db_store.go` | `DBStore`（业务库后端）：`Save` 在匿名会话后来被认领时回填 `sessions.user_id`；`Load` / `List` / `Delete` |

## 归属模型（三条不可破）

1. `Session.UserID` 为空 = 未认领/匿名。`ClaimOwner` 是**唯一**能写归属的地方：两个并发首写者不可能都成功，后来者永远不能顶掉已有 owner。
2. 上层入口 `Agent.ClaimSession(id, owner)` = `GetOrCreateSession` + `ClaimOwner`；server 侧唯一前置门是 `claimConversation`，foreign conversation 返回 **404 而不是 403**（id 不可探测）。
3. 患者上下文命令：`@region` / `@g6pd` / `@thal`，随会话快照持久。

## 内存上限

`MAX_ACTIVE_SESSIONS`（默认 500，LRU 逐出）与 `SESSION_IDLE_MINUTES`（默认 120），见 `internal/config/config.go:224-225`，实现落在 `agent.go` 的会话 map 上。

## 测试

`session_test.go`（Claim 语义、跨重启归属）。
