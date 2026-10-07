# internal/knowledge — 知识层（MariaDB + Qdrant，二进制内零数据）

编译产物里**只有逻辑**，所有数据在外部 MariaDB（`doctor_knowledge`）与可选 Qdrant。运行时检索 = 查库；冷读打 MariaDB，热读走内存缓存。

## 数据在哪、有多少（2026-10-04 实测）

- `internal/knowledge/data/<dataset>/<name>.json` —— 37 个数据集目录，119 个 JSON（118 tracked + 1 个 gitignored 的 `corpus/corpus_statpearls.json`）。
- `internal/knowledge/gz/<dataset>/<name>.json.zst` —— **37 个目录 / 119 个归档**，与逻辑源严格 1:1（`medical` 73 个文件、`corpus` 10、`milestones` 2，其余各 1）。**2026-10-07 起不再有任何 `.partNNN` 分片**：原先「121 = 119 + 2 份分片源」的那 2 个多出来数是 `huatuo/huatuo_qa.json`(140.6MiB→2 片) 与 `medicalqa/medical_qa_pairs.json`(327.0MiB→4 片)，两份问答语料连同分片一起删掉了，所以目录数从 39 掉到 37、归档从 121 掉到 119，而「仓库里没有超过 100MiB 的 blob」重新变成一句成立的话。
- **布局就是分类**：数据集名 = 目录名，顶层必须是 JSON 数组、每个元素一行。没有任何文件名登记表、没有 envelope 解析模式。平铺在 `data/` 或 `gz/` 根的文件、空目录树都是硬错误。

## 播种与烘焙（两条链路，同一次目录扫描）

| 文件 | 作用 |
|---|---|
| `archive.go` | 唯一的分类器。`listSeedArchives:70-117` 扫 `gz/<dataset>/`，glob `*.json.*z*`（:26）同时收 .gz/.zst；空树报错（:100-102）、根目录平铺文件报错（:105-109）；magic bytes 解压（:141-162） |
| `seed.go` | `Seed():27` → 按数据集聚合 → **串行 Clear 树内数据集**（:71-75）→ **Clear 树里已消失的库内数据集**（:81-97，删目录等于删库里的行）→ 4 worker 并行 InsertBatch（:55、:104-123）。唯一解析器 `seedList:131`（非数组/空数组即错）。另含 `buildSearchText` 的字段白名单、`IngestUpload`（/admin 上传）、`DatasetForSeedPath`、`DatasetStats`、`ExportDataset` |
| `kb.go` | `KB` 层。`kb_items(id BIGINT AI PK, dataset VARCHAR(64) utf8mb4_bin, key VARCHAR(255), data MEDIUMBLOB, UNIQUE(dataset,key))`（schema :171-178），`data` gzip 压缩；`Insert`/`InsertBatch:297`（`INSERT … ON DUPLICATE KEY UPDATE`，拆事务 + 1213/1205 整笔重放）、`All`/`Search:317`/`Clear`/`ListDatasets:326`/`HasDataset`/`Health` |
| `loader.go` | `Store` 单例（sync.Once）。36 个 `ensureXxx()`（:590-704，全部返回 error）经通用 `ensure(name,fn):274` 懒加载并从 MariaDB 缓存，`ensureAll:711` 全量预热；`ingest():319` 有 37 个 `case DS*`，default 仍是 `// Unknown dataset — ignore.`（:584）；`GetDataVersion:811` 读 `DSVersion`（版本行来自库，不是文件）；`foldProseIntoBody()` 把 `details_zh` 折进 `Body`；**投影层** `projectedID:823`（内容派生身份）+ `projectProsePage:836` + 七个 `projectXxx`（msd/medlineplus/aap/fhs/nhc/diseaseenc/literature）—— 五个 `XAsKnowledge()` getter 与向量腿的 `bakedProjections` 调的是同一批函数 |
| `bake.go` | gz → Qdrant 离线烘焙（`Bake():120-198`），复用同一个 `listSeedArchives`；`vectorSkipDatasets:62-89` 共 25 项（10 个专用查找/图谱库 + 15 个实测零返回的查找表，见坑 7），**跳过判定在解压前**（:165-168）；按文本长度排序消 padding（:233）；点 ID = `uuidFromSourceHash(dataset+"|"+key, sha256(data))`（:330）；payload 不带 `text`/`timestamp`（`bakePayload:100-111`） |
| `syncer.go` | 运行时 /admin 同步（`FullSync`/`IncrementalSync`）。见下方"坑" |
| `vector_store.go` | Qdrant 客户端：`NewVectorStore`（懒连接，启动不 ping）、`EnsureCollection`（`datatype=float16`）、`Upsert`/`Search`/`Delete`/`Count`/`WaitReady` |
| `verify.go` | `VerifyData:61` + `ReportText:262` + `CheckURLLiveness:320`（DOI/PMID 格式、唯一性、可溯源、版本） |

