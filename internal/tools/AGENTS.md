# internal/tools — 工具层

`Tool` 接口实现 + `Registry` + 关键词路由器。模型在一轮里能调什么，由此包决定：`Registry` 只管"存在"，`Router` 管"这一轮看得见"。

## 接线关系（先读这三行）

- 注册点**只有一处**：`internal/agent/agent.go:153-169` 的 13 次 `Register`。除此之外全仓库没有生产代码 import 本包（只有 `*_test.go`）。
- 可见性闸门：`agent.go:928` / `:1271` 调 `Router.ClassifyKG`，把名字列表交给 `GetToolDescriptionsByNames`（进提示词）与 `GetGenericToolDefinitionsByNames`（进 API 的 tools 字段）。**没被路由选中的工具，模型既看不到也调不到。**
- 因此"注册了"≠"能用"：`router_visibility_test.go TestEveryActiveToolIsRoutable` 逐个核对 13 个在册工具是否出现在 `toolGroups` 或 `relationToTools` 里，新加工具忘了接线会直接红（2026-10-04 之前的三个死角已修，见文末第 1 条）。

## 13 个在册工具

| Name() | 文件 | 干什么 |
|---|---|---|
| drug_safety_check | drug_safety_check.go | 特殊人群（孕/哺/儿童/老年/肝肾）用药禁忌查表 |
| genetic_risk_calculator | genetic_risk.go | 地贫/G6PD 遗传风险组合推算（纯规则表，不读知识库） |
| food_risk_analyzer | food_risk.go | G6PD 蚕豆病等食物风险清单命中（纯规则表） |
| symptom_triage | symptom_triage.go | 症状 → 分诊级别 + 就医科室建议 |
| drug_interaction_check | drug_interaction_check.go | 药效学相互作用，走 TTD 靶点表 |
| medical_image_analyze | medical_image_analyze.go | 医学图像多模态分析（唯一需要 vision 模型的工具） |
| lab_report_analyze | lab_report_analyze.go | 化验单结构化解读 |
| visit_prep | visit_prep.go | 生成就诊准备清单（带什么问题、带什么材料） |
| drug_label_lookup | drug_label_lookup.go | FDA 中文标签章节查询 |
| knowledge_search | knowledge_search.go | **统一检索入口**，24 个 dataset 分支 |
| exact_lookup | exact_lookup.go | **统一精确查表入口**，12 个 type 分支 |
| medical_kg_lookup | medical_kg_lookup.go | OpenCMKG 三元组（10 种关系） |
| cpubmed_kg_lookup | cpubmed_kg_lookup.go | CPubMed-KG 三元组 |

## 三个统一入口的分支

- `knowledge_search.go:158-192` 的 switch 有 **24 个数据集值**：medical / msd / nhc / fhs / aap / medline / literature / disease_encyclopedia / huatuo_qa / medical_qa / body_part / milestone / newborn_care / public_resources + corpus 族（statpearls、medgen、lactmed、otc_safety、medlinezh、cdc_kp、otc_labels、nhc_mental、firstaid、travel_health）。**三份对外清单（`Description():94`、`Schema():99` 的 dataset 说明、default 错误串）现在全部由 `knowledgeSearchDatasets:49` 这一张表派生**，加/删数据集只改这张表；回归门 `knowledge_search_datasets_test.go` 用 AST 把表和本机 `switch dataset` 的 case 双向对齐，谁漏一侧就红。
- `knowledge_search.go` 派发到的下层检索器在 `internal/knowledge/`：`retriever_msd.go:17 RetrieveMSD`、`retriever_nhc.go:16 RetrieveNHCGuide`、`retriever_fhs.go:16 RetrieveFHSGuide`、`retriever_aap.go:15 RetrieveAAP`、`retriever_corpus.go:17 RetrieveCorpus`、`retriever_medline.go:15 RetrieveMedlinePlus`、`retriever_literature.go:17 RetrieveLiterature`；另有 ICD 编码形状查询的前置自动派发（`Execute:138` 调 `tryICDCode:208`）。
- `exact_lookup.go:99-129` 的 12 个 type：icd10 / icd11 / hpo / nmpa / variant / eml / fda_label / ttd / sider / medins / orphanet / icdo3。三处声明（`Description():45`、`Schema():65`、错误串 :127）**完全一致**，这块不用同步担心。
- 两个工具都**没有** JSON-schema `enum` 约束 dataset/type，写错只会在 Execute 里落到 default 错误串。

## 44 个非测试文件里的 26 个死工具

以下文件定义了完整的 `Tool` 实现，但构造函数只在 `*_test.go` 里被调用过，生产路径永不注册：
`aap_search.go` `body_part_lookup.go`* `disease_drug_lookup.go` `disease_encyclopedia_lookup.go` `disease_symptom_lookup.go` `drug_lookup.go` `eml_lookup.go` `fhs_search.go` `growth_assessment.go` `huatuo_qa_lookup.go`* `icd10_lookup.go` `lab_interpreter.go` `lab_report_interpret.go` `literature_search.go` `medical_qa_lookup.go` `medline_search.go` `milestone_lookup.go` `msd_search.go` `newborn_care_lookup.go` `nhc_search.go` `nmpa_drug_lookup.go` `reference_lookup.go`* `sider_lookup.go` `target_disease_lookup.go` `triage_department.go` `ttd_lookup.go` `variant_lookup.go`

