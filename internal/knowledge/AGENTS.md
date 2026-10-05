# internal/knowledge — 知识层（MariaDB + Qdrant，二进制内零数据）

编译产物里**只有逻辑**，所有数据在外部 MariaDB（`doctor_knowledge`）与可选 Qdrant。运行时检索 = 查库；冷读打 MariaDB，热读走内存缓存。

## 数据在哪、有多少（2026-10-04 实测）

- `internal/knowledge/data/<dataset>/<name>.json` —— 39 个数据集目录，119 个 JSON（118 tracked + 1 个 gitignored 的 `corpus/corpus_statpearls.json`）。
- `internal/knowledge/gz/<dataset>/<name>.json.zst` —— **39 个目录 / 121 个归档**，与逻辑源 1:1（`medical` 73 个文件、`corpus` 10、`milestones` 2，其余各 1）。多出的 2 个来自 `.partNNN` 分片（`huatuo/huatuo_qa.json.part000-001`、`medicalqa/medical_qa_pairs.json.part000-003` + 2 份 `.parts` 清单，均 tracked）。
- **布局就是分类**：数据集名 = 目录名，顶层必须是 JSON 数组、每个元素一行。没有任何文件名登记表、没有 envelope 解析模式。平铺在 `data/` 或 `gz/` 根的文件、空目录树都是硬错误。

## 播种与烘焙（两条链路，同一次目录扫描）

| 文件 | 作用 |
|---|---|
| `archive.go` | 唯一的分类器。`listSeedArchives:70-117` 扫 `gz/<dataset>/`，glob `*.json.*z*`（:26）同时收 .gz/.zst；空树报错（:100-102）、根目录平铺文件报错（:105-109）；magic bytes 解压（:141-162） |
| `seed.go` | `Seed():27` → 按数据集聚合 → **串行 Clear 树内数据集**（:71-75）→ **Clear 树里已消失的库内数据集**（:81-97，删目录等于删库里的行）→ 4 worker 并行 InsertBatch（:55、:104-123）。唯一解析器 `seedList:131`（非数组/空数组即错）。另含 `buildSearchText` 的字段白名单、`IngestUpload`（/admin 上传）、`DatasetForSeedPath`、`DatasetStats`、`ExportDataset` |
| `kb.go` | `KB` 层。`kb_items(id BIGINT AI PK, dataset VARCHAR(64) utf8mb4_bin, key VARCHAR(255), data MEDIUMBLOB, UNIQUE(dataset,key))`（schema :171-178），`data` gzip 压缩；`Insert`/`InsertBatch:297`（`INSERT … ON DUPLICATE KEY UPDATE`，拆事务 + 1213/1205 整笔重放）、`All`/`Search:317`/`Clear`/`ListDatasets:326`/`HasDataset`/`Health` |
| `loader.go` | `Store` 单例（sync.Once）。38 个 `ensureXxx()`（:612-732）经通用 `ensure(name,fn):278` 懒加载并从 MariaDB 缓存；`ingest():323` 有 41 个 `case DS*`，default 仍是 `// Unknown dataset — ignore.`（:606）；`GetDataVersion:733` 读 `DSVersion`（版本行来自库，不是文件）；`foldProseIntoBody()` 把 `details_zh` 折进 `Body` |
| `bake.go` | gz → Qdrant 离线烘焙（`Bake():98-176`），复用同一个 `listSeedArchives`；`vectorSkipDatasets:56-67` 共 10 项，**跳过判定在解压前**（:143-146）；按文本长度排序消 padding（:211-213）；点 ID = `uuidFromSourceHash(dataset+"|"+key, sha256(data))`（:307-308）；payload 不带 `text`/`timestamp`（:78-89） |
| `syncer.go` | 运行时 /admin 同步（`FullSync`/`IncrementalSync`）。见下方"坑" |
| `vector_store.go` | Qdrant 客户端：`NewVectorStore`（懒连接，启动不 ping）、`EnsureCollection`（`datatype=float16`）、`Upsert`/`Search`/`Delete`/`Count`/`WaitReady` |
| `verify.go` | `VerifyData:61` + `ReportText:262` + `CheckURLLiveness:320`（DOI/PMID 格式、唯一性、可溯源、版本） |

## 检索家族