## 检索家族

| 文件 | 作用 |
|---|---|
| `retriever.go` | `Retriever` 接口 + `QueryPrewarmer`（一轮内多条查询用一次 `EmbedBatch` 预热） |
| `retriever_keyword.go` | BM25 + CJK 子串/bigram。`NewRetriever`/`Retrieve`/`RetrieveDrugs`/`RetrieveEmergencyRules`/`RetrievePublicResources`。打分家族 `scoreEntry`：rule3 **双向包含**（query 含 condition_zh 或反之，中英各 +5.0），rule4 让 `clinicalFeatures/RiskFactors/Complications/DifferentialDiagnosis/Prevention` 里的整句各 +2.0（与关键词独立，打印的命中数会低估真实分数） |
| `retriever_hybrid.go` | RRF 融合（vectorWeight=0.4），同义词扩展查询只喂关键词腿、**不再算第二次 embedding** |
| `retriever_vector.go` | 语义检索。查询→向量 256 项 LRU 记忆化（knowledge 与 drug 共用）；**载荷解码走 `bakedProjections` 单表**（7 个家族 → 各自结构体 → 与关键词腿共用的同一个 `projectXxx`），`decodeBakedPayload:224` 只有三种出口：解得出正文就返回、是查找表就丢弃、解不开也丢弃。解码前先 `foldProseIntoBody`，防向量层将来再丢正文 |
| `retriever_corpus.go` | `scoreProse()`（唯一 prose 打分家族，标题+20/正文+8）+ `scoreCorpus`（加 summary 中间区 +10/+6）+ `scoreEnglish()`；`scoreMSD`/`scoreNHC`/`scoreFHS` 是薄包装。**新 corpus 检索器照抄调用，不要复制窗口打分循环** |
| 每源 retriever_*.go | aap / clinvar / corpus / eml / fda / fhs / growth / literature / medins / medline / milestones / msd / nhd→nhc / newborn —— 各对应一个 `RetrieveXxx`，被 `knowledge_search`/`exact_lookup` 派发 |
| `query_expansion.go` + `alias_map.json` | `ExpandQuery` + `LoadAliasFile`（`//go:embed`，即时生效、重跑幂等）。1,844 个键 / 3,701 条对照（2026-10-07 实测）。两个来源：`external/medical_terminology_2024.json` 经 `convert_medical_terminology.py` 整理的同义词组，和 `external/ahospital_aliases.py` 从重定向页收割、**人工逐条审过**的口语↔正式名对。生效方式是重编 app 镜像（不走 `make_gz`/`seed-knowledge`，它不在库里） |
| `reranker.go` | `Reranker` 接口 + `RerankCandidates`（重排融合池并截到 `KNOWLEDGE_TOP_K`） |
| `excerpt.go` | `ExcerptAround(body, query, 700)` 按行边界取窗口 —— **调用点必须自己 clipRunes 到 450 字**，缺换行的正文会回给出远宽于预算的切片 |
| `citation.go` | `CitationFormatter`：`BuildCitationMap`/`BuildCitationMapOffset`/`FlatCitationCount`/`BuildCitedSources`/`AddToolSource`/`SourceTierLabel:98`/`BuildKnowledgeExcerpts:87`（提示词里的"检索到的知识原文摘录"，最多 4 条） |
| `schemas.go` | `KnowledgeEntry`（含 `TitleZH/SummaryZH/DetailsZH` —— 缺了它们正文会在解码时被静默丢掉）、`Citation`、`ICD10Disease`、`NMPADrug`、`SIDERDrug` 等 |
| `corpus.go` | `CorpusDoc` ↔ `external/medkb/schema.py` 双侧同步；`ICD11Term/HPOTerm/OrphanetDisease/ICDO3Morphology` 是 exact_lookup 的形状 |
| 其余类型文件 | aap.go / clinvar.go / eml.go / fda_labels.go / fhs.go / literature.go / medins.go / medline.go / msd.go / nhc.go / pediatric.go / ttd_types.go —— 解码目标结构体 |

