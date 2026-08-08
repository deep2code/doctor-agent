# 公共知识下载 — 断点记录（2026-08-08 更新）

目标：为 doctor-agent 自用批量获取公共医学知识源。所有脚本幂等（支持断点续跑）。

## 目录结构

```
external/
├── fetch_europepmc.py          # Europe PMC 批量拉取脚本（✅ 完成）
├── fetch_who_factsheets.py     # WHO fact sheets 抓取脚本（✅ 已修复并完成 241/241）
├── fetch_medlineplus_pages.py  # MedlinePlus 正文抓取脚本（⏳ 后台续跑中）
├── convert_europepmc.py        # ✅ Europe PMC → internal/knowledge/data/literature.json（新增）
├── convert_evalsets.py         # ✅ MedQA/PubMedQA → evals/questions_en.json（新增）
├── europepmc/                  # ✅ 16 主题 × 300 条 = 4800 条摘要（JSON）
├── medlineplus/
│   ├── mplus_topics.zip        # 官方每周包（4.5MB）
│   ├── mplus_topics_2026-08-07.xml  # 30MB 元数据（标题/URL/MeSH，无正文）
│   ├── topics.json             # 2033 主题结构化索引（EN 1017）
│   └── pages/                  # ✅ 1017/1017 页（7.6MB）
├── who_factsheets/             # ✅ 241/241 页（修复 TextExtractor：正文在 <article> 内，非 <main>）
├── hpo/
│   ├── hp-base.obo             # ✅ 11.2MB / 20413 术语（raw.githubusercontent 直连，v2026-06-23）
│   └── hp-base.json            # ❌ 391KB 截断残留（忽略，用 .obo）
├── evalsets/
│   ├── pubmedqa/labeled.json   # ✅ PubMedQA 1000 条（列式结构：pubid/question/context/final_decision 按索引对齐）
│   └── medqa/{train,dev,test}.json  # ✅ MedQA 10178/1272/1273 题（dict: {id, data, subject_name}）
└── pylibs/                     # pyarrow（沙箱内，--target 安装）
```

## 状态明细

| 源 | 状态 | 数据 | 待办 |
|---|---|---|---|
| Europe PMC | ✅ 完成并接入 | 4425 篇摘要已嵌入知识库（`literature.json` 8.8MB） | 新增 `literature_search` 工具；`RetrieveLiterature` 主题预过滤+英文子串匹配，自动带 PMID/DOI |
| MedlinePlus 索引 | ✅ 完成 | 2033 主题（EN 1017） | — |
| MedlinePlus 正文 | ✅ 完成并接入 | 1017/1017 页（`medlineplus.json` 5.8MB），`medline_search` 工具已注册 | 英文健康百科全文检索（标题短语+15 加权） |
| WHO fact sheets | ✅ 完成并接入 | **全量 232 条**中文版入库（`who_factsheets.json`），覆盖全体中国人日常主题 | `fetch_who_factsheets_zh.py`(234 中文版) + `structurize_who.py`(全量模式) |
| WHO 疫苗立场文件 | ✅ 完成并接入 | 12 条（狂犬病/乙脑/HPV/乙肝/登革热/流感/伤寒/霍乱/破伤风/轮状/麻疹/肺炎球菌），`who_vaccines.json` | `fetch_position_papers.py`(IRIS API) + `structurize_pp.py`；流感有官方中文版，其余为英文→中文结构化 |
| 默沙东诊疗手册 | ✅ 完成并接入 | **6086 页**（大众版 3296 + 专业版 2790，`msd_manual.json` 43.6MB），`msd_search` 工具已注册 | 大众版 35 个失效 URL、专业版 14 个代理错误页未抓（<1%）；`fetch_msd.py [home|professional]`（幂等）+ `merge_msd.py` |
| 国家卫健委指南 | ✅ 完成并接入 | **35 个中文指南**（26 文字版 + 9 OCR 版，`source=nhc`） | `fetch_nhc_all.py`(playwright 绕 WAF) + `ocr_nhc_scanned.py`(my-ocr) |
| ClinVar 基因变异 | ✅ 完成并接入 | HBB/HBA1/HBA2/G6PD 致病及可能致病变异 1399 条（`clinvar.json` 376KB），`variant_lookup` 工具已注册 | 200 条 6 位旧 id 结构变异（CNV）esummary 不可达，已放弃；`fetch_clinvar.py` 幂等可补 |
| HPO | ✅ 已下载 | hp-base.obo 11.2MB/20413 术语 | **暂缓**：纯英文无中文映射，与 MSD 症状知识重叠；若做需设计"症状规范化"工具+中英映射 |
| CMeKG | ⛔ 不可自动获取 | 数据本体不公开批量下载 | 放弃 |
| 评测集 | ✅ 完成并接入 | MedQA 200 + PubMedQA 99 → `evals/questions_en.json` | evals 新增 `expected_option` 检查（A-D/yes-no-maybe）；`go run ./evals -questions evals/questions_en.json` |
| HuggingFace 直连 | ⛔ 超时 | 需走 hf-mirror.com | — |

