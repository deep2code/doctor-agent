# internal/llm — Provider 抽象

provider-agnostic 的消息/工具类型 + 三个实现。agent 层永远不 import 任何 SDK 类型，只有本包的 anthropic 文件碰 `anthropic-sdk-go`。

| 文件 | 作用 |
|---|---|
| `provider.go` | 契约与类型：`LLMProvider`（`Chat` / `StreamChat(ctx, messages, tools, systemPrompt, onDelta)` / `Name()`）、可选的 `PromptCacheProvider`（`StreamChatCached(..., cachedPrefix, rest, onDelta)`）、`Message`/`ToolDefinition`/`ToolCall`/`ContentPart`/`ImageInput`、`HasImages`、`Add` |
| `anthropic_provider.go` | Claude 实现，含 `NewStreaming` 路径与 `StreamChatCached`。**只有这个 provider 实现 `PromptCacheProvider`**：静态系统前缀打 `ephemeral` `cache_control` 断点（`cacheableSystemBlocks`）。注意 Anthropic 把 tools+system 当**一个有序前缀**缓存，命中还要求工具集完全一致（同一轮 ≤5 次迭代内成立） |
| `deepseek_provider.go` | DeepSeek 实现 |
| `openai_compat_provider.go` | 泛化的 OpenAI 协议实现（Zhipu / Qwen / 豆包-火山方舟 等任意兼容端点）；`WithThinkingDisabled` 对应豆包 glm/seed 系需要 `thinking:{"type":"disabled"}` 否则 content 为空 |
| `openai_stream.go` | DeepSeek 与 OpenAI-compat **共用**的 SSE 解析 + tool-call 分片累积。改流式行为改这里，不要在各 provider 里复制 |
| `pricing.go` | `ModelPrice` / `PriceForModel` / `CostUSD`，成本统计用 |

## 约定

- `agent.streamWithRetry` 用类型断言取 `PromptCacheProvider`，断言失败就退回普通 `StreamChat` —— DeepSeek/OpenAI-compat 靠各家**服务端自动缓存**，不需要显式断点。
- 新增 provider：实现 `LLMProvider` 即可接线；只有当它真有显式前缀缓存能力时才实现 `PromptCacheProvider`，否则不要实现（假装支持会让命中数看着对、实际每次 miss）。
- 多模态是类型系统的一部分（`ContentPart`/`ImageInput`），不是某个 provider 的私有能力。

## 测试

`anthropic_provider_test.go`、`openai_stream_test.go`（流式分片与 tool-call 累积）。
