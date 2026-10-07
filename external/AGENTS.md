# external — 数据管线（Python + 原始语料，不参与 Go 编译）

`external/go.mod` 把本目录声明成**嵌套 module 边界**，因此仓库根的 `go build ./...` / `go vet ./...` / `golangci-lint run ./...` 会跳过这里（这里只有脚本和原始数据，没有可编译的东西）。不要删这个 go.mod。

规模：**101 个顶层 .py** + 约 90 个源目录 + `medkb/`（5 个模块 + `plugins/` 9 个源插件）。

## 只有 6 条是活管线

| 文件 | 作用 |
|---|---|
| `make_gz.py` | `data/<dataset>/<name>.json → gz/<dataset>/<name>.json.zst`（:7），zstd-19（:65-68）。**增量三态** cached/adopt/build（:231-256），指纹存 `.cache/make_gz_state.json`（:204）；`--check` 是只读解压比对（:272-312，CI 门用它）；`--force` 全量重压，`--jobs N` 并行；平铺游离文件直接失败（:110-134）；`NO_SOURCE_IN_GIT = {"corpus/corpus_statpearls.json"}`（:62）永不清理（gz 是它唯一副本）。压缩是唯一大头：本机 ~2.1MB/s，全量约 7 分钟，而解压比对整批约 3 秒 |
| `split_data.py` | GitHub 100MiB 硬限的应对：`split / merge / verify / status`（:22-25），产物 `<dataset>/X.json.partNNN` + `X.json.parts` 清单（:12-15），默认 `--max-mib 90`（:36）。`make_gz.py` 在整文件缺失时按清单**在内存合并**并逐片 + 整体 sha256 校验 |
| `bake_onnx.py` | Go 端 `cmd/vector-bake` 的逐条镜像：`VECTOR_SKIP_DATASETS` 与 `BUILD_SEARCH_KEYS` / `EXTRACT_KEY_PRIORITY` 都是 Go 侧名单的字面量抄本（**项数不写在这里，也不钉行号**——`internal/knowledge/bake_mirror_sync_test.go TestBakeMirrorMatchesGoSeedRules` 的三个子门 `vector_skip_datasets` / `search_text_keys` / `extract_key_fields` 逐名比对才是权威），扫同一棵 `gz/<dataset>/`（`list_seed_archives`），`--model` 必填。由 `bake-gpu.sh` 在 AutoDL 上调用。**它没有自己的登记表** |
| `embed_server.py` | onnxruntime CPU + CLS pooling + L2，OpenAI 兼容 `POST /v1/embeddings`、`GET /healthz`（:19-21），flags :182-188。`Dockerfile.embed` COPY 的就是它 |
| `export_onnx.py` | optimum 导出 + 可选 INT8 动态量化（:44-58）→ `bge-m3-onnx/`。是 `Dockerfile.embed` 与 `bake-gpu.sh` 的前置（该目录 gitignored，只有跑过这台机器才能建 embed 镜像） |
| `medkb/` | 统一语料管线：`python3 -m medkb fetch\|convert\|validate\|stats <source\|all>`（`__main__.py:1,66-74`）。共享 http 重试/缓存（`http.py`）、`schema.py`（↔ `internal/knowledge/corpus.go` 的 `CorpusDoc` 双侧同步）、`llm.py`（预留）。**输出已是嵌套布局**：`plugins/base.py:38-41 → data/corpus/corpus_<source>.json`，`:44-45 → data/icd11/icd11_terms.json`。新 prose 源 = 加一个插件，零 Go 改动 |

## 其余 ~90 个目录是一次性历史管线

它们的 `fetch_*.py` / `structurize_*.py` / `convert_*.py` 是当年建库时的一次性转换器，目录里通常还留着 `raw/` 原文（txt/PDF）与 `MANIFEST.txt`（标题/URL/机构日期）。

⚠️ **已精确核实：40 个顶层 .py 的输出路径仍写平铺的 `internal/knowledge/data/<name>.json`**（如 `convert_who_zh_health.py:24`、`convert_jkb_health.py:31`、`convert_hpo.py:26`、`fetch_medlinezh.py:100`、`structurize_who.py:20`、`clean_fda_labels.py:16`）。那些目标文件现在都在 `<dataset>/` 子目录里，路径已失效。**重跑它们会在 `make_gz.py:110-134` 报"平铺游离文件"而失败 —— 这是设计内的响亮失败，不是静默错误**；真要重跑，先把输出改成 `<dataset>/` 子目录，且注意这些脚本会覆盖掉手工清洗过的 keywords/正文。

新增数据的正确姿势：`data/<dataset>/<name>.json`（顶层数组、每个元素一行）→ `python3 external/make_gz.py` → `go run . seed-knowledge`。**不需要在任何地方登记文件名**。

## 其他约定

- 大原始包一律 gitignore：`cmb/`、`orphanet/`、`statpearls/`、`.venv/`；缓存目录已挪出本目录（`.cache/gomodcache`、`.cache/pylibs`，见根 AGENTS.md 的 Build caches 一节）。
- `medical_terminology_2024.json` 是**资源清单**（不是种子），留在 `external/` 防 `make_gz` 造孤儿 zst；它的产物经 `convert_medical_terminology.py` 分流进 `internal/knowledge/alias_map.json` 与 `data/public_resources`。
- `ahospital_aliases.py` → `ahospital_aliases.json`（43 条候选，**这两个文件是唯一进 git 的产物**）：从 www.a-hospital.com 已抓页面的「（重定向自 …）」行收割「口语名 ↔ 正式名」对，每条都必须锚在我们已发行的名字上（ICD-10/11、NMPA、Orphanet、HPO、疾病百科、MSD、medical 的 `condition_zh` 八张官方名字表现场比对）。**它不写 `alias_map.json`**，也不该写：合入要人工逐条审（2026-10-07 那轮 35 收 / 8 拒，拒的是「病菌↔细菌」这类医学上不等价的，和「泌尿↔泌尿系统」这类键是目标最短形式的），再审完再改。原始抓取目录 `external/ahospital/`（142MB）整目录 gitignore，全部可重抓。**同一个站点的正文一律不入库**（网友可编辑、实测仅 6% 带来源标记），这条路已经走过并回退，别再加 `medkb` 插件或 `corpus` 数据集。
- `*.log`（`cdc_struct.log`、`dailymed_*.log`、`europepmc.log`、`medlineplus_fetch.log`、`who_factsheets.log`）是抓取历史日志，非管线输入。
- `DOWNLOAD_PROGRESS.md` 记外部数据源的抓取状态；`LLM_PROVIDERS.md` 记结构化脚本用的 provider 降级链（智谱免费优先）。
- 官方 PDF 全文优先本地逐字核对，**商业聚合站/医院科普站的 AI 改写稿数字不可信**（曾把 482 万新发改成"482万宗每60秒9人"）；nhc.gov.cn 有 WAF（WebFetch 412），可用 browser-use 真浏览器；图片版核心信息页要另找文字版。