## 已核实的坑

1. **syncer 与 bake 的跳过名单不是同一道闸**：`vectorBakeEligible` 只在 `syncer.go:148`（cpubmed）和 `syncer.go:707`（按文件同步，`FullSync`/`IncrementalSync` 的 `cfg.FilePath` 分支都走它）两处生效，其余 typed 路径（medical/drug/foodrisk/labtest/literature/msd/medlineplus/diseaseenc）无守卫。等价性靠的是**「跳过名单 ⊆ 没有 typed 路径的数据集」这个不变量**：2026-10-07 把 skip 名单从 10 项扩到 25 项时逐条核过——新加的 15 个（medins/clinvar/eml/fda/…）都没有 typed 路径，只会经 `syncFile` 那条受守卫的路进来，所以运行时同步不会把它们重新写回 Qdrant。**以后往名单里加一个有 typed 路径的数据集，必须先给那条路径补守卫**，否则 /admin 同步会绕过跳过名单、用另一套点 ID 把它加回去。
2. **typed 同步路径与 bake 产出不兼容**：typed 路径的 payload 仍写 `text`/`timestamp`（`syncer.go:774-781`，bake 已删），且点 ID 派生是 `uuidFromSourceHash(source, sha256(entryJSON))`（`syncer.go:766`）≠ bake 的 `dataset+"|"+key`，而它的 `entry_id` 写的就是这个 UUID（bake 写的是种子 key）—— 同一内容会产生两套并存的点。跨引擎也不能混用：Go 的 `json.Marshal` 转义 `<>&`、Python 不转义，而 data 的 sha256 参与派生。
3. **`tokenize()` 不分词中文**，CJK 召回全靠 `retriever_keyword.go` 的子串 + bigram；但它**会保留纯数字 token**（只丢单字母 ASCII），rule1 全等匹配 → 关键词里含"牢记 3 件事""（2018年版）"这类独立数字，会给任何冒出该数字的查询白送 +3.0。批次17 洗掉 52 个，**仍有 102 条老条目带此缺陷**（who_zh_health / piyao_more / who_factsheets / piyao_selected / home_monitoring / checkup_labs），干净修法是 `tokenize()` 一行，属产品决策未动。检测办法：按分隔符（Unicode Z/P/S）自己切一遍，找 `^\d+$`，别用子串 grep。
4. ~~`seed.go` 的注释写着 `TestBakeMirrorMatchesGoSeedLists`~~ —— 2026-10-05 已改成现门名 `TestBakeMirrorMatchesGoSeedRules`（`bake_mirror_sync_test.go:90`）。
5. 同分排序天然脆弱：keyword 层三处 `sort.Slice` 与 `rrfFuse`（`retriever_hybrid.go:172`，由 map 迭代构建候选）都不稳定，卡在 top5 边界的同分断言每次进程都可能换名次；**定稿门必须在全量重播后的库上跑**（增量 upsert 一旦让条目集合变化，结果不作数）。
6. **载荷解不开不是错误，是静默死点**：`VectorRetriever` 对解不出正文的行直接丢弃，既不报错也不留痕，所以「烘焙付了钱、检索永远不返回」这个状态在代码里是完全静默的。2026-10-07 实测 34,289 点里只有 11,869 能被取回，全部由它造成（7,580 个全文页没有顶层 `id`、8,807 个疾病百科行 `category` 是数组导致整个 `Unmarshal` 报错、4,502 行解成空壳）。加新 prose 源时必须**同一次改动里**给 `bakedProjections` 加一条，否则门①不覆盖它（门只保证已登记的家族 100% 可解），这条要靠门③（getter 用到的 `projectXxx` 必须在表里）兜。
7. **无正文的查找表已离开烘焙范围（2026-10-07）**：medins 3,618 / clinvar 1,399 / eml 564 / fda 344 / medicaldialogues 90 / essential 20 / healthmyths 16 / literature_topics 16 / milestones 13 / emergency 12 / growth 1 / newborn 1 / sider 1 / ttd 1 / version 1 = **6,097 行**实测被向量腿返回 **0 条**（能算 embedding，但载荷里没有可展示的正文，其中 6,033 行连顶层 `id` 都没有），已整批加进 `vectorSkipDatasets`。**改名单的完整动作是三步，少一步就是不一致**：① Go 与 Python 两侧同步（`bake_mirror_sync_test.go` 的 `vector_skip_datasets` 子门逐名比对，只改一侧必红）；② 按 `source`（= `gz/` 目录名，不是 syncer 那套旧名）删掉在库产物里的这些点；③ 重算期望点数——`bake-gpu.sh compute_expected_points` 用 Python 名单现算且与在库点数做**等值**判定，所以只改名单不删点会红（那不是要你重烘的信号，而是产物里有名单外内容的信号）。改完实测：期望 **28,192 点 / 12 数据集 / 84 归档**（跳过侧 25 数据集 / 35 归档，合计仍是 119 个归档），在库产物删点后冷重载同为 28,192。
   **`bodypart` 29→18 与 `labtest` 8→6 是部分返回，不是死家族**，留在烘焙范围内（`TestVectorBakeEligible` 现在双向钉住这 15 个跳过 + 这两个不跳过），跳过它们会删掉真正能用的行。
