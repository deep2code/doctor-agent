# 国内大模型大脑切换指南

agent 通过 `LLM_PROVIDER` 选择大脑(LLM)。`openai-compat` 通道可接入**任何 OpenAI 协议兼容的国内大模型**,无需改代码。

## 快速开始(当前可用:智谱)

```bash
export LLM_PROVIDER=openai-compat
export OPENAI_COMPAT_BASE_URL=https://open.bigmodel.cn/api/paas/v4   # 智谱
export OPENAI_COMPAT_API_KEY=$ZHIPU_API_KEY
export OPENAI_COMPAT_MODEL=glm-4-flash                                # 免费
go run . chat
```

## 各平台配置表(2026-08-08 实测状态)

| 平台 | base_url | model 示例 | 状态 |
|---|---|---|---|
| **智谱 BigModel** | `https://open.bigmodel.cn/api/paas/v4` | `glm-4-flash`(免费) | ✅ 可用 |
| **火山方舟(豆包)** | `https://ark.cn-beijing.volces.com/api/v3` | **ep-xxxx 接入点 ID**(非裸模型名) | ⚠️ 需控制台创建接入点 |
| **通义 DashScope** | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `qwen-plus` | ⚠️ 当前 key 无效(401) |
| **硅基流动 SiliconFlow** | `https://api.siliconflow.cn/v1` | `deepseek-ai/DeepSeek-V3` | ⚠️ 账户欠费(402) |
| **DeepSeek 官方** | 内置 `deepseek_provider` | `deepseek-v4-pro` | 用 `LLM_PROVIDER=deepseek` + `DEEPSEEK_API_KEY` |
| **Anthropic Claude** | 内置 `anthropic_provider` | `claude-sonnet-4-20250514` | 用 `LLM_PROVIDER=anthropic` + `ANTHROPIC_API_KEY` |

## 豆包接入步骤(需用户操作一次)

1. 火山方舟控制台(volcengine.com → 方舟)开通豆包模型
2. 创建"推理接入点",复制 endpoint ID(形如 `ep-20240821xxxxx`)
3. 配置:
```bash
export LLM_PROVIDER=openai-compat
export OPENAI_COMPAT_BASE_URL=https://ark.cn-beijing.volces.com/api/v3
export OPENAI_COMPAT_API_KEY=<VOLCENGINE_API_KEY>
export OPENAI_COMPAT_MODEL=ep-2024xxxxxxxx
```

## 备注

- `POST_VERIFY_SEMANTIC=false` 可关闭 LLM-as-judge 语义核查(省一半调用量,评测/调试时常用)
- judge(语义核查)默认复用同一 provider/模型;`POST_VERIFY_JUDGE_MODEL` 可指定更便宜的模型
- 换大脑后建议重跑 `go run ./evals`(中文 36 题)对比通过率
