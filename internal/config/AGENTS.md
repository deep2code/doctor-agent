# internal/config — 环境变量

`Load()` 读环境 → `Validate()` → `SecurityWarnings()`。配置只有这一条来源链，没有配置文件解析器（`.env` 由 `main.go loadDotenv()` 注入环境）。

| 文件 | 作用 |
|---|---|
| `config.go` | `Config` 结构体；`Load():168-252` 读 **63 个变量**（另有 `KNOWLEDGE_DB_DSN` / `APP_DB_DSN` 在 :340 / :348 经 `os.Getenv` 单独处理）；`Validate`；`SecurityWarnings:301-330`；DSN 组装 `MariaDBDSN` / `KnowledgeDBDSN` / `AppDBDSN` / `MariaDBServerDSN`；`EnsureKnowledgeDB` / `EnsureAppDB` |
| `errors.go` | 包内错误值 |

## 关键行为

- `KnowledgeDBDSN()`：显式 `KNOWLEDGE_DB_DSN` 覆盖，否则由 `MARIA_DB_*` 组装。
- `EnsureKnowledgeDB()`：**`KNOWLEDGE_DB_DSN` 非空时直接返回 nil**。原来它用 `MariaDBServerDSN()`，等于绕过"知识库在另一台机器"这个事实、去业务实例上建一个空的 `doctor_knowledge`。回归门 `config_test.go TestDSNSplit`（把业务端口指向 127.0.0.1:1，任何拨号尝试都会让门变红）。
- `AuthSecret` 为空时生成**随机每进程密钥**（启动告警 + `SecurityWarnings` 一条）：本地开发可以，生产等于每次重启把所有人登出、多实例互不认账。
- `MaxToolIterations` 默认 5（:185）、`MaxToolCalls` 默认 5（:186）、`DEEPSEEK_MODEL` 默认 `deepseek-flash`（:174）。

## 约定

新加变量必须四处齐全：`config.Config` + `Load()` + `.env.example` + `main.go printUsage()`；不安全默认值还会静默出事的，补一条 `SecurityWarnings()` —— 那个函数是运维唯一的启动期建议，`config_test.go` 双向钉死（加固配置产出**零**条告警，fail-open 默认值必须点名每个变量）。

## 已核实的缺口（本轮只记录，未擅自改）

1. **`.env.example` 缺 17 个 `Load()` 实际读取的变量**：`ANTHROPIC_API_KEY`、`ANTHROPIC_MODEL`、`OPENAI_COMPAT_VISION_MODEL`、`MAX_TOOL_ITERATIONS`、`MAX_TOOL_CALLS`、`QUERY_UNDERSTANDING_ENABLED`、`QUERY_UNDERSTANDING_BRANCHES`、`UNDERSTAND_MODEL`、`ALIAS_MAP_PATH`、`MEDIA_DIR`、`VECTOR_DB_PROVIDER`、`QDRANT_HOST`、`QDRANT_PORT`、`EMBEDDING_PROVIDER`、`VOYAGE_API_KEY`、`PUBLIC_BASE_URL`、`ADMIN_PASSWORD`。另有 3 个**只在示例里、`Load()` 不读**（compose 级）：`MARIA_DB_ROOT_PASSWORD`、`QDRANT_IMAGE`、`HF_ENDPOINT`。
2. `SecurityWarnings()` **不覆盖 `ADMIN_PASSWORD`** —— 随机口令的告警打在 `main.go:399-401`，不在这里。
3. `main.go:128 printUsage` 说 `DEEPSEEK_MODEL` 默认 `deepseek-v4-pro`，实际是 `deepseek-flash`（`config.go:174`）。
