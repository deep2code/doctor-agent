# AGENTS.md — doctor-agent

循证医学 (evidence-based medicine) AI assistant for the entire Chinese population — everyday health problems first, with China's high-burden conditions (地贫, G6PD, 鼻咽癌, 乙肝, 乳糖不耐受, 登革热, 南方省份尤为高发) as an additional layer. Answers structure: 可能的原因 → 相似情况/常见病例 → 家庭护理 → 何时就医. Go module `github.com/doctor-agent` (Go 1.26), pluggable LLM (Anthropic/DeepSeek/OpenAI-compat: Zhipu/Qwen/豆包) + embedded JSON knowledge base.

## Project

- **Entry point**: root `main.go` (package `main` at module root) — NOT `./cmd/doctor-agent`. Subcommands: `chat`, `serve`, `verify-knowledge`, `version`.
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
go run ./evals                                  # offline eval on sample_answers.json (26-question golden set)
go run ./evals -online                          # online eval: runs real agent per question (needs API key; slow)
go run ./evals -answers my.json -report out.json # eval custom answers + JSON report; exit 1 on any failure (CI-friendly)
./builder.sh [-l|-c]                             # install script: builds to $GOPATH/bin or ./build
make lint                                        # golangci-lint v2.12.2 (installed at /Users/junjunyi/gopath/bin; .golangci.yml is v2 format, same version in CI)
make gz                                          # regenerate internal/knowledge/gz/*.json.gz after editing data/*.json
```

⚠️ **Fixed 2026-08-09**: `Makefile` and `README.md` previously referenced `./cmd/doctor-agent` (nonexistent) — `make build/chat/serve` failed. Both now use root `main.go` (`go build -o bin/doctor-agent .`, `go run . chat/serve/verify-knowledge`).

## Architecture

Pipeline (in `internal/agent/agent.go` `ProcessMessageStream` — `ProcessMessage` is a non-streaming wrapper): L1 emergency detection → L2 scope guard → knowledge retrieval → layered system prompt → agent loop (≤5 iterations, tool-use, streaming deltas) → L3 citation post-verification → L4 disclaimer. Sessions live in a mutex-guarded `map[string]*Session`; with `SESSION_DIR` set they are snapshotted to JSON files and restored across restarts.

- `internal/agent` — orchestrator `Agent`; builds provider-agnostic messages, dispatches tools, applies safety layers.
- `internal/knowledge` — `//go:embed gz/*.gz` (23 source JSONs in `data/` are gzip-compressed into `gz/` by `external/make_gz.py`; binary 95MB→52MB) → `Store` (sync.Once singleton, RWMutex-guarded maps); `Retriever` interface with `retriever_keyword` (BM25 + CJK substring/bigram matching), `retriever_vector` (qdrant), `retriever_hybrid` (RRF fusion); `CitationFormatter`, schemas in `schemas.go`. `verify.go` also hosts `CheckURLLiveness` (probes citation URLs).
- `internal/llm` — `LLMProvider` interface (`Chat`, `StreamChat(ctx, messages, tools, systemPrompt, onDelta)`, `Name()`); `anthropic_provider.go` (NewStreaming), `deepseek_provider.go` + `openai_compat_provider.go` sharing `openai_stream.go` (SSE parsing + tool-call fragment accumulation); provider-agnostic `Message`/`ToolDefinition`/`ToolCall`.
- `internal/tools` — `Tool` interface (`Name/Description/Schema/Execute` → `*ToolResult{Success, Data, Error, Citations}`) + `Registry` (mutex, insertion-ordered). 13 tools: drug_safety_check, genetic_risk_calculator, food_risk_analyzer, symptom_triage, reference_lookup, lab_interpreter, literature_search, msd_search, variant_lookup, medline_search, drug_lookup, eml_lookup, drug_label_lookup.
- `internal/safety` — `EmergencyDetector`, `ScopeGuard`, `PostVerifier` (citation realness + optional LLM-as-judge claim-support check via `POST_VERIFY_SEMANTIC`), `DisclaimerService`.
- `internal/knowledge` — also hosts `verify.go` (`VerifyData` integrity report, `ReportText`) and `BuildCitedSources` (flat citation-number → source map for post-verification).
- `evals/` — anti-hallucination golden set (`questions.json`, 26 questions) + `main.go` CLI (offline/online modes, keyword/refusal/citation/must-not checks). Add new questions here; run before changing prompts or knowledge data.
- `internal/prompt` — `Composer` assembling the 5-layer system prompt; `BuildPatientContext`.
- `internal/server` — stdlib HTTP server: `/health`, `/chat`, `/chat/stream` (real token-level SSE: `delta`/`done`/`error` events), middleware chain: CORS allowlist (`CORS_ORIGINS`) → OPTIONS → rate limit (`RATE_LIMIT`, per-IP fixed window) → Bearer auth (`API_KEY`, `/health` exempt) → slog logging.
- `internal/session` — conversation history as `[]llm.Message` (provider-agnostic) + patient context (`@region`, `@g6pd`, `@thal` CLI commands); `FileStore` persists JSON snapshots to `SESSION_DIR`.
- `internal/config` — env-based `Config`, `Load()`, `Validate()`.