8. **同义词扩展键不是关键词，磁铁那套判据不适用**：`alias_map.json` 的键只往查询里**加词**，不参与打分，所以「≥3 字才安全」这类关键词长度规则是错的口径（表自己的 `_comment` 写的是「key 须 ≥2 字、双向成对维护」，实测最短键就是 2 字、这类键有 559 个，是主力）。真正要拒的是**键成为自己的目标的最短形式**（泌尿→泌尿系统、风湿→风湿病）——扩展出来的词永远共现，等于给无关查询加了一条无差别信号。**改任何一个键都要重跑召回门**：它影响每一条查询的扩展结果，不限于新键所在的族。顺序是先 `TestExpandQueryAppendsSynonyms`/`TestLoadAliasFileAndExpand`/`TestColloquialQueriesFromRealPatients`（离线、秒级），再 `TestRetrieverSymptomStyleChineseRecall`+`TestRetrieverNoRecallForUnrelated`，最后十三道批量门（2026-10-07 实测单进程 447s）。**这张表是 `//go:embed` 的，不进 `gz/`、不进 MariaDB**，所以 `make_gz`/`seed-knowledge` 都不会带上它，线上生效只能重编 app 镜像。

## 回归门（本包 40+ 个测试文件）

- `seed_layout_test.go` —— 目录形状（平铺即红）。
- `bake_mirror_sync_test.go:90 TestBakeMirrorMatchesGoSeedRules` —— Python 镜像与 Go 的 (dataset, 文件, 是否烘焙) 三元组逐行一致。
- `zst_big_test.go` —— 按体积取 `gz/` 里**前三大**归档（2026-10-07 起由目录扫描现算，不再手写文件名，所以删数据集不会把它改红）解压 + `seedList` 分类，并断言最大的一份解压后仍 >100MB（否则这个「大语料」门已退化，该删而不是留着骗自己）；`-short` 跳过（CI 故意不带 `-short`）。实测前三大解压后 216.7 / 49.1 / 43.8 MB。
- `wire_new_datasets_smoke_test.go` —— 离线校验 seed 分类 + 结构反序列化 + 主键唯一，**不需要 MariaDB**。
- 九道科普召回门 `retriever_popsupplement*_test.go` + 四道批次7 `retriever_batch7_*_test.go` + `retriever_batch17/18/19_*`；另有 `TestRetrieverSymptomStyleChineseRecall` / `TestRetrieverNoRecallForUnrelated`。**除 smoke 类外全部需要本地 MariaDB**（`MARIA_DB_PORT=3307`）。
- `TestSeedTreeDatasetsAreLoaded` —— 要求每个 `gz/` 数据集目录都被 `loader.go` 的某个 `ensureXxx` 摄取（不给红门留例外）。
- `vector_payload_decode_test.go` —— **离线**（读 `gz/` 与包源码，不碰 MariaDB，5 秒）。三条断言：① `bakedProjections` 列出的每个家族必须 100% 行解得出正文、且没有任何一行连 JSON 对象都解不开；② 全集合 `entry_id` 唯一（RRF 严格按 id 去重）；③ `loader.go` 里 `*Store` 的 `XAsKnowledge` getter 调到的每个 `projectXxx` 都必须在 `bakedProjections` 有一项（AST 读源码，没有手写名单）。两条变异实测都真的红：把 `projectMSDEntry` 换成空返回 → 报「msd 只有 0/6127 行解得出」；删掉 `DSMSD` 整项 → 报「bakedProjections 缺 projectMSDEntry」。
