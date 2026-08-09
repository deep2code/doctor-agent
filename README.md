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

## 🚀 快速开始

### 环境要求
- Go 1.26.2+
- LLM API Key（通过 `LLM_PROVIDER` 选择，默认 **DeepSeek** 国内模型）：
  - **DeepSeek**（默认）：`DEEPSEEK_API_KEY`（不设 `LLM_PROVIDER` 时自动启用）
  - **OpenAI 兼容端点**（智谱 Zhipu / 豆包 / Qwen 等国内服务）：`LLM_PROVIDER=openai-compat` + `OPENAI_COMPAT_BASE_URL/API_KEY/MODEL`

### 安装与运行

```bash
# 设置 API Key（以 DeepSeek 为例；其他 provider 见 .env.example）
export DEEPSEEK_API_KEY=sk-...

# CLI 交互模式
go run . chat

# HTTP 服务器模式
go run . serve
# 然后 POST http://localhost:8080/chat
# Body: {"message": "我吃了蚕豆后脸色发黄、尿色深，是怎么回事？"}

# 验证知识库
go run . verify-knowledge

# 编译二进制
go build -o bin/doctor-agent .
./bin/doctor-agent chat
```

### HTTP API

```bash
# 健康检查
curl http://localhost:8080/health

# 对话
curl -X POST http://localhost:8080/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "我来自广西，准备结婚，需要做什么遗传病筛查？"}'

# 流式对话（SSE：delta 事件推送增量文本，done 事件返回完整结果）
curl -N -X POST http://localhost:8080/chat/stream \
  -H "Content-Type: application/json" \
  -d '{"message": "我一喝牛奶就拉肚子，是乳糖不耐受吗？"}'
```

若设置了 `API_KEY`，所有 `/chat*` 请求需携带 `Authorization: Bearer <API_KEY>` 头（`/health` 免鉴权）。

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
