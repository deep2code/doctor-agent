# Doctor Agent — 循证医学AI助手

权威、专业、最高水平的医生智能体，专为**中国南方人群**定制。

## 🎯 核心特性

- **纯循证医学**：所有回答基于已发表文献和临床指南，拒绝中医、偏方及未经科学验证的说法
- **Zero Hallucination**：三重反幻觉机制——提示词约束 + 知识库绑定 + 响应后引用验证
- **每条回答有据可查**：事实性陈述必须标注引用编号 `[N]`，附DOI/PMID和证据等级
- **南方人群专精**：覆盖地贫、G6PD缺乏症、鼻咽癌、乙肝、乳糖不耐受、ALDH2酒精代谢缺陷等南方高发疾病
- **4层安全防护**：紧急检测(零延迟120响应) → 范围检查 → 引用验证 → 免责声明

## 📋 南方人群健康特征覆盖

| 疾病/特征 | 南方流行率 | 知识库覆盖 |
|-----------|-----------|-----------|
| α-地中海贫血 | 广西14.95%, 广东8.53% | ✅ 诊断+治疗+遗传咨询 |
| β-地中海贫血 | 广西6.78%, 广东4.53% | ✅ 诊断+治疗+遗传咨询 |
| G6PD缺乏症 | 南宁17.45%, 广东4% | ✅ 药物禁忌+诱因+管理 |
| 鼻咽癌(NPC) | 广东/广西/香港 ASR 20-30/10万 | ✅ EBV筛查+风险因素 |
| 慢性乙肝 | 南方~8-15% | ✅ 诊断+抗病毒治疗 |
| 乳糖不耐受 | 南方成人>80% | ✅ 诊断+饮食管理 |
| ALDH2缺乏 | 东亚~36% | ✅ 酒精-癌症风险 |
| 登革热 | 广东/广西/海南/云南 | ✅ 诊断+分诊 |
| 湿疹/真菌感染 | 湿热气候高发 | ✅ 诊断+治疗 |

## 🚀 快速开始

### 环境要求
- Go 1.21+
- Anthropic API Key

### 安装与运行

```bash
# 设置 API Key
export ANTHROPIC_API_KEY=sk-ant-...

# CLI 交互模式
go run ./cmd/doctor-agent chat

# HTTP 服务器模式
go run ./cmd/doctor-agent serve
# 然后 POST http://localhost:8080/chat
# Body: {"message": "我吃了蚕豆后脸色发黄、尿色深，是怎么回事？"}

# 验证知识库
go run ./cmd/doctor-agent verify-knowledge

# 编译二进制
go build -o bin/doctor-agent ./cmd/doctor-agent
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
```

## 🏗️ 架构

```
用户输入
  ├─ [L1 紧急检测] 关键词匹配 → 120响应（零LLM延迟）
  ├─ [L2 范围检查] 排除兽医/法医/偏方/自残
  ├─ [L3 知识检索] 关键词+TF-IDF检索嵌入式知识库
  ├─ [提示词组装] 5层系统提示词 + 检索知识注入
  ├─ [Agent循环] Claude API ← → 6个医疗工具
  ├─ [L4 响应验证] 引用真实性核查 + 诊断断言检查
  └─ [返回] 响应 + 引用列表 + 免责声明
```

## 🛠️ 6个医疗工具

1. **drug_safety_check** — G6PD药物禁忌查询（安全/不安全/谨慎/未知）
2. **genetic_risk_calculator** — 地贫遗传概率计算（Punnett方阵）
3. **food_risk_analyzer** — 南方食物风险分析（蚕豆/咸鱼/老火汤/海鲜/牛奶）
4. **symptom_triage** — 症状紧急分诊（EMERGENCY/URGENT/ROUTINE/SELF_CARE）
5. **reference_lookup** — 循证医学文献检索
6. **lab_interpreter** — 实验室检查解读（含南方地贫筛查注意事项）

## 📁 项目结构

```
doctor-agent/
├── cmd/doctor-agent/main.go          # CLI + HTTP 入口
├── internal/
│   ├── agent/agent.go                # Agent 核心循环
│   ├── config/                       # 配置加载
│   ├── session/                      # 会话管理
│   ├── prompt/                       # 5层系统提示词
│   ├── knowledge/                    # 知识库(embed) + 检索器 + 引用系统
│   │   └── data/                     # 10个结构化医学知识JSON
│   ├── tools/                        # 6个医疗工具 + 注册表
│   ├── safety/                       # 4层安全防护
│   └── server/                       # HTTP API Server
├── Makefile
├── .env.example
└── README.md
```

## ⚠️ 免责声明

本系统是**AI辅助工具**，仅供医学教育和信息参考，**不能替代执业医师的专业诊断、治疗建议或处方**。紧急情况请立即拨打120。