\* 带星号的三个类型本身是死的，但同文件还导出了**活的下层助手**，删文件前必须先把助手搬走：`body_part_lookup.go:116 normalizePart` / `:134 bodyPartData` / `:145 toCitationRefs`、`huatuo_qa_lookup.go:162 truncate`、`reference_lookup.go:117 evidenceLevelLabel`（三者都被 `knowledge_search.go` 调用）。

同一份逻辑在死文件和统一入口里各存一份，改动要双改或先删死的那份：`icd10_lookup.go:51-69`↔`exact_lookup.go:138-155`、`eml_lookup.go:60`↔`:493`、`sider_lookup.go:58`↔`:683`、`milestone_lookup.go:57-64`↔`knowledge_search.go:762-769`、`newborn_care_lookup.go:55`↔`:793`。

## 助手文件

- `base.go` — `Tool` / `ToolResult{Success,Data,Error,Citations}` / `CitationRef` 接口定义。工具永远返回 `*ToolResult`，不要裸返回数据。
- `registry.go` — mutex 保护的 `Registry`，插入序遍历（提示词里工具顺序因此稳定，这对提示词缓存是有意义的）。
- `router.go` — `QueryCategory` 常量、`toolGroups:32-64`、`relationToTools:191-201`、`symptomVocabulary:206-225`、`ClassifyMulti:125`（关键词单级）、`ClassifyKG:230`（症状→疾病候选→KG 关系→工具，两级，合并去重上限 8）、`ParamsHash:168`。两张表是**唯一的可见性来源**，`router_visibility_test.go` 保证 13 个在册工具都在里面。
- `qa_score.go:10 scoreQAPair` — huatuo_qa / medical_qa 共用的问答打分，被 `knowledge_search.go:569,634` 调用。

## 已核实的坑

1. ~~三个注册工具在正常聊天里不可能被选中~~ —— **已修 (2026-10-04)**：`drug_label_lookup` / `lab_report_analyze` / `visit_prep` 此前既不在 `toolGroups` 的任何一组、也不在 `relationToTools` 的任何一值里，而 `ClassifyKG`/`ClassifyMulti` 的每条 return 路径（含 :139/:160 的 CatGeneral 兜底）返回的都是这两张表的值，所以化验单解读、标签查询、就诊准备三条能力注册了却对模型不可见。现在的归属：`drug_label_lookup` → CatDrug/CatGeneral + `disease_recommand_drug`/`disease_common_drug`；`lab_report_analyze` → CatLab/CatImage + `disease_need_check`（CatImage 带它是因为上传化验单图片要先 `medical_image_analyze` 提字、再交给本工具）；`visit_prep` → CatSymptom/CatDisease/CatGeneral + `disease_belong_department`（该工具的输出就含建议挂号科室）。**回归门 `router_visibility_test.go TestEveryActiveToolIsRoutable` 双向钉死**：13 个在册工具必须至少出现在一张路由表里，反向也查（路由指向已退休的名字会白占 8 个槽位之一）。`medical_image_analyze` 另外还在带图管线被强制附加（`agent.go:1273-1284`）。
2. **`drug_interaction_check` 实际上测不出相互作用**：TTD 没有 drug→target 边，:95-115 的"共同靶点"判据退化成"靶点名同时包含两个药名"，源码注释自己承认了这点。
3. `exact_lookup` 的 `type=sider` 只能按 `drug.ID` 匹配（:692），而 `Description():48` 宣称可查 1430 种药物；`SIDERDrug`（`internal/knowledge/schemas.go:356-360`）根本没有药名字段。
4. `FoodRiskAnalyzer`（food_risk.go:14/19）与 `GeneticRiskCalculator`（genetic_risk.go:12/17）注入了 `*knowledge.Store` 却从不读它 —— 是硬编码规则表，不是图谱支撑。
5. `lookupICD10` 收下 `top_k` 参数但丢弃，硬编码 20（`exact_lookup.go:136`、`:155`）；死文件 `icd10_lookup.go:69` 同病。
6. 注释里的工具计数有历史残留，别拿它们对数：`agent.go:150`("9 action")、`:163`("~28 retired")、`knowledge_search.go:13`("~20")、`registry.go:116`("all 35 tools")、`agent.go:925`("from 35 to <=10")。实际就是 13 在册 / 26 死。`router.go` 里"unified 8-tool set"的措辞已在 2026-10-04 改为按 13 个工具描述（8 是**合并上限**，不是工具总数）。

## 改这里之前

- 新增工具：写文件 → 在 `agent.go:New` 注册 → **同时加进 `router.go` 的 `toolGroups`（以及需要时 `relationToTools`）**，否则等于没加。
- 删死工具：先确认没有活助手住在同一文件（见上），删完跑 `go build ./...` + `go test ./internal/tools/`。
- `Schema()` 用 snake_case 键；返回体要经得起 `internal/agent` 的压缩层（`compactToolResult` 的 knowledge_search 白名单在 `agent.go:1627-1647`，新字段不加白名单就会被丢掉）。
