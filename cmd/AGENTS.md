# cmd — 两个独立入口

这两个命令刻意**不带业务 DB、不带 LLM**，只服务数据链路。主程序 `main.go` 的子命令是另一套（见根 AGENTS.md）。

| 入口 | 干什么 | 依赖 |
|---|---|---|
| `./cmd/kbseed` | gz → MariaDB 播种（开发/CI，以及 `update-kb.sh` 第 1 步） | 需要可连的 MariaDB |
| `./cmd/vector-bake` | gz → Qdrant 离线烘焙（数据镜像构建期） | 需要 Qdrant + `EMBEDDING_BASE_URL`，**不需要 MariaDB** |

## cmd/kbseed/main.go

只有两个 flag（:17-18）：`-dsn`、`-gz`。`-dsn` 为空时取 `KNOWLEDGE_DB_DSN`（:21-23），然后调 `knowledge.Seed`。播种语义（按树 Clear、清掉已消失数据集、并行 INSERT 的事务重放）全在 `internal/knowledge/seed.go`，本文件不重复实现。

## cmd/vector-bake/main.go

手写 `--key=value` 解析（:39-60），支持：`--src`、`--host`、`--port`、`--collection`、`--batch-size`、`--workers`、`--max-text-chars`、`--recreate`、`--wait-green`。环境变量 `EMBEDDING_BASE_URL`（**必填**，:64-72）、`EMBEDDING_API_KEY`、`EMBEDDING_MODEL`、`EMBEDDING_DIMENSIONS`。

- `--recreate` 先删 collection（:106-111）—— 干净重烘必带，因为跨引擎/跨模型的旧点不能混用。
- 结尾有**点数校验 + 一次幂等重放**，仍不足则 exit 1（:162-182）。这是整条烘焙链路唯一的静默风险闸门，别绕过它。
- 期望点数（2026-10-03 口径）：**638,970 点 / 29 数据集 / 102 归档** = `gz/` 的行数减去 `bake.go vectorSkipDatasets` 的 10 个数据集。
- `main.go:98-102` 的 `vector-bake` 子命令只是**退出码 2 的迁移桩**，指向这里；真正的烘焙只有 `go run ./cmd/vector-bake`。

## 相关脚本（在仓库根）

`bake-gpu.sh`（AutoDL fp32 生产烘焙 → 产物回收到 `./qdrant-storage/`）、`build.sh qdrant`（把 `qdrant-storage/` 打包成镜像）。`bake-gpu.sh` 远端跑的是 `python3 external/bake_onnx.py`，它是本命令的逐条镜像 —— 两侧的 (dataset, 文件, 是否烘焙) 三元组由 `internal/knowledge/bake_mirror_sync_test.go:90 TestBakeMirrorMatchesGoSeedRules` 钉住。
