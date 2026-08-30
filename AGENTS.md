# AGENTS.md — doctor-agent

循证医学 (evidence-based medicine) AI assistant for the entire Chinese population — everyday health problems first, with China's high-burden conditions (地贫, G6PD, 鼻咽癌, 乙肝, 乳糖不耐受, 登革热, 南方省份尤为高发) as an additional layer. Answers structure: 可能的原因 → 相似情况/常见病例 → 家庭护理 → 何时就医. Go module `github.com/doctor-agent` (Go 1.26), pluggable LLM (Anthropic/DeepSeek/OpenAI-compat: Zhipu/Qwen/豆包) + embedded JSON knowledge base.

## Project

- **Entry point**: root `main.go` (package `main` at module root) — NOT `./cmd/doctor-agent`. Subcommands: `chat`, `serve`, `verify-knowledge`, `version`, `seed-knowledge`.
- Stack: stdlib `net/http` server, `log/slog` logging, `github.com/anthropics/anthropic-sdk-go` (Claude), `github.com/qdrant/go-client` (optional vector retrieval). No external logger/framework.
- Config: env vars (see `.env.example`); `main.go` `loadDotenv()` loads `.env` if present; `internal/config.Load()` applies defaults.
- Domain content is Chinese; code identifiers/comments are English.

## Commands

```bash
go build ./...                                   # compiles (verified)
go build -o bin/doctor-agent .                   # correct binary build
go vet ./...                                     # passes
go test ./...                                    # passes (agent, server, session, config, safety, knowledge, llm, evals)
go run . chat                                    # interactive CLI (streaming output; needs API key)
go run . serve                                   # HTTP on 0.0.0.0:8080 (/health, /chat, /chat/stream real SSE)
go run . verify-knowledge                        # knowledge-base integrity check (DOI/PMID format, uniqueness, traceability, version)
go run . verify-knowledge -urls                 # also probe citation URL liveness (online, slow)
go run . seed-knowledge                         # build knowledge store (MariaDB) from internal/knowledge/gz/*.json.*z*
go run . seed-knowledge --db='root:pass@tcp(localhost:3306)/doctor_knowledge?parseTime=true' --src=internal/knowledge/gz
go run ./cmd/vector-bake                      # offline gz -> Qdrant bake (RAG data image; no MariaDB needed)
go run ./evals                                  # offline eval on sample_answers.json (26-question golden set)
go run ./evals -online                          # online eval: runs real agent per question (needs API key; slow)
go run ./evals -answers my.json -report out.json # eval custom answers + JSON report; exit 1 on any failure (CI-friendly)
./build.sh [app|qdrant|full]                       # 唯一打包入口: 构建+推送镜像到阿里云
golangci-lint run ./...                            # lint (v2.12.2 installed at /Users/junjunyi/gopath/bin; .golangci.yml is v2 format, same version in CI)
python3 external/make_gz.py                        # regenerate internal/knowledge/gz/*.json.zst after editing data/*.json
```

⚠️ Makefile was removed 2026-08-30 (local-dev targets folded into plain go/lint commands above; Docker packaging lives solely in `./build.sh`).

⚠️ **Fixed 2026-08-09**: `Makefile` and `README.md` previously referenced `./cmd/doctor-agent` (nonexistent) — `make build/chat/serve` failed. Both now use root `main.go` (`go build -o bin/doctor-agent .`, `go run . chat/serve/verify-knowledge`).

## Architecture

Pipeline (in `internal/agent/agent.go` `ProcessMessageStream` — `ProcessMessage` is a non-streaming wrapper): L1 emergency detection → L2 scope guard → knowledge retrieval → layered system prompt → agent loop (≤5 iterations, tool-use, streaming deltas) → L3 citation post-verification → L4 disclaimer. Sessions live in a mutex-guarded `map[string]*Session`; with `SESSION_DIR` set they are snapshotted to JSON files and restored across restarts.

