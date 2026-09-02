# 公共知识下载 — 断点记录（2026-08-09 更新）

目标：为 doctor-agent 自用批量获取公共医学知识源。所有脚本幂等（支持断点续跑）。

## 目录结构

```
external/
├── fetch_europepmc.py          # Europe PMC 批量拉取脚本（✅ 完成）
├── fetch_who_factsheets.py     # WHO fact sheets 抓取脚本（✅ 已修复并完成 241/241）
├── fetch_medlineplus_pages.py  # MedlinePlus 正文抓取脚本（✅ 1017/1017 页完成）
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
| WHO 基本药物清单 (EML) | ✅ 完成并接入 | **第24版 564 种药物**（`who_eml.json`，core 441/complementary 123，含剂型+一线/二线适应症），`eml_lookup` 工具 | `external/parse_eml.py`（PDF 文本解析）；`name_zh` 全量 LLM 翻译待办（检索已内置 ~200 常用中文药名映射） |
| FDA 药品标签 (DailyMed) | ✅ 完成并接入 | **344 种常用药中文要点**（`fda_drug_labels.json`，适应症/禁忌/警告/相互作用/不良反应/剂量，URL 引用），`drug_label_lookup` 工具（第 13 个） | `fetch_dailymed.py`（EML 药 → RxNorm RXCUI → OpenFDA label sections；OpenFDA 无 openfda.rxcui、generic_name 子串匹配→limit=5 打分选纯品、sort=submission_status 非法 400、setid 用 label["id"]）+ `structurize_dailymed.py`（**仅 Zhipu glm-4.7-flash 免费，需 `thinking:{"type":"disabled"}` 关闭推理模式**；Qwen/SiliconFlow fallback 已移除防付费）+ `clean_fda_labels.py`（keywords 单串拆分、死 URL 修复）。3 个正文过短放弃（deferoxamine/nitrous_oxide/oxygen） |
| 中国疾控中心 (China CDC) | ✅ 完成并接入 | jkts 健康提示 → 26 条按疾病合并条目（`cdc_entries.json`，含预防/症状/就医指征，URL 引用） | `fetch_cdc.py`（✅ 已验证：列表页 `./{yyyymm}/t{...}.html` 单引号链接、标题在 #articleCon 前 h5、正文容器 trs_editor_view、旧文章 404 跳过）；`structurize_cdc.py`（LLM 按疾病拆分，Zhipu glm-4-flash）+ `convert_cdc.py`（月度重复疾病合并+剔除通用症状关键词，避免挤占专病条目检索）。注意 jkkp 等栏目为 JS 渲染需浏览器 |
| 默沙东诊疗手册 | ✅ 完成并接入 | **6086 页**（大众版 3296 + 专业版 2790，`msd_manual.json` 43.6MB），`msd_search` 工具已注册 | 大众版 35 个失效 URL、专业版 14 个代理错误页未抓（<1%）；`fetch_msd.py [home|professional]`（幂等）+ `merge_msd.py` |
| 国家卫健委指南 | ✅ 完成并接入（2026-08-09 补） | **39 个中文指南全文**（30 文字版 + 9 OCR 版，`nhc_guides.json` 1.4MB，覆盖流感/脑血管病/肝癌/诺如/猴痘/拉沙热/基孔肯雅/儿童支原体肺炎/新冠/86 罕见病等），`nhc_search` 工具（第 14 个，全文检索层仿 MSD） | ⚠️ 历史坑：此前 DOWNLOAD_PROGRESS 误标"已接入 source=nhc"，实际 internal/ 零 nhc 代码；2026-08-09 才真正接入。管线：`fetch_nhc_all.py`(playwright 绕 WAF) + `ocr_nhc_scanned.py`(my-ocr) + `convert_nhc.py`（标题清洗/截断回退；OCR 版 9 篇 url 为空属设计） |
| 育儿知识（2026-08-09） | ✅ 完成并接入 | **4 源**：① 国家免疫规划疫苗程序（`china_vaccines.json` 10 条，官方 PDF 广东疾控镜像）② 中国婴幼儿喂养指南 2022 三册+卫健委喂养核心信息（`feeding_guidelines.json` 4 条）③ 香港卫生署 FHS 育儿百科（`fhs_guides.json` 103 页中文全文，`fhs_search` 工具第 15 个；⚠️ fhs.gov.hk 全 JS 渲染无链接/sitemap，URL 靠 websearch `site:` 收集存 `external/fhs/urls.txt`，正文取 #container-wrapper）④ AAP healthychildren（`aap_articles.json` 264 页英文全文，`aap_search` 工具第 16 个；⚠️ sitemap 是 UTF-16） | 管线：`fetch_fhs.py`+`convert_fhs.py`、`fetch_aap.py`（sitemap 过滤 ages-stages 等 5 板块）+`convert_aap.py`、`convert_immunization.py`、`convert_feeding.py`；`fhsSynonyms` 大陆→香港用词映射（辅食→固体食物） |
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

| 新生儿/儿童成长（2026-09-02） | ✅ 完成并接入 | **三数据集**：① `growth_standards.json`（WHO 儿童生长标准 0-60 月 SD 表 男女×3指标 + WS/T 423-2022 附录B 12 张 SD 表含判定规则）→ `growth_assessment` 工具（双标准 z 分数线性插值，修复 hcfa/lhfa xlsx 多一列 SD 的解析 bug）② `development_milestones.json`（CDC 里程碑 2022 修订版 12 年龄档 159 条四域，中文 LLM 翻译 159/159）→ `milestone_lookup` 工具 ③ `newborn_care.json`（WHO 早产/LBW 护理建议 26 条中英 + 中国新生儿筛查 3 项）→ `newborn_care_lookup` 工具。共 3 个新工具（第 17-19 个），版本 1.32.0 | 管线：`convert_growth.py`（WHO xlsx 表头名索引取列 + PDF 表 B 状态机解析）+ `convert_milestones.py`（glm-4-flash 按年龄批量翻译，长度对齐校验）+ `convert_newborn.py`（LLM 清洗表格文本流+翻译，缓存 external/newborn/who_cache/）；CDC 页面 Akamai 拦 curl 须走 web_reader 服务端抓取；WS/T 423-2022 官方 PDF 在 wsbz.nhc.gov.cn 标准库附件（GB18030 页面相对路径上溯 5 级）。seed 修复：key 列改 `utf8mb4_bin` collation（unicode_ci 全半角折叠致 MSD 中英 title 撞唯一键）+ InsertBatch 改 ON DUPLICATE KEY UPDATE 幂等 + extractKey 加 clinvar_id/icd10_code + dedupeKey 后缀去重；新 CLI `cmd/kbseed` 本地 seed 入口 |

## 接下来（按序）

1. **MedlinePlus 内容利用**：1017 页无官方中文，暂缓或极精选（如需做，走 WHO 同款 LLM 结构化管线）。
2. **HPO 利用**：决定是否做中文术语映射或仅作参考。
3. **在线评测基线**：`go run ./evals -online`（中文）+ `go run ./evals -online -questions evals/questions_en.json`（英文）需要 ANTHROPIC_API_KEY，跑出真实准确率。
4. 改提示词/知识库前先跑 `go run ./evals`（中文 36 题）+ 英文评测集。
5. WHO 全量扩展（可选）：已全量入库 232/234 页（`fetch_who_factsheets_zh.py` 234 页中文，`who_factsheets.json` 232 条；2 页未结构化）；如需补充可查缺后重跑 `structurize_who.py`（幂等）。
6. 疫苗立场文件扩展（可选）：`fetch_position_papers.py` 的 VACCINES 表加条目（如乙肝 2025 新版/水痘/流感中文版已含）后重跑。
7. NHC 指南扩展（可选）：`fetch_nhc_all.py` 已抓 39 篇；`convert_nhc.py` 幂等可重跑，新抓指南放入 `external/nhc/guides/` 或 `guides_ocr/` 后重跑即可自动并入。

## 环境备注

- 网络：europepmc/medlineplus/who/gitee 直连可用；huggingface.co 超时，须用 hf-mirror.com；GitHub API/raw 直连可用（HPO 用 raw.githubusercontent.com）
- Go 编译：`export GOMODCACHE="$PWD/.cache/gomodcache"`（2026-08-30 起缓存收于项目内 .cache/，见 AGENTS.md；当前环境走默认 GOPATH 即可）
- pip 装包：沙箱禁止写系统 site-packages，用 `pip3 install --target $PWD/.cache/pylibs <pkg>` + `PYTHONPATH=$PWD/.cache/pylibs`
- 磁盘：42GB 可用（external/ 约 500MB，构建缓存已移到项目内 .cache/）
