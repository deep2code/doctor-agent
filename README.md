# Doctor Agent — 循证医学AI助手

权威、专业、最高水平的医生智能体，专为**全中国人群**定制。

## 🎯 核心特性

- **纯循证医学**：所有回答基于已发表文献和临床指南，拒绝中医、偏方及未经科学验证的说法
- **Zero Hallucination**：三重反幻觉机制——提示词约束 + 知识库绑定 + 响应后引用验证
- **每条回答有据可查**：事实性陈述必须标注引用编号 `[N]`，附DOI/PMID和证据等级
- **全人群覆盖**：覆盖地贫、G6PD缺乏症、鼻咽癌、乙肝、乳糖不耐受、ALDH2酒精代谢缺陷等中国重点高发疾病
- **4层安全防护**：紧急检测(零延迟120响应) → 范围检查 → 引用验证 → 免责声明

## 📋 中国重点疾病覆盖

| 疾病/特征 | 中国流行情况 / 高发地区 | 知识库覆盖 |
|-----------|-----------|-----------|
| α-地中海贫血 | 广西14.95%, 广东8.53% | ✅ 诊断+治疗+遗传咨询 |
| β-地中海贫血 | 广西6.78%, 广东4.53% | ✅ 诊断+治疗+遗传咨询 |
| G6PD缺乏症 | 南宁17.45%, 广东4% | ✅ 药物禁忌+诱因+管理 |
| 鼻咽癌(NPC) | 广东/广西/香港 ASR 20-30/10万 | ✅ EBV筛查+风险因素 |
| 慢性乙肝 | 全国约5-6%（南方偏高） | ✅ 诊断+抗病毒治疗 |
| 乳糖不耐受 | 中国成人>80% | ✅ 诊断+饮食管理 |
| ALDH2缺乏 | 东亚~36% | ✅ 酒精-癌症风险 |
| 登革热 | 南方省份为主（广东/广西/海南/云南） | ✅ 诊断+分诊 |
| 湿疹/真菌感染 | 全国常见（南方湿热地区更易发） | ✅ 诊断+治疗 |

## 🚀 快速开始（零配置）

**不用装 Go、不用配环境变量，下载就能用：**