- `internal/agent` — orchestrator `Agent`; builds provider-agnostic messages, dispatches tools, applies safety layers.
- `internal/knowledge` — **database-backed (no embedded data)**: the compiled binary contains ONLY logic. Every dataset lives in an external **MariaDB** knowledge store (`doctor_knowledge` database; DSN via `MARIA_DB_*` env or explicit `KNOWLEDGE_DB_DSN`). `kb.go` is the `KB` layer (`kb_items(id, dataset, key, data MEDIUMBLOB)` + `InsertBatch`/`All`/`Search`/`Clear`; upsert via `INSERT ... ON DUPLICATE KEY UPDATE`; `data` column gzip-compressed); `loader.go` `Store` is a sync.Once singleton whose `ensureXxx()` methods load a dataset **lazily from the DB on first use** and cache it in RWMutex-guarded maps (runtime retrieval = "检索时直接查库"; cold read hits MariaDB, warm read cached). `seed.go` `Seed()` reads `gz/` archives (`.json.gz` legacy gzip / `.json.zst` zstd-19, auto-detected by magic bytes) and bulk-inserts rows (`seed-knowledge` CLI). Source JSONs in `data/` are zstd-compressed (level 19) into `gz/` by `external/make_gz.py` (~38% smaller than the old gzip-9); LFS-pointer sources are skipped. Go loaders read both formats via magic-byte detection (`internal/knowledge/archive.go`). `Retriever` interface with `retriever_keyword` (BM25 + CJK substring/bigram matching), `vector_store.go` (Qdrant vector storage + retrieval), `retriever_vector.go` (semantic search via embeddings), `retriever_hybrid` (RRF fusion); `CitationFormatter`, schemas in `schemas.go`. `verify.go` also hosts `CheckURLLiveness` (probes citation URLs).
- `internal/llm` — `LLMProvider` interface (`Chat`, `StreamChat(ctx, messages, tools, systemPrompt, onDelta)`, `Name()`); `anthropic_provider.go` (NewStreaming), `deepseek_provider.go` + `openai_compat_provider.go` sharing `openai_stream.go` (SSE parsing + tool-call fragment accumulation); provider-agnostic `Message`/`ToolDefinition`/`ToolCall`; **multimodal support** (`ContentPart`/`ImageInput` for medical image analysis).
- `internal/tools` — `Tool` interface (`Name/Description/Schema/Execute` → `*ToolResult{Success, Data, Error, Citations}`) + `Registry` (mutex, insertion-ordered). 35 tools: drug_safety_check, genetic_risk_calculator, food_risk_analyzer, symptom_triage, reference_lookup, lab_interpreter, literature_search, msd_search, variant_lookup, medline_search, drug_lookup, eml_lookup, drug_label_lookup, nhc_search, fhs_search, aap_search, icd10_lookup, nmpa_drug_lookup, medical_kg_lookup, disease_encyclopedia_lookup, cpubmed_kg_lookup, huatuo_qa_lookup, ttd_lookup, drug_interaction_check, disease_symptom_lookup, target_disease_lookup, disease_drug_lookup, medical_qa_lookup, sider_lookup, triage_department, body_part_lookup, lab_report_interpret, medical_image_analyze.
- `internal/safety` — `EmergencyDetector`, `ScopeGuard`, `PostVerifier` (citation realness + optional LLM-as-judge claim-support check via `POST_VERIFY_SEMANTIC`), `DisclaimerService`.
- `internal/knowledge` — also hosts `verify.go` (`VerifyData` integrity report, `ReportText`) and `BuildCitedSources` (flat citation-number → source map for post-verification).
- `evals/` — anti-hallucination golden set (`questions.json`, 26 questions) + `main.go` CLI (offline/online modes, keyword/refusal/citation/must-not checks). Add new questions here; run before changing prompts or knowledge data.
- `internal/prompt` — `Composer` assembling the 5-layer system prompt; `BuildPatientContext`.
- `internal/server` — stdlib HTTP server: `/health`, `/chat`, `/chat/stream` (real token-level SSE: `delta`/`done`/`error` events), `/feedback` (user rating endpoint), middleware chain: CORS allowlist (`CORS_ORIGINS`) → OPTIONS → rate limit (`RATE_LIMIT`, per-IP fixed window) → Bearer auth (`API_KEY`, `/health` exempt) → slog logging.
- `internal/session` — conversation history as `[]llm.Message` (provider-agnostic) + patient context (`@region`, `@g6pd`, `@thal` CLI commands); `FileStore` persists JSON snapshots to `SESSION_DIR`.
- `internal/dialogue` — multi-turn dialogue state management: `IntentRecognizer` (keyword-based intent classification), `DialogueState` (state machine with slots for symptom/drug/body part), `Manager` (session-aware dialogue flow control).
- `internal/database` — MariaDB database layer (`go-sql-driver/mysql`, pure Go, no CGO); tables: users, sessions, messages, feedback (database `doctor_agent`).
- `internal/auth` — user authentication service (admin-only user creation/login/token); SHA256+salt password hashing; `AdminCreateUser` for admin-only user management.
- `internal/embedding` — OpenAI-compatible embedding provider for semantic search.
- `internal/config` — env-based `Config`, `Load()`, `Validate()`.

## Conventions