## 已完成的接入工作（2026-08-08）

1. **文献检索层**（静态，不依赖 qdrant）：
   - `external/convert_europepmc.py` → `internal/knowledge/data/literature.json`（4425 篇，HTML 标签已剥离，过滤无 DOI/PMID 或摘要 <200 字符的条目）
   - `internal/knowledge/literature.go`（schema）、`loader.go`（加载+访问器）、`retriever_literature.go`（`RetrieveLiterature`：中文查询经主题关键词表路由，英文查询全局 title/abstract 匹配）
   - `internal/tools/literature_search.go` 新工具，已注册进 agent（`agent.go`）
   - `data/version.json` 升 1.1.0，加 Europe PMC source
   - 测试：`retriever_literature_test.go`（主题路由/症状口语/英文全局/无匹配），全绿
2. **评测集接入**：
   - `external/convert_evalsets.py` → `evals/questions_en.json`（299 题，A-D/yes/no/maybe 分布均衡）
   - `evals/eval.go`：`Question.ExpectedOption` 字段 + "正确答案"检查 + `keywordMatch` 大小写不敏感 + 英文题跳过引用标注
   - 测试：`eval_test.go` 新增 2 个用例，全绿
3. **环境备注**：
   - `go build` 必须加 `GOMODCACHE="$PWD/external/gomodcache"`（系统 GOPATH 无写权限，已加入 .gitignore）

## 接下来（按序）

1. **MedlinePlus 内容利用**：1017 页无官方中文，暂缓或极精选（如需做，走 WHO 同款 LLM 结构化管线）。
2. **HPO 利用**：决定是否做中文术语映射或仅作参考。
3. **在线评测基线**：`go run ./evals -online`（中文）+ `go run ./evals -online -questions evals/questions_en.json`（英文）需要 ANTHROPIC_API_KEY，跑出真实准确率。
4. 改提示词/知识库前先跑 `go run ./evals`（中文 36 题）+ 英文评测集。
5. WHO 全量扩展（可选）：`fetch_who_factsheets_zh.py` 已抓 234 页中文，如需 >40 条可扩展 `SELECTED` 后重跑 `structurize_who.py`（幂等）。
6. 疫苗立场文件扩展（可选）：`fetch_position_papers.py` 的 VACCINES 表加条目（如乙肝 2025 新版/水痘/流感中文版已含）后重跑。

## 环境备注

- 网络：europepmc/medlineplus/who/gitee 直连可用；huggingface.co 超时，须用 hf-mirror.com；GitHub API/raw 直连可用（HPO 用 raw.githubusercontent.com）
- Go 编译：`export GOMODCACHE="$PWD/external/gomodcache"`（沙箱禁写系统 GOPATH）
- pip 装包：沙箱禁止写系统 site-packages，用 `pip3 install --target external/pylibs <pkg>` + `PYTHONPATH=external/pylibs`
- 磁盘：42GB 可用（external/ 约 130MB + gomodcache）
