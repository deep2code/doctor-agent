# internal/config — 环境变量

`Load()` 读环境 → `Validate()` → `SecurityWarnings()`。配置只有这一条来源链，没有配置文件解析器（`.env` 由 `main.go loadDotenv()` 注入环境）。

| 文件 | 作用 |
|---|---|
| `config.go` | `Config` 结构体；`Load():162-241` 读 **58 个变量**（另有 `KNOWLEDGE_DB_DSN` / `APP_DB_DSN` 在 :324 / :332 经 `os.Getenv` 单独处理）；`Validate:260`；`SecurityWarnings:285-311`；DSN 组装 `MariaDBDSN` / `KnowledgeDBDSN` / `AppDBDSN` / `MariaDBServerDSN`；`EnsureKnowledgeDB` / `EnsureAppDB` |
| `errors.go` | 包内错误值 |

## 关键行为

- `KnowledgeDBDSN()`：显式 `KNOWLEDGE_DB_DSN` 覆盖，否则由 `MARIA_DB_*` 组装。
- `EnsureKnowledgeDB()`：**`KNOWLEDGE_DB_DSN` 非空时直接返回 nil**。原来它用 `MariaDBServerDSN()`，等于绕过"知识库在另一台机器"这个事实、去业务实例上建一个空的 `doctor_knowledge`。回归门 `config_test.go:187 TestKnowledgeDSNSplit`（把业务端口指向 127.0.0.1:1，任何拨号尝试都会让门变红）。
- `AuthSecret` 为空时生成**随机每进程密钥**（启动告警 + `SecurityWarnings` 一条）：本地开发可以，生产等于每次重启把所有人登出、多实例互不认账。
- `MaxToolIterations` 默认 5（:179）、`MaxToolCalls` 默认 5（:180）、`DEEPSEEK_MODEL` 默认 `deepseek-flash`（:168）。

## 约定

新加变量必须四处齐全：`config.Config` + `Load()` + `.env.example` + `main.go printUsage()`；不安全默认值还会静默出事的，补一条 `SecurityWarnings()` —— 那个函数是运维唯一的启动期建议，`config_test.go` 双向钉死（加固配置产出**零**条告警，fail-open 默认值必须点名每个变量）。

## 缺口状态（2026-10-04 审计发现 → 10-05 五项修完、10-06 清掉死变量 + 改掉 hint + 定案「不补改密端点」，六项全部收口）

1. **`.env.example` 与 `Load()` 的双向差**：审计时示例缺 17 个 `Load()` 读取的变量。**12 个是真缺口，已补**（`ANTHROPIC_API_KEY`、`ANTHROPIC_MODEL`、`OPENAI_COMPAT_VISION_MODEL`、`MAX_TOOL_ITERATIONS`、`MAX_TOOL_CALLS`、`QUERY_UNDERSTANDING_ENABLED`、`QUERY_UNDERSTANDING_BRANCHES`、`UNDERSTAND_MODEL`、`ALIAS_MAP_PATH`、`MEDIA_DIR`、`PUBLIC_BASE_URL`、`ADMIN_PASSWORD`）。**剩下 5 个是死变量，字段已随第 2 项删除**，因此反向核对现在**没有豁免项**。回归门 `env_example_test.go`：正向（示例里出现 `config.go` 读不到的键就红，`MARIA_DB_ROOT_PASSWORD`/`QDRANT_IMAGE` 是 compose-only 豁免项）、反向（`config.go` 读的键没出现在示例就红）。
2. ~~**5 个"读进来却没人用"的变量**~~ —— **2026-10-06 已删**：`VECTOR_DB_PROVIDER`、`QDRANT_HOST`、`QDRANT_PORT`、`EMBEDDING_PROVIDER`、`VOYAGE_API_KEY` 五个 `Config` 字段 + `Load()` 的五次读取 + `Validate()` 的 `VECTOR_DB_PROVIDER=qdrant && EMBEDDING_PROVIDER==""` 分支 + `errors.go` 的 `ErrMissingEmbedding`（唯一使用者就是那条分支）。活的对等开关一直是 `VECTOR_STORE_HOST`/`VECTOR_STORE_PORT` 和 `EMBEDDING_BASE_URL`，删前逐名 grep 过：仓库里除本包外零引用。注意 `bake-gpu.sh` 与 `docker-entrypoint.sh` 里的 `QDRANT_HOST`/`QDRANT_PORT` 是**shell 环境变量**（容器名/端口探测），不经 `Config`，与本次删除无关。`env_example_test.go` 的抽取器下限从 60 降到 50（删除后实测 60 个名字，正好贴在上限上会让下一次正常增删误报"抽取器失效"）。
3. `SecurityWarnings()` **不覆盖 `ADMIN_PASSWORD`** —— 随机口令的告警打在 `main.go:404-406`，不在这里。**同轮核出的新缺陷（2026-10-05 记录 → 2026-10-06 按用户口径「去掉」已改）**: 那条告警的 `hint` 原本写"请立刻登录 /admin 修改"，但**全仓库没有改密路径** —— `/admin/users/{id}` 只有 GET/DELETE（`server.go:1728-1760`），`internal/auth` 没有 `ChangePassword`/`UpdatePassword` 之类的函数，`admin.html` 只有登录表单。现在 hint 直接写真实出路（把生成的口令写进 `.env` 的 `ADMIN_PASSWORD` 再重启；丢了只能改业务库 `users` 表或删掉 admin 用户重建），与 `.env.example:178-181` 早已写清的口径一致。**回归门 `internal/server/admin_password_surface_test.go TestAdminUsersSurfaceHasNoPasswordChange`**（离线 AST，不需要 MariaDB）把这个事实钉住：`handleAdminUser` 的 `switch r.Method` 只能是 GET/DELETE，且 `internal/auth` 不得出现名字带 `Password` 的导出方法。**2026-10-06 用户定案「不要改密端点」**：没有 UI 改密从此是**设计**而不是缺口——随机口令的唯一出路是把它写进 `.env` 的 `ADMIN_PASSWORD` 再重启，丢了按 `.env.example:178-181` 的口径直接改业务库 `users` 表（或删掉 admin 用户再重启，`main.go:377-386` 每次启动都检查 `admin` 这个用户名、不存在就重建，所以重建这条路真的走得通）。这道门因此是**决策守卫**：谁补了改密路由或 `internal/auth` 的改密方法它就红，而红的意思是"你在推翻上面这个决定"，要连带翻 hint、`.env.example` 和这份文档。与 `RemoveReferralSentences`、L4 免责声明同类，同属"别再顺手修"那一类，见根 AGENTS.md。
4. ~~`main.go:128 printUsage` 说 `DEEPSEEK_MODEL` 默认 `deepseek-v4-pro`~~ —— 已改为实际的 `deepseek-flash`（`config.go:168`），同时补上漏掉的 `anthropic` provider 与 `ANTHROPIC_API_KEY`/`ANTHROPIC_MODEL` 两行，并把 `VECTOR_STORE_PORT` 的默认值从 6333 改成代码里的 6334（6333 是 Qdrant 的 HTTP 端口，go-client 走 gRPC）。
5. `ALIAS_MAP_PATH` 的默认值 `data/alias_map.json` 在仓库里**不存在**，所以每次启动都会打一条 `Alias map failed to load; using built-in synonyms only` 的 warn（`internal/agent/agent.go:75-77`）——内置表是 `//go:embed` 的 `internal/knowledge/alias_map.json`，永远生效，因此这只是噪音不是缺陷；要外挂自己的表就把文件放到那个路径。