- Errors: `fmt.Errorf("context: %w", err)`; wrap at boundaries.
- Logging: `slog` everywhere; `slog.Info` for lifecycle, `slog.Debug` for detail, `slog.Warn` for safety hits. No logrus/zap.
- Concurrency: `sync.RWMutex` for Store/Registry reads, `sync.Once` for the knowledge singleton; channels + goroutines in hybrid retriever. New shared state must be mutex-guarded.
- Tools: JSON-schema `Schema()` (snake_case keys), `Execute` returns `ToolResult` — never returns raw data without `Success`/`Citations`.
- Config changes: add to `config.Config` + `Load()` + `.env.example` + `printUsage()` in `main.go`.
- New knowledge: add JSON to `internal/knowledge/data/` AND register its filename in `knowledge/seed.go` `seedFile()` switch (classify → dataset + rows), or it is silently ignored. Then run `python3 external/make_gz.py` to regenerate the compressed `gz/` copies, then `go run . seed-knowledge` to seed the MariaDB knowledge store. Ensure a matching lazy loader exists: add an `ensureXxx()` + `loadXxx()` pair in `loader.go` and a getter that calls `ensureXxx()` so runtime retrieval populates it from MariaDB. Update `data/version.json` sources on data changes.
- Retrieval gotcha: `tokenize()` does NOT segment Chinese — CJK recall depends on substring + bigram matching against keywords/symptom fields in `retriever_keyword.go`. Symptom-style questions ("我一喝牛奶就拉肚子") now recall correctly (probe: 12/12); keep new entries' `keywords` in Chinese symptom vocabulary.
- Institutional publications (WHO/IARC/NCCN/中国指南) have no DOI/PMID by design — leave their `journal` empty so `verify-knowledge` doesn't flag them as untraceable journal articles.
- Keep UI strings Chinese; don't translate domain content to English.

## Notes