| 文件 | 作用 |
|---|---|
| `retriever.go` | `Retriever` 接口 + `QueryPrewarmer`（一轮内多条查询用一次 `EmbedBatch` 预热） |
| `retriever_keyword.go` | BM25 + CJK 子串/bigram。`NewRetriever`/`Retrieve`/`RetrieveDrugs`/`RetrieveEmergencyRules`/`RetrievePublicResources`。打分家族 `scoreEntry`：rule3 **双向包含**（query 含 condition_zh 或反之，中英各 +5.0），rule4 让 `clinicalFeatures/RiskFactors/Complications/DifferentialDiagnosis/Prevention` 里的整句各 +2.0（与关键词独立，打印的命中数会低估真实分数） |
| `retriever_hybrid.go` | RRF 融合（vectorWeight=0.4），同义词扩展查询只喂关键词腿、**不再算第二次 embedding** |
| `retriever_vector.go` | 语义检索。查询→向量 256 项 LRU 记忆化（knowledge 与 drug 共用）；解码 baked payload 时同样折进正文，防向量层将来回归 |
| `retriever_corpus.go` | `scoreProse()`（唯一 prose 打分家族，标题+20/正文+8）+ `scoreCorpus`（加 summary 中间区 +10/+6）+ `scoreEnglish()`；`scoreMSD`/`scoreNHC`/`scoreFHS` 是薄包装。**新 corpus 检索器照抄调用，不要复制窗口打分循环** |
| 每源 retriever_*.go | aap / clinvar / corpus / eml / fda / fhs / growth / literature / medins / medline / milestones / msd / nhd→nhc / newborn —— 各对应一个 `RetrieveXxx`，被 `knowledge_search`/`exact_lookup` 派发 |
| `query_expansion.go` + `alias_map.json` | `ExpandQuery` + `LoadAliasFile`（`//go:embed`，即时生效、重跑幂等） |
| `reranker.go` | `Reranker` 接口 + `RerankCandidates`（重排融合池并截到 `KNOWLEDGE_TOP_K`） |
| `excerpt.go` | `ExcerptAround(body, query, 700)` 按行边界取窗口 —— **调用点必须自己 clipRunes 到 450 字**，缺换行的正文会回给出远宽于预算的切片 |
| `citation.go` | `CitationFormatter`：`BuildCitationMap`/`BuildCitationMapOffset`/`FlatCitationCount`/`BuildCitedSources`/`AddToolSource`/`SourceTierLabel:98`/`BuildKnowledgeExcerpts:87`（提示词里的"检索到的知识原文摘录"，最多 4 条） |
| `schemas.go` | `KnowledgeEntry`（含 `TitleZH/SummaryZH/DetailsZH` —— 缺了它们正文会在解码时被静默丢掉）、`Citation`、`ICD10Disease`、`NMPADrug`、`SIDERDrug` 等 |
| `corpus.go` | `CorpusDoc` ↔ `external/medkb/schema.py` 双侧同步；`ICD11Term/HPOTerm/OrphanetDisease/ICDO3Morphology` 是 exact_lookup 的形状 |
| 其余类型文件 | aap.go / clinvar.go / eml.go / fda_labels.go / fhs.go / huatuo_types.go / literature.go / medins.go / medline.go / msd.go / nhc.go / pediatric.go / ttd_types.go —— 解码目标结构体 |

## 已核实的坑

1. **syncer 与 bake 的跳过名单不是同一道闸**：`vectorBakeEligible` 只在 `syncer.go:148`（cpubmed）和 `syncer.go:809`（按文件同步）两处生效，其余 9 条 typed 路径无守卫。眼下恰好这 10 个数据集本来就不在 typed 路径里所以等价，但"改一个 skip 项"不会自动传导。
2. **typed 同步路径与 bake 产出不兼容**：typed 路径的 payload 仍写 `text`/`timestamp`（`syncer.go:881-882`，bake 已删），且点 ID 派生是 `uuidFromSourceHash(source, sha256(entryJSON))`（`syncer.go:868`）≠ bake 的 `dataset+"|"+key` —— 同一内容会产生两套并存的点。跨引擎也不能混用：Go 的 `json.Marshal` 转义 `<>&`、Python 不转义，而 data 的 sha256 参与派生。
3. **`tokenize()` 不分词中文**，CJK 召回全靠 `retriever_keyword.go` 的子串 + bigram；但它**会保留纯数字 token**（只丢单字母 ASCII），rule1 全等匹配 → 关键词里含"牢记 3 件事""（2018年版）"这类独立数字，会给任何冒出该数字的查询白送 +3.0。批次17 洗掉 52 个，**仍有 102 条老条目带此缺陷**（who_zh_health / piyao_more / who_factsheets / piyao_selected / home_monitoring / checkup_labs），干净修法是 `tokenize()` 一行，属产品决策未动。检测办法：按分隔符（Unicode Z/P/S）自己切一遍，找 `^\d+$`，别用子串 grep。
4. ~~`seed.go` 的注释写着 `TestBakeMirrorMatchesGoSeedLists`~~ —— 2026-10-05 已改成现门名 `TestBakeMirrorMatchesGoSeedRules`（`bake_mirror_sync_test.go:90`）。
5. 同分排序天然脆弱：keyword 层三处 `sort.Slice` 与 `rrfFuse`（`retriever_hybrid.go:172`，由 map 迭代构建候选）都不稳定，卡在 top5 边界的同分断言每次进程都可能换名次；**定稿门必须在全量重播后的库上跑**（增量 upsert 一旦让条目集合变化，结果不作数）。

## 回归门（本包 40+ 个测试文件）

- `seed_layout_test.go` —— 目录形状（平铺即红）。
- `bake_mirror_sync_test.go:90 TestBakeMirrorMatchesGoSeedRules` —— Python 镜像与 Go 的 (dataset, 文件, 是否烘焙) 三元组逐行一致。
- `zst_big_test.go` —— >100MB 的两个 QA 种子解压+分类，`-short` 跳过（CI 故意不带 `-short`）。
- `wire_new_datasets_smoke_test.go` —— 离线校验 seed 分类 + 结构反序列化 + 主键唯一，**不需要 MariaDB**。
- 九道科普召回门 `retriever_popsupplement*_test.go` + 四道批次7 `retriever_batch7_*_test.go` + `retriever_batch17/18/19_*`；另有 `TestRetrieverSymptomStyleChineseRecall` / `TestRetrieverNoRecallForUnrelated`。**除 smoke 类外全部需要本地 MariaDB**（`MARIA_DB_PORT=3307`）。
- `TestSeedTreeDatasetsAreLoaded` —— 要求每个 `gz/` 数据集目录都被 `loader.go` 的某个 `ensureXxx` 摄取（不给红门留例外）。
