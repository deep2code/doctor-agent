# internal/config — 环境变量

`Load()` 读环境 → `Validate()` → `SecurityWarnings()`。配置只有这一条来源链，没有配置文件解析器（`.env` 由 `main.go loadDotenv()` 注入环境）。

| 文件 | 作用 |
|---|---|
| `config.go` | `Config` 结构体；`Load():168-253` 读 **63 个变量**（另有 `KNOWLEDGE_DB_DSN` / `APP_DB_DSN` 在 :340 / :348 经 `os.Getenv` 单独处理）；`Validate:272`；`SecurityWarnings:301-327`；DSN 组装 `MariaDBDSN` / `KnowledgeDBDSN` / `AppDBDSN` / `MariaDBServerDSN`；`EnsureKnowledgeDB` / `EnsureAppDB` |
| `errors.go` | 包内错误值 |

## 关键行为

- `KnowledgeDBDSN()`：显式 `KNOWLEDGE_DB_DSN` 覆盖，否则由 `MARIA_DB_*` 组装。
- `EnsureKnowledgeDB()`：**`KNOWLEDGE_DB_DSN` 非空时直接返回 nil**。原来它用 `MariaDBServerDSN()`，等于绕过"知识库在另一台机器"这个事实、去业务实例上建一个空的 `doctor_knowledge`。回归门 `config_test.go:187 TestKnowledgeDSNSplit`（把业务端口指向 127.0.0.1:1，任何拨号尝试都会让门变红）。
- `AuthSecret` 为空时生成**随机每进程密钥**（启动告警 + `SecurityWarnings` 一条）：本地开发可以，生产等于每次重启把所有人登出、多实例互不认账。
- `MaxToolIterations` 默认 5（:185）、`MaxToolCalls` 默认 5（:186）、`DEEPSEEK_MODEL` 默认 `deepseek-flash`（:174）。

## 约定

新加变量必须四处齐全：`config.Config` + `Load()` + `.env.example` + `main.go printUsage()`；不安全默认值还会静默出事的，补一条 `SecurityWarnings()` —— 那个函数是运维唯一的启动期建议，`config_test.go` 双向钉死（加固配置产出**零**条告警，fail-open 默认值必须点名每个变量）。

## 缺口状态（2026-10-04 审计发现 → 同日修完；第 2、3 项后半按指示只记录）

1. **`.env.example` 与 `Load()` 的双向差**：审计时示例缺 17 个 `Load()` 读取的变量。**12 个是真缺口，已补**（`ANTHROPIC_API_KEY`、`ANTHROPIC_MODEL`、`OPENAI_COMPAT_VISION_MODEL`、`MAX_TOOL_ITERATIONS`、`MAX_TOOL_CALLS`、`QUERY_UNDERSTANDING_ENABLED`、`QUERY_UNDERSTANDING_BRANCHES`、`UNDERSTAND_MODEL`、`ALIAS_MAP_PATH`、`MEDIA_DIR`、`PUBLIC_BASE_URL`、`ADMIN_PASSWORD`）。**剩下 5 个故意仍不写进示例**，因为它们是死变量（见下条）。回归门 `env_example_test.go`：正向（示例里出现 `config.go` 读不到的键就红，`MARIA_DB_ROOT_PASSWORD`/`QDRANT_IMAGE` 是 compose-only 豁免项）、反向（`config.go` 读的键没出现在示例就红，5 个死变量豁免）。
2. **5 个"读进来却没人用"的变量**：`VECTOR_DB_PROVIDER`、`QDRANT_HOST`、`QDRANT_PORT`、`EMBEDDING_PROVIDER`、`VOYAGE_API_KEY` 被 `Load()` 填进 `Config`，除本包 `Validate()` 的一条 `VECTOR_DB_PROVIDER=qdrant && EMBEDDING_PROVIDER==""` 判断外**没有任何消费方**；活的对等开关是 `VECTOR_STORE_HOST`/`VECTOR_STORE_PORT` 和 `EMBEDDING_BASE_URL`。它们没进 `.env.example` 是有意的：示例广告一个无效的旋钮比不写更糟（用户设了会以为生效）。删掉这五个字段 + 那条 Validate 分支是一行跟手的清理，等指示。
3. `SecurityWarnings()` **不覆盖 `ADMIN_PASSWORD`** —— 随机口令的告警打在 `main.go:401-403`，不在这里。**同轮核出的新缺陷（2026-10-04，只记录未改）**: 那条告警的 `hint` 写着"请立刻登录 /admin 修改"，但**全仓库没有改密路径** —— `/admin/users/{id}` 只有 GET/DELETE（`server.go:1728-1760`），`internal/auth` 没有 `ChangePassword`/`UpdatePassword` 之类的函数，`admin.html` 只有登录表单。所以随机口令丢了只能改业务库 `users` 表或删掉 admin 用户重建；`.env.example` 里已经按实情写清，`main.go:403` 那句 hint 本身怎么改（去掉承诺 / 补一个改密端点）等产品决策。
4. ~~`main.go:128 printUsage` 说 `DEEPSEEK_MODEL` 默认 `deepseek-v4-pro`~~ —— 已改为实际的 `deepseek-flash`（`config.go:174`），同时补上漏掉的 `anthropic` provider 与 `ANTHROPIC_API_KEY`/`ANTHROPIC_MODEL` 两行，并把 `VECTOR_STORE_PORT` 的默认值从 6333 改成代码里的 6334（6333 是 Qdrant 的 HTTP 端口，go-client 走 gRPC）。
5. `ALIAS_MAP_PATH` 的默认值 `data/alias_map.json` 在仓库里**不存在**，所以每次启动都会打一条 `Alias map failed to load; using built-in synonyms only` 的 warn（`internal/agent/agent.go:75-77`）——内置表是 `//go:embed` 的 `internal/knowledge/alias_map.json`，永远生效，因此这只是噪音不是缺陷；要外挂自己的表就把文件放到那个路径。