- (add quick notes here — e.g. decisions, gotchas, future work)
- **Knowledge base in MariaDB (2026-08-24)**: all `//go:embed` knowledge JSON removed; data lives in **MariaDB** (`doctor_knowledge` database; DSN via `MARIA_DB_*` env or `KNOWLEDGE_DB_DSN`), seeded by `go run . seed-knowledge` from `gz/` archives (zstd, magic-byte-detected). Binary dropped 95MB→43MB and contains only logic. Loading is **lazy per dataset** at retrieval time (`Store.ensureXxx()` → MariaDB read → in-memory cache). `config.KnowledgeDBDSN()` composes the DSN from `MARIA_DB_*` (an explicit `KNOWLEDGE_DB_DSN` env overrides). Storage: `kb_items(id, dataset, key, data MEDIUMBLOB)` with gzip-compressed `data` (magic-byte check allows back-compat reads); upsert via `INSERT ... ON DUPLICATE KEY UPDATE`. The `search_text` column was dropped and rebuilt in Go inside `KB.Search` (only the optional vector-retrieval candidate path uses it). Business store (users/sessions/messages/feedback) is also MariaDB (`doctor_agent` database, `config.AppDBDSN()`). Docker Compose bundles a `mariadb` service alongside Qdrant and the app.
- **Vector retrieval 默认开启 + 本地无模型 embedding (2026-08-24)**: `VECTOR_STORE_ENABLED`/`EMBEDDING_ENABLED` 默认值改为 `true`。检索在 `agent.New` 中默认构建 **HybridRetriever**(keyword + vector, RRF 融合, vectorWeight=0.4)。向量库仍用 Qdrant(`internal/knowledge/vector_store.go`, `NewVectorStore` 改为**懒连接**——创建时不 ping,`EnsureCollection` 由 syncer 在 `FullSync`/`IncrementalSync` 前调用,故启动不阻塞;Qdrant 不可达时 `VectorRetriever.Retrieve` 报错 → `HybridRetriever` 自动降级为关键词检索,无报错)。embedding 默认 `internal/embedding/local.go` 的 `LocalProvider`=进程内 FNV 哈希词向量(CJK 单字+bigram + Latin 词 token,L2 归一化固定 1024 维,零模型/零网络/纯 Go),由 `embedding.NewDefault(baseURL,apiKey,model)` 选择:有 `EMBEDDING_BASE_URL`+`EMBEDDING_API_KEY` 才走 OpenAI 兼容远程,否则本地。语义召回属"弱语义≈词面重叠"(用户确认的方案)。**激活向量召回需手动跑一次 `go run . sync-knowledge`**(本地 embedding 离线可跑,约 1.2M 文档;首次为空集合时向量腿返回空,检索退化为关键词)。server.go:712 与 main.go:462 的 sync 路径已改用 `embedding.NewDefault`。
- ✅ Fixed: `Makefile`/`README.md` stale `cmd/doctor-agent` path (2026-08-09) — both now use root `main.go`; `make build/chat/serve/verify-knowledge` work again.
- `verify-knowledge` now passes clean (0 warnings): 50 medical entries, 90 citations (28 DOI + 7 PMID + 40 WHO URL; DOI/PMID traceability 35.6%).
- Hallucination guard: when retrieval returns nothing, `prompt.NoKnowledgeGuidance` is injected — the model must state the KB doesn't cover the topic and steer (ask follow-up / advise clinic) instead of improvising.
- Semantic verification (`POST_VERIFY_SEMANTIC`) now **defaults to false** (2026-08-09) — set `true` or `POST_VERIFY_JUDGE_MODEL` to a cheap model to re-enable; it roughly doubles LLM cost per response.
- Known honest gap: ~~no true baseline from `go run ./evals -online` yet~~ **baseline established 2026-08-08**: 中文 36 题通过率 72.2%(26/36,拒答 7/7 正确),模型 = Zhipu glm-4-flash(免费)经 `LLM_PROVIDER=openai-compat`。英文 299 题与 Claude 基线未跑(需 ANTHROPIC_API_KEY;glm-4-flash 约 1 题/30s)。
- **OpenAI-compatible LLM provider (2026-08-08)**: `LLM_PROVIDER=openai-compat` + `OPENAI_COMPAT_BASE_URL/API_KEY/MODEL`(Zhipu/Qwen/SiliconFlow 等任意 OpenAI 协议端点;实测 Zhipu glm-4-flash 免费可用)。实现 `internal/llm/openai_compat_provider.go`(泛化 DeepSeekProvider)。judge 复用同端点。
- **Literature retrieval layer (2026-08-08)**: `literature_search` tool searches `internal/knowledge/data/literature.json` (4425 Europe PMC abstracts, 16 topics, 8.8MB embedded). `RetrieveLiterature` routes Chinese queries via topic keywords (`TOPICS` table in `external/convert_europepmc.py` — add symptom vocabulary there, then regenerate) and matches English via title/abstract substrings. Tool-returned PMIDs register as `[PMID]` citation sources in post-verification (`knowledge.AddToolSource`). Regenerate with `python3 external/convert_europepmc.py`.
- **WHO 官方中文知识库 (2026-08-08)**: `internal/knowledge/data/who_factsheets.json` — **232 条** WHO 官方中文 fact sheets（全量；2026-08 全量结构化入库，SELECTED 40 条的历史描述已过时）,URL 引用(WHO 无 DOI/PMID 属设计)。生成管线: `external/fetch_who_factsheets_zh.py`(抓 `/zh/` 中文版)→ `external/structurize_who.py`(LLM 结构化,provider 自动降级: Zhipu glm-4-flash 免费优先 → Qwen → SiliconFlow; 已验证 SiliconFlow 余额不足/Qwen access denied 时切换可用)。人工审核+清洗: 删除无数据 prevalence、空数组、补口语关键词(症状检索召回)。检索口语词补充规则: 新增条目后跑 `go run ./evals` 类似的中文口语查询验证。
- **国家卫健委诊疗指南全文层 (2026-08-09)**: `nhc_guides.json` 39 篇中文全文（30 文字 + 9 OCR 无 URL），`nhc_search` 工具（第 14 个），检索仿 MSD 评分 + `nhcSynonyms` 口语→病名映射。管线 `external/convert_nhc.py`（标题清洗/截断回退）。注意正文含用药方案（查"阿莫西林"命中动物致伤规范属真实命中）。
- **育儿知识层 (2026-08-09)**: 4 个源一次性接入。
  1. `china_vaccines.json` — 国家免疫规划疫苗儿童免疫程序（2021 版）10 条 KnowledgeEntry（总览+9 疫苗族+特殊健康状态儿童），`external/convert_immunization.py`（官方 PDF 广东疾控镜像 3442966.pdf 提取），category="vaccine_cn"，无新工具（走现有检索）。citation type "national_guideline"→"国家级指南"。
  2. `feeding_guidelines.json` — 中国婴幼儿喂养指南(2022)三册 + 卫健委喂养核心信息 4 条 KnowledgeEntry，`external/convert_feeding.py`（准则经 cnsoc 官方页/东莞疾控/卫健委核心信息交叉验证；0-6月龄 6 准则/7-24月龄 6 准则/学龄前 5 准则/核心信息 10 条）。
  3. `fhs_guides.json` — 香港卫生署家庭健康服务育儿百科 103 页简体中文全文检索层，`fhs_search` 工具（第 15 个）。**坑: fhs.gov.hk 全站 JS 渲染菜单, HTML 无文章链接、无 sitemap、数字 id 不连续——URL 靠 websearch `site:fhs.gov.hk sc_chi ...` 收集(存 `external/fhs/urls.txt`), 正文在 `#content` 外的容器（提取 #container-wrapper 文本）**。`fetch_fhs.py` + `convert_fhs.py`。检索含 `fhsSynonyms` 大陆→香港用词映射（辅食→固体食物、睡姿→仰睡）。
  4. `aap_articles.json` — 美国儿科学会 healthychildren.org 育儿百科 264 页英文全文检索层，`aap_search` 工具（第 16 个）。sitemap.xml 是 **UTF-16** 编码（解析坑）；正文 `#mainContent` 从 "Page Content" 后截断。`fetch_aap.py`（sitemap URL 过滤 ages-stages 等 5 个板块）+ `convert_aap.py`。检索仿 medlineplus（英文 token）。
  ⚠️ 三份知识库共用 `cjkWindows`/`nhcSynonyms`/`scoreNHC` 模式——新增中文全文检索层照抄 retriever_nhc.go 即可。