## Conventions

- Errors: `fmt.Errorf("context: %w", err)`; wrap at boundaries.
- Logging: `slog` everywhere; `slog.Info` for lifecycle, `slog.Debug` for detail, `slog.Warn` for safety hits. No logrus/zap.
- Concurrency: `sync.RWMutex` for Store/Registry reads, `sync.Once` for the knowledge singleton; channels + goroutines in hybrid retriever. New shared state must be mutex-guarded.
- Tools: JSON-schema `Schema()` (snake_case keys), `Execute` returns `ToolResult` — never returns raw data without `Success`/`Citations`.
- Config changes: add to `config.Config` + `Load()` + `.env.example` + `printUsage()` in `main.go`.
- New knowledge: add JSON to `internal/knowledge/data/` AND register its filename in `knowledge/loader.go` `doLoad()` switch, or it is silently ignored. Then run `make gz` (or `python3 external/make_gz.py`) to regenerate the embedded compressed copies. Update `data/version.json` sources on data changes.
- Retrieval gotcha: `tokenize()` does NOT segment Chinese — CJK recall depends on substring + bigram matching against keywords/symptom fields in `retriever_keyword.go`. Symptom-style questions ("我一喝牛奶就拉肚子") now recall correctly (probe: 12/12); keep new entries' `keywords` in Chinese symptom vocabulary.
- Institutional publications (WHO/IARC/NCCN/中国指南) have no DOI/PMID by design — leave their `journal` empty so `verify-knowledge` doesn't flag them as untraceable journal articles.
- Keep UI strings Chinese; don't translate domain content to English.

## Notes