1. **下载程序**：打开 [GitHub Releases](https://github.com/deep2code/doctor-agent/releases) 页面，下载对应你系统的文件（macOS / Windows / Linux）
2. **解压**，双击 `start-chat`（Windows 是 `start-chat.bat`，macOS/Linux 是 `start-chat.sh`）
3. **首次运行**会问你一个 API Key——推荐 **智谱 glm-4-flash（免费）**，去 [open.bigmodel.cn](https://open.bigmodel.cn) 注册免费获取，粘贴一次自动保存，之后永不再问
4. **直接输入问题**，回车即答

> 💡 想用别的模型？设置环境变量即可覆盖：`DEEPSEEK_API_KEY=sk-...` 等（详见下方「🔧 配置」）。

### 开发者用（源码运行）

<details>
<summary>源码构建 / 校验 / 评测（点击展开）</summary>

```bash
cp .env.example .env          # 或用首次运行引导（推荐）
go run . chat                 # 命令行聊天（首次同样会引导填 key）
go run . serve                # HTTP 服务 http://localhost:8080
go run . verify-knowledge     # 校验知识库
go run ./evals                # 离线防幻觉评测
```
</details>

### HTTP API

<details>
<summary>curl 示例（点击展开）</summary>

```bash
curl http://localhost:8080/health          # 健康检查

curl -X POST http://localhost:8080/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "我吃了蚕豆后脸色发黄，是怎么回事？"}'

curl -N -X POST http://localhost:8080/chat/stream \   # SSE 流式
  -H "Content-Type: application/json" \
  -d '{"message": "我一喝牛奶就拉肚子，是乳糖不耐受吗？"}'
```
</details>

> 🔒 若设置了 `API_KEY`，所有 `/chat*` 请求需携带 `Authorization: Bearer <API_KEY>` 头（`/health` 免鉴权）。

### 想换模型 / 开高级功能？

默认 DeepSeek 开箱即用。换国内其他模型（智谱 / 豆包 / Qwen）、开鉴权限流、会话持久化等，见下方「🔧 配置」表和 `.env.example` 里的注释——**都是可选项，不配也能用**。

## 🔧 配置（环境变量，详见 .env.example）

| 变量 | 默认 | 说明 |
|---|---|---|
| `LLM_PROVIDER` | `deepseek` | `deepseek` / `openai-compat` |
| `POST_VERIFY_SEMANTIC` | `false` | 语义二次校验（LLM-as-judge，开启后 LLM 成本约翻倍） |
| `API_KEY` | 空 | `/chat` 端点 Bearer 鉴权；空 = 不鉴权 |
| `CORS_ORIGINS` | 空 | 逗号分隔的允许来源；空 = 允许全部 (`*`) |
| `RATE_LIMIT` | `0` | 每 IP 每分钟最大请求数；`0` = 不限 |
| `SESSION_DIR` | 空 | 会话 JSON 快照目录（重启后对话不丢）；空 = 仅内存 |

## 🏗️ 架构

```
用户输入
  ├─ [L1 紧急检测] 关键词匹配 → 120响应（零LLM延迟）
  ├─ [L2 范围检查] 排除兽医/法医/偏方/自残
  ├─ [知识检索] 关键词(BM25+CJK) + 可选向量/混合检索，命中嵌入式知识库
  ├─ [提示词组装] 5层系统提示词 + 检索知识注入
  ├─ [Agent循环] LLM Provider(流式) ← → 13个医疗工具
  ├─ [L3 引用验证] 引用真实性核查 + 诊断断言检查
  └─ [L4 免责声明] 返回 响应 + 引用列表 + 免责声明
```

HTTP 层另有可选安全中间件：Bearer 鉴权 → 每 IP 限流 → CORS 白名单。

## 🛠️ 13个医疗工具

1. **drug_safety_check** — G6PD药物禁忌查询（安全/不安全/谨慎/未知）
2. **genetic_risk_calculator** — 地贫遗传概率计算（Punnett方阵）
3. **food_risk_analyzer** — 中国常见食物风险分析（蚕豆/咸鱼/老火汤/海鲜/牛奶）
4. **symptom_triage** — 症状紧急分诊（EMERGENCY/URGENT/ROUTINE/SELF_CARE）
5. **reference_lookup** — 循证医学文献引用检索（内置知识库）
6. **literature_search** — Europe PMC 文献检索（4425篇摘要，真实DOI/PMID）
7. **msd_search** — 默沙东诊疗手册中文版全文检索（6086页）
8. **variant_lookup** — ClinVar 基因变异查询（HBB/HBA1/HBA2/G6PD，1399条）
9. **medline_search** — MedlinePlus 健康百科检索（1017页，英文）
10. **drug_lookup** — 国家医保药品目录查询（2024版）
11. **eml_lookup** — WHO基本药物清单查询（第24版，564种）
12. **drug_label_lookup** — FDA药品标签中文摘要查询（344条）
13. **lab_interpreter** — 实验室检查解读（含地贫筛查注意事项）

## 📁 项目结构

```
doctor-agent/
├── main.go                         # CLI + HTTP 入口
├── internal/
│   ├── agent/agent.go                # Agent 核心循环（含流式 ProcessMessageStream）
│   ├── config/                       # 配置加载
│   ├── session/                      # 会话管理 + JSON 文件持久化
│   ├── prompt/                       # 5层系统提示词
│   ├── knowledge/                    # 知识库(gzip embed) + 检索器 + 引用系统
│   │   ├── data/                     # 23个源JSON（编辑后运行 make gz）
│   │   └── gz/                       # gzip 压缩的嵌入文件（go:embed）
│   ├── tools/                        # 13个医疗工具 + 注册表
│   ├── safety/                       # 4层安全防护
│   └── server/                       # HTTP API Server（鉴权/限流/CORS）
├── evals/                           # 防幻觉黄金评测集（中文36题 + 英文299题）
├── external/                        # 知识抓取/转换管线（WHO/MSD/EML/ClinVar…）
├── .github/workflows/ci.yml         # CI：build/vet/test/verify-knowledge/evals/lint
├── Makefile
├── .env.example
└── README.md
```

## ⚠️ 免责声明

本系统是**AI辅助工具**，仅供医学教育和信息参考，**不能替代执业医师的专业诊断、治疗建议或处方**。紧急情况请立即拨打120。