- **WHO 疫苗立场文件 (2026-08-08)**: `internal/knowledge/data/who_vaccines.json` — 12 条疫苗 position papers(狂犬病 2018/乙脑 2015/HPV 2017/乙肝 2017/登革热 2018/流感 2022 中文/伤寒 2018/霍乱 2017/破伤风 2017/轮状 2021/麻疹 2017/肺炎球菌 2019),category="vaccine",evidence=international_guideline,URL 引用(journal 留空防 verify 误报)。管线: `external/fetch_position_papers.py`(IRIS DSpace API:搜索→取 ORIGINAL/TEXT bitstream;注意整期 WER 为英法双语、多语言版本是独立条目、bitstream 可能错配→必须验证内容含疫苗名)→ pypdf 提取(`PYTHONPATH=$PWD/.cache/pylibs`;TEXT bundle 的 .txt 有时不可靠,PDF 提取更稳)→ `external/structurize_pp.py`(LLM 结构化;`extract_section` 对整期文本按 "position paper"/疫苗名定位截取,避免截到其他文章)。
- **MSD 默沙东诊疗手册中文版 (2026-08-08)**: `internal/knowledge/data/msd_manual.json` — 大众版+专业版全文检索层(6086 页,43.6MB)。`msd_search` 工具(第 9 个)。检索 `RetrieveMSD`(retriever_msd.go):完整中文查询词标题匹配 +20 优先,3+ 字窗口次之,2 字窗口弱信号;Latin token(G6PD/HPV)大小写不敏感。管线: `external/fetch_msd.py [home|professional]`(zh sitemap 过滤 4 段正文页,排除 multimedia/resources;source 字段区分版本)→ `external/merge_msd.py` 合并嵌入。注意:MSD 是百科式全文(非 KnowledgeEntry),独立检索层;页面含"完整评审/上次更新"元数据。
- **WHO 基本药物清单 (2026-08-09)**: `internal/knowledge/data/who_eml.json` — WHO EML 第24版(2025) 564 种药物(core 441/complementary 123)，含剂型规格与一线/二线适应症。`eml_lookup` 工具(第 12 个)。解析管线 `external/parse_eml.py`(PDF 提取文本→结构化；坑: PDF 丢失缩进→需把顶格剂型行并入当前条目、private-use 方块符 \uf06f 需清除、子标题正则必须 `\.?` 以匹配 `6.2.1 Access`)。检索 `RetrieveEMLDrug`(retriever_eml.go):中文名经内置 ~200 常用药别名表映射到 INN，英文精确/子串匹配。`name_zh` 全量 LLM 翻译待办(需 API key，检索已可用)。
- **ClinVar 基因变异库 (2026-08-08)**: `internal/knowledge/data/clinvar.json` — 地贫/G6PD 核心基因(HBB/HBA1/HBA2/G6PD)的致病及可能致病变异 1399 条(376KB)。`variant_lookup` 工具(第 10 个)。检索 `RetrieveClinVar`(retriever_clinvar.go):cDNA 变异名(c.79G>A)精确 +10、基因符号/中文别名(HBB/β地中海贫血)+4~5、trait 疾病名 +3。管线: `external/fetch_clinvar.py`(NCBI E-Utilities:esearch 按基因→esummary 批量;注意 esummary 批量对 6 位旧 id/结构变异返回空,已含重试;缺失 200 条为 CNV/大片缺失,非点突变)。缓存 `external/clinvar/{gene}.json` 幂等。
- **English eval set (2026-08-08)**: `evals/questions_en.json` (MedQA 200 + PubMedQA 99, generated by `external/convert_evalsets.py`). evals now support `Question.ExpectedOption` (A-D / yes-no-maybe); English MCQ/PubMedQA categories skip the `[N]` citation requirement. Run: `go run ./evals -questions evals/questions_en.json`.
- **FDA 药品标签与 China CDC 管线 (2026-08-09)**: `external/fetch_dailymed.py`(EML 564 药 → RxNorm RXCUI → OpenFDA label sections → `external/dailymed/`; 注意 OpenFDA label 无 openfda.rxcui 且 generic_name 是子串匹配→取 limit=5 打分选纯品标签; sort=submission_status 非法 400; setid 取 label["id"]) + `structurize_dailymed.py`(LLM 中文化 → `fda_drug_labels.json` 344 条; **仅 Zhipu glm-4.7-flash 免费, 必须带 `thinking:{"type":"disabled"}` 否则 content 为空; Qwen/SiliconFlow fallback 已移除防付费; 失败幂等可重试) + `clean_fda_labels.py`(keywords 逗号单串拆分、死 URL 修复)。`drug_label_lookup` 工具(第 13 个)。CDC: `fetch_cdc.py`(jkts 健康提示; 文章链接单引号 `./{yyyymm}/t{...}.html`, 标题在 #articleCon 前 h5, 正文 trs_editor_view, 旧文章 404) + `structurize_cdc.py`(LLM 按疾病拆分) + `convert_cdc.py`(月度重复疾病合并+剔除通用症状词, 避免挤占专病条目检索; 文件名日期正则须 `t(\d{6})(\d{2})_(\d+)`) → `cdc_entries.json` 26 条。
- **Build caches (2026-08-30)**: the sandbox-era module cache and Python deps
  used to live at `external/gomodcache` (645MB) and `external/pylibs` (325MB)
  — moved OUT of `external/` (keep it a pure data pipeline) into the repo's
  own `.cache/` (`$PWD/.cache/gomodcache`, `$PWD/.cache/pylibs`; gitignored,
  dockerignored). Current env uses the default GOPATH (`go env GOMODCACHE`).
  If a sandbox ever blocks system GOPATH again: `GOMODCACHE="$PWD/.cache/
  gomodcache"`, `PYTHONPATH="$PWD/.cache/pylibs"`. `external/go.mod` makes
  `external/` a nested module boundary so `./...` patterns skip it (otherwise
  golangci-lint/go list fail with "directory ... outside main module").
- **Built-in web chat UI (2026-08-09)**: `internal/server/web/index.html` (single-file HTML+CSS+JS, embedded via `//go:embed`, zero deps/offline). `GET /` serves it; binary with **no args defaults to web mode** (`startWebUI` → serve + one-time key setup, prints "打开 http://localhost:8080"). UI: chat bubbles + SSE streaming (`/chat/stream` delta/done/error), localStorage conversation persistence, "新对话" button, example question chips, mobile responsive, minimal inline Markdown renderer (bold/lists/headings/blockquote). `/` and `/health` are exempt from auth/rate-limit; API paths stay gated. Server test `TestWebUIServed` covers it.
- **Zero-config first-run setup (2026-08-09)**: 下载 Release 二进制后双击 `start-chat`（release.yml 每平台附带的一键脚本）即可用。无 API Key 时 `runChat` 触发交互引导（三选一：**智谱 glm-4-flash 免费 / DeepSeek / 豆包(火山方舟)**，火山方舟 `model` 直接用模型名如 `doubao-seed-2-1-pro-260628` 或接入点 ID `ep-xxxx`），把 `LLM_PROVIDER`/key 写入 `~/.doctor-agent/config.env`（0600）。**配置优先级：当前目录 `.env` > 用户主目录 `~/.env`（文件级回退，只用一个）> 全局环境变量 > `~/.doctor-agent/config.env`（最低，仅填空）**。`runServe` 缺 key 时打印指引。用户侧零技术门槛；开发侧一切照旧。
- **golangci-lint (2026-08-09)**: local install is **v2.12.2** (`/Users/junjunyi/gopath/bin/golangci-lint`); `.golangci.yml` is the v2 format (`version: "2"`, `linters.default: standard`). CI installs the same via `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`. `make lint` passes clean (0 issues); v2 standard set is stricter than v1 (QF1012/QF1003/errcheck) — all fixed.
- **External knowledge downloads (in progress)**: see `external/DOWNLOAD_PROGRESS.md` for status & resume steps (Europe PMC ✅ 接入, MedlinePlus ✅ 1017/1017 页, WHO ✅ 241/241, HPO ✅ hp-base.obo, evalsets ✅ 接入). Run resumed scripts from repo root.
- **Optimization batch (2026-08-09)**: 真流式 `/chat/stream`(token 级 SSE) + `StreamChat`(三个 provider); session 类型解耦(不再依赖 anthropic-sdk-go) + JSON 文件持久化(`SESSION_DIR`); server 安全中间件(`API_KEY`/`CORS_ORIGINS`/`RATE_LIMIT`); `POST_VERIFY_SEMANTIC` 默认 false; 补测试(agent/server/session/config, 全量含 race 通过); CI workflow + `.golangci.yml`; 知识库 gzip embed(二进制 95MB→52MB, `make gz`); `verify-knowledge -urls` 引用 URL 可达性检查(发现 4 个 WHO 疫苗链接 404 待修); `cdc_alerts.json` 移出 embed 至 `external/cdc/`(死文件); CLI `clear` 命令 bug 修复。
- **ICD-10 疾病编码库 + NMPA 药品目录 (2026-08-10)**: `internal/knowledge/data/icd10_diseases.json` — 35,862 种疾病 ICD-10 编码(国家临床版2.0),`nmpa_drugs.json` — 167,615 种药品(164,474 国产 + 3,141 进口,国家药品编码本位码)。数据源: [hint-lab/chinese-medical-kg](https://github.com/hint-lab/chinese-medical-kg) Excel 文件(981KB + 11.9MB + 298KB) → Python 转换 → JSON。`icd10_lookup` 工具(第 17 个):按编码或中文名查询疾病分类。`nmpa_drug_lookup` 工具(第 18 个):按药品名查询 NMPA 批准信息。Go 结构体 `ICD10Disease`/`NMPADrug` 在 `schemas.go`,加载器在 `loader.go`,getter 方法支持并发安全访问。gzip 压缩后嵌入(4.24MB→0.36MB + 20.49MB→1.26MB)。
- **OpenCMKG 医学知识图谱 (2026-08-10)**: `internal/knowledge/data/medical_kg_triples.json` — 354,752 条医学三元组(疾病-症状-药物-食物-检查-治疗-科室关系)。数据源: [RuiqingDing/OpenCMKG](https://github.com/RuiqingDing/OpenCMKG) triples.txt(19.5MB) → Python 转换 → JSON。`medical_kg_lookup` 工具(第 19 个):查询医学知识图谱,支持 10 种关系类型(disease_has_symptom/disease_recommand_drug/disease_recommand_food/disease_noteat_food/disease_need_check/disease_acompany_disease/disease_eat_food/disease_need_treatment/disease_common_drug/disease_belong_department)。Go 结构体 `MedicalKGTriple` 在 `schemas.go`。gzip 压缩后嵌入(45.49MB→3.29MB)。
- **MedicalGPT-zh 对话种子 (2026-08-10)**: `internal/knowledge/data/medical_dialogues.json` — 90 条医患对话种子(用药建议/病因分析/病情诊断/治疗方案等 29 类)。数据源: [2132660698/MedicalGPT-zh](https://github.com/2132660698/MedicalGPT-zh) dialogue_seed_task.json(86KB)。Go 结构体 `MedicalDialogue` 在 `schemas.go`。gzip 压缩后嵌入(0.09MB→0.03MB)。
- **CMeKG 疾病百科 (2026-08-10)**: `internal/knowledge/data/disease_encyclopedias.json` — 8,807 种疾病百科(症状/病因/预防/治疗/药物/食物/并发症/检查/科室/费用等 24 字段)。数据源: [liuhuanyong/QASystemOnMedicalKG](https://github.com/liuhuanyong/QASystemOnMedicalKG) medical.json(45MB NDJSON) → Python 转换 → JSON。`disease_encyclopedia_lookup` 工具(第 21 个):查询疾病百科数据库。Go 结构体 `DiseaseEncyclopedia` 在 `schemas.go`。gzip 压缩后嵌入(58.4MB→4.5MB)。
- **CPubMed-KG 医学知识图谱 (2026-08-21)**: `internal/knowledge/data/cpubmed_kg.json` — 77,265 条医学三元组(药物治疗 16,366/辅助治疗 15,493/实验室检查 13,069/临床表现 5,518/影像学检查 5,097 等 15+ 关系类型),覆盖 48 种疾病(高血压/糖尿病/冠心病/脑卒中/慢阻肺/慢性肾病/肝硬化/肺癌/抑郁症/癫痫/帕金森病/痛风/贫血/肺炎等)。数据源: CPubMed-KG API (`cpubmed.openi.org.cn`) → Python 抓取。`cpubmed_kg_lookup` 工具(第 22 个):查询 PubMed 文献挖掘的知识三元组。Go 结构体 `CPubMedTriple` 在 `schemas.go`。gzip 压缩后嵌入(6.06MB→1.45MB)。
- **Huatuo26M-Lite 医疗问答 (2026-08-21)**: `internal/knowledge/data/huatuo_qa.json` — 177,703 条真实医患问答(16 科室: 妇产科 34K/内科 30K/皮肤科 25K/儿科 21K 等,覆盖 2,701 种疾病)。数据源: [FreedomIntelligence/Huatuo26M-Lite](https://huggingface.co/datasets/FreedomIntelligence/Huatuo26M-Lite) (Apache 2.0)。`huatuo_qa_lookup` 工具(第 23 个):支持关键词+科室筛选,按相关性评分排序。Go 结构体 `HuatuoQAPairs` 在 `huatuo_types.go`。gzip 压缩后嵌入(140.6MB→~30MB)。注意:该数据集是社区贡献的 QA 对,非结构化知识条目,适用于患者教育和症状问答场景。

## Data-image architecture (2026-08-29)

- **MariaDB = business only** (users/sessions/messages/feedback, DB `doctor_agent`).
- **Qdrant = professional RAG**: the `doctor-agent-qdrant` image bakes all 51
  gz datasets into Qdrant storage **at build time** via `doctor-agent vector-bake`
  (`internal/knowledge/bake.go`: reads gz → `seedFile` classification → offline
  `LocalProvider` embedding → upsert with full entry JSON in payload `data`).
  The image is self-contained: start → retrieval works, no seed/sync wait.
- `vector-bake` does NOT need MariaDB. `VectorRetriever.Retrieve` prefers the
  self-contained payload `data` (JSON → entry) and only falls back to the
  in-memory MariaDB store for legacy runtime-synced indexes.
- `docker-compose.yml`: qdrant service uses `QDRANT_IMAGE` (default public
  repo `doctor-agent-qdrant:latest`); app depends on mariadb + qdrant only;
  optional MariaDB keyword fallback = mount gz volume + `SEED_MARIADB_KB=true`
  (see `docker-entrypoint.sh`).

## Image layering (2026-08-30, v18) — 双镜像触发条件解耦

- `doctor-agent-qdrant` (root `Dockerfile.qdrant`): ONE image
  = pure gz knowledge base (alpine layer, 51 datasets at `/opt/knowledge/gz`) +
  standard Qdrant + vectors baked at build time. Built from the repo root
  (`docker build -f Dockerfile.qdrant .`), using a per-Dockerfile whitelist
  ignore file `Dockerfile.qdrant.dockerignore` (BuildKit picks it up
  automatically) that re-includes cmd/ + internal/ + internal/knowledge/gz,
  overriding the root `.dockerignore` which excludes gz to slim the app image.
  The bake tool is the standalone business-free command `./cmd/vector-bake`
  (2026-08-30 split from root main.go), compiled at build time inside the
  image. It does NOT depend on the app image — no `COPY --from` — so building
  the qdrant image no longer requires building doctor-agent first. Rebuild
  ONLY when knowledge (or bake tool) changes: `./build.sh qdrant`.
  (The old `docker/qdrant-context/` independent context + src-sync machinery
  was removed 2026-08-30 in favour of this; `docker/mariadb-init/` deleted as
  unreferenced.)
- `doctor-agent` (Dockerfile): Go source + frontend (HTML embedded via
  `go:embed web/*.html`). `.dockerignore` excludes gz/data/external etc. Rebuild
  only when code/frontend changes.
- `doctor-agent-data` was removed 2026-08-30 — gz knowledge merged into the
  qdrant image. Only two images remain.
- Packaging is consolidated (2026-08-30): `./build.sh` is the single entry;
  Makefile docker-* targets, builder.sh and docker-compose.build.yml were
  removed as duplicates. The Makefile itself was removed later the same day;
  the `docker/` directory too (replaced by root `Dockerfile.qdrant` +
  `Dockerfile.qdrant.dockerignore`; per-Dockerfile ignore requires
  BuildKit/Docker ≥ 23 — local is Docker 29, remote verified).

## Build & remote deploy (2026-08-30)

- `build.sh [app|qdrant|full]` — the single packaging entry:
  - `./build.sh` (default `app`): build+push ONLY the app image (code/frontend
    changes). Fast; qdrant untouched.
  - `./build.sh qdrant`: build+push ONLY the qdrant image (gz knowledge
    changes; bake tool compiles itself, no app-image dependency).
  - `./build.sh full`: build+push app THEN qdrant (both changed; slow —
    bakes 51 datasets into Qdrant inside the build).
  Local dev binary: `go build -o bin/doctor-agent .` (Makefile removed
  2026-08-30; no wrapper needed).
- `remote-deploy.sh [app|qdrant|full|--dry-run]` — dev-machine mode (2026-08-30):
  run from the dev machine; the builder/deployer is 114.55.170.79. Local phase:
  `git fetch` + upstream check — aborts if local has unpushed commits (builder
  pulls from git, so it can only build pushed code), warns on uncommitted
  changes. Remote phase: `git checkout -- internal/knowledge/data/` (drop any
  scp-overwritten LFS entities, else `M` status blocks pull) → git pull →
  **git lfs pull** (the two big corpora huatuo_qa/medical_qa_pairs are
  Git-LFS; without this, make_gz.py + vector-bake would process LFS pointer
  text and fail) → python3 external/make_gz.py → ./build.sh MODE → docker
  compose up -d. `--dry-run` prints local checks + remote command without
  running.
