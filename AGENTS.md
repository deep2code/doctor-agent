# AGENTS.md — doctor-agent

循证医学 (evidence-based medicine) AI assistant for the Chinese population — everyday health problems first, with southern-China high-burden conditions (地贫, G6PD, 鼻咽癌, 乙肝, 登革热) as an additional layer. Answers structure: 可能的原因 → 相似情况/常见病例 → 家庭护理 → 何时就医. Go module `github.com/doctor-agent` (Go 1.26), pluggable LLM (Anthropic/DeepSeek/OpenAI-compat: Zhipu/Qwen/豆包) + embedded JSON knowledge base.

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
go test ./...                                    # passes (tests: internal/safety, internal/knowledge, evals)
go run . chat                                    # interactive CLI (needs ANTHROPIC_API_KEY)
go run . serve                                   # HTTP on 0.0.0.0:8080 (/health, /chat, /chat/stream SSE)
go run . verify-knowledge                        # knowledge-base integrity check (DOI/PMID format, uniqueness, traceability, version)
go run ./evals                                  # offline eval on sample_answers.json (26-question golden set)
go run ./evals -online                          # online eval: runs real agent per question (needs API key; slow)
go run ./evals -answers my.json -report out.json # eval custom answers + JSON report; exit 1 on any failure (CI-friendly)
./builder.sh [-l|-c]                             # install script: builds to $GOPATH/bin or ./build
make lint                                        # golangci-lint (not installed in this env)
```

⚠️ **Stale docs**: `Makefile` and `README.md` reference `./cmd/doctor-agent`, which does not exist — `make build/chat/serve` fail (`stat .../cmd/doctor-agent: directory not found`). Use the `go build`/`go run` forms above, or fix the Makefile to `.`.

## Architecture

Pipeline (in `internal/agent/agent.go` `ProcessMessage`): L1 emergency detection → L2 scope guard → knowledge retrieval → layered system prompt → agent loop (≤5 iterations, tool-use) → L3 citation post-verification → L4 disclaimer. Sessions are an in-memory `map[string]*Session`.

- `internal/agent` — orchestrator `Agent`; builds provider-agnostic messages, dispatches tools, applies safety layers.
- `internal/knowledge` — `//go:embed data/*.json` (13 files: 10 medical + drug_contraindications + emergency_triage + food_risk + lab_tests + version) → `Store` (sync.Once singleton, RWMutex-guarded maps); `Retriever` interface with `retriever_keyword` (BM25 + CJK substring/bigram matching), `retriever_vector` (qdrant), `retriever_hybrid` (RRF fusion); `CitationFormatter`, schemas in `schemas.go`.
- `internal/llm` — `LLMProvider` interface (`Chat(ctx, messages, tools, systemPrompt)`, `Name()`); `anthropic_provider.go`, `deepseek_provider.go`; provider-agnostic `Message`/`ToolDefinition`/`ToolCall`.
- `internal/tools` — `Tool` interface (`Name/Description/Schema/Execute` → `*ToolResult{Success, Data, Error, Citations}`) + `Registry` (mutex, insertion-ordered). 6 tools: drug_safety_check, genetic_risk_calculator, food_risk_analyzer, symptom_triage, reference_lookup, lab_interpreter.
- `internal/safety` — `EmergencyDetector`, `ScopeGuard`, `PostVerifier` (citation realness + optional LLM-as-judge claim-support check via `POST_VERIFY_SEMANTIC`), `DisclaimerService`.
- `internal/knowledge` — also hosts `verify.go` (`VerifyData` integrity report, `ReportText`) and `BuildCitedSources` (flat citation-number → source map for post-verification).
- `evals/` — anti-hallucination golden set (`questions.json`, 26 questions) + `main.go` CLI (offline/online modes, keyword/refusal/citation/must-not checks). Add new questions here; run before changing prompts or knowledge data.
- `internal/prompt` — `Composer` assembling the 5-layer system prompt; `BuildPatientContext`.
- `internal/server` — stdlib HTTP server: `/health`, `/chat`, `/chat/stream` (SSE), CORS + slog middleware.
- `internal/session` — conversation history + patient context (`@region`, `@g6pd`, `@thal` CLI commands).
- `internal/config` — env-based `Config`, `Load()`, `Validate()`.