- (add quick notes here — e.g. decisions, gotchas, future work)
- Git history: single commit `e0f6e2f`.
- ✅ Fixed: `Makefile`/`README.md` stale `cmd/doctor-agent` path (2026-08-09) — both now use root `main.go`; `make build/chat/serve/verify-knowledge` work again.
- `verify-knowledge` now passes clean (0 warnings): 50 medical entries, 90 citations (28 DOI + 7 PMID + 40 WHO URL; DOI/PMID traceability 35.6%).
- Hallucination guard: when retrieval returns nothing, `prompt.NoKnowledgeGuidance` is injected — the model must state the KB doesn't cover the topic and steer (ask follow-up / advise clinic) instead of improvising.
- Semantic verification (`POST_VERIFY_SEMANTIC`) now **defaults to false** (2026-08-09) — set `true` or `POST_VERIFY_JUDGE_MODEL` to a cheap model to re-enable; it roughly doubles LLM cost per response.
- Known honest gap: ~~no true baseline from `go run ./evals -online` yet~~ **baseline established 2026-08-08**: 中文 36 题通过率 72.2%(26/36,拒答 7/7 正确),模型 = Zhipu glm-4-flash(免费)经 `LLM_PROVIDER=openai-compat`。英文 299 题与 Claude 基线未跑(需 ANTHROPIC_API_KEY;glm-4-flash 约 1 题/30s)。
- **OpenAI-compatible LLM provider (2026-08-08)**: `LLM_PROVIDER=openai-compat` + `OPENAI_COMPAT_BASE_URL/API_KEY/MODEL`(Zhipu/Qwen/SiliconFlow 等任意 OpenAI 协议端点;实测 Zhipu glm-4-flash 免费可用)。实现 `internal/llm/openai_compat_provider.go`(泛化 DeepSeekProvider)。judge 复用同端点。
- **Literature retrieval layer (2026-08-08)**: `literature_search` tool searches `internal/knowledge/data/literature.json` (4425 Europe PMC abstracts, 16 topics, 8.8MB embedded). `RetrieveLiterature` routes Chinese queries via topic keywords (`TOPICS` table in `external/convert_europepmc.py` — add symptom vocabulary there, then regenerate) and matches English via title/abstract substrings. Tool-returned PMIDs register as `[PMID]` citation sources in post-verification (`knowledge.AddToolSource`). Regenerate with `python3 external/convert_europepmc.py`.
- **WHO 官方中文知识库 (2026-08-08)**: `internal/knowledge/data/who_factsheets.json` — 40 条 WHO 官方中文 fact sheets(传染病 26/慢病 3/环境 4/妇幼 2/伤害 3/血液 2),URL 引用(WHO 无 DOI/PMID 属设计)。生成管线: `external/fetch_who_factsheets_zh.py`(抓 `/zh/` 中文版)→ `external/structurize_who.py`(LLM 结构化,provider 自动降级: Zhipu glm-4-flash 免费优先 → Qwen → SiliconFlow; 已验证 SiliconFlow 余额不足/Qwen access denied 时切换可用)。人工审核+清洗: 删除无数据 prevalence、空数组、补口语关键词(症状检索召回)。检索口语词补充规则: 新增条目后跑 `go run ./evals` 类似的中文口语查询验证。
- **WHO 疫苗立场文件 (2026-08-08)**: `internal/knowledge/data/who_vaccines.json` — 12 条疫苗 position papers(狂犬病 2018/乙脑 2015/HPV 2017/乙肝 2017/登革热 2018/流感 2022 中文/伤寒 2018/霍乱 2017/破伤风 2017/轮状 2021/麻疹 2017/肺炎球菌 2019),category="vaccine",evidence=international_guideline,URL 引用(journal 留空防 verify 误报)。管线: `external/fetch_position_papers.py`(IRIS DSpace API:搜索→取 ORIGINAL/TEXT bitstream;注意整期 WER 为英法双语、多语言版本是独立条目、bitstream 可能错配→必须验证内容含疫苗名)→ pypdf 提取(`PYTHONPATH=external/pylibs`;TEXT bundle 的 .txt 有时不可靠,PDF 提取更稳)→ `external/structurize_pp.py`(LLM 结构化;`extract_section` 对整期文本按 "position paper"/疫苗名定位截取,避免截到其他文章)。
- **MSD 默沙东诊疗手册中文版 (2026-08-08)**: `internal/knowledge/data/msd_manual.json` — 大众版+专业版全文检索层(6086 页,43.6MB)。`msd_search` 工具(第 9 个)。检索 `RetrieveMSD`(retriever_msd.go):完整中文查询词标题匹配 +20 优先,3+ 字窗口次之,2 字窗口弱信号;Latin token(G6PD/HPV)大小写不敏感。管线: `external/fetch_msd.py [home|professional]`(zh sitemap 过滤 4 段正文页,排除 multimedia/resources;source 字段区分版本)→ `external/merge_msd.py` 合并嵌入。注意:MSD 是百科式全文(非 KnowledgeEntry),独立检索层;页面含"完整评审/上次更新"元数据。
- **WHO 基本药物清单 (2026-08-09)**: `internal/knowledge/data/who_eml.json` — WHO EML 第24版(2025) 564 种药物(core 441/complementary 123)，含剂型规格与一线/二线适应症。`eml_lookup` 工具(第 12 个)。解析管线 `external/parse_eml.py`(PDF 提取文本→结构化；坑: PDF 丢失缩进→需把顶格剂型行并入当前条目、private-use 方块符 \uf06f 需清除、子标题正则必须 `\.?` 以匹配 `6.2.1 Access`)。检索 `RetrieveEMLDrug`(retriever_eml.go):中文名经内置 ~200 常用药别名表映射到 INN，英文精确/子串匹配。`name_zh` 全量 LLM 翻译待办(需 API key，检索已可用)。
- **ClinVar 基因变异库 (2026-08-08)**: `internal/knowledge/data/clinvar.json` — 地贫/G6PD 核心基因(HBB/HBA1/HBA2/G6PD)的致病及可能致病变异 1399 条(376KB)。`variant_lookup` 工具(第 10 个)。检索 `RetrieveClinVar`(retriever_clinvar.go):cDNA 变异名(c.79G>A)精确 +10、基因符号/中文别名(HBB/β地中海贫血)+4~5、trait 疾病名 +3。管线: `external/fetch_clinvar.py`(NCBI E-Utilities:esearch 按基因→esummary 批量;注意 esummary 批量对 6 位旧 id/结构变异返回空,已含重试;缺失 200 条为 CNV/大片缺失,非点突变)。缓存 `external/clinvar/{gene}.json` 幂等。
- **English eval set (2026-08-08)**: `evals/questions_en.json` (MedQA 200 + PubMedQA 99, generated by `external/convert_evalsets.py`). evals now support `Question.ExpectedOption` (A-D / yes-no-maybe); English MCQ/PubMedQA categories skip the `[N]` citation requirement. Run: `go run ./evals -questions evals/questions_en.json`.
- **FDA 药品标签与 China CDC 管线 (2026-08-09)**: `external/fetch_dailymed.py`(EML 564 药 → RxNorm RXCUI → OpenFDA label sections → `external/dailymed/`; 注意 OpenFDA label 无 openfda.rxcui 且 generic_name 是子串匹配→取 limit=5 打分选纯品标签; sort=submission_status 非法 400; setid 取 label["id"]) + `structurize_dailymed.py`(LLM 中文化 → `fda_drug_labels.json` 344 条; **仅 Zhipu glm-4.7-flash 免费, 必须带 `thinking:{"type":"disabled"}` 否则 content 为空; Qwen/SiliconFlow fallback 已移除防付费; 失败幂等可重试) + `clean_fda_labels.py`(keywords 逗号单串拆分、死 URL 修复)。`drug_label_lookup` 工具(第 13 个)。CDC: `fetch_cdc.py`(jkts 健康提示; 文章链接单引号 `./{yyyymm}/t{...}.html`, 标题在 #articleCon 前 h5, 正文 trs_editor_view, 旧文章 404) + `structurize_cdc.py`(LLM 按疾病拆分) + `convert_cdc.py`(月度重复疾病合并+剔除通用症状词, 避免挤占专病条目检索; 文件名日期正则须 `t(\d{6})(\d{2})_(\d+)`) → `cdc_entries.json` 26 条。
- **Sandbox build env**: system GOPATH is not writable — every `go build/test/run` needs `GOMODCACHE="$PWD/external/gomodcache"` (gitignored). `external/go.mod` makes `external/` a nested module boundary so `./...` patterns skip the module cache inside the repo (otherwise golangci-lint/go list fail with "directory ... outside main module").
- **Built-in web chat UI (2026-08-09)**: `internal/server/web/index.html` (single-file HTML+CSS+JS, embedded via `//go:embed`, zero deps/offline). `GET /` serves it; binary with **no args defaults to web mode** (`startWebUI` → serve + one-time key setup, prints "打开 http://localhost:8080"). UI: chat bubbles + SSE streaming (`/chat/stream` delta/done/error), localStorage conversation persistence, "新对话" button, example question chips, mobile responsive, minimal inline Markdown renderer (bold/lists/headings/blockquote). `/` and `/health` are exempt from auth/rate-limit; API paths stay gated. Server test `TestWebUIServed` covers it.
- **Zero-config first-run setup (2026-08-09)**: 下载 Release 二进制后双击 `start-chat`（release.yml 每平台附带的一键脚本）即可用。无 API Key 时 `runChat` 触发交互引导（推荐智谱 glm-4-flash 免费），把 `LLM_PROVIDER`/key 写入 `~/.doctor-agent/config.env`（0600），`loadUserConfig()` 以最低优先级注入环境变量（真实 env > `.env` > 用户配置）。`runServe` 缺 key 时打印指引。用户侧零技术门槛；开发侧一切照旧。
- **golangci-lint (2026-08-09)**: local install is **v2.12.2** (`/Users/junjunyi/gopath/bin/golangci-lint`); `.golangci.yml` is the v2 format (`version: "2"`, `linters.default: standard`). CI installs the same via `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`. `make lint` passes clean (0 issues); v2 standard set is stricter than v1 (QF1012/QF1003/errcheck) — all fixed.
- **External knowledge downloads (in progress)**: see `external/DOWNLOAD_PROGRESS.md` for status & resume steps (Europe PMC ✅ 接入, MedlinePlus ✅ 1017/1017 页, WHO ✅ 241/241, HPO ✅ hp-base.obo, evalsets ✅ 接入). Run resumed scripts from repo root.
- **Optimization batch (2026-08-09)**: 真流式 `/chat/stream`(token 级 SSE) + `StreamChat`(三个 provider); session 类型解耦(不再依赖 anthropic-sdk-go) + JSON 文件持久化(`SESSION_DIR`); server 安全中间件(`API_KEY`/`CORS_ORIGINS`/`RATE_LIMIT`); `POST_VERIFY_SEMANTIC` 默认 false; 补测试(agent/server/session/config, 全量含 race 通过); CI workflow + `.golangci.yml`; 知识库 gzip embed(二进制 95MB→52MB, `make gz`); `verify-knowledge -urls` 引用 URL 可达性检查(发现 4 个 WHO 疫苗链接 404 待修); `cdc_alerts.json` 移出 embed 至 `external/cdc/`(死文件); CLI `clear` 命令 bug 修复。