## Conventions

- Errors: `fmt.Errorf("context: %w", err)`; wrap at boundaries.
- Logging: `slog` everywhere; `slog.Info` for lifecycle, `slog.Debug` for detail, `slog.Warn` for safety hits. No logrus/zap.
- Concurrency: `sync.RWMutex` for Store/Registry reads, `sync.Once` for the knowledge singleton; channels + goroutines in hybrid retriever. New shared state must be mutex-guarded.
- Tools: JSON-schema `Schema()` (snake_case keys), `Execute` returns `ToolResult` — never returns raw data without `Success`/`Citations`.
- Config changes: add to `config.Config` + `Load()` + `.env.example` + `printUsage()` in `main.go`.
- New knowledge: add JSON to `internal/knowledge/data/` AND register its filename in `knowledge/loader.go` `doLoad()` switch, or it is silently ignored. Update `data/version.json` sources on data changes.
- Retrieval gotcha: `tokenize()` does NOT segment Chinese — CJK recall depends on substring + bigram matching against keywords/symptom fields in `retriever_keyword.go`. Symptom-style questions ("我一喝牛奶就拉肚子") now recall correctly (probe: 12/12); keep new entries' `keywords` in Chinese symptom vocabulary.
- Institutional publications (WHO/IARC/NCCN/中国指南) have no DOI/PMID by design — leave their `journal` empty so `verify-knowledge` doesn't flag them as untraceable journal articles.
- Keep UI strings Chinese; don't translate domain content to English.

## Notes

- (add quick notes here — e.g. decisions, gotchas, future work)
- Git history: single commit `e0f6e2f`.
- TODO: fix `Makefile`/`README.md` stale `cmd/doctor-agent` path (still broken — use `go build .`).
- `verify-knowledge` now passes clean (0 warnings): 50 medical entries, 90 citations (28 DOI + 7 PMID + 40 WHO URL; DOI/PMID traceability 35.6%).
- Hallucination guard: when retrieval returns nothing, `prompt.NoKnowledgeGuidance` is injected — the model must state the KB doesn't cover the topic and steer (ask follow-up / advise clinic) instead of improvising.
- Semantic verification (`POST_VERIFY_SEMANTIC=true` default) doubles LLM cost per response; set `false` or `POST_VERIFY_JUDGE_MODEL` to a cheap model for cost control.
- Known honest gap: ~~no true baseline from `go run ./evals -online` yet~~ **baseline established 2026-08-08**: 中文 36 题通过率 72.2%(26/36,拒答 7/7 正确),模型 = Zhipu glm-4-flash(免费)经 `LLM_PROVIDER=openai-compat`。英文 299 题与 Claude 基线未跑(需 ANTHROPIC_API_KEY;glm-4-flash 约 1 题/30s)。
- **OpenAI-compatible LLM provider (2026-08-08)**: `LLM_PROVIDER=openai-compat` + `OPENAI_COMPAT_BASE_URL/API_KEY/MODEL`(Zhipu/Qwen/SiliconFlow 等任意 OpenAI 协议端点;实测 Zhipu glm-4-flash 免费可用)。实现 `internal/llm/openai_compat_provider.go`(泛化 DeepSeekProvider)。judge 复用同端点。
- **Literature retrieval layer (2026-08-08)**: `literature_search` tool searches `internal/knowledge/data/literature.json` (4425 Europe PMC abstracts, 16 topics, 8.8MB embedded). `RetrieveLiterature` routes Chinese queries via topic keywords (`TOPICS` table in `external/convert_europepmc.py` — add symptom vocabulary there, then regenerate) and matches English via title/abstract substrings. Tool-returned PMIDs register as `[PMID]` citation sources in post-verification (`knowledge.AddToolSource`). Regenerate with `python3 external/convert_europepmc.py`.
- **WHO 官方中文知识库 (2026-08-08)**: `internal/knowledge/data/who_factsheets.json` — 40 条 WHO 官方中文 fact sheets(传染病 26/慢病 3/环境 4/妇幼 2/伤害 3/血液 2),URL 引用(WHO 无 DOI/PMID 属设计)。生成管线: `external/fetch_who_factsheets_zh.py`(抓 `/zh/` 中文版)→ `external/structurize_who.py`(LLM 结构化,provider 自动降级: Zhipu glm-4-flash 免费优先 → Qwen → SiliconFlow; 已验证 SiliconFlow 余额不足/Qwen access denied 时切换可用)。人工审核+清洗: 删除无数据 prevalence、空数组、补口语关键词(症状检索召回)。检索口语词补充规则: 新增条目后跑 `go run ./evals` 类似的中文口语查询验证。
- **WHO 疫苗立场文件 (2026-08-08)**: `internal/knowledge/data/who_vaccines.json` — 12 条疫苗 position papers(狂犬病 2018/乙脑 2015/HPV 2017/乙肝 2017/登革热 2018/流感 2022 中文/伤寒 2018/霍乱 2017/破伤风 2017/轮状 2021/麻疹 2017/肺炎球菌 2019),category="vaccine",evidence=international_guideline,URL 引用(journal 留空防 verify 误报)。管线: `external/fetch_position_papers.py`(IRIS DSpace API:搜索→取 ORIGINAL/TEXT bitstream;注意整期 WER 为英法双语、多语言版本是独立条目、bitstream 可能错配→必须验证内容含疫苗名)→ pypdf 提取(`PYTHONPATH=external/pylibs`;TEXT bundle 的 .txt 有时不可靠,PDF 提取更稳)→ `external/structurize_pp.py`(LLM 结构化;`extract_section` 对整期文本按 "position paper"/疫苗名定位截取,避免截到其他文章)。
- **MSD 默沙东诊疗手册中文版 (2026-08-08)**: `internal/knowledge/data/msd_manual.json` — 大众版+专业版全文检索层(6086 页,43.6MB)。`msd_search` 工具(第 9 个)。检索 `RetrieveMSD`(retriever_msd.go):完整中文查询词标题匹配 +20 优先,3+ 字窗口次之,2 字窗口弱信号;Latin token(G6PD/HPV)大小写不敏感。管线: `external/fetch_msd.py [home|professional]`(zh sitemap 过滤 4 段正文页,排除 multimedia/resources;source 字段区分版本)→ `external/merge_msd.py` 合并嵌入。注意:MSD 是百科式全文(非 KnowledgeEntry),独立检索层;页面含"完整评审/上次更新"元数据。
- **ClinVar 基因变异库 (2026-08-08)**: `internal/knowledge/data/clinvar.json` — 地贫/G6PD 核心基因(HBB/HBA1/HBA2/G6PD)的致病及可能致病变异 1399 条(376KB)。`variant_lookup` 工具(第 10 个)。检索 `RetrieveClinVar`(retriever_clinvar.go):cDNA 变异名(c.79G>A)精确 +10、基因符号/中文别名(HBB/β地中海贫血)+4~5、trait 疾病名 +3。管线: `external/fetch_clinvar.py`(NCBI E-Utilities:esearch 按基因→esummary 批量;注意 esummary 批量对 6 位旧 id/结构变异返回空,已含重试;缺失 200 条为 CNV/大片缺失,非点突变)。缓存 `external/clinvar/{gene}.json` 幂等。
- **English eval set (2026-08-08)**: `evals/questions_en.json` (MedQA 200 + PubMedQA 99, generated by `external/convert_evalsets.py`). evals now support `Question.ExpectedOption` (A-D / yes-no-maybe); English MCQ/PubMedQA categories skip the `[N]` citation requirement. Run: `go run ./evals -questions evals/questions_en.json`.
- **Sandbox build env**: system GOPATH is not writable — every `go build/test/run` needs `GOMODCACHE="$PWD/external/gomodcache"` (gitignored).
- **External knowledge downloads (in progress)**: see `external/DOWNLOAD_PROGRESS.md` for status & resume steps (Europe PMC ✅ 接入, MedlinePlus ⏳ 686/1017, WHO ✅ 241/241, HPO ✅ hp-base.obo, evalsets ✅ 接入). Run resumed scripts from repo root.
